// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"
)

func Float64Ptr(f float64) *float64 {
	return &f
}

var (
	ErrInvalidCode  = errors.New("invalid code")
	ErrCodeNotFound = errors.New("code not found")
)

var UrbanGreenNamespace = uuid.MustParse("a7c3e8f1-5b2d-4a9e-8c6f-1d3e5a7b9c2d")

func generateUrbanGreenID(provider, rawID string) string {
	name := fmt.Sprintf("%s:%s", provider, rawID)
	return fmt.Sprintf("%s:%s", UrbanGreenIDTemplate, uuid.NewSHA1(UrbanGreenNamespace, []byte(name)).String())
}

// MapUrbanGreenRowToUrbanGreen converts a raw UrbanGreenRow to the UrbanGreen content model
func MapUrbanGreenRowToUrbanGreen(raw dto.UrbanGreenRow, standards *Standards, syncTime time.Time) (odhContentModel.UrbanGreen, error) {
	id := generateUrbanGreenID(raw.Provider, raw.ID)

	urbanGreen := odhContentModel.UrbanGreen{
		Generic: odhContentModel.Generic{
			ID:     clib.StringPtr(id),
			Active: raw.State == "on_site",
			Source: clib.StringPtr(UrbanGreenSource),
			LicenseInfo: &clib.LicenseInfo{
				ClosedData: false,
				License:    clib.StringPtr("CC0"),
			},
			Geo: make(map[string]clib.GpsInfo),
		},
		GreenCode:        raw.Code,
		GreenCodeVersion: raw.SpecVersion,
	}

	// Mapping
	urbanGreen.Mapping.ProviderR3GIS = odhContentModel.ProviderR3GIS{
		Id:             raw.ID,
		RemoteProvider: raw.Provider,
		SyncTime:       syncTime,
	}

	// Parse code to get type and subtype
	parsed, err := ParseCode(raw.Code)
	if err != nil {
		return odhContentModel.UrbanGreen{}, err
	}

	urbanGreen.GreenCodeType = parsed.MainType
	urbanGreen.GreenCodeSubtype = parsed.SubType

	// Get standard for this version and set tag IDs
	standard := standards.GetVersion(raw.SpecVersion)
	if standard != nil {
		// Set Detail with Title from GreenCode names (all localizations)
		greenCode := standard.LookupCode(raw.Code)
		if greenCode == nil {
			return odhContentModel.UrbanGreen{}, ErrCodeNotFound
		}

		urbanGreen.Detail = make(map[string]*clib.DetailGeneric)
		languages := make([]string, 0, len(greenCode.Names))
		for lang, name := range greenCode.Names {
			urbanGreen.Detail[lang] = &clib.DetailGeneric{
				Title:    clib.StringPtr(name),
				Language: clib.StringPtr(lang),
			}
			languages = append(languages, lang)
		}
		sort.Strings(languages)
		urbanGreen.HasLanguage = languages

		tagSet := make(map[string]struct{})
		var tagIds []string

		mainType := standard.LookupMainType(parsed.MainType)
		if mainType != nil {
			tagSet[mainType.TagID] = struct{}{}
			tagIds = append(tagIds, mainType.TagID)
		}

		subType := standard.LookupSubType(parsed.SubType)
		if subType != nil {
			if _, exists := tagSet[subType.TagID]; !exists {
				tagIds = append(tagIds, subType.TagID)
			}
		}

		urbanGreen.TagIds = tagIds
	}

	// Parse additional information as Taxonomy
	if raw.AdditionalInformation != "" {
		taxonomy, err := parseJsonToMap(raw.AdditionalInformation)
		if err == nil && len(taxonomy) > 0 {
			urbanGreen.AdditionalProperties.UrbanGreenProperties = odhContentModel.UrbanGreenProperties{
				Taxonomy: taxonomy,
			}
		}
	}

	// Parse geometry
	if raw.TheGeom != "" {
		geoInfo := clib.GpsInfo{
			Default:  true,
			Geometry: clib.StringPtr(raw.TheGeom),
		}

		// Only populate Latitude/Longitude if geometry is a POINT
		if strings.HasPrefix(raw.TheGeom, "POINT") {
			lat, lon := extractPointCoordinates(raw.TheGeom)
			if lat != 0 && lon != 0 {
				geoInfo.Latitude = Float64Ptr(lat)
				geoInfo.Longitude = Float64Ptr(lon)
			}
		}

		urbanGreen.Geo["position"] = geoInfo
	}

	// Parse dates
	if raw.PutOnSite != "" {
		putOnSite, err := parseDateTime(raw.PutOnSite)
		if err == nil {
			urbanGreen.PutOnSite = &putOnSite
		}
	}

	if raw.RemovedFromSite != "" {
		removedFromSite, err := parseDateTime(raw.RemovedFromSite)
		if err == nil {
			urbanGreen.RemovedFromSite = &removedFromSite
		}
	}

	return urbanGreen, nil
}

// parseDateTime parses date strings in format "2022-06-04 12:02:10.000 +0200"
func parseDateTime(s string) (time.Time, error) {
	layouts := []string{
		"2006-01-02 15:04:05.000 -0700",
		"2006-01-02 15:04:05 -0700",
		"2006-01-02",
	}

	for _, layout := range layouts {
		t, err := time.Parse(layout, s)
		if err == nil {
			return t.UTC(), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse date: %s", s)
}

// parseJsonToMap parses JSON like {"it": "Lagerstroemia indica (Lagerstroemia)"}
func parseJsonToMap(s string) (map[string]string, error) {
	var parsed map[string]string
	if err := json.Unmarshal([]byte(s), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

// extractPointCoordinates extracts lat/lon from a POINT WKT geometry
func extractPointCoordinates(wkt string) (lat, lon float64) {
	re := regexp.MustCompile(`POINT\s*\(\s*([-\d.]+)\s+([-\d.]+)\s*\)`)
	matches := re.FindStringSubmatch(wkt)
	if len(matches) == 3 {
		lon, _ = strconv.ParseFloat(matches[1], 64)
		lat, _ = strconv.ParseFloat(matches[2], 64)
		return lat, lon
	}
	return 0, 0
}

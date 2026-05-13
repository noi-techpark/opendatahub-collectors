// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"sort"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"
)

// MapUrbanGreenMessageToUrbanGreen converts a JSON message DTO to the UrbanGreen content model.
// Follows the exact same logic as MapUrbanGreenRowToUrbanGreen.
func MapUrbanGreenMessageToUrbanGreen(msg dto.UrbanGreenMessage, standards *Standards, syncTime time.Time) (odhContentModel.UrbanGreen, error) {
	id := generateUrbanGreenID(msg.Source, msg.Id)

	urbanGreen := odhContentModel.UrbanGreen{
		Generic: odhContentModel.Generic{
			ID:     clib.StringPtr(id),
			Active: msg.Active,
			Source: clib.StringPtr(UrbanGreenSource),
			LicenseInfo: &clib.LicenseInfo{
				ClosedData: false,
				License:    clib.StringPtr("CC0"),
			},
			Shortname: clib.StringPtr(msg.Shortname),
			Geo:       make(map[string]clib.GpsInfo),
		},
		GreenCode:        msg.GreenCode,
		GreenCodeVersion: msg.GreenCodeVersion,
	}

	// Mapping
	urbanGreen.Mapping.ProviderR3GIS = odhContentModel.ProviderR3GIS{
		Id:             msg.Id,
		RemoteProvider: msg.Source,
		SyncTime:       syncTime,
	}

	// Parse code to get numeric type and subtype
	parsed, err := ParseCode(msg.GreenCode)
	if err != nil {
		return odhContentModel.UrbanGreen{}, err
	}

	urbanGreen.GreenCodeType = parsed.MainType
	urbanGreen.GreenCodeSubtype = parsed.SubType

	// Get standard for this version and set tag IDs + Detail
	standard := standards.GetVersion(msg.GreenCodeVersion)
	if standard != nil {
		greenCode := standard.LookupCode(msg.GreenCode)
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

	// Map additional information as Taxonomy
	if len(msg.AdditionalInformation) > 0 {
		urbanGreen.AdditionalProperties.UrbanGreenProperties = odhContentModel.UrbanGreenProperties{
			Taxonomy: msg.AdditionalInformation,
		}
	}

	// Map Geo entries
	for key, geo := range msg.Geo {
		geoInfo := clib.GpsInfo{
			Default:  geo.Default,
			Geometry: clib.StringPtr(geo.Geometry),
		}

		if geo.Gpstype != "" {
			geoInfo.Gpstype = clib.StringPtr(geo.Gpstype)
		}

		// Extract lat/lon from POINT geometry
		if strings.HasPrefix(geo.Geometry, "POINT") {
			lat, lon := extractPointCoordinates(geo.Geometry)
			if lat != 0 && lon != 0 {
				geoInfo.Latitude = Float64Ptr(lat)
				geoInfo.Longitude = Float64Ptr(lon)
			}
		}

		urbanGreen.Geo[key] = geoInfo
	}

	// Parse dates as RFC3339
	if msg.FirstImport != "" {
		if t, err := time.Parse(time.RFC3339, msg.FirstImport); err == nil {
			urbanGreen.FirstImport = &t
		}
	}

	if msg.LastChange != "" {
		if t, err := time.Parse(time.RFC3339, msg.LastChange); err == nil {
			urbanGreen.LastChange = &t
		}
	}

	if msg.PutOnSite != "" {
		if t, err := time.Parse(time.RFC3339, msg.PutOnSite); err == nil {
			urbanGreen.PutOnSite = &t
		}
	}

	if msg.RemovedFromSite != "" {
		if t, err := time.Parse(time.RFC3339, msg.RemovedFromSite); err == nil {
			urbanGreen.RemovedFromSite = &t
		}
	}

	return urbanGreen, nil
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"opendatahub.com/tr-traffic-event-a22-opendata/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22-opendata/odh-content-model"
)

func Float64Ptr(f float64) *float64 { return &f }

const opendataDateLayout = "02/01/2006 15:04:05"

// sentinelDate is the A22 placeholder for "no end date".
var sentinelDate = time.Date(1969, 12, 31, 23, 59, 59, 0, time.UTC)

var opendataDirectionLabels = map[string]map[string]string{
	"Sud":      {"it": "Sud", "de": "Süd", "en": "South"},
	"Nord":     {"it": "Nord", "de": "Nord", "en": "North"},
	"Entrambe": {"it": "Entrambe le direzioni", "de": "Beide Richtungen", "en": "Both directions"},
}

func parseOpendataDate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}
	return time.Parse(opendataDateLayout, s)
}

func generateOpendataID(event dto.A22OpendataEvent) string {
	return clib.GenerateID(ID_TEMPLATE, event.IDNotizia)
}

func buildOpendataLocationText(event dto.A22OpendataEvent, lang string) string {
	direction := ""
	if labels, ok := opendataDirectionLabels[event.Direzione]; ok {
		direction = labels[lang]
	}
	if direction != "" {
		return fmt.Sprintf("A22, km %.1f-%.1f, %s", event.KmInizio, event.KmFine, direction)
	}
	return fmt.Sprintf("A22, km %.1f-%.1f", event.KmInizio, event.KmFine)
}

func mapOpendataBase(rd *roadData, event dto.A22OpendataEvent, tagID string) (odhContentModel.Announcement, error) {
	id := generateOpendataID(event)

	ann := odhContentModel.Announcement{
		Generic: odhContentModel.Generic{
			Active: true,
			Source: clib.StringPtr(SOURCE),
			LicenseInfo: &clib.LicenseInfo{
				ClosedData: false,
				License:    clib.StringPtr("CC0"),
			},
			Geo: map[string]clib.GpsInfo{},
		},
	}

	ann.ID = clib.StringPtr(id)
	ann.Mapping.ProviderA22Open.Id = event.IDNotizia
	ann.Mapping.ProviderA22Open.Iddirezione = event.Direzione
	ann.Mapping.ProviderA22Open.MetroInizio = fmt.Sprintf("%.1f", event.KmInizio)
	ann.Mapping.ProviderA22Open.MetroFine = fmt.Sprintf("%.1f", event.KmFine)

	// StartTime
	startTime, err := parseOpendataDate(event.DataInizio)
	if err != nil {
		return odhContentModel.Announcement{}, fmt.Errorf("failed to parse DataInizio: %w", err)
	}
	ann.StartTime = &startTime

	// EndTime (nil = open-ended)
	if event.DataFine != nil && *event.DataFine != "" {
		endTime, err := parseOpendataDate(*event.DataFine)
		if err != nil {
			return odhContentModel.Announcement{}, fmt.Errorf("failed to parse DataFine: %w", err)
		}
		if !endTime.Equal(sentinelDate) {
			ann.EndTime = &endTime
		}
	}

	// Geometry from road axis
	wkt := rd.KmRangeToWKT(event.KmInizio, event.KmFine)
	startPoint := rd.interpolatePoint(rd.KmToDistance(event.KmInizio))

	ann.Geo["position"] = clib.GpsInfo{
		Latitude:  Float64Ptr(startPoint.Lat),
		Longitude: Float64Ptr(startPoint.Lon),
		Default:   true,
		Geometry:  clib.StringPtr(wkt),
	}

	// Tags
	ann.TagIds = []string{
		"announcement:traffic-event",
		tagID,
	}

	// Detail: tag title + location per language
	tag := tags.FindById(tagID)
	ann.Detail = map[string]*clib.DetailGeneric{
		"it": {
			Title:    clib.StringPtr(tag.NameIt),
			BaseText: clib.StringPtr(buildOpendataLocationText(event, "it")),
		},
		"de": {
			Title:    clib.StringPtr(tag.NameDe),
			BaseText: clib.StringPtr(buildOpendataLocationText(event, "de")),
		},
		"en": {
			Title:    clib.StringPtr(tag.NameEn),
			BaseText: clib.StringPtr(buildOpendataLocationText(event, "en")),
		},
	}

	locationEn := buildOpendataLocationText(event, "en")
	ann.Shortname = clib.StringPtr(fmt.Sprintf("%s - %s", tag.NameEn, locationEn))
	ann.HasLanguage = []string{"it", "de", "en"}

	return ann, nil
}

// MapLavoriToAnnouncement maps an A22 opendata "lavori" (road works) event.
func MapLavoriToAnnouncement(rd *roadData, event dto.A22OpendataEvent) (odhContentModel.Announcement, error) {
	return mapOpendataBase(rd, event, "traffic-event:road-work")
}

// MapTrafficoToAnnouncement maps an A22 opendata "traffico" (traffic) event.
func MapTrafficoToAnnouncement(rd *roadData, event dto.A22OpendataEvent) (odhContentModel.Announcement, error) {
	return mapOpendataBase(rd, event, "traffic-event:current")
}

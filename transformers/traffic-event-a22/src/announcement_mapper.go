// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	dto "opendatahub.com/tr-traffic-event-a22/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22/odh-content-model"
)

func Float64Ptr(f float64) *float64 {
	return &f
}

// A22 event type (idtipoevento) to shared tag ID mapping.
var eventTypeTagMap = map[int64]string{
	1:  "traffic-event:accident",
	2:  "traffic-event:congestion",
	4:  "traffic-event:weather-related",
	5:  "traffic-event:weather-related",
	6:  "traffic-event:weather-related",
	7:  "traffic-event:road-work",
	8:  "traffic-event:hindrance",
	9:  "traffic-event:prohibition",
	10: "traffic-event:hindrance",
	11: "traffic-event:caution",
	12: "traffic-event:current",
	25: "traffic-event:closure",
}

// Direction labels per language.
var directionLabels = map[int64]map[string]string{
	1: {"it": "Sud", "de": "Süd", "en": "South"},
	2: {"it": "Nord", "de": "Nord", "en": "North"},
	3: {"it": "Entrambe le direzioni", "de": "Beide Richtungen", "en": "Both directions"},
}

// Default position: Trento Nord - Interporto interchange on A22
var defaultPosition = clib.GpsInfo{
	Latitude:  Float64Ptr(46.11739),
	Longitude: Float64Ptr(11.08773),
	Default:   true,
	Geometry:  clib.StringPtr("POINT (11.08773 46.11739)"),
}

func mapEventTypeToTagID(idtipoevento int64) string {
	if tagID, ok := eventTypeTagMap[idtipoevento]; ok {
		return tagID
	}
	// Fallback for unknown event types
	return "traffic-event:hindrance"
}

func generateID(event dto.A22Event) string {
	fields := map[string]interface{}{
		"id":                event.Id,
		"data_inizio":       event.DataInizio,
		"idtipoevento":      event.Idtipoevento,
		"idsottotipoevento": event.Idsottotipoevento,
		"lat_inizio":        event.LatInizio,
		"lon_inizio":        event.LonInizio,
		"lat_fine":          event.LatFine,
		"lon_fine":          event.LonFine,
	}
	jsonBytes, _ := json.Marshal(fields)
	return clib.GenerateID(ID_TEMPLATE, string(jsonBytes))
}

func buildLocationText(event dto.A22Event, lang string) string {
	kmStart := float64(event.MetroInizio) / 1000.0
	kmEnd := float64(event.MetroFine) / 1000.0

	direction := ""
	if labels, ok := directionLabels[event.Iddirezione]; ok {
		direction = labels[lang]
	}

	if direction != "" {
		return fmt.Sprintf("%s, km %.1f-%.1f, %s", event.Autostrada, kmStart, kmEnd, direction)
	}
	return fmt.Sprintf("%s, km %.1f-%.1f", event.Autostrada, kmStart, kmEnd)
}

// MapA22EventToAnnouncement converts a raw A22 event to the Announcement model.
func MapA22EventToAnnouncement(tags clib.TagDefs, event dto.A22Event, id string) (odhContentModel.Announcement, error) {
	announcement := odhContentModel.Announcement{
		Generic: odhContentModel.Generic{
			Active: true,
			Source: clib.StringPtr(SOURCE),
			LicenseInfo: &clib.LicenseInfo{
				ClosedData: true,
				License:    clib.StringPtr("CC0"),
			},
			Geo: map[string]clib.GpsInfo{},
		},
	}

	// ID and provider metadata
	announcement.ID = clib.StringPtr(id)
	announcement.Mapping.ProviderA22.Id = fmt.Sprintf("%d", event.Id)
	announcement.Mapping.ProviderA22.FasciaOraria = fmt.Sprintf("%t", event.FasciaOraria != nil && *event.FasciaOraria)
	announcement.Mapping.ProviderA22.Idcorsia = fmt.Sprintf("%d", event.Idcorsia)
	announcement.Mapping.ProviderA22.Iddirezione = fmt.Sprintf("%d", event.Iddirezione)
	announcement.Mapping.ProviderA22.MetroInizio = fmt.Sprintf("%d", event.MetroInizio)
	announcement.Mapping.ProviderA22.MetroFine = fmt.Sprintf("%d", event.MetroFine)
	announcement.Mapping.ProviderA22.Idsottotipoevento = fmt.Sprintf("%d", event.Idsottotipoevento)

	// StartTime
	startTime, err := ParseA22Date(event.DataInizio)
	if err != nil {
		return odhContentModel.Announcement{}, fmt.Errorf("failed to parse data_inizio: %w", err)
	}
	announcement.StartTime = &startTime

	// EndTime (null if event is still active)
	if event.DataFine != nil && *event.DataFine != "" {
		endTime, err := ParseA22Date(*event.DataFine)
		if err != nil {
			return odhContentModel.Announcement{}, fmt.Errorf("failed to parse data_fine: %w", err)
		}
		announcement.EndTime = &endTime
	}

	// Geometry
	hasStartCoords := event.LatInizio != 0 || event.LonInizio != 0
	hasEndCoords := event.LatFine != 0 || event.LonFine != 0
	samePoint := event.LatInizio == event.LatFine && event.LonInizio == event.LonFine

	if hasStartCoords {
		if !hasEndCoords || samePoint {
			// Point event
			announcement.Geo["position"] = clib.GpsInfo{
				Latitude:  Float64Ptr(event.LatInizio),
				Longitude: Float64Ptr(event.LonInizio),
				Default:   true,
				Geometry:  clib.StringPtr(fmt.Sprintf("POINT (%f %f)", event.LonInizio, event.LatInizio)),
			}
		} else {
			// Linear event: start point as position, linestring as geometry
			announcement.Geo["position"] = clib.GpsInfo{
				Latitude:  Float64Ptr(event.LatInizio),
				Longitude: Float64Ptr(event.LonInizio),
				Default:   true,
				Geometry: clib.StringPtr(fmt.Sprintf("LINESTRING (%f %f, %f %f)",
					event.LonInizio, event.LatInizio,
					event.LonFine, event.LatFine)),
			}
		}
	}
	if !hasStartCoords {
		announcement.Geo["position"] = defaultPosition
	}

	// Tags
	typeTagID := mapEventTypeToTagID(event.Idtipoevento)
	announcement.TagIds = []string{
		"announcement:traffic-event",
		typeTagID,
	}

	typeTag := tags.FindById(typeTagID)

	// Details (multilingual)
	locationEn := buildLocationText(event, "en")
	if typeTag != nil {
		announcement.Shortname = clib.StringPtr(fmt.Sprintf("%s - %s", typeTag.NameEn, locationEn))
		announcement.Detail = map[string]*clib.DetailGeneric{
			"it": {
				Title:    clib.StringPtr(typeTag.NameIt),
				BaseText: clib.StringPtr(buildLocationText(event, "it")),
			},
			"de": {
				Title:    clib.StringPtr(typeTag.NameDe),
				BaseText: clib.StringPtr(buildLocationText(event, "de")),
			},
			"en": {
				Title:    clib.StringPtr(typeTag.NameEn),
				BaseText: clib.StringPtr(buildLocationText(event, "en")),
			},
		}
	} else {
		// Fallback if tag not found
		announcement.Shortname = clib.StringPtr(fmt.Sprintf("A22 Event %d - %s", event.Idtipoevento, locationEn))
		announcement.Detail = map[string]*clib.DetailGeneric{
			"it": {
				Title:    clib.StringPtr(fmt.Sprintf("Evento A22 tipo %d", event.Idtipoevento)),
				BaseText: clib.StringPtr(buildLocationText(event, "it")),
			},
			"de": {
				Title:    clib.StringPtr(fmt.Sprintf("A22 Ereignis Typ %d", event.Idtipoevento)),
				BaseText: clib.StringPtr(buildLocationText(event, "de")),
			},
			"en": {
				Title:    clib.StringPtr(fmt.Sprintf("A22 Event type %d", event.Idtipoevento)),
				BaseText: clib.StringPtr(buildLocationText(event, "en")),
			},
		}
	}

	announcement.HasLanguage = []string{"it", "de", "en"}

	return announcement, nil
}

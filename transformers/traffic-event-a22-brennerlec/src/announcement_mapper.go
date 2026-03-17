// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	dto "opendatahub.com/tr-traffic-event-a22-brennerlec/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22-brennerlec/odh-content-model"
)

func Float64Ptr(f float64) *float64 {
	return &f
}

func generateID(event dto.BrennerLECEvent) string {
	return clib.GenerateID(ID_TEMPLATE, event.Idtratta)
}

// MapBrennerLECEventToAnnouncement converts a BrennerLEC speed limit event to the Announcement model.
func MapBrennerLECEventToAnnouncement(tags clib.TagDefs, event dto.BrennerLECEvent, id string) (odhContentModel.Announcement, error) {
	announcement := odhContentModel.Announcement{
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

	// ID and provider metadata
	announcement.ID = clib.StringPtr(id)
	announcement.Mapping.ProviderA22BrennerLEC.Id = event.Idtratta

	// Speed limit in metadata
	if event.Limite != nil {
		announcement.Mapping.ProviderA22BrennerLEC.Limite = fmt.Sprintf("%d", *event.Limite)
	}

	// StartTime: enforcement date
	if event.Dataattuazione != nil && *event.Dataattuazione != "" {
		startTime, err := ParseA22Date(*event.Dataattuazione)
		if err != nil {
			return odhContentModel.Announcement{}, fmt.Errorf("failed to parse dataattuazione: %w", err)
		}
		announcement.StartTime = &startTime
	}

	// Tags
	announcement.TagIds = []string{
		"announcement:traffic-event",
		"traffic-event:speed-limit",
	}

	// Details (multilingual)
	speedLimitStr := ""
	if event.Limite != nil {
		speedLimitStr = fmt.Sprintf("%d km/h", *event.Limite)
	}

	announcement.Shortname = clib.StringPtr(fmt.Sprintf("BrennerLEC Dynamic Speed Limit - A22, section %s, limit %s", event.Idtratta, speedLimitStr))
	announcement.Detail = map[string]*clib.DetailGeneric{
		"it": {
			Title:    clib.StringPtr("Limite di Velocità Dinamico BrennerLEC"),
			BaseText: clib.StringPtr(fmt.Sprintf("A22, tratta %s, limite %s", event.Idtratta, speedLimitStr)),
		},
		"de": {
			Title:    clib.StringPtr("Dynamische Geschwindigkeitsbegrenzung BrennerLEC"),
			BaseText: clib.StringPtr(fmt.Sprintf("A22, Strecke %s, Limit %s", event.Idtratta, speedLimitStr)),
		},
		"en": {
			Title:    clib.StringPtr("BrennerLEC Dynamic Speed Limit"),
			BaseText: clib.StringPtr(fmt.Sprintf("A22, section %s, limit %s", event.Idtratta, speedLimitStr)),
		},
	}
	announcement.HasLanguage = []string{"it", "de", "en"}

	// Default position: Trento Nord - Interporto interchange on A22
	announcement.Geo["position"] = clib.GpsInfo{
		Latitude:  Float64Ptr(46.11739),
		Longitude: Float64Ptr(11.08773),
		Default:   true,
		Geometry:  clib.StringPtr("POINT (11.08773 46.11739)"),
	}

	return announcement, nil
}

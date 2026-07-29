// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	odhmodel "opendatahub.com/momentus-events/odh-content-model"
)

func TestTransformer(t *testing.T) {
	inBytes, err := os.ReadFile("./testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}

	var event MomentusEvent
	if err := json.Unmarshal(inBytes, &event); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}

	venue := &ODHVenue{
		Id: "urn:venue:noi:6b3f0a14-3c5b-5d09-81f3-3ebe5b7885ea",
		Mapping: ODHVenueMapping{
			Tag: map[string]string{"eventlocation": "noi"},
		},
		RoomDetails: []ODHRoomDetails{
			{
				Id: "urn:room:noi:1",
				Mapping: map[string]map[string]string{
					"momentus": {
						"id": "room1",
					},
				},
			},
		},
	}

	result := ParseMomentusEvent(event, venue, nil, true)
	if result != nil {
		// Normalize volatile fields for deterministic testing
		result.FirstImport = "2026-06-24T10:00:00Z"
		result.LastChange = "2026-06-24T10:00:00Z"
	}

	var expected odhmodel.EventLinked
	err = testsuite.LoadOutput(&expected, "./testdata/out_full.json")
	if err != nil {
		t.Fatalf("Failed to load output: %v", err)
	}

	// Normalize expected FirstImport and LastChange as well if they are different in JSON
	expected.FirstImport = "2026-06-24T10:00:00Z"
	expected.LastChange = "2026-06-24T10:00:00Z"

	if !reflect.DeepEqual(result, &expected) {
		t.Errorf("Result does not match expected output.\nGot: %+v\nExpected: %+v", result, &expected)
	}
}

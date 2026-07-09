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
	odhmodel "opendatahub.com/momentus-venues/odh-content-model"
)

func TestParseMomentusVenue(t *testing.T) {
	// Read input
	inBytes, err := os.ReadFile("../testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}

	var inRoom odhmodel.MomentusRoom
	if err := json.Unmarshal(inBytes, &inRoom); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}

	// Read base venue
	baseBytes, err := os.ReadFile("../testdata/base_venue.json")
	if err != nil {
		t.Fatalf("Failed to read base_venue.json: %v", err)
	}

	var baseVenue odhmodel.VenueV2
	if err := json.Unmarshal(baseBytes, &baseVenue); err != nil {
		t.Fatalf("Failed to unmarshal base_venue.json: %v", err)
	}

	// Process
	outVenue := ParseMomentusVenue(inRoom, &baseVenue)

	// Verify
	if outVenue == nil {
		t.Fatalf("Expected outVenue to be non-nil")
	}

	var expected odhmodel.VenueV2
	err = testsuite.LoadOutput(&expected, "../testdata/out_full.json")
	if err != nil {
		t.Fatalf("Failed to load output: %v", err)
	}

	if !reflect.DeepEqual(outVenue, &expected) {
		t.Errorf("Result does not match expected output.\nGot: %+v\nExpected: %+v", outVenue, &expected)
	}
}

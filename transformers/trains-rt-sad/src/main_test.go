// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/go-netex"
	"gotest.tools/v3/assert"
)

func Test_find(t *testing.T) {
	data, err := os.ReadFile("netex.xml")
	assert.NilError(t, err)
	var delivery netex.PublicationDelivery
	err = xml.Unmarshal(data, &delivery)
	assert.NilError(t, err)

	c := NewCache()

	// Test findJourney
	journey := findJourney(delivery, c, "1853")
	assert.Assert(t, journey != nil)
	assert.Assert(t, journey.Id == "it:apb:ServiceJourney:024002B-Pizzin-100-1-36540:38")

	// Test findJourneyPattern
	pattern := findJourneyPattern(delivery, c, journey.ServiceJourneyPatternRef.Ref)
	assert.Assert(t, pattern != nil)
	assert.Assert(t, pattern.Name == "58")

	// Test findLine
	line := findLine(delivery, c, journey.LineRef.Ref)
	assert.Assert(t, line != nil)
	assert.Assert(t, line.Name == "B400")

	// Test findDestinationDisplay
	display := findDestinationDisplay(delivery, c, "it:apb:DestinationDisplay:137")
	assert.Assert(t, display != nil)
	assert.Assert(t, display.Name == "Caldaro")
}

func Test_map(t *testing.T) {
	nb, err := os.ReadFile("netex.xml")
	assert.NilError(t, err)
	var n netex.PublicationDelivery
	err = xml.Unmarshal(nb, &n)
	assert.NilError(t, err)

	c := NewCache()

	ex, err := os.ReadFile("testdata/example.json")
	assert.NilError(t, err)
	var dto Dto
	json.Unmarshal(ex, &dto)
	assert.NilError(t, err)

	refTime, _ := time.Parse(time.RFC3339, "2026-02-03T19:45:00.000+01:00")

	s, err := raw2Siri(c, refTime, dto, n)
	assert.NilError(t, err)
	fmt.Println(s)
	xmlBytes, err := json.MarshalIndent(s, "", "  ")
	assert.NilError(t, err)
	os.WriteFile("siri.json", xmlBytes, 0644)
}

func Test_download(t *testing.T) {
	if err := downloadLatestNetex(); err != nil {
		t.Fatal("could not download latest netex", err)
	}

}

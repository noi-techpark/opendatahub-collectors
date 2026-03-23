// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/json"
	"encoding/xml"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/noi-techpark/go-netex"
	"gotest.tools/v3/assert"
)

func Test_find(t *testing.T) {
	data, err := os.ReadFile("testdata/netex.xml")
	assert.NilError(t, err)
	var delivery netex.PublicationDelivery
	err = xml.Unmarshal(data, &delivery)
	assert.NilError(t, err)

	c := NewCache()

	// Test findJourney
	journey := findJourney(delivery, c, "1853", "2025-12-14")
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

func Test_validate(t *testing.T) {
	t.Log("reading netex")
	nb, err := os.ReadFile("testdata/netex.xml")
	assert.NilError(t, err)
	var n netex.PublicationDelivery
	err = xml.Unmarshal(nb, &n)
	assert.NilError(t, err)

	c := NewCache()

	t.Log("reading example.json")
	ex, err := os.ReadFile("testdata/example.json")
	assert.NilError(t, err)
	var dto Dto
	json.Unmarshal(ex, &dto)
	assert.NilError(t, err)

	refTime, _ := time.Parse(time.RFC3339, "2026-02-03T19:45:00.000+01:00")

	t.Log("converting to SIRI")
	s, err := raw2Siri(c, refTime, dto, n)
	assert.NilError(t, err)

	t.Log("writing SIRI json")
	jsonBytes, err := json.MarshalIndent(s, "", "  ")
	assert.NilError(t, err)
	os.WriteFile("siri.json", jsonBytes, 0644)

	t.Log("writing SIRI xml")
	xmlBytes, err := xml.MarshalIndent(s, "", "  ")
	assert.NilError(t, err)
	os.WriteFile("siri.xml", xmlBytes, 0644)

	t.Log("validating xml")
	if out, err := exec.Command("xmllint", "--noout", "--schema", "testdata/SIRI/xsd/siri.xsd", "siri.xml").CombinedOutput(); err != nil {
		t.Fatalf("xml validation failed:\n %s", out)
	}
}

func Test_download(t *testing.T) {
	if err := downloadLatestNetex(); err != nil {
		t.Fatal("could not download latest netex", err)
	}
}

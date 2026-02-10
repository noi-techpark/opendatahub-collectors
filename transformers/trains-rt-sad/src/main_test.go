// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later
package main

import (
	"encoding/xml"
	"os"
	"testing"

	"github.com/noi-techpark/go-netex"
	"gotest.tools/v3/assert"
)

func Test_main(t *testing.T) {
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

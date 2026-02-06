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
	// unmarshal testing netex
}

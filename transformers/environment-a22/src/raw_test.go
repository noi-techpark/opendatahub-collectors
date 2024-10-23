// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func TestRawUnmarshal(t *testing.T) {
	f, err := os.ReadFile("../test/raw_example.json")
	assert.NilError(t, err)
	raw := raw{}
	err = json.Unmarshal(f, &raw)
	assert.NilError(t, err)
}

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func TestUnmarshal(t *testing.T) {
	f, err := os.ReadFile("./testdata/noi.matomo.cloud.json")
	assert.NilError(t, err)
	_, err = unmarshalRawJson(string(f))
	assert.NilError(t, err)
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMapDto(t *testing.T) {
	for _, fname := range []string{"driwe.json", "neogy.json"} {
		f, err := os.ReadFile("../test/" + fname)
		assert.NoError(t, err, "error opening file %s", fname)
		j := OCPILocations{}
		err = json.Unmarshal(f, &j)
		assert.NoErrorf(t, err, "error unmarshalling file %s: %w", fname, err)

		// must use go test -test.v to see this output
		s, _ := json.MarshalIndent(j, "", " ")
		println(string(s))
	}
}

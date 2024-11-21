// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"encoding/json"
	"os"
	"testing"

	"gotest.tools/v3/assert"
)

func Test_readStations(t *testing.T) {
	first := Station{
		"001bc506701005c2",
		"C-S1",
		"area viale Druso",
		11.33998888888889,
		46.494825,
		"Piazza Adriano 1",
		"Bolzano - Bozen",
	}
	ss := readStations()
	assert.Equal(t, ss[0], first)
}

func Test_exampleJson(t *testing.T) {
	f, err := os.ReadFile("test/example.json")
	assert.NilError(t, err, "failed reading example json")

	var p Payload
	err = json.Unmarshal(f, &p)
	assert.NilError(t, err, "failed unmarshalling example json: %s", string(f))

	assert.Equal(t, p, Payload{
		DeviceInfo: struct{ DevEui string }{DevEui: "001bc50670100519"},
		Time:       "2024-11-13T13:59:23.407020+00:00",
		Data:       "gAAAAy8S9AABZm8jSQE6AAARzAAi3BsA2CE=",
	})

	_, err = parseTime(p.Time)
	assert.NilError(t, err, "wrong time format: %s", p.Time)

	s, err := decodeBinary(p.Data)
	assert.NilError(t, err, "error decoding binary data: %s", p.Data)

	assert.Equal(t, s, 1)
}

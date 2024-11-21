// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
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
	}
	ss := readStations()
	assert.Equal(t, ss[0], first)
}

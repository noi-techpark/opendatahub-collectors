// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUnmarshal(t *testing.T) {
	testFiles := []string{"testdata/in_camera.json", "testdata/in_radar.json"}

	for _, file := range testFiles {
		t.Run(file, func(t *testing.T) {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("Failed to read file: %v", err)
			}
			var rawType RawType
			if err := json.Unmarshal(data, &rawType); err != nil {
				t.Fatalf("Failed to unmarshal JSON: %v", err)
			}
			if rawType.Topic == "" {
				t.Error("Topic should not be empty")
			}
			if rawType.MsgId == 0 {
				t.Error("MsgId should not be zero")
			}
			if rawType.Payload.Type == "" {
				t.Error("Payload.Type should not be empty")
			}
		})
	}
}

func TestReadCSV(t *testing.T) {
	m, err := readStationCsv("testdata/stations.csv")
	if err != nil {
		t.Errorf("readCSV failed: %v", err)
	}

	assert.Equal(t, m["1cd5c31b-4b28-4c3e-be09-d75545e5b7b5"].Name, "KI Kamera Meran 2000")
}

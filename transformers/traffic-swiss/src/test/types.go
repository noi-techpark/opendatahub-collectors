// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import "time"

// Minimal type duplicates needed for integration testing without importing main.

type StationDTO struct {
	ID        string         `json:"id"`
	Lat       float64        `json:"lat"`
	Lon       float64        `json:"lon"`
	Metadata  map[string]any `json:"metadata"`
	DataTypes []string       `json:"data_types"`
}

type MeasurementDTO struct {
	StationID string    `json:"station_id"`
	DataType  string    `json:"data_type"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

type Root struct {
	Stations     []StationDTO     `json:"stations"`
	Measurements []MeasurementDTO `json:"measurements"`
}

// TestMetrics holds basic counts extracted from a Root payload.
type TestMetrics struct {
	Stations     int
	Measurements int
}

// GetMetrics returns basic counts from a Root.
func (r Root) GetMetrics() TestMetrics {
	return TestMetrics{
		Stations:     len(r.Stations),
		Measurements: len(r.Measurements),
	}
}

// getFallbackRoot returns a minimal hardcoded Root used when endpoints are unreachable.
func getFallbackRoot() Root {
	return Root{
		Stations: []StationDTO{
			{
				ID:        "CH:0002.01",
				Lat:       46.998864,
				Lon:       8.311130,
				DataTypes: []string{"average-speed-light-vehicles", "average-flow-light-vehicles"},
				Metadata:  map[string]any{"lane": "lane1", "carriageway": "exitSlipRoad"},
			},
		},
		Measurements: []MeasurementDTO{
			{StationID: "CH:0002.01", DataType: "average-speed-light-vehicles", Value: 112.4, Timestamp: time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC)},
		},
	}
}

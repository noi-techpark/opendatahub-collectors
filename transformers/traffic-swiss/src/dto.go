// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "time"

// Root is the JSON payload produced by the dc-traffic-swiss collector.
type Root struct {
	Stations     []StationDTO     `json:"stations"`
	Measurements []MeasurementDTO `json:"measurements"`
}

// StationDTO represents a traffic sensor station from the collector.
type StationDTO struct {
	ID        string         `json:"id"`
	Lat       float64        `json:"lat"`
	Lon       float64        `json:"lon"`
	Metadata  map[string]any `json:"metadata"`
	DataTypes []string       `json:"data_types"`
}

// MeasurementDTO represents a single 10-minute aggregated measurement.
type MeasurementDTO struct {
	StationID string    `json:"station_id"`
	DataType  string    `json:"data_type"`
	Value     float64   `json:"value"`
	Timestamp time.Time `json:"timestamp"`
}

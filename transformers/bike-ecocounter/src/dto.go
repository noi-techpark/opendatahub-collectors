// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// EcocounterSite represents a single site/station from the Ecocounter API
type EcocounterSite struct {
	ID                  int           `json:"id"`
	Name                string        `json:"name"`
	Description         string        `json:"description"`
	Directional         bool          `json:"directional"`
	FirstData           string        `json:"firstData"`
	LastData            string        `json:"lastData"`
	Granularity         string        `json:"granularity"`
	HasTimestampedData  bool          `json:"hasTimestampedData"`
	HasWeather          bool          `json:"hasWeather"`
	Location            Location      `json:"location"`
	Counters            []Counter     `json:"counters"`
	Measurements        []Measurement `json:"measurements"`
	TravelModes         []string      `json:"travelModes"`
}

// Location represents geographic coordinates
type Location struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// Counter represents a physical counter device
type Counter struct {
	ID               int    `json:"id"`
	InstallationDate string `json:"installationDate"`
	Serial           string `json:"serial"`
}

// Measurement represents a flow measurement stream
type Measurement struct {
	FlowID     int         `json:"flowID"`
	FlowName   string      `json:"flowName"`
	Direction  string      `json:"direction"`
	TravelMode string      `json:"travelMode"`
	Data       []DataPoint `json:"data"`
}

// DataPoint represents a single measurement data point
type DataPoint struct {
	Counts      int    `json:"counts"`
	Granularity string `json:"granularity"`
	Timestamp   string `json:"timestamp"`
}

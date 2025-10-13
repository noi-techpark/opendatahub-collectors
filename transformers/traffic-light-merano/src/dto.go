// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strconv"
	"strings"
	"time"
)

// TrafficData represents the root structure of the API response
type TrafficData struct {
	XMLName    struct{}  `xml:"data"`
	Timestamp  string    `xml:"timestamp"`
	Timestamp2 string    `xml:"timestamp2"`
	Sezioni    []Sezione `xml:"SEZIONI>SEZIONE"`
}

// Sezione represents a traffic monitoring point
type Sezione struct {
	ID        string `xml:"id,attr"`
	Name      string `xml:"name,attr"`
	Date      string `xml:"date,attr"`
	TScanType int    `xml:"TScanType,attr"`
	Intril    int    `xml:"INTRIL"`
	Day_0     Day    `xml:"DAY_0"`
}

// Day represents the data for a specific day
type Day struct {
	Date    string          `xml:"date,attr"`
	IsoDate string          `xml:"iso_date,attr"`
	FT      MeasurementData `xml:"FT"`
	T       MeasurementData `xml:"T"`
}

// GetBaseMidnightTimestamp parses the iso_date and returns the midnight timestamp in milliseconds
// Returns 0 if parsing fails
func (d *Day) GetBaseMidnightTimestamp() int64 {
	if d.IsoDate == "" {
		return 0
	}

	t, err := time.Parse(time.RFC3339, d.IsoDate)
	if err != nil {
		return 0
	}

	return t.UnixMilli()
}

// MeasurementData represents a complex measurement structure (FT or T)
type MeasurementData struct {
	TScanType int    `xml:"TScanType,attr"`
	Bin       int    `xml:"bin,attr"`
	Um        string `xml:"um,attr"`
	Title     string `xml:"title,attr"`
	Inter     int    `xml:"inter,attr"`
	Data      string `xml:",chardata"` // Comma-separated values
}

// ParseValues parses the comma-separated string into an array of integers
// Returns nil for values equal to -32766 (no data)
func (m *MeasurementData) ParseValues() []int {
	if m.Data == "" {
		return []int{}
	}

	parts := strings.Split(m.Data, ",")
	values := make([]int, 0, len(parts))

	for _, part := range parts {
		val, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			continue
		}
		values = append(values, val)
	}

	return values
}

// GetLatestValue returns the latest non-default value from the measurement data
// Returns nil if no valid value is found
func (m *MeasurementData) GetLatestValue() *int {
	values := m.ParseValues()

	// Iterate from the end to find the latest valid value
	for i := len(values) - 1; i >= 0; i-- {
		if values[i] != -32766 {
			return &values[i]
		}
	}

	return nil
}

// TimestampedValue represents a measurement value with its timestamp
type TimestampedValue struct {
	Timestamp int64
	Value     int
}

// GetAllValidValues returns all valid (non -32766) values with their calculated timestamps
// baseTimestamp should be the midnight timestamp in milliseconds for the reference day
// Each index represents a 10-minute interval from midnight
func (m *MeasurementData) GetAllValidValues(baseTimestamp int64) []TimestampedValue {
	values := m.ParseValues()
	result := make([]TimestampedValue, 0)

	const tenMinutesInMillis = 10 * 60 * 1000

	for i, val := range values {
		if val != -32766 {
			// Calculate timestamp: midnight + (index * 10 minutes)
			ts := baseTimestamp + int64(i)*tenMinutesInMillis
			result = append(result, TimestampedValue{
				Timestamp: ts,
				Value:     val,
			})
		}
	}

	return result
}

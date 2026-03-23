// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strings"
	"sync"
	"time"
)

const windowSize = 10

type sample struct {
	value float64
	ts    time.Time
}

// Aggregator buffers 1-minute measurements per station/data-type and emits
// aggregated results once a 10-sample window is complete.
// Flow types (data type names containing "flow") are summed; speed types are averaged.
type Aggregator struct {
	mu     sync.Mutex
	buffer map[string]map[string][]sample // [stationID][odhDataType][]sample
}

// NewAggregator returns an initialised Aggregator.
func NewAggregator() *Aggregator {
	return &Aggregator{
		buffer: make(map[string]map[string][]sample),
	}
}

// Add appends a sample for the given station/data-type.
// When the window of 10 samples is reached it returns (aggregatedValue, lastTimestamp, true)
// and resets the bucket. Otherwise it returns (0, zero, false).
func (a *Aggregator) Add(stationID, dataType string, value float64, ts time.Time) (float64, time.Time, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.buffer[stationID] == nil {
		a.buffer[stationID] = make(map[string][]sample)
	}
	a.buffer[stationID][dataType] = append(a.buffer[stationID][dataType], sample{value: value, ts: ts})

	bucket := a.buffer[stationID][dataType]
	if len(bucket) < windowSize {
		return 0, time.Time{}, false
	}

	var result float64
	lastTs := bucket[len(bucket)-1].ts
	if strings.Contains(dataType, "flow") {
		for _, s := range bucket {
			result += s.value
		}
	} else {
		for _, s := range bucket {
			result += s.value
		}
		result /= float64(len(bucket))
	}

	// Reset bucket
	a.buffer[stationID][dataType] = nil
	return result, lastTs, true
}

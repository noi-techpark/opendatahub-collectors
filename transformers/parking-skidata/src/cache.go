// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strings"
	"sync"
)

// LatestRecord is the most recent (value, timestamp) seen for a single
// (carpark provider id, BDP datatype name) pair.
type LatestRecord struct {
	Value     int
	Timestamp int64 // milliseconds since unix epoch
}

// Cache holds the most recent free/occupied measurement for every
// (carpark provider id, datatype) pair. It is hydrated at startup from
// BDP and updated on every Skidata push event. Aggregation methods
// derive carpark- and facility-level totals from the cache contents.
//
// Cache key shape: data[childProviderID][datatypeName] -> LatestRecord
// where childProviderID looks like "0600015_0" and datatypeName looks
// like "free", "occupied", "free_short_stay", etc.
type Cache struct {
	mu   sync.RWMutex
	data map[string]map[string]LatestRecord
}

func NewCache() *Cache {
	return &Cache{data: map[string]map[string]LatestRecord{}}
}

// Set replaces the cached value for a single (childProviderID, datatype).
func (c *Cache) Set(childID, datatype string, value int, ts int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	row, ok := c.data[childID]
	if !ok {
		row = map[string]LatestRecord{}
		c.data[childID] = row
	}
	row[datatype] = LatestRecord{Value: value, Timestamp: ts}
}

// Get returns the cached LatestRecord for a (childProviderID, datatype).
func (c *Cache) Get(childID, datatype string) (LatestRecord, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if row, ok := c.data[childID]; ok {
		rec, ok := row[datatype]
		return rec, ok
	}
	return LatestRecord{}, false
}

// CarparkOverall returns the carpark's "overall" free/occupied value
// using Skidata's category 3 (Totale) when available, otherwise summing
// every cached non-cat-3 category for that prefix.
//
// prefix is "free" or "occupied". The bare prefix (no suffix) corresponds
// to category 3; suffixed datatypes ("free_short_stay", etc.) are the
// other categories.
func (c *Cache) CarparkOverall(childID, prefix string) (int, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	row, ok := c.data[childID]
	if !ok {
		return 0, false
	}
	if rec, ok := row[prefix]; ok {
		return rec.Value, true
	}
	sum := 0
	found := false
	for k, rec := range row {
		if strings.HasPrefix(k, prefix+"_") {
			sum += rec.Value
			found = true
		}
	}
	return sum, found
}

// FacilityOverall returns the sum of CarparkOverall across every
// carpark belonging to facilityID (its child IDs start with
// facilityID + "_").
func (c *Cache) FacilityOverall(facilityID, prefix string) int {
	c.mu.RLock()
	childIDs := make([]string, 0, len(c.data))
	for k := range c.data {
		if strings.HasPrefix(k, facilityID+"_") {
			childIDs = append(childIDs, k)
		}
	}
	c.mu.RUnlock()

	sum := 0
	for _, id := range childIDs {
		if v, ok := c.CarparkOverall(id, prefix); ok {
			sum += v
		}
	}
	return sum
}

// FacilityPerCategory returns the sum of a specific datatype across all
// carparks of the facility (e.g. all "free_short_stay" rows for facility
// 0607242). Used to publish per-category facility totals.
func (c *Cache) FacilityPerCategory(facilityID, datatype string) int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	sum := 0
	for k, row := range c.data {
		if !strings.HasPrefix(k, facilityID+"_") {
			continue
		}
		if rec, ok := row[datatype]; ok {
			sum += rec.Value
		}
	}
	return sum
}

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
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
// derive facility-level totals from the cache contents.
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

// CarparkValue returns a single cached datatype value for a carpark.
//
// The carpark's overall free/occupied is the provider's own category-3 figure
// and nothing else. Per-category values are never summed into it: categories
// share physical slots and repartition at runtime, so adding them up overcounts
// a total the provider already states.
func (c *Cache) CarparkValue(childID, datatype string) (int, bool) {
	rec, ok := c.Get(childID, datatype)
	return rec.Value, ok
}

// FacilityTotal sums a datatype across exactly the carparks the facility is
// known to contain, and reports whether the sum is complete.
//
// carparkIDs comes from the counting categories, not from whatever happens to
// be in the cache. That distinction is the whole point: the provider reports
// one carpark at a time and a carpark can stay silent for many hours, so
// summing only what has been heard from produces a total that is quietly too
// low. An incomplete sum is not published at all.
func (c *Cache) FacilityTotal(carparkIDs []string, datatype string) (int, bool) {
	if len(carparkIDs) == 0 {
		return 0, false
	}
	c.mu.RLock()
	defer c.mu.RUnlock()

	sum := 0
	for _, id := range carparkIDs {
		row, ok := c.data[id]
		if !ok {
			return 0, false
		}
		rec, ok := row[datatype]
		if !ok {
			return 0, false
		}
		sum += rec.Value
	}
	return sum, true
}

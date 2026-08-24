// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "sort"

// Counting categories describe what a facility contains: its carparks, and per
// counting category the capacity and limits. They are collected by
// rest-push-skidata as a second flow and read here keyed by facility.
//
// They are the only source for two things a push event cannot supply. An event
// describes one carpark and one category, so it can never state the
// facility→carpark topology, and it carries a capacity that for some carparks
// is a live quota rather than a fact.

// Counting category ids used by the provider. They are stable across every
// facility — verified against four months of production traffic and against
// every counting-categories response: category 3 is the total everywhere
// ("Totale" or "Gesamt", never anything else), and no other id ever carries
// those names.
const (
	catShortStay   = 1
	catSubscribers = 2
	catTotal       = 3
)

const (
	// capacityUnknown is published when the provider does not state a usable
	// capacity. It is a value, not an absence: a station has to say "I do not
	// know" rather than imply a number nobody can act on.
	capacityUnknown = -1

	// sentinelCapacity is what the provider sends for a carpark with no
	// configured limit. Taken literally it produced free counts near ten
	// thousand, and facility aggregates near twenty thousand.
	sentinelCapacity = 9999
)

// CountingCategory is one row of the provider's countingcategories response,
// as republished by the collector.
type CountingCategory struct {
	CarparkId          int    `json:"carparkId"`
	CountingCategoryId int    `json:"countingCategoryId"`
	Name               string `json:"name"`
	Capacity           int    `json:"capacity"`
	OccupancyLimit     int    `json:"occupancyLimit"`
	FreeLimit          int    `json:"freeLimit"`
}

// FacilityCategories is one facility's complete category list — the payload of
// a single document in the counting-categories collection.
type FacilityCategories []CountingCategory

// CarparkIDs returns the facility's carpark ids, sorted.
//
// This is the topology, and it is why the flow exists: a facility total is the
// sum over its carparks, so it cannot be computed without knowing which
// carparks there are before any of them has reported.
func (f FacilityCategories) CarparkIDs() []int {
	seen := map[int]bool{}
	out := []int{}
	for _, c := range f {
		if !seen[c.CarparkId] {
			seen[c.CarparkId] = true
			out = append(out, c.CarparkId)
		}
	}
	sort.Ints(out)
	return out
}

// TotalCapacity returns a carpark's overall capacity — the category-3 entry,
// normalised. Returns capacityUnknown when there is no total row or the
// provider sent the sentinel.
//
// Only the total is read. The per-category capacities are a live quota for at
// least two carparks (short-stay and subscribers repartitioning a fixed pool
// minute by minute, always summing to the total), so storing them would be
// recording a measurement as though it were a fact.
func (f FacilityCategories) TotalCapacity(carparkID int) int {
	for _, c := range f {
		if c.CarparkId == carparkID && c.CountingCategoryId == catTotal {
			return normalizeCapacity(c.Capacity)
		}
	}
	return capacityUnknown
}

// FacilityCapacity sums the known carpark totals.
//
// Carparks whose capacity is unknown contribute nothing rather than zero, and a
// facility where none is known is itself unknown — a partial sum presented as a
// total is the failure this whole change is about.
func (f FacilityCategories) FacilityCapacity() int {
	sum, known := 0, false
	for _, id := range f.CarparkIDs() {
		if c := f.TotalCapacity(id); c != capacityUnknown {
			sum += c
			known = true
		}
	}
	if !known {
		return capacityUnknown
	}
	return sum
}

// normalizeCapacity turns anything unusable into capacityUnknown.
func normalizeCapacity(v int) int {
	if v >= sentinelCapacity || v < 0 {
		return capacityUnknown
	}
	return v
}

// suffixFor maps a counting category id to its datatype suffix, and reports
// whether the category is published at all.
//
// Only the total, short stay and subscribers are kept. Everything else the
// provider sends — per-floor occupancy arriving as countingAreaId, and the
// site-specific categories — is dropped here rather than deeper down, so there
// is exactly one place that decides what reaches the public API.
func suffixFor(categoryID int) (string, bool) {
	switch categoryID {
	case catTotal:
		return "", true
	case catShortStay:
		return "short_stay", true
	case catSubscribers:
		return "subscribers", true
	default:
		return "", false
	}
}

func freeType(suffix string) string {
	if suffix == "" {
		return "free"
	}
	return "free_" + suffix
}

func occupiedType(suffix string) string {
	if suffix == "" {
		return "occupied"
	}
	return "occupied_" + suffix
}

// publishedSuffixes is the complete, fixed set. It no longer depends on what
// the provider happens to send, which is what allows the datatype registration
// to be static.
var publishedSuffixes = []string{"", "short_stay", "subscribers"}

// allDataTypeNames returns every datatype this transformer publishes, sorted.
func allDataTypeNames() []string {
	out := make([]string, 0, 2*len(publishedSuffixes))
	for _, s := range publishedSuffixes {
		out = append(out, freeType(s), occupiedType(s))
	}
	sort.Strings(out)
	return out
}

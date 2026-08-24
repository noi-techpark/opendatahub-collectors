// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// ParkingEvent is a single Skidata push notification for one carpark and one
// counting category. It never describes a whole facility.
//
// Only the fields that are actually consumed are declared; everything else the
// provider sends is ignored on decode:
//
//   - freeLimit and occupancyLimit are signage thresholds, not availability.
//     They equal capacity (or capacity-1) on 87% of rows and carry the same
//     9999 sentinel, so they describe when the sign at the entrance turns red.
//   - trafficSignalState / trafficSignalMode are that sign's current state.
//   - countingAreaId, which some facilities send instead of countingCategoryId,
//     is deliberately not decoded. Those messages are per-floor occupancy; they
//     land here with CountingCategoryId 0 and are dropped as an unpublished
//     category, which is the intended outcome.
type ParkingEvent struct {
	// Name is the provider's label for the counting category, kept for logging
	// — the category id, not this, decides what is published.
	Name               string  `json:"name"`
	Level              int     `json:"level"`
	Capacity           int     `json:"capacity"`
	Carpark            Carpark `json:"carpark"`
	CountingCategoryId int     `json:"countingCategoryId"`
}

type Carpark struct {
	Name       string `json:"name"`
	FacilityNr int    `json:"facilityNr"`
	Id         int    `json:"id"`
}

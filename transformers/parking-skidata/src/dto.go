// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// ParkingEvent is a single Skidata push notification for one carpark + counting category.
type ParkingEvent struct {
	TrafficSignalState int     `json:"trafficSignalState"`
	Name               string  `json:"name"`
	FreeLimit          int     `json:"freeLimit"`
	Level              int     `json:"level"`
	Capacity           int     `json:"capacity"`
	OccupancyLimit     int     `json:"occupancyLimit"`
	ExternalCounting   bool    `json:"externalCounting"`
	TrafficSignalMode  int     `json:"trafficSignalMode"`
	Carpark            Carpark `json:"carpark"`
	CountingCategoryId int     `json:"countingCategoryId"`
}

type Carpark struct {
	Name       string `json:"name"`
	FacilityNr int    `json:"facilityNr"`
	Id         int    `json:"id"`
	ShortName  string `json:"shortName"`
}

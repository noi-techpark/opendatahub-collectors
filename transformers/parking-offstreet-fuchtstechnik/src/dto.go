// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// ParkingEvent is a single Fuchtstechnik parking event with one or more
// availability measurements for a parking facility.
type ParkingEvent struct {
	Id           string        `json:"id"`
	NameIT       string        `json:"name_IT"`
	NameDE       string        `json:"name_DE"`
	Latitude     float64       `json:"latitude"`
	Longitude    float64       `json:"longitude"`
	Capacity     int           `json:"capacity"`
	Measurements []Measurement `json:"measurements"`
}

type Measurement struct {
	Timestamp    string `json:"timestamp"`
	Availability int    `json:"availability"`
}

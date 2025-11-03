// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

type BikeBoxRawData struct {
	It  []BikeLocation `json:"it"`
	De  []BikeLocation `json:"de"`
	En  []BikeLocation `json:"en"`
	Lld []BikeLocation `json:"lld"`
}

type BikeLocation struct {
	LocationID int                   `json:"locationID"`
	Name       string                `json:"name"`
	Stations   []BikeLocationStation `json:"stations"`
}

type BikeLocationStation struct {
	StationID                              int         `json:"stationID"`
	LocationName                           string      `json:"locationName"`
	LocationID                             int         `json:"locationID"`
	Name                                   string      `json:"name"`
	Address                                string      `json:"address"`
	Latitude                               float64     `json:"latitude"`
	Longitude                              float64     `json:"longitude"`
	Type                                   int         `json:"type"`
	State                                  int         `json:"state"`
	CountFreePlacesAvailable_MuscularBikes int         `json:"countFreePlacesAvailable_MuscularBikes"`
	CountFreePlacesAvailable_AssistedBikes int         `json:"countFreePlacesAvailable_AssistedBikes"`
	CountFreePlacesAvailable               int         `json:"countFreePlacesAvailable"`
	TotalPlaces                            int         `json:"totalPlaces"`
	Places                                 []BikePlace `json:"places"`
}

type BikePlace struct {
	Position int `json:"position"`
	State    int `json:"state"`
	Level    int `json:"level"`
	Type     int `json:"type"`
}

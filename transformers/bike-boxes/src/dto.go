// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

type BikeBoxRawData struct {
	Locations []BikeLocation `json:"locations"`
}

type BikeLocation struct {
	LocationID              int                   `json:"locationID"`
	Name                    string                `json:"name"`
	Stations                []BikeLocationStation `json:"stations"`
	TranslatedLocationNames map[string]string     `json:"translatedLocationNames"`
}

type BikeLocationStation struct {
	StationID int    `json:"stationID"`
	Type      int    `json:"type"`
	Name      string `json:"name"`
}

type BikeStation struct {
	StationID                              int               `json:"stationID"`
	LocationName                           string            `json:"locationName"`
	TranslatedNames                        map[string]string `json:"translatedNames"`
	LocationID                             int               `json:"locationID"`
	Name                                   string            `json:"name"`
	Address                                string            `json:"address"`
	Addresses                              map[string]string `json:"addresses"`
	Latitude                               float64           `json:"latitude"`
	Longitude                              float64           `json:"longitude"`
	Type                                   int               `json:"type"`
	State                                  int               `json:"state"`
	CountFreePlacesAvailable_MuscularBikes int               `json:"countFreePlacesAvailable_MuscularBikes"`
	CountFreePlacesAvailable_AssistedBikes int               `json:"countFreePlacesAvailable_AssistedBikes"`
	CountFreePlacesAvailable               int               `json:"countFreePlacesAvailable"`
	TotalPlaces                            int               `json:"totalPlaces"`
	Places                                 []BikePlace       `json:"places"`
}
	
type BikePlace struct {
	Position int `json:"position"`
	State    int `json:"state"`
	Level    int `json:"level"`
	Type     int `json:"type"`
}

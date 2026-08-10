// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// RawData is the payload published by the api-crawler collector: the
// response of /resources/stations, fetched once per language.
type RawData struct {
	It  []Station `json:"it"`
	De  []Station `json:"de"`
	En  []Station `json:"en"`
	Lld []Station `json:"lld"`
}

// Station mirrors the Station schema of
// documentation/260430_openapi-suedtirolmobile-bikebox-api-loewenbytes-v1.0.1.yaml.
// stationID/locationID are strings per the OpenAPI schema (not integers).
type Station struct {
	StationID                             string  `json:"stationID"`
	LocationID                            string  `json:"locationID"`
	LocationName                          string  `json:"locationName"`
	Name                                  string  `json:"name"`
	Address                               string  `json:"address"`
	Latitude                              float64 `json:"latitude"`
	Longitude                             float64 `json:"longitude"`
	Type                                  int     `json:"type"`
	State                                 int     `json:"state"`
	CountFreePlacesAvailableMuscularBikes int     `json:"countFreePlacesAvailable_MuscularBikes"`
	CountFreePlacesAvailableAssistedBikes int     `json:"countFreePlacesAvailable_AssistedBikes"`
	CountFreePlacesAvailable              int     `json:"countFreePlacesAvailable"`
	TotalPlaces                           int     `json:"totalPlaces"`
	Places                                []Place `json:"places"`
}

// Place mirrors the Place schema (a parking slot within a station).
type Place struct {
	Position int `json:"position"`
	Type     int `json:"type"`
	State    int `json:"state"`
	Level    int `json:"level"`
}

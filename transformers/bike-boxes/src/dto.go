// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
)

type BikeBoxRawData struct {
	It  []BikeLocation `json:"it"`
	De  []BikeLocation `json:"de"`
	En  []BikeLocation `json:"en"`
	Lld []BikeLocation `json:"lld"`
}

type BikeLocation struct {
	LocationID FlexID                `json:"locationID"`
	Name       string                `json:"name"`
	Stations   []BikeLocationStation `json:"stations"`
}

type BikeLocationStation struct {
	StationID                              FlexID      `json:"stationID"`
	LocationName                           string      `json:"locationName"`
	LocationID                             FlexID      `json:"locationID"`
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

// FlexID accepts locationID/stationID as either a JSON string (loewenbytes) or a JSON number (bicincitta),
// preserving it as a string so arbitrary, non-numeric IDs are supported too.
type FlexID string

func (f *FlexID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err == nil {
		*f = FlexID(s)
		return nil
	}

	var n json.Number
	if err := json.Unmarshal(data, &n); err != nil {
		return fmt.Errorf("FlexID: cannot unmarshal %s as string or number", data)
	}
	*f = FlexID(n.String())
	return nil
}

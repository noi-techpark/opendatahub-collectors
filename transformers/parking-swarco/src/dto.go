// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"time"
)

// SwarcoData is the merged payload produced by the api-crawler configuration
// parking-swarco.silky.yaml: one crawl fetches both SMI endpoints and merges
// them under the static/dynamic keys.
type SwarcoData struct {
	Static  StaticPOIs  `json:"static"`
	Dynamic DynamicPOIs `json:"dynamic"`
}

type StaticPOIs struct {
	Total         int             `json:"total"`
	StaticPOIData []StaticPOIData `json:"staticPOIData"`
}

type DynamicPOIs struct {
	Total          int              `json:"total"`
	DynamicPOIData []DynamicPOIData `json:"dynamicPOIData"`
}

// StaticPOIData gives typed access to the fields the transformer needs, and
// keeps the complete raw object so every other provider field can be passed
// through to station metadata, as the integration specification requires.
type StaticPOIData struct {
	ObjectID          string           `json:"objectID"`
	GuID              *string          `json:"guID"`
	Name              string           `json:"name"`
	Type              string           `json:"type"`
	Location          *Location        `json:"location"`
	Capacity          []CapacityData   `json:"capacity"`
	Areas             []StaticAreaData `json:"areas"`
	OperationalStatus *bool            `json:"operationalStatus"`

	raw map[string]any
}

func (s *StaticPOIData) UnmarshalJSON(b []byte) error {
	type alias StaticPOIData
	var a alias
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	*s = StaticPOIData(a)
	s.raw = m
	return nil
}

type StaticAreaData struct {
	ObjectID string        `json:"objectID"`
	GuID     *string       `json:"guID"`
	Name     string        `json:"name"`
	Capacity *CapacityData `json:"capacity"`
}

type CapacityData struct {
	ParkingSpaceType string `json:"parkingSpaceType"`
	Capacity         int    `json:"capacity"`
}

type DynamicPOIData struct {
	TimeStamp      time.Time         `json:"timeStamp"`
	OccupancyTotal []OccupancyData   `json:"occupancyTotal"`
	OccupancyAreas []DynamicAreaData `json:"occupancyAreas"`
	ObjectID       string            `json:"objectID"`
	Name           string            `json:"name"`
}

type DynamicAreaData struct {
	Name      string         `json:"name"`
	ObjectID  string         `json:"objectId"`
	GuID      *string        `json:"guID"`
	Occupancy *OccupancyData `json:"occupancy"`
}

// OccupancyData fields are pointers because the SMI declares them optional:
// absent must be distinguishable from zero.
type OccupancyData struct {
	ParkingSpaceType string `json:"parkingSpaceType"`
	Capacity         *int   `json:"capacity"`
	VacantSpaces     *int   `json:"vacantSpaces"`
	OccupiedSpaces   *int   `json:"occupiedSpaces"`
}

// Location is one of point, polyline or polygon (SMI XSD 5.4.13).
type Location struct {
	Point    *Point    `json:"point"`
	Polyline *Polyline `json:"polyline"`
	Polygon  *Polygon  `json:"polygon"`
}

type Point struct {
	Coordinates *Coordinates `json:"coordinates"`
	Index       *int         `json:"index"`
}

type Polyline struct {
	Vertices []Point `json:"vertices"`
}

type Polygon struct {
	Vertices []Point `json:"vertices"`
	Center   *Point  `json:"center"`
}

type Coordinates struct {
	CoordinateSystem string  `json:"coordinateSystem"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
}

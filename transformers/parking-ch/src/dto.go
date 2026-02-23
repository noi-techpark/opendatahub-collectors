// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

// Root holds the top-level payload structure from the collector
type Root struct {
	BikeParking GeoJSONFeatureCollection `json:"bike_parking"`
	CarParking  GeoJSONFeatureCollection `json:"car_parking"`
}

// GeoJSONFeatureCollection represents a standard GeoJSON Feature Collection
type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

// GeoJSONFeature represents a single GeoJSON Feature
type GeoJSONFeature struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Geometry   GeoJSONGeometry        `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

// GeoJSONGeometry represents a Point geometry
// Coordinates are [longitude, latitude] per GeoJSON spec
type GeoJSONGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"` // [longitude, latitude]
}

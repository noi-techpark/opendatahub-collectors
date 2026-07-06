// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "encoding/json"

// Root holds the top-level payload structure from the collector: a single
// GeoJSON FeatureCollection mixing bike and car parking facilities,
// distinguished by the "parkingFacilityCategory" property.
type Root struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

// GeoJSONFeature represents a single GeoJSON Feature
type GeoJSONFeature struct {
	Type       string                    `json:"type"`
	ID         string                    `json:"id"`
	Geometry   GeoJSONGeometryCollection `json:"geometry"`
	Properties map[string]interface{}    `json:"properties"`
}

// GeoJSONGeometryCollection represents the feature's geometry, which is a
// GeometryCollection: always includes one Point (the facility location) and
// may include a Polygon/MultiPolygon (the facility area).
type GeoJSONGeometryCollection struct {
	Type       string            `json:"type"`
	Geometries []GeoJSONGeometry `json:"geometries"`
}

// GeoJSONGeometry represents a single geometry within a GeometryCollection.
// Coordinates are kept raw since their shape depends on Type (a flat
// [lon, lat] pair for Point, nested rings for Polygon/MultiPolygon).
type GeoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

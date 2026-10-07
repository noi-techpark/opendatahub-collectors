// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package test contains integration tests for the parking-ch transformer
package test

import "encoding/json"

// GeoJSON structures matching the transformer DTO
type GeoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type GeoJSONGeometryCollection struct {
	Type       string            `json:"type"`
	Geometries []GeoJSONGeometry `json:"geometries"`
}

type GeoJSONFeature struct {
	Type       string                    `json:"type"`
	ID         string                    `json:"id"`
	Geometry   GeoJSONGeometryCollection `json:"geometry"`
	Properties map[string]interface{}    `json:"properties"`
}

type ParkingData struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

// TestMetrics holds statistics about test data
type TestMetrics struct {
	BikeFeatures int
	CarFeatures  int
	Measurements int
}

// Helper to count test data
func (p ParkingData) GetMetrics() TestMetrics {
	var metrics TestMetrics
	for _, feature := range p.Features {
		props := feature.Properties
		if props == nil {
			continue
		}
		switch props["parkingFacilityCategory"] {
		case "BIKE":
			metrics.BikeFeatures++
		case "CAR":
			metrics.CarFeatures++
		}
		if props["currentEstimatedOccupancy"] != nil ||
			props["currentEstimatedOccupancyLevel"] != nil {
			metrics.Measurements++
		}
	}
	return metrics
}

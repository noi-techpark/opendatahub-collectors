// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package test contains integration tests for the parking-ch transformer
package test

// GeoJSON structures matching the transformer DTO
type GeoJSONGeometry struct {
	Type        string    `json:"type"`
	Coordinates []float64 `json:"coordinates"` // [longitude, latitude]
}

type GeoJSONFeature struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Geometry   GeoJSONGeometry        `json:"geometry"`
	Properties map[string]interface{} `json:"properties"`
}

type GeoJSONFeatureCollection struct {
	Type     string           `json:"type"`
	Features []GeoJSONFeature `json:"features"`
}

type ParkingData struct {
	BikeParking GeoJSONFeatureCollection `json:"bike_parking"`
	CarParking  GeoJSONFeatureCollection `json:"car_parking"`
}

// TestMetrics holds statistics about test data
type TestMetrics struct {
	BikeFeatures int
	CarFeatures  int
	Measurements int
}

// Helper to count test data
func (p ParkingData) GetMetrics() TestMetrics {
	measurements := 0
	for _, feature := range p.CarParking.Features {
		props := feature.Properties
		if props == nil {
			continue
		}
		if props["currentEstimatedOccupancy"] != nil ||
			props["currentEstimatedOccupancyLevel"] != nil ||
			props["predictedForecastedOccupancy"] != nil {
			measurements++
		}
	}
	return TestMetrics{
		BikeFeatures: len(p.BikeParking.Features),
		CarFeatures:  len(p.CarParking.Features),
		Measurements: measurements,
	}
}

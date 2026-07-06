// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

// bikeAndCarParkingURL is the permalink for the merged Swiss OGD bike-and-car
// parking dataset (same source used by the rest-poller collector).
const bikeAndCarParkingURL = "https://data.opentransportdata.swiss/dataset/bike-and-car-parking/resource_permalink/bike-and-car-parking.json"

// TestTransformerIntegration validates parking data structure using real data from Swiss OGD
func TestTransformerIntegration(t *testing.T) {
	t.Log("\n*** Starting Parking-CH Transformer Integration Test ***")
	t.Log("   Fetching real data from the merged Swiss OGD bike-and-car-parking endpoint")

	// Fetch real data from the Swiss OGD source (same source used by rest-poller)
	parkingData := fetchParkingData(t)

	// If we couldn't fetch real data, fall back to minimal test data
	if parkingData == nil {
		t.Log("   Using fallback test data (endpoint not accessible)")
		parkingData = getFallbackTestData()
	}

	metrics := parkingData.GetMetrics()
	t.Logf("   Loaded: %d bike features, %d car features\n",
		metrics.BikeFeatures, metrics.CarFeatures)

	// Validate feature structure (geometry, category, station code fields)
	t.Run("ValidateFeatureStructure", func(t *testing.T) {
		if len(parkingData.Features) == 0 {
			t.Fatal("Expected parking features")
		}

		for i, feature := range parkingData.Features {
			props := feature.Properties

			category, _ := props["parkingFacilityCategory"].(string)
			if category != "BIKE" && category != "CAR" {
				t.Errorf("Feature %d: unexpected parkingFacilityCategory %q", i, category)
			}

			if _, ok := props["displayName"]; !ok {
				t.Errorf("Feature %d: Missing displayName", i)
			}

			switch category {
			case "BIKE":
				if _, ok := props["uic"]; !ok {
					t.Errorf("Feature %d: Missing uic", i)
				}
			case "CAR":
				if _, ok := props["didokId"]; !ok {
					t.Errorf("Feature %d: Missing didokId", i)
				}
			}

			// Check required Point geometry within the GeometryCollection
			hasPoint := false
			for _, g := range feature.Geometry.Geometries {
				if g.Type != "Point" {
					continue
				}
				hasPoint = true
				var coords []float64
				if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
					t.Errorf("Feature %d: invalid point coordinates: %v", i, err)
					continue
				}
				if len(coords) != 2 {
					t.Errorf("Feature %d: Expected 2 coordinates [lon,lat], got %d", i, len(coords))
				}
			}
			if !hasPoint {
				t.Errorf("Feature %d: Expected a Point geometry", i)
			}
		}
		t.Log("✓ Feature structure validation passed")
	})

	// Validate coordinate transformation
	t.Run("ValidateCoordinateTransformation", func(t *testing.T) {
		fmt.Println("\n--- Coordinate Transformation Validation ---")

		for _, feature := range parkingData.Features {
			for _, g := range feature.Geometry.Geometries {
				if g.Type != "Point" {
					continue
				}
				var coords []float64
				if err := json.Unmarshal(g.Coordinates, &coords); err != nil || len(coords) < 2 {
					continue
				}
				lon, lat := coords[0], coords[1]

				if lon < -180 || lon > 180 {
					t.Errorf("Invalid longitude: %f", lon)
				}
				if lat < -90 || lat > 90 {
					t.Errorf("Invalid latitude: %f", lat)
				}

				fmt.Printf("Feature %s: [%f, %f] (lon, lat)\n", feature.ID, lon, lat)
			}
		}
		t.Log("✓ Coordinate validation passed")
	})

	// Validate measurement data types
	t.Run("ValidateMeasurementDataTypes", func(t *testing.T) {
		fmt.Println("\n--- Measurement Data Type Validation ---")

		validateCount := 0
		for _, feature := range parkingData.Features {
			props := feature.Properties

			// Only validate features that have measurements
			hasOccupancy := props["currentEstimatedOccupancy"] != nil
			hasLevel := props["currentEstimatedOccupancyLevel"] != nil

			if !hasOccupancy && !hasLevel {
				continue // Skip features without measurements
			}

			validateCount++

			// currentEstimatedOccupancy should be number (if present)
			if occ, ok := props["currentEstimatedOccupancy"]; ok && occ != nil {
				if _, ok := occ.(float64); !ok {
					t.Errorf("Feature %s: Expected float64 for occupancy, got %T", feature.ID, occ)
				}
			}

			// currentEstimatedOccupancyLevel should be string (if present)
			if level, ok := props["currentEstimatedOccupancyLevel"]; ok && level != nil {
				if _, ok := level.(string); !ok {
					t.Errorf("Feature %s: Expected string for level, got %T", feature.ID, level)
				}
			}
		}

		// Use pre-computed metrics (single source of truth via GetMetrics) to assert
		// at least one feature has occupancy data in any healthy data feed
		if metrics.Measurements == 0 {
			t.Errorf("Expected at least one feature with occupancy data, got none")
		} else {
			t.Logf("%d/%d features have occupancy data", metrics.Measurements, len(parkingData.Features))
		}

		t.Logf("Validated %d features with measurement data\n", validateCount)
		t.Log("✓ Measurement data type validation passed")
	})

	t.Log("\n*** All Integration Tests Passed ***\n")
}

// fetchParkingData fetches real parking data from the merged Swiss OGD endpoint
func fetchParkingData(t *testing.T) *ParkingData {
	resp, err := http.Get(bikeAndCarParkingURL)
	if err != nil {
		t.Logf("Warning: Failed to fetch %s: %v", bikeAndCarParkingURL, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Warning: Endpoint returned status %d: %s", resp.StatusCode, bikeAndCarParkingURL)
		return nil
	}

	var data ParkingData
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		t.Logf("Warning: Failed to decode JSON from %s: %v", bikeAndCarParkingURL, err)
		return nil
	}

	if len(data.Features) == 0 {
		return nil
	}

	return &data
}

// getFallbackTestData provides minimal test data for when the real endpoint is not accessible
func getFallbackTestData() *ParkingData {
	return &ParkingData{
		Type: "FeatureCollection",
		Features: []GeoJSONFeature{
			{
				Type: "Feature",
				ID:   "bike-001",
				Geometry: GeoJSONGeometryCollection{
					Type: "GeometryCollection",
					Geometries: []GeoJSONGeometry{
						{Type: "Point", Coordinates: json.RawMessage(`[11.3521, 46.4983]`)},
					},
				},
				Properties: map[string]interface{}{
					"parkingFacilityCategory": "BIKE",
					"uic":                     8503010,
					"displayName":             "Merano Central Station Bike Parking",
				},
			},
			{
				Type: "Feature",
				ID:   "car-001",
				Geometry: GeoJSONGeometryCollection{
					Type: "GeometryCollection",
					Geometries: []GeoJSONGeometry{
						{Type: "Point", Coordinates: json.RawMessage(`[8.2275, 46.1991]`)},
					},
				},
				Properties: map[string]interface{}{
					"parkingFacilityCategory":        "CAR",
					"didokId":                        "8596002",
					"displayName":                    "Bern City Center Parking",
					"operator":                       "City of Bern",
					"predictedForecastedOccupancy":   []interface{}{0.5, 0.55, 0.6, 0.65, 0.7},
					"currentEstimatedOccupancy":      75.5,
					"currentEstimatedOccupancyLevel": "HIGH",
				},
			},
		},
	}
}

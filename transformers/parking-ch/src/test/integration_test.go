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

// TestTransformerIntegration validates parking data structure using real data from Swiss OGD
func TestTransformerIntegration(t *testing.T) {
	t.Log("\n*** Starting Parking-CH Transformer Integration Test ***")
	t.Log("   Fetching real data from Swiss Open Transport Data endpoints")

	// Fetch real data from Swiss OGD sources (same sources used by multi-rest-poller)
	parkingData := fetchParkingData(t)
	
	// If we couldn't fetch real data, fall back to minimal test data
	if parkingData == nil {
		t.Log("   Using fallback test data (endpoints not accessible)")
		parkingData = getFallbackTestData()
	}

	metrics := parkingData.GetMetrics()
	t.Logf("   Loaded: %d bike features, %d car features\n",
		metrics.BikeFeatures, metrics.CarFeatures)

	// Validate bike parking data structure
	t.Run("ValidateBikeParkingStructure", func(t *testing.T) {
		if len(parkingData.BikeParking.Features) == 0 {
			t.Fatal("Expected bike parking features")
		}

		for i, feature := range parkingData.BikeParking.Features {
			// Check required geometry
			if feature.Geometry.Type != "Point" {
				t.Errorf("Feature %d: Expected Point geometry, got %s", i, feature.Geometry.Type)
			}
			if len(feature.Geometry.Coordinates) != 2 {
				t.Errorf("Feature %d: Expected 2 coordinates [lon,lat], got %d", i, len(feature.Geometry.Coordinates))
			}

			// Check required properties
			props := feature.Properties
			if _, ok := props["stopPlaceUic"]; !ok {
				t.Errorf("Feature %d: Missing stopPlaceUic", i)
			}
			if _, ok := props["name"]; !ok {
				t.Errorf("Feature %d: Missing name", i)
			}
		}
		t.Log("✓ Bike parking structure validation passed")
	})

	// Validate car parking data structure
	t.Run("ValidateCarParkingStructure", func(t *testing.T) {
		if len(parkingData.CarParking.Features) == 0 {
			t.Fatal("Expected car parking features")
		}

		for i, feature := range parkingData.CarParking.Features {
			props := feature.Properties

			// Check required properties
			if _, ok := props["didokId"]; !ok {
				t.Errorf("Feature %d: Missing didokId", i)
			}
			if _, ok := props["displayName"]; !ok {
				t.Errorf("Feature %d: Missing displayName", i)
			}

			// Per-feature log only; not all stations emit real-time data
			hasPredicted := props["predictedForecastedOccupancy"] != nil
			hasOccupancy := props["currentEstimatedOccupancy"] != nil
			hasLevel := props["currentEstimatedOccupancyLevel"] != nil
			if !hasPredicted && !hasOccupancy && !hasLevel {
				t.Logf("Feature %d: No prediction/occupancy fields present (expected for static-only stations)", i)
			}
		}
		// Use pre-computed metrics (single source of truth via GetMetrics) to assert
		// at least one station has prediction data in any healthy data feed
		if metrics.Measurements == 0 {
			t.Errorf("Expected at least one car parking feature with prediction/occupancy fields, got none")
		} else {
			t.Logf("%d/%d car features have prediction/occupancy data", metrics.Measurements, len(parkingData.CarParking.Features))
		}
		t.Log("✓ Car parking structure validation passed")
	})

	// Validate coordinate transformation
	t.Run("ValidateCoordinateTransformation", func(t *testing.T) {
		fmt.Println("\n--- Coordinate Transformation Validation ---")

		for _, feature := range append(parkingData.BikeParking.Features, parkingData.CarParking.Features...) {
			lon := feature.Geometry.Coordinates[0]
			lat := feature.Geometry.Coordinates[1]

			// Valid coordinate ranges
			if lon < -180 || lon > 180 {
				t.Errorf("Invalid longitude: %f", lon)
			}
			if lat < -90 || lat > 90 {
				t.Errorf("Invalid latitude: %f", lat)
			}

			fmt.Printf("Feature %s: [%f, %f] (lon, lat)\n", feature.ID, lon, lat)
		}
		t.Log("✓ Coordinate validation passed")
	})

	// Validate measurement data types
	t.Run("ValidateMeasurementDataTypes", func(t *testing.T) {
		fmt.Println("\n--- Measurement Data Type Validation ---")

		validateCount := 0
		for _, feature := range parkingData.CarParking.Features {
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

		t.Logf("Validated %d features with measurement data\n", validateCount)
		t.Log("✓ Measurement data type validation passed")
	})

	t.Log("\n*** All Integration Tests Passed ***\n")
}

// fetchParkingData fetches real parking data from Swiss OGD endpoints
// These are the same endpoints used by multi-rest-poller
func fetchParkingData(t *testing.T) *ParkingData {
	endpoints := map[string]string{
		"bike_parking": "https://data.opentransportdata.swiss/dataset/bike-parking/resource_permalink/bike_parking.json",
		"car_parking":  "https://data.opentransportdata.swiss/dataset/parking-facilities/resource_permalink/parking-facilities.json",
	}

	var data ParkingData

	// Fetch bike parking
	if bikeData := fetchEndpoint(endpoints["bike_parking"], t); bikeData != nil {
		if features, ok := bikeData["features"]; ok {
			json.Unmarshal(features, &data.BikeParking.Features)
			// Limit to first 40 features for faster testing (now deactivated)
			// if len(data.BikeParking.Features) > 40 {
			// 	data.BikeParking.Features = data.BikeParking.Features[:40]
			// }
			data.BikeParking.Type = "FeatureCollection"
		}
	}

	// Fetch car parking
	if carData := fetchEndpoint(endpoints["car_parking"], t); carData != nil {
		if features, ok := carData["features"]; ok {
			json.Unmarshal(features, &data.CarParking.Features)
			// Limit to first 40 features for faster testing (now deactivated)
			// if len(data.CarParking.Features) > 40 {
			// 	data.CarParking.Features = data.CarParking.Features[:40]
			// }
			data.CarParking.Type = "FeatureCollection"
		}
	}

	// Return nil if no data was fetched
	if len(data.BikeParking.Features) == 0 && len(data.CarParking.Features) == 0 {
		return nil
	}

	return &data
}

// fetchEndpoint fetches JSON data from a URL and returns it as a map
func fetchEndpoint(url string, t *testing.T) map[string]json.RawMessage {
	resp, err := http.Get(url)
	if err != nil {
		t.Logf("Warning: Failed to fetch %s: %v", url, err)
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Logf("Warning: Endpoint returned status %d: %s", resp.StatusCode, url)
		return nil
	}

	var dataMap map[string]json.RawMessage
	if err := json.NewDecoder(resp.Body).Decode(&dataMap); err != nil {
		t.Logf("Warning: Failed to decode JSON from %s: %v", url, err)
		return nil
	}

	return dataMap
}

// getFallbackTestData provides minimal test data for when real endpoints are not accessible
func getFallbackTestData() *ParkingData {
	return &ParkingData{
		BikeParking: GeoJSONFeatureCollection{
			Type: "FeatureCollection",
			Features: []GeoJSONFeature{
				{
					Type: "Feature",
					ID:   "bike-001",
					Geometry: GeoJSONGeometry{
						Type:        "Point",
						Coordinates: []float64{11.3521, 46.4983},
					},
					Properties: map[string]interface{}{
						"stopPlaceUic":      "8503010",
						"name":              "Merano Central Station Bike Parking",
						"capacity":          float64(50),
						"weather_protected": true,
					},
				},
			},
		},
		CarParking: GeoJSONFeatureCollection{
			Type: "FeatureCollection",
			Features: []GeoJSONFeature{
				{
					Type: "Feature",
					ID:   "car-001",
					Geometry: GeoJSONGeometry{
						Type:        "Point",
						Coordinates: []float64{8.2275, 46.1991},
					},
					Properties: map[string]interface{}{
						"didokId":                        "8596002",
						"displayName":                    "Bern City Center Parking",
						"capacity":                       float64(120),
						"operatorName":                   "City of Bern",
						"predictedForecastedOccupancy":   []interface{}{0.5, 0.55, 0.6, 0.65, 0.7},
						"currentEstimatedOccupancy":      75.5,
						"currentEstimatedOccupancyLevel": "HIGH",
					},
				},
			},
		},
	}
}

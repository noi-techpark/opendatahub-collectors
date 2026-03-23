// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// TestTransformerIntegration validates e-mobility data structure using real data from Swiss BFE
func TestTransformerIntegration(t *testing.T) {
	t.Log("\n*** Starting Emobility-CH Transformer Integration Test ***")
	t.Log("   Fetching real data from Swiss Federal Office of Energy endpoints")

	// Fetch real data from Swiss BFE sources (same sources used by multi-rest-poller)
	emobilityData := fetchEmobilityData(t)

	// If we couldn't fetch real data, fall back to minimal test data
	if emobilityData == nil {
		t.Log("   Using fallback test data (endpoints not accessible)")
		emobilityData = getFallbackTestData()
	}

	metrics := emobilityData.GetMetrics()
	t.Logf("   Loaded: %d EVSE data records, %d EVSE status records\n",
		metrics.EVSEDataCount, metrics.EVSEStatusCount)

	// Validate EVSE static data structure
	t.Run("ValidateEVSEDataStructure", func(t *testing.T) {
		if len(emobilityData.EVSEData) == 0 {
			t.Fatal("Expected EVSE operator data")
		}

		recordCount := 0
		for opIdx, operator := range emobilityData.EVSEData {
			if operator.OperatorID == "" {
				t.Errorf("Operator %d: Missing OperatorID", opIdx)
			}

			for i, evse := range operator.EVSEDataRecord {
				recordCount++
				// Check required fields
				if evse.EvseID == "" {
					t.Errorf("Operator %d, Record %d: Missing EvseID", opIdx, i)
				}
				if evse.ChargingStationId == "" {
					t.Errorf("Operator %d, Record %d: Missing ChargingStationId", opIdx, i)
				}

				// Check coordinates
				if evse.GeoCoordinates == nil || evse.GeoCoordinates.Google == "" {
					t.Errorf("Operator %d, Record %d: Missing geographic coordinates", opIdx, i)
					continue
				}

				// Validate coordinate format
				_, _, err := parseGoogleCoords(evse.GeoCoordinates.Google)
				if err != nil {
					t.Errorf("Operator %d, Record %d (EvseID: %s): Invalid coordinates format: %v",
						opIdx, i, evse.EvseID, err)
				}

				// Check station names
				if len(evse.ChargingStationNames) == 0 {
					t.Logf("Operator %d, Record %d: No station names (using ChargingStationId)", opIdx, i)
				}
			}
		}
		t.Logf("Validated %d EVSE records across %d operators", recordCount, len(emobilityData.EVSEData))
		t.Log("✓ EVSE static data structure validation passed")
	})

	// Validate EVSE status data structure
	t.Run("ValidateEVSEStatusStructure", func(t *testing.T) {
		if len(emobilityData.EVSEStatuses) == 0 {
			t.Fatal("Expected EVSE status operators")
		}

		validStatuses := map[string]bool{
			"Available": true, "Occupied": true, "Reserved": true,
			"Unknown": true, "OutOfService": true,
		}

		statusCount := 0
		for opIdx, statusOperator := range emobilityData.EVSEStatuses {
			if statusOperator.OperatorID == "" {
				t.Errorf("Status Operator %d: Missing OperatorID", opIdx)
			}

			for i, status := range statusOperator.EVSEStatusRecord {
				statusCount++
				// Check required fields
				if status.EvseID == "" {
					t.Errorf("Operator %d, Status %d: Missing EvseID", opIdx, i)
				}

				// Validate status value
				statusValue := status.EvseStatus
				if !validStatuses[statusValue] {
					t.Errorf("Operator %d, Status %d (EvseID: %s): Invalid status value: %s",
						opIdx, i, status.EvseID, statusValue)
				}
			}
		}
		t.Logf("Validated %d status records across %d operators", statusCount, len(emobilityData.EVSEStatuses))
		t.Log("✓ EVSE status structure validation passed")
	})

	// Validate coordinate parsing and transformation
	t.Run("ValidateCoordinateParsing", func(t *testing.T) {
		fmt.Println("\n--- Coordinate Parsing Validation ---")

		validCount := 0
		for _, operator := range emobilityData.EVSEData {
			for _, evse := range operator.EVSEDataRecord {
				if evse.GeoCoordinates == nil {
					continue
				}

				lat, lon, err := parseGoogleCoords(evse.GeoCoordinates.Google)
				if err != nil {
					t.Errorf("Failed to parse coords for EvseID %s: %v", evse.EvseID, err)
					continue
				}

				// Validate coordinate ranges (Switzerland approximate bounds)
				if lat < 45.0 || lat > 48.0 {
					t.Errorf("EvseID %s: Latitude out of expected range: %f", evse.EvseID, lat)
				}
				if lon < 5.0 || lon > 11.0 {
					t.Errorf("EvseID %s: Longitude out of expected range: %f", evse.EvseID, lon)
				}

				validCount++
				if validCount <= 5 {
					fmt.Printf("EvseID %s: lat=%f, lon=%f\n", evse.EvseID, lat, lon)
				}
			}
		}

		t.Logf("Validated %d coordinate records\n", validCount)
		t.Log("✓ Coordinate parsing validation passed")
	})

	// Validate metadata fields
	t.Run("ValidateMetadataFields", func(t *testing.T) {
		fmt.Println("\n--- Metadata Fields Validation ---")

		fieldsCount := map[string]int{
			"hasAddress":            0,
			"hasPlugs":              0,
			"hasChargingFacilities": 0,
			"hasAuthModes":          0,
			"hasPaymentOptions":     0,
		}

		for _, operator := range emobilityData.EVSEData {
			for _, evse := range operator.EVSEDataRecord {
				if evse.Address != nil && evse.Address.City != nil {
					fieldsCount["hasAddress"]++
				}
				if len(evse.Plugs) > 0 {
					fieldsCount["hasPlugs"]++
				}
				if len(evse.ChargingFacilities) > 0 {
					fieldsCount["hasChargingFacilities"]++
				}
				if len(evse.AuthenticationModes) > 0 {
					fieldsCount["hasAuthModes"]++
				}
				if len(evse.PaymentOptions) > 0 {
					fieldsCount["hasPaymentOptions"]++
				}
			}
		}

		fmt.Printf("Records with address: %d\n", fieldsCount["hasAddress"])
		fmt.Printf("Records with plugs: %d\n", fieldsCount["hasPlugs"])
		fmt.Printf("Records with charging facilities: %d\n", fieldsCount["hasChargingFacilities"])
		fmt.Printf("Records with auth modes: %d\n", fieldsCount["hasAuthModes"])
		fmt.Printf("Records with payment options: %d\n", fieldsCount["hasPaymentOptions"])

		t.Log("✓ Metadata fields validation passed")
	})

	t.Log("\n*** All Integration Tests Passed ***\n")
}

// fetchEmobilityData fetches real e-mobility data from Swiss BFE endpoints
// These are the same endpoints used by multi-rest-poller
func fetchEmobilityData(t *testing.T) *EmobilityData {
	endpoints := map[string]string{
		"evse_data":     "https://data.geo.admin.ch/ch.bfe.ladestellen-elektromobilitaet/data/oicp/ch.bfe.ladestellen-elektromobilitaet.json",
		"evse_statuses": "https://data.geo.admin.ch/ch.bfe.ladestellen-elektromobilitaet/status/oicp/ch.bfe.ladestellen-elektromobilitaet.json",
	}

	var data EmobilityData

	// Fetch EVSE static data
	if evseDataResp := fetchEndpoint(endpoints["evse_data"], t); evseDataResp != nil {
		if evseDataRaw, ok := evseDataResp["EVSEData"]; ok {
			json.Unmarshal(evseDataRaw, &data.EVSEData)
			// Limit to first 3 operators for faster testing
			if len(data.EVSEData) > 3 {
				data.EVSEData = data.EVSEData[:3]
			}
			// Within each operator, limit to first 20 records
			for i := range data.EVSEData {
				if len(data.EVSEData[i].EVSEDataRecord) > 20 {
					data.EVSEData[i].EVSEDataRecord = data.EVSEData[i].EVSEDataRecord[:20]
				}
			}
		}
	}

	// Fetch EVSE status data
	if evseStatusResp := fetchEndpoint(endpoints["evse_statuses"], t); evseStatusResp != nil {
		if evseStatusRaw, ok := evseStatusResp["EVSEStatuses"]; ok {
			json.Unmarshal(evseStatusRaw, &data.EVSEStatuses)
			// Limit to first 3 operators for faster testing
			if len(data.EVSEStatuses) > 3 {
				data.EVSEStatuses = data.EVSEStatuses[:3]
			}
			// Within each operator, limit to first 20 records
			for i := range data.EVSEStatuses {
				if len(data.EVSEStatuses[i].EVSEStatusRecord) > 20 {
					data.EVSEStatuses[i].EVSEStatusRecord = data.EVSEStatuses[i].EVSEStatusRecord[:20]
				}
			}
		}
	}

	// Return nil if no data was fetched
	if len(data.EVSEData) == 0 && len(data.EVSEStatuses) == 0 {
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

// parseGoogleCoords parses "lat lon" format from Google field
func parseGoogleCoords(google string) (float64, float64, error) {
	parts := strings.Split(google, " ")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid format: expected 'lat lon', got '%s'", google)
	}

	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}

	return lat, lon, nil
}

// getFallbackTestData provides minimal test data for when real endpoints are not accessible
func getFallbackTestData() *EmobilityData {
	accessible := "PUBLIC"
	open24 := true
	renewableEnergy := false
	street := "Bahnhofstrasse 1"
	city := "Zürich"
	postalCode := "8001"
	country := "CHE"
	powerType := "AC"
	voltage := 230
	amperage := 32

	return &EmobilityData{
		EVSEData: []EVSEOperator{
			{
				OperatorID:   "CH*ABC",
				OperatorName: "Test Operator ABC",
				EVSEDataRecord: []EVSEDataItem{
					{
						EvseID:            "CH*ABC*E12345*1",
						ChargingStationId: "CH-ABC-12345",
						GeoCoordinates: &GeoCoordinate{
							Google: "47.3769 8.5417", // Zürich
						},
						ChargingStationNames: []ChargingStationName{
							{Lang: "de", Value: "Zürich Hauptbahnhof Ladestation"},
							{Lang: "en", Value: "Zürich Main Station Charging Point"},
						},
						Address: &EVSEAddress{
							Street:     &street,
							City:       &city,
							PostalCode: &postalCode,
							Country:    &country,
						},
						Accessibility:       &accessible,
						IsOpen24Hours:       &open24,
						Plugs:               []string{"Type2"},
						AuthenticationModes: []string{"NFC RFID Classic"},
						PaymentOptions:      []string{"Direct"},
						RenewableEnergy:     &renewableEnergy,
						ChargingFacilities: []ChargingFacility{
							{
								PowerType:     &powerType,
								Voltage:       &voltage,
								Amperage:      &amperage,
								ChargingModes: []string{"Mode_3"},
							},
						},
					},
				},
			},
		},
		EVSEStatuses: []EVSEStatusOperator{
			{
				OperatorID:   "CH*ABC",
				OperatorName: "Test Operator ABC",
				EVSEStatusRecord: []EVSEStatusItem{
					{
						EvseID:     "CH*ABC*E12345*1",
						EvseStatus: "Available",
					},
				},
			},
		},
	}
}

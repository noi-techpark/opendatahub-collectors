// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/noi-techpark/opendatahub-collectors/transformers/utils/testutils"
)

// TestTransformerIntegration validates data fetching and type resolution
func TestTransformerIntegration(t *testing.T) {
	fmt.Println("\n Starting Transformer Integration Test")
	fmt.Println("   Fetching real data from collector endpoints")
	
	// 1. Fetch real data
	root := fetchAllData(t)
	
	// Count input data
	inputMetrics := testutils.TransformMetrics{
		TotalProviders: len(root.Providers),
		TotalStations:  len(root.StationInformation),
		TotalVehicles:  len(root.FreeBikeStatus),
		StationsByType: make(map[string]int),
		VehiclesByType: make(map[string]int),
		DataPointsByType: make(map[string]int),
	}
	
	fmt.Printf("   Fetched %d providers, %d stations, %d vehicles\n", 
		inputMetrics.TotalProviders, inputMetrics.TotalStations, inputMetrics.TotalVehicles)
	
	// 2. Build provider map
	providersMap := make(map[string]Provider)
	for _, p := range root.Providers {
		providersMap[p.ProviderID] = p
	}
	
	// 3. Analyze vehicle types
	fmt.Println("\n  Analyzing vehicle types...")
	vehicleTypeCounts := make(map[string]int)
	
	for _, v := range root.FreeBikeStatus {
		serviceType := GetVehicleTypeFromVehicleTypeID(v.VehicleTypeID, providersMap)
		vehicleType := GetStationTypeForVehicle(serviceType)
		vehicleTypeCounts[vehicleType]++
		
		if serviceType == "SharingMobilityService" {
			inputMetrics.GenericTypeVehicles++
		}
	}
	
	for vType, count := range vehicleTypeCounts {
		inputMetrics.VehiclesByType[vType] = count
	}
	
	// 4. Count orphaned stations
	for _, s := range root.StationInformation {
		if s.RegionID == "" || s.RegionID == ":" {
			inputMetrics.OrphanedStations++
		}
	}
	
	// 5. Print report
	inputMetrics.PrintReport()
	
	// 6. Assertions
	if len(root.Providers) == 0 {
		t.Error("Expected providers, got 0")
	}
	
	if len(root.FreeBikeStatus) == 0 {
		t.Error("Expected vehicles, got 0")
	}
	
	fmt.Println("\n Integration test completed successfully!")
}

// fetchAllData fetches data from all Swiss mobility endpoints
func fetchAllData(t *testing.T) Root {
	var root Root
	
	endpoints := map[string]string{
		"providers":           "https://sharedmobility.ch/providers.json",
		"station_information": "https://sharedmobility.ch/station_information.json",
		"free_bike_status":    "https://sharedmobility.ch/free_bike_status.json",
		"system_regions":      "https://sharedmobility.ch/system_regions.json",
	}
	
	// Fetch providers
	if data := fetchEndpoint(endpoints["providers"], t); data != nil {
		if providers, ok := data["providers"]; ok {
			json.Unmarshal(providers, &root.Providers)
		}
	}
	
	// Fetch stations
	if data := fetchEndpoint(endpoints["station_information"], t); data != nil {
		if stations, ok := data["stations"]; ok {
			json.Unmarshal(stations, &root.StationInformation)
		}
	}
	
	// Fetch vehicles
	if data := fetchEndpoint(endpoints["free_bike_status"], t); data != nil {
		if bikes, ok := data["bikes"]; ok {
			json.Unmarshal(bikes, &root.FreeBikeStatus)
		}
	}
	
	// Fetch regions
	if data := fetchEndpoint(endpoints["system_regions"], t); data != nil {
		if regions, ok := data["regions"]; ok {
			json.Unmarshal(regions, &root.SystemRegions)
		}
	}
	
	return root
}

func fetchEndpoint(url string, t *testing.T) map[string]json.RawMessage {
	resp, err := http.Get(url)
	if err != nil {
		t.Logf("Warning: Failed to fetch %s: %v", url, err)
		return nil
	}
	defer resp.Body.Close()
	
	if resp.StatusCode != 200 {
		t.Logf("Warning: HTTP %d from %s", resp.StatusCode, url)
		return nil
	}
	
	var wrapper struct {
		Data json.RawMessage `json:"data"`
	}
	
	if err := json.NewDecoder(resp.Body).Decode(&wrapper); err != nil {
		return nil
	}
	
	var dataMap map[string]json.RawMessage
	json.Unmarshal(wrapper.Data, &dataMap)
	return dataMap
}

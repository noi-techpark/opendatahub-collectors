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

func TestFullAnalysis(t *testing.T) {
	providersMap := make(map[string]Provider)
	
	fmt.Println("\n--- 1. ANALISI PROVIDER ---")
	if data := fetchAnalysis("https://sharedmobility.ch/providers.json", t); data != nil {
		for _, p := range data.Data.Providers {
			providersMap[p.ProviderID] = p
		}
	}
	fmt.Printf("Provider Mappati: %d\n", len(providersMap))

	// 2. Analisi Stazioni
	fmt.Println("\n--- 2. ANALISI STAZIONI (Parent Check) ---")
	stNoParent := 0
	
	if data := fetchAnalysis("https://sharedmobility.ch/station_information.json", t); data != nil {
		fmt.Printf("Totale Stazioni: %d\n", len(data.Data.Stations))
		
		stationCounts := make(map[string]int)
		// Removed redefined stNoParent
		
		for _, s := range data.Data.Stations {
			if s.RegionID == "" {
				stNoParent++ // Uses the outer scope variable defined at line 27
			}
			
			// Reimplements the logic from main.go to show the user what will happen
			var stationType string
			
			// 1. Initial guess based on most common (simplified simulation)
			// In main.go we use complex logic with virtual parents, here we simplify for the report
			// If we want to be accurate to the fix, we should apply the deduction logic on orphans
			
			if s.RegionID != "" {
				// Assume correct mapping for regional stations (mostly bikes)
				stationType = "BikesharingStation" // Simplified assumption for report
			} else {
				// Orphan: likely Generic or Bike default
				stationType = "SharingMobilityStation" 
				
				// APPLY THE FIX LOGIC: Deduce from ID
				deduced := deduceProviderFromStationID(s.StationID, providersMap)
				if deduced != nil {
					stationType = GetStationTypeForPhysicalStation(deduced.GetStationType())
				}
			}
			stationCounts[stationType]++
		}
		
		fmt.Printf(" -> Di cui Orfane (RegionID vuoto): %d\n", stNoParent)
		
		fmt.Println("\nDETTAGLIO STAZIONI (Stimato con logica Fix):")
		for t, c := range stationCounts {
			fmt.Printf(" -> %-25s: %d\n", t, c)
		}
	}

	// 3. Analisi Veicoli
	fmt.Println("\n--- 3. ANALISI VEICOLI ---")
	vhCounts := make(map[string]int)
	// Map to track distinct vehicle_type_ids and their counts
	vehicleTypeIDCounts := make(map[string]int)
	
	if data := fetchAnalysis("https://sharedmobility.ch/free_bike_status.json", t); data != nil {
		for _, b := range data.Data.Bikes {
			serviceType := GetVehicleTypeFromVehicleTypeID(b.VehicleTypeID, providersMap)
			vehicleType := GetStationTypeForVehicle(serviceType)
			
			vhCounts[vehicleType]++
			vehicleTypeIDCounts[b.VehicleTypeID]++
		}
		fmt.Printf("Totale Veicoli: %d\n", len(data.Data.Bikes))
	}

	fmt.Println("\nDETTAGLIO TIPI (Conteggio Finale):")
	for tipo, count := range vhCounts {
		fmt.Printf(" -> %-25s: %d\n", tipo, count)
	}
}

func fetchAnalysis(url string, t *testing.T) *AnalysisData {
	resp, err := http.Get(url)
	if err != nil { return nil }
	defer resp.Body.Close()
	var root AnalysisData
	json.NewDecoder(resp.Body).Decode(&root)
	return &root
}
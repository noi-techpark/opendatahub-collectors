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
		for _, s := range data.Data.Stations {
			if s.RegionID == "" {
				stNoParent++
			}
		}
		fmt.Printf("Totale Stazioni: %d\n", len(data.Data.Stations))
		fmt.Printf(" -> Di cui Orfane (RegionID vuoto): %d\n", stNoParent)
	}

	// 3. Analisi Veicoli
	fmt.Println("\n--- 3. ANALISI VEICOLI ---")
	vhCounts := make(map[string]int)
	unknownSample := []string{}
	
	if data := fetchAnalysis("https://sharedmobility.ch/free_bike_status.json", t); data != nil {
		for _, b := range data.Data.Bikes {
			serviceType := GetVehicleTypeFromVehicleTypeID(b.VehicleTypeID, providersMap)
			vehicleType := GetStationTypeForVehicle(serviceType)
			
			vhCounts[vehicleType]++
			
			if serviceType == "SharingMobilityService" {
				if len(unknownSample) < 5 {
					unknownSample = append(unknownSample, b.VehicleTypeID)
				}
			}
		}
		fmt.Printf("Totale Veicoli: %d\n", len(data.Data.Bikes))
	}

	fmt.Println("\nDETTAGLIO TIPI:")
	for tipo, count := range vhCounts {
		fmt.Printf(" -> %-15s: %d\n", tipo, count)
	}

	if len(unknownSample) > 0 {
		fmt.Println("\n!!! ALLARME: Veicoli con tipo generico !!!")
		fmt.Println("Ecco 5 esempi di 'vehicle_type_id' che non corrispondono a provider conosciuti:")
		for _, id := range unknownSample {
			fmt.Printf(" - '%s'\n", id)
		}
		fmt.Println("Questi veicoli sono stati categorizzati come 'SharingMobilityService' generico.")
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
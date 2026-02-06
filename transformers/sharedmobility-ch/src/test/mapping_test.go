// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import (
	"fmt"
	"testing"
)

func TestMappingLogic(t *testing.T) {
	scenarios := []struct {
		InputServiceType string
		ExpectedStation  string
		ExpectedVehicle  string
	}{
		{"BikeSharingService", "BikesharingStation", "Bicycle"},
		{"ScooterSharingService", "ScooterSharingStation", "ScooterSharingVehicle"},
		{"CarSharingService", "CarsharingStation", "CarsharingCar"},
		{"UnknownService", "SharingMobilityStation", "SharingMobilityVehicle"},
	}

	fmt.Println("\n--- VERIFICA MAPPING ---")
	for _, s := range scenarios {
		stationRes := GetStationTypeForPhysicalStation(s.InputServiceType)
		vehicleRes := GetStationTypeForVehicle(s.InputServiceType)

		fmt.Printf("INPUT: %s\n", s.InputServiceType)
		fmt.Printf(" -> Station Type: %s (Atteso: %s)\n", stationRes, s.ExpectedStation)
		fmt.Printf(" -> Vehicle Type: %s (Atteso: %s)\n", vehicleRes, s.ExpectedVehicle)
		
		if stationRes == s.ExpectedStation && vehicleRes == s.ExpectedVehicle {
			fmt.Println(" -> ESITO: OK")
		} else {
			fmt.Println(" -> ESITO: ERRORE DI MAPPING")
			t.Fail()
		}
		fmt.Println("-------------------------")
	}
}
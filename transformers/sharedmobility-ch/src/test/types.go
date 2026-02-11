// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package test contains integration and analysis tests for the sharedmobility-ch transformer
package test

import (
	"strings"
)

// Shared type definitions for tests (matching transformer DTOs)

type Provider struct {
	ProviderID  string `json:"provider_id"`
	Name        string `json:"name"`
	VehicleType string `json:"vehicle_type"`
}

func (p Provider) GetStationType() string {
	return MapVehicleType(p.VehicleType)
}

type StationInformation struct {
	StationID string  `json:"station_id"`
	Name      string  `json:"name"`
	RegionID  string  `json:"region_id"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
}

type FreeBikeStatus struct {
	BikeID        string  `json:"bike_id"`
	VehicleTypeID string  `json:"vehicle_type_id"`
	Lat           float64 `json:"lat"`
	Lon           float64 `json:"lon"`
}

type StationStatusItem struct {
	StationID         string `json:"station_id"`
	NumBikesAvailable int    `json:"num_bikes_available"`
}

type SystemRegion struct {
	RegionID string `json:"region_id"`
	Name     string `json:"name"`
}

// Root holds all fetched data
type Root struct {
	Providers          []Provider
	StationInformation []StationInformation
	FreeBikeStatus     []FreeBikeStatus
	StationStatus      []StationStatusItem
	SystemRegions      []SystemRegion
}

// AnalysisData is a wrapper for JSON responses
type AnalysisData struct {
	Data struct {
		Providers []Provider          `json:"providers"`
		Stations  []StationInformation `json:"stations"`
		Bikes     []FreeBikeStatus     `json:"bikes"`
		Regions   []SystemRegion       `json:"regions"`
	} `json:"data"`
}

// GetVehicleTypeFromVehicleTypeID resolves vehicle type from composite ID
func GetVehicleTypeFromVehicleTypeID(vehicleTypeID string, providersMap map[string]Provider) string {
	// 1. Try exact match
	if p, ok := providersMap[vehicleTypeID]; ok {
		return MapVehicleType(p.VehicleType)
	}
	
	// 2. Try splitting composite IDs (e.g. "provider:1" -> "provider")
	parts := strings.Split(vehicleTypeID, ":")
	if len(parts) > 1 {
		providerID := parts[0]
		if p, ok := providersMap[providerID]; ok {
			return MapVehicleType(p.VehicleType)
		}
	}
	
	return "SharingMobilityService"
}

// MapVehicleType maps raw vehicle type to service type
func MapVehicleType(vehicleType string) string {
	switch vehicleType {
	case "E-Moped", "E-scooter":
		return "ScooterSharingService"
	case "E-Bike", "Bike", "E-CargoBike":
		return "BikeSharingService"
	case "E-Car", "Car":
		return "CarSharingService"
	default:
		return "SharingMobilityService"
	}
}

// GetStationTypeForVehicle maps service type to vehicle station type
func GetStationTypeForVehicle(serviceType string) string {
	switch serviceType {
	case "BikeSharingService":
		return "Bicycle"
	case "ScooterSharingService":
		return "ScooterSharingVehicle"
	case "CarSharingService":
		return "CarsharingCar"
	default:
		return "SharingMobilityVehicle"
	}
}

// Helper to simulate the fix logic in tests
func deduceProviderFromStationID(stationID string, providersMap map[string]Provider) *Provider {
	stationIDLower := strings.ToLower(stationID)
	for _, p := range providersMap {
		pID := strings.ToLower(p.ProviderID)
		if len(pID) < 3 { continue }
		if strings.Contains(stationIDLower, pID) {
			// Need to return a pointer, but p is a copy in range over map value?
			// Range over map value gives a copy.
			// We can return a pointer to a new copy or valid object.
			// Since this is just a test helper, returning &p where p is the range var is unsafe in older Go versions (loop var reuse),
			// but safe in Go 1.22+. Given we don't know the version, let's be safe.
			// Actually providersMap values are `Provider` structs.
			found := p
			return &found
		}
	}
	return nil
}

// GetStationTypeForPhysicalStation maps service type to station type
func GetStationTypeForPhysicalStation(serviceType string) string {
	switch serviceType {
	case "BikeSharingService":
		return "BikesharingStation"
	case "ScooterSharingService":
		return "ScooterSharingStation"
	case "CarSharingService":
		return "CarsharingStation"
	default:
		return "SharingMobilityStation"
	}
}

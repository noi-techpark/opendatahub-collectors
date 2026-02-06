// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "strings"

// Root holds the top-level fields as mapped by the multi-rest-poller.
type Root struct {
	Providers          []Provider           `json:"providers"`
	SystemInformation  SystemInformation    `json:"system_information"`
	StationInformation []StationInformation `json:"station_information"`
	FreeBikeStatus     []FreeBikeStatus     `json:"free_bike_status"`
	StationStatus      []StationStatus      `json:"station_status"`
	SystemHours        []RentalHour         `json:"system_hours"`
	SystemRegions      []SystemRegion       `json:"system_regions"`
	Plans              []PricingPlan        `json:"plans"`
	GeofencingZones    GeofencingZone       `json:"geofencing_zones"`
}

// GBFS Provider information (from providers.json)
type Provider struct {
	ProviderID  string `json:"provider_id"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	VehicleType string `json:"vehicle_type"`
	AppStore    string `json:"app_store"`
	PlayStore   string `json:"play_store"`
}

// GBFS System Information (from system_information.json)
type SystemInformation struct {
	SystemID string `json:"system_id"`
	Language string `json:"language"`
	Name     string `json:"name"`
	Operator string `json:"operator"`
	URL      string `json:"url"`
}

// GBFS Station Information (from station_information.json)
type StationInformation struct {
	StationID string  `json:"station_id"`
	Name      string  `json:"name"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	RegionID  string  `json:"region_id"`
	Address   string  `json:"address"`
}

// GBFS Free Bike Status (from free_bike_status.json)
type FreeBikeStatus struct {
	BikeID             string  `json:"bike_id"`
	Lat                float64 `json:"lat"`
	Lon                float64 `json:"lon"`
	IsReserved         bool    `json:"is_reserved"`
	IsDisabled         bool    `json:"is_disabled"`
	VehicleTypeID      string  `json:"vehicle_type_id"`
	PricingPlanID      string  `json:"pricing_plan_id"`
	CurrentRangeMeters float64 `json:"current_range_meters"`
}

// GBFS Station Status (from station_status.json)
type StationStatus struct {
	StationID         string `json:"station_id"`
	NumBikesAvailable int    `json:"num_bikes_available"`
	NumDocksAvailable int    `json:"num_docks_available"`
	IsInstalled       bool   `json:"is_installed"`
	IsRenting         bool   `json:"is_renting"`
	IsReturning       bool   `json:"is_returning"`
	LastReported      int64  `json:"last_reported"`
}

// GBFS Rental Hours (from system_hours.json)
type RentalHour struct {
	UserTypes []string `json:"user_types"`
	Days      []string `json:"days"`
	StartTime string   `json:"start_time"`
	EndTime   string   `json:"end_time"`
}

// GBFS System Region (from system_regions.json)
type SystemRegion struct {
	RegionID string `json:"region_id"`
	Name     string `json:"name"`
}

// GBFS Pricing Plan (from system_pricing_plans.json)
type PricingPlan struct {
	PlanID      string  `json:"plan_id"`
	Name        string  `json:"name"`
	Currency    string  `json:"currency"`
	Price       float64 `json:"price"`
	IsTaxable   bool    `json:"is_taxable"`
	Description string  `json:"description"`
}

// GBFS Geofencing Zone (from external URL)
type GeofencingZone struct {
	Type     string        `json:"type"`
	Features []interface{} `json:"features"` // GeoJSON features
}

func (p Provider) GetStationType() string {
	switch p.VehicleType {
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

// GetStationTypeForPhysicalStation converts a service type to a physical station type
// e.g., "BikeSharingService" -> "BikeSharingStation"
func GetStationTypeForPhysicalStation(serviceType string) string {
	switch serviceType {
	case "ScooterSharingService":
		return "ScooterSharingStation"
	case "BikeSharingService":
		return "BikesharingStation"
	case "CarSharingService":
		return "CarsharingStation"
	default:
		return "SharingMobilityStation"
	}
}

// GetStationTypeForVehicle converts a service type to a vehicle station type
// e.g., "BikeSharingService" -> "BikeSharingVehicle"
func GetStationTypeForVehicle(serviceType string) string {
	switch serviceType {
	case "ScooterSharingService":
		return "ScooterSharingVehicle"
	case "BikeSharingService":
		return "Bicycle"
	case "CarSharingService":
		return "CarsharingCar"
	default:
		return "SharingMobilityVehicle"
	}
}

// GetVehicleTypeFromVehicleTypeID attempts to determine vehicle type from vehicle_type_id
// by looking it up in providers. Returns the most common vehicle type if not found.
func (r Root) GetVehicleTypeFromVehicleTypeID(vehicleTypeID string, providersMap map[string]Provider) string {
	if vehicleTypeID == "" {
		return r.getMostCommonProviderType()
	}

	// 1. Try exact match
	if provider, ok := providersMap[vehicleTypeID]; ok {
		return provider.GetStationType()
	}

	// 2. Try splitting composite IDs (e.g. "provider:1" -> "provider")
	parts := strings.Split(vehicleTypeID, ":")
	if len(parts) > 1 {
		providerID := parts[0]
		if provider, ok := providersMap[providerID]; ok {
			return provider.GetStationType()
		}
	}

	// 3. Fallback: check if any provider ID matches the vehicleTypeID directly (legacy check)
	for _, provider := range r.Providers {
		if provider.ProviderID == vehicleTypeID {
			return provider.GetStationType()
		}
	}

	return r.getMostCommonProviderType()
}

// getMostCommonProviderType returns the most common vehicle type among providers
func (r Root) getMostCommonProviderType() string {
	if len(r.Providers) == 0 {
		return "SharingMobilityService"
	}

	typeCount := make(map[string]int)
	for _, provider := range r.Providers {
		stationType := provider.GetStationType()
		typeCount[stationType]++
	}

	// Find the most common type
	maxCount := 0
	mostCommonType := "SharingMobilityService"
	for stationType, count := range typeCount {
		if count > maxCount {
			maxCount = count
			mostCommonType = stationType
		}
	}

	return mostCommonType
}

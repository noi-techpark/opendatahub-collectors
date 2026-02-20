// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "encoding/json"

// Envelope wraps collector messages with a type indicator
type Envelope struct {
	Type string          `json:"type"` // "static" or "realtime"
	Data json.RawMessage `json:"data"`
}

// --- Static Data Structures (OICP) ---

type StaticResponse struct {
	EVSEData []EVSEDataOperator `json:"EVSEData"`
}

type EVSEDataOperator struct {
	OperatorID     string           `json:"OperatorID"`
	OperatorName   string           `json:"OperatorName"`
	EVSEDataRecord []EVSEDataRecord `json:"EVSEDataRecord"`
}

type EVSEDataRecord struct {
	Accessibility                  string              `json:"Accessibility,omitempty"`
	AccessibilityLocation          string              `json:"AccessibilityLocation,omitempty"`
	AdditionalInfo                 interface{}         `json:"AdditionalInfo,omitempty"`
	Address                        *EVSEAddress        `json:"Address,omitempty"`
	AuthenticationModes            []string            `json:"AuthenticationModes,omitempty"`
	CalibrationLawDataAvailability string              `json:"CalibrationLawDataAvailability,omitempty"`
	ChargingFacilities             []ChargingFacility  `json:"ChargingFacilities,omitempty"`
	ChargingPoolID                 *string             `json:"ChargingPoolID,omitempty"`
	ChargingStationId              string              `json:"ChargingStationId"`
	ChargingStationLocationRef     interface{}         `json:"ChargingStationLocationReference,omitempty"`
	ChargingStationNames           []ChargingStationName `json:"ChargingStationNames,omitempty"`
	ClearinghouseID                *string             `json:"ClearinghouseID,omitempty"`
	DynamicInfoAvailable           string              `json:"DynamicInfoAvailable,omitempty"`
	DynamicPowerLevel              interface{}         `json:"DynamicPowerLevel,omitempty"`
	EnergySource                   interface{}         `json:"EnergySource,omitempty"`
	EnvironmentalImpact            interface{}         `json:"EnvironmentalImpact,omitempty"`
	EvseID                         string              `json:"EvseID"`
	GeoChargingPointEntrance       *GeoCoordinate      `json:"GeoChargingPointEntrance,omitempty"`
	GeoCoordinates                 GeoCoordinate       `json:"GeoCoordinates"`
	HardwareManufacturer           *string             `json:"HardwareManufacturer,omitempty"`
	HotlinePhoneNumber             string              `json:"HotlinePhoneNumber,omitempty"`
	HubOperatorID                  *string             `json:"HubOperatorID,omitempty"`
	IsHubjectCompatible            bool                `json:"IsHubjectCompatible,omitempty"`
	IsOpen24Hours                  bool                `json:"IsOpen24Hours,omitempty"`
	LocationImage                  interface{}         `json:"LocationImage,omitempty"`
	MaxCapacity                    interface{}         `json:"MaxCapacity,omitempty"`
	OpeningTimes                   interface{}         `json:"OpeningTimes,omitempty"`
	PaymentOptions                 []string            `json:"PaymentOptions,omitempty"`
	Plugs                          []string            `json:"Plugs,omitempty"`
	RenewableEnergy                bool                `json:"RenewableEnergy,omitempty"`
	SuboperatorName                *string             `json:"SuboperatorName,omitempty"`
	ValueAddedServices             []string            `json:"ValueAddedServices,omitempty"`
	DeltaType                      string              `json:"deltaType,omitempty"`
	LastUpdate                     string              `json:"lastUpdate,omitempty"`
}

type EVSEAddress struct {
	City            string  `json:"City,omitempty"`
	Country         string  `json:"Country,omitempty"`
	Floor           *string `json:"Floor,omitempty"`
	HouseNum        string  `json:"HouseNum,omitempty"`
	ParkingFacility *string `json:"ParkingFacility,omitempty"`
	ParkingSpot     *string `json:"ParkingSpot,omitempty"`
	PostalCode      string  `json:"PostalCode,omitempty"`
	Region          *string `json:"Region,omitempty"`
	Street          string  `json:"Street,omitempty"`
	TimeZone        string  `json:"TimeZone,omitempty"`
}

type ChargingFacility struct {
	Power         interface{} `json:"power,omitempty"`
	PowerType     string      `json:"powertype,omitempty"`
	Amperage      string      `json:"Amperage,omitempty"`
	Voltage       string      `json:"Voltage,omitempty"`
	ChargingModes []string    `json:"ChargingModes,omitempty"`
}

type ChargingStationName struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type GeoCoordinate struct {
	Google string `json:"Google"`
}

// --- Real-Time Status Data Structures (OICP) ---

type StatusResponse struct {
	EVSEStatuses []EVSEStatusOperator `json:"EVSEStatuses"`
}

type EVSEStatusOperator struct {
	OperatorID       string             `json:"OperatorID"`
	OperatorName     string             `json:"OperatorName"`
	EVSEStatusRecord []EVSEStatusRecord `json:"EVSEStatusRecord"`
}

type EVSEStatusRecord struct {
	EvseID     string `json:"EvseID"`
	EVSEStatus string `json:"EVSEStatus"`
}

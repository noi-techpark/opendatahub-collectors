// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
)

// ChargingStationNameList handles the API inconsistency where this field
// is sometimes a single object and sometimes an array.
type ChargingStationNameList []ChargingStationName

func (l *ChargingStationNameList) UnmarshalJSON(data []byte) error {
	if len(data) == 0 || string(data) == "null" {
		*l = nil
		return nil
	}
	switch data[0] {
	case '[':
		var list []ChargingStationName
		if err := json.Unmarshal(data, &list); err != nil {
			return err
		}
		*l = list
	case '{':
		var single ChargingStationName
		if err := json.Unmarshal(data, &single); err != nil {
			return err
		}
		*l = []ChargingStationName{single}
	default:
		return fmt.Errorf("unexpected ChargingStationNames value: %s", data)
	}
	return nil
}

// Root holds the top-level payload structure from multi-rest-poller
// Uses snake_case keys as delivered by the collector
type Root struct {
	EVSEData     []EVSEOperator       `json:"evse_data"`
	EVSEStatuses []EVSEStatusOperator `json:"evse_statuses"`
}

// --- Static Data Structures (OICP Format with Operator nesting) ---

type EVSEOperator struct {
	OperatorID     string         `json:"OperatorID"`
	OperatorName   string         `json:"OperatorName"`
	EVSEDataRecord []EVSEDataItem `json:"EVSEDataRecord"`
}

type EVSEDataItem struct {
	Accessibility                  *string               `json:"Accessibility"`
	AccessibilityLocation          *string               `json:"AccessibilityLocation"`
	AdditionalInfo                 interface{}           `json:"AdditionalInfo"`
	Address                        *EVSEAddress          `json:"Address"`
	AuthenticationModes            []string              `json:"AuthenticationModes"`
	CalibrationLawDataAvailability *string               `json:"CalibrationLawDataAvailability"`
	ChargingFacilities             []ChargingFacility    `json:"ChargingFacilities"`
	ChargingPoolID                 *string               `json:"ChargingPoolID"`
	ChargingStationId              string                `json:"ChargingStationId"`
	ChargingStationLocationRef     interface{}           `json:"ChargingStationLocationReference"`
	ChargingStationNames           ChargingStationNameList `json:"ChargingStationNames"`
	ClearinghouseID                *string               `json:"ClearinghouseID"`
	DynamicInfoAvailable           *string               `json:"DynamicInfoAvailable"`
	DynamicPowerLevel              interface{}           `json:"DynamicPowerLevel"`
	EnergySource                   interface{}           `json:"EnergySource"`
	EnvironmentalImpact            interface{}           `json:"EnvironmentalImpact"`
	EvseID                         string                `json:"EvseID"`
	GeoChargingPointEntrance       *GeoCoordinate        `json:"GeoChargingPointEntrance"`
	GeoCoordinates                 *GeoCoordinate        `json:"GeoCoordinates"`
	HardwareManufacturer           *string               `json:"HardwareManufacturer"`
	HotlinePhoneNumber             *string               `json:"HotlinePhoneNumber"`
	HubOperatorID                  *string               `json:"HubOperatorID"`
	IsHubjectCompatible            *bool                 `json:"IsHubjectCompatible"`
	IsOpen24Hours                  *bool                 `json:"IsOpen24Hours"`
	LocationImage                  interface{}           `json:"LocationImage"`
	MaxCapacity                    interface{}           `json:"MaxCapacity"`
	OpeningTimes                   interface{}           `json:"OpeningTimes"`
	PaymentOptions                 []string              `json:"PaymentOptions"`
	Plugs                          []string              `json:"Plugs"`
	RenewableEnergy                *bool                 `json:"RenewableEnergy"`
	SuboperatorName                *string               `json:"SuboperatorName"`
	ValueAddedServices             []string              `json:"ValueAddedServices"`
	DeltaType                      *string               `json:"deltaType"`
	LastUpdate                     *string               `json:"lastUpdate"`
}

type EVSEAddress struct {
	City            *string `json:"City"`
	Country         *string `json:"Country"`
	Floor           *string `json:"Floor"`
	HouseNum        *string `json:"HouseNum"`
	ParkingFacility *bool   `json:"ParkingFacility"`
	ParkingSpot     *string `json:"ParkingSpot"`
	PostalCode      *string `json:"PostalCode"`
	Region          *string `json:"Region"`
	Street          *string `json:"Street"`
	TimeZone        *string `json:"TimeZone"`
}

type ChargingFacility struct {
	Power         interface{} `json:"power"`
	PowerType     *string     `json:"powertype"`
	Amperage      interface{} `json:"Amperage"`
	Voltage       interface{} `json:"Voltage"`
	ChargingModes []string    `json:"ChargingModes"`
}

type ChargingStationName struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type GeoCoordinate struct {
	Google string `json:"Google"` // Format: "lat lon" (space-separated)
}

// --- Real-Time Status Data Structures (OICP Format with Operator nesting) ---

type EVSEStatusOperator struct {
	OperatorID       string           `json:"OperatorID"`
	OperatorName     string           `json:"OperatorName"`
	EVSEStatusRecord []EVSEStatusItem `json:"EVSEStatusRecord"`
}

type EVSEStatusItem struct {
	EvseID     string `json:"EvseID"`
	EvseStatus string `json:"EvseStatus"`
}

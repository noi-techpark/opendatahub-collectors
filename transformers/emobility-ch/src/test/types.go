// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

// EmobilityData holds test data structure matching transformer DTOs
type EmobilityData struct {
	EVSEData     []EVSEOperator         `json:"EVSEData"`
	EVSEStatuses []EVSEStatusOperator   `json:"EVSEStatuses"`
}

type EVSEOperator struct {
	OperatorID     string         `json:"OperatorID"`
	OperatorName   string         `json:"OperatorName"`
	EVSEDataRecord []EVSEDataItem `json:"EVSEDataRecord"`
}

type EVSEDataItem struct {
	EvseID               string              `json:"EvseID"`
	ChargingStationId    string              `json:"ChargingStationId"`
	GeoCoordinates       *GeoCoordinate      `json:"GeoCoordinates"`
	ChargingStationNames []ChargingStationName `json:"ChargingStationNames"`
	Address              *EVSEAddress        `json:"Address"`
	Accessibility        *string             `json:"Accessibility"`
	IsOpen24Hours        *bool               `json:"IsOpen24Hours"`
	Plugs                []string            `json:"Plugs"`
	ChargingFacilities   []ChargingFacility  `json:"ChargingFacilities"`
	AuthenticationModes  []string            `json:"AuthenticationModes"`
	PaymentOptions       []string            `json:"PaymentOptions"`
	RenewableEnergy      *bool               `json:"RenewableEnergy"`
	HotlinePhoneNumber   *string             `json:"HotlinePhoneNumber"`
}

type EVSEAddress struct {
	Street     *string `json:"Street"`
	City       *string `json:"City"`
	PostalCode *string `json:"PostalCode"`
	Country    *string `json:"Country"`
}

type ChargingFacility struct {
	Power         interface{} `json:"Power"`
	PowerType     *string     `json:"PowerType"`
	Amperage      *int        `json:"Amperage"`
	Voltage       *int        `json:"Voltage"`
	ChargingModes []string    `json:"ChargingModes"`
}

type ChargingStationName struct {
	Lang  string `json:"lang"`
	Value string `json:"value"`
}

type GeoCoordinate struct {
	Google string `json:"Google"` // Format: "lat lon"
}

type EVSEStatusOperator struct {
	OperatorID       string           `json:"OperatorID"`
	OperatorName     string           `json:"OperatorName"`
	EVSEStatusRecord []EVSEStatusItem `json:"EVSEStatusRecord"`
}

type EVSEStatusItem struct {
	EvseID     string `json:"EvseID"`
	EvseStatus string `json:"EvseStatus"`
}

// GetMetrics returns test data summary
func (d *EmobilityData) GetMetrics() TestMetrics {
	totalEVSE := 0
	totalStatus := 0
	
	for _, operator := range d.EVSEData {
		totalEVSE += len(operator.EVSEDataRecord)
	}
	
	for _, statusOp := range d.EVSEStatuses {
		totalStatus += len(statusOp.EVSEStatusRecord)
	}
	
	return TestMetrics{
		EVSEDataCount:   totalEVSE,
		EVSEStatusCount: totalStatus,
	}
}

type TestMetrics struct {
	EVSEDataCount   int
	EVSEStatusCount int
}

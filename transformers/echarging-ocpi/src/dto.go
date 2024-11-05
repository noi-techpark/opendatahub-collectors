// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"time"
)

type OCPILocationsOperator struct {
	Name    string `json:"name"`
	Website string `json:"website"`
	Logo    string `json:"logo"`
}

type OCPIEvse struct {
	UID          string    `json:"uid"`
	EvseID       string    `json:"evse_id"`
	Status       string    `json:"status"`
	Capabilities *[]string `json:"capabilities,omitempty"`
	Connectors   *[]struct {
		ID               string    `json:"id,omitempty"`
		Standard         string    `json:"standard,omitempty"`
		Format           string    `json:"format,omitempty"`
		PowerType        string    `json:"power_type,omitempty"`
		LastUpdated      time.Time `json:"last_updated,omitempty"`
		MaxVoltage       int       `json:"max_voltage,omitempty"`
		MaxAmperage      int       `json:"max_amperage,omitempty"`
		MaxElectricPower int       `json:"max_electric_power,omitempty"`
		TariffIds        *[]string `json:"tariff_ids,omitempty"`
	} `json:"connectors,omitempty"`
	LastUpdated time.Time `json:"last_updated"`
}

type OCPILocations struct {
	CountryCode string
	PartyID     string
	ID          string
	Publish     *bool
	Name        string
	Address     string
	City        string
	PostalCode  string
	Country     string
	Coordinates struct {
		Latitude  string
		Longitude string
	}
	Evses        []OCPIEvse
	ParkingType  string
	Operator     OCPILocationsOperator
	Suboperator  OCPILocationsOperator
	Owner        OCPILocationsOperator
	Facilities   []string
	TimeZone     string
	OpeningTimes *struct {
		Twentyfourseven     bool
		RegularHours        []interface{}
		ExceptionalOpenings []interface{}
		ExceptionalClosings []interface{}
	}
	LastUpdated      time.Time
	PublishAllowedTo []interface{}
	RelatedLocations []interface{}
	Images           []interface{}
	Directions       *[]interface{}
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"time"
)

type OCPILocationsOperator struct {
	Name    string
	Website string
	Logo    string
}

type OCPIEvse struct {
	UID          string `bson:"uid"`
	EvseID       string `bson:"evse_id"`
	Status       string
	Capabilities []string
	Connectors   []struct {
		ID               string
		Standard         string
		Format           string
		PowerType        string    `bson:"power_type"`
		LastUpdated      time.Time `bson:"last_updated"`
		MaxVoltage       int       `bson:"max_voltage"`
		MaxAmperage      int       `bson:"max_amperage"`
		MaxElectricPower int       `bson:"max_electric_power"`
		TariffIds        *[]string `bson:"tariff_ids"`
	}
	LastUpdated time.Time `bson:"last_updated"`
}

type OCPILocations struct {
	CountryCode string `bson:"country_code"`
	PartyID     string `bson:"party_id"`
	ID          string `bson:"id"`
	Publish     *bool
	Name        string
	Address     string
	City        string
	PostalCode  string `bson:"postal_code"`
	Country     string
	Coordinates struct {
		Latitude  string
		Longitude string
	}
	Evses            []OCPIEvse
	ParkingType      string `bson:"parking_type"`
	Operator         OCPILocationsOperator
	Suboperator      OCPILocationsOperator
	Owner            OCPILocationsOperator
	Facilities       []string
	TimeZone         string        `bson:"time_zone"`
	OpeningTimes     *OpeningTimes `bson:"opening_times"`
	LastUpdated      time.Time     `bson:"last_updated"`
	PublishAllowedTo []interface{} `bson:"publish_allowed_to"`
	RelatedLocations []interface{} `bson:"related_locations"`
	Images           []interface{}
	Directions       []DisplayText
}

type DisplayText struct {
	Language string `json:"language"`
	Text     string `json:"text"`
}

type OpeningTimes struct {
	Twentyfourseven *bool `json:"twentyfourseven,omitempty"`
	RegularHours    []struct {
		Weekday int `json:"weekday"`
		OpeningHoursPeriod
	} `bson:"regular_hours" json:"regular_hours,omitempty"`
	ExceptionalOpenings []OpeningHoursPeriod `bson:"exceptional_openings" json:"exceptional_openings,omitempty"`
	ExceptionalClosings []OpeningHoursPeriod `bson:"exceptional_closings" json:"exceptional_closings,omitempty"`
}

type OpeningHoursPeriod struct {
	PeriodBegin string `bson:"period_begin" json:"period_begin"`
	PeriodEnd   string `bson:"period_end" json:"period_end"`
}

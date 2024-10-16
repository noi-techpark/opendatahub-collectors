// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "time"

type OCPILocationsOperator struct {
	Name    string `json:"name"`
	Website string `json:"website"`
	Logo    string `json:"logo"`
}

type OCPILocations struct {
	Data []struct {
		CountryCode string `json:"country_code"`
		PartyID     string `json:"party_id"`
		ID          string `json:"id"`
		Publish     *bool  `json:"publish"`
		Name        string `json:"name"`
		Address     string `json:"address"`
		City        string `json:"city"`
		PostalCode  string `json:"postal_code"`
		Country     string `json:"country"`
		Coordinates struct {
			Latitude  float64 `json:"latitude,string"`
			Longitude float64 `json:"longitude,string"`
		} `json:"coordinates"`
		Evses []struct {
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
		} `json:"evses"`
		ParkingType  string                `json:"parking_type"`
		Operator     OCPILocationsOperator `json:"operator"`
		Suboperator  OCPILocationsOperator `json:"suboperator"`
		Owner        OCPILocationsOperator `json:"owner"`
		Facilities   []string              `json:"facilities"`
		TimeZone     string                `json:"time_zone"`
		OpeningTimes *struct {
			Twentyfourseven     bool          `json:"twentyfourseven,omitempty"`
			RegularHours        []interface{} `json:"regular_hours,omitempty"`
			ExceptionalOpenings []interface{} `json:"exceptional_openings,omitempty"`
			ExceptionalClosings []interface{} `json:"exceptional_closings,omitempty"`
		} `json:"opening_times,omitempty"`
		LastUpdated      time.Time      `json:"last_updated"`
		PublishAllowedTo []interface{}  `json:"publish_allowed_to"`
		RelatedLocations []interface{}  `json:"related_locations"`
		Images           []interface{}  `json:"images"`
		Directions       *[]interface{} `json:"directions,omitempty"`
	} `json:"data"`
	StatusCode    int       `json:"status_code"`
	StatusMessage string    `json:"status_message"`
	Timestamp     time.Time `json:"timestamp"`
	PageInfo      struct {
		PageIndex int `json:"pageIndex"`
		PageSize  int `json:"pageSize"`
		Total     int `json:"total"`
	} `json:"pageInfo"`
	HTTPStatusCode int `json:"httpStatusCode"`
	UuAppErrorMap  struct {
	} `json:"uuAppErrorMap"`
}

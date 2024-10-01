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
		Publish     bool   `json:"publish"`
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
			UID          string   `json:"uid"`
			EvseID       string   `json:"evse_id"`
			Status       string   `json:"status"`
			Capabilities []string `json:"capabilities"`
			Connectors   []struct {
				ID               string    `json:"id"`
				Standard         string    `json:"standard"`
				Format           string    `json:"format"`
				PowerType        string    `json:"power_type"`
				LastUpdated      time.Time `json:"last_updated"`
				MaxVoltage       int       `json:"max_voltage"`
				MaxAmperage      int       `json:"max_amperage"`
				MaxElectricPower int       `json:"max_electric_power"`
				TariffIds        []string  `json:"tariff_ids"`
			} `json:"connectors"`
			LastUpdated time.Time `json:"last_updated"`
		} `json:"evses"`
		ParkingType  string                `json:"parking_type"`
		Operator     OCPILocationsOperator `json:"operator"`
		Suboperator  OCPILocationsOperator `json:"suboperator"`
		Owner        OCPILocationsOperator `json:"owner"`
		Facilities   []string              `json:"facilities"`
		TimeZone     string                `json:"time_zone"`
		OpeningTimes struct {
			Twentyfourseven     bool          `json:"twentyfourseven"`
			RegularHours        []interface{} `json:"regular_hours"`
			ExceptionalOpenings []interface{} `json:"exceptional_openings"`
			ExceptionalClosings []interface{} `json:"exceptional_closings"`
		} `json:"opening_times"`
		LastUpdated      time.Time     `json:"last_updated"`
		PublishAllowedTo []interface{} `json:"publish_allowed_to"`
		RelatedLocations []interface{} `json:"related_locations"`
		Images           []interface{} `json:"images"`
		Directions       []interface{} `json:"directions"`
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

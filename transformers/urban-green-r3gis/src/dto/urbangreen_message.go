// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// UrbanGreenMessage represents a single JSON message from the raw data bridge
// representing a POST, PUT, or DELETE operation on an urban green entity
type UrbanGreenMessage struct {
	Method                string            `json:"Method"`
	Active                bool              `json:"Active"`
	Geo                   map[string]GeoInfo `json:"Geo"`
	GreenCode             string            `json:"GreenCode"`
	GreenCodeType         string            `json:"GreenCodeType"`
	GreenCodeSubtype      string            `json:"GreenCodeSubtype"`
	GreenCodeVersion      string            `json:"GreenCodeVersion"`
	Id                    string            `json:"Id"`
	Meta                  *MetaInfo         `json:"_Meta"`
	Shortname             string            `json:"Shortname"`
	FirstImport           string            `json:"FirstImport"`
	LastChange            string            `json:"LastChange"`
	Source                string            `json:"Source"`
	PutOnSite             string            `json:"PutOnSite,omitempty"`
	RemovedFromSite       string            `json:"RemovedFromSite,omitempty"`
	AdditionalInformation map[string]string `json:"AdditionalInformation,omitempty"`
}

type GeoInfo struct {
	Gpstype   string  `json:"Gpstype"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
	Geometry  string  `json:"Geometry"`
	Default   bool    `json:"Default"`
}

type MetaInfo struct {
	Id         string `json:"Id"`
	LastUpdate string `json:"LastUpdate"`
	Source     string `json:"Source"`
}

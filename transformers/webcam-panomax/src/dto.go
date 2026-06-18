// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

// --- Panomax Raw JSON Schema ---

type PanomaxCamera struct {
	Id              int            `json:"id"`
	Name            string         `json:"name"`
	Logo            string         `json:"logo"`
	CamId           int            `json:"camId"`
	ViewAngleDegree float64        `json:"viewAngleDegree"`
	Latitude        string         `json:"latitude"`
	Longitude       string         `json:"longitude"`
	ZeroDirection   string         `json:"zeroDirection"`
	Elevation       string         `json:"elevation"`
	Country         string         `json:"country"`
	CountryName     string         `json:"countryName"`
	State           string         `json:"state"`
	City            string         `json:"city"`
	Area            *string        `json:"area"`
	WebcamUrl       string         `json:"webcamUrl"`
	CustomerId      int            `json:"customerId"`
	CustomerUrl     *string        `json:"customerUrl"`
	CustomerName    string         `json:"customerName"`
	Images          []PanomaxImage `json:"images"`
}

type PanomaxImage struct {
	Url    string `json:"url"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"bytes"
	"encoding/json"
)

// --- Panomax Raw JSON Schema ---

// PanomaxRawData is the collector payload: the webcams (instances/lists/public)
// and their videos (cams/videos/public, the "cams" list).
type PanomaxRawData struct {
	Webcams []PanomaxCamera   `json:"webcams"`
	Videos  []PanomaxVideoCam `json:"videos"`
}

// UnmarshalJSON also accepts the former payload, a plain webcam array, so raw
// messages collected before the videos were added can still be processed.
func (d *PanomaxRawData) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return json.Unmarshal(trimmed, &d.Webcams)
	}
	type alias PanomaxRawData
	return json.Unmarshal(trimmed, (*alias)(d))
}

// PanomaxVideoCam lists the videos of one cam; Id is the webcam's camId.
type PanomaxVideoCam struct {
	Id     int            `json:"id"`
	Videos []PanomaxVideo `json:"videos"`
}

type PanomaxVideo struct {
	Url      string `json:"url"`
	Width    string `json:"width"`
	Height   string `json:"height"`
	FileName string `json:"fileName"`
}

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
	TourCam         bool           `json:"tourCam"`
	Images          []PanomaxImage `json:"images"`
}

type PanomaxImage struct {
	Url    string `json:"url"`
	Width  string `json:"width"`
	Height string `json:"height"`
}

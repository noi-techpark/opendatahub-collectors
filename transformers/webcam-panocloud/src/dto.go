// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"bytes"
	"encoding/json"
)

// --- Panocloud Raw JSON Schema ---

// oneOrMany accepts a single object or an array. The Panocloud feed is converted
// from XML, so a list with only one element arrives as a plain object.
type oneOrMany[T any] []T

func (o *oneOrMany[T]) UnmarshalJSON(b []byte) error {
	trimmed := bytes.TrimSpace(b)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		*o = nil
		return nil
	}
	if trimmed[0] == '[' {
		var list []T
		if err := json.Unmarshal(trimmed, &list); err != nil {
			return err
		}
		*o = list
		return nil
	}
	var single T
	if err := json.Unmarshal(trimmed, &single); err != nil {
		return err
	}
	*o = oneOrMany[T]{single}
	return nil
}

type PanocloudResponse struct {
	LiveCam []PanocloudCamera `json:"LiveCam"`
}

type PanocloudCamera struct {
	Attributes PanocloudAttributes `json:"@attributes"`
	Images     PanocloudImages     `json:"Images"`
	Videos     PanocloudVideos     `json:"Videos"`
	Logos      PanocloudLogos      `json:"Logos"`
}

type PanocloudAttributes struct {
	CameraStatus    string `json:"cameraStatus"`
	Full360         string `json:"full360"`
	HasVR           string `json:"hasVR"`
	ViewerType      string `json:"viewerType"`
	LocationId      string `json:"locationId"`
	LastModified    string `json:"lastModified"`
	Name            string `json:"name"`
	Url             string `json:"url"`
	GeoLat          string `json:"geoLat"`
	GeoLong         string `json:"geoLong"`
	GeoAlt          string `json:"geoAlt"`
	DefaultLang     string `json:"defaultLang"`
	Description     string `json:"description"`
	LongDescription string `json:"longdescription"`
	GeoRegion       string `json:"geoRegion"`
	GeoPlacename    string `json:"geoPlacename"`
	PageTitle       string `json:"pageTitle"`
	AddressIso      string `json:"addressIso"`
	AddressZip      string `json:"addressZip"`
	AddressStreet   string `json:"addressStreet"`
}

type PanocloudImages struct {
	Image oneOrMany[PanocloudImage] `json:"image"`
}

type PanocloudImage struct {
	Attributes PanocloudImageAttr `json:"@attributes"`
}

type PanocloudImageAttr struct {
	FileType  string `json:"fileType"`
	FileUrl   string `json:"fileUrl"`
	MimeType  string `json:"mimeType"`
	Panorama  string `json:"panorama"`
	FileName  string `json:"fileName"`
	ImgWidth  string `json:"imgWidth"`
	ImgHeight string `json:"imgHeight"`
}

type PanocloudVideos struct {
	Video oneOrMany[PanocloudVideo] `json:"video"`
}

type PanocloudVideo struct {
	Attributes PanocloudVideoAttr `json:"@attributes"`
}

type PanocloudVideoAttr struct {
	VideoClipUrl string `json:"videoClipUrl"`
	Resolution   string `json:"resolution"`
	Definition   string `json:"definition"`
	VideoBitRate string `json:"videoBitRate"`
	Duration     string `json:"duration"`
	MimeType     string `json:"mimeType"`
}

type PanocloudLogos struct {
	Logo oneOrMany[PanocloudLogo] `json:"Logo"`
}

type PanocloudLogo struct {
	Attributes struct {
		LogoUrl string `json:"logoUrl"`
	} `json:"@attributes"`
}

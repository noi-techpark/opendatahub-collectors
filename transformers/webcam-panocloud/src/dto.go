// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

// --- Panocloud Raw JSON Schema ---

type PanocloudResponse struct {
	LiveCam []PanocloudCamera `json:"LiveCam"`
}

type PanocloudCamera struct {
	Attributes PanocloudAttributes `json:"@attributes"`
	Images     PanocloudImages     `json:"Images"`
	Videos     PanocloudVideos     `json:"Videos"`
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
}

type PanocloudImages struct {
	Image []PanocloudImage `json:"image"`
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
	Video PanocloudVideo `json:"video"`
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

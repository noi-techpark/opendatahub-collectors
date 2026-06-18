// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package odhmodel

import "time"

// --- ODH Target Schema (WebcamInfo) ---

type WebcamInfo struct {
	Id               string                       `json:"Id"`
	Source           string                       `json:"Source"`
	Active           bool                         `json:"Active"`
	SmgActive        bool                         `json:"SmgActive"`
	WebCamProperties WebCamProperties             `json:"WebCamProperties"`
	LastChange       time.Time                    `json:"LastChange,omitempty"`
	Shortname        string                       `json:"Shortname,omitempty"`
	Detail           map[string]Detail            `json:"Detail,omitempty"`
	GpsInfo          []GpsInfo                    `json:"GpsInfo,omitempty"`
	ContactInfos     map[string]ContactInfo       `json:"ContactInfos,omitempty"`
	ImageGallery     []ImageGallery               `json:"ImageGallery,omitempty"`
	VideoItems       map[string][]VideoItem       `json:"VideoItems,omitempty"`
	Mapping          map[string]map[string]string `json:"Mapping,omitempty"`
	HasLanguage      []string                     `json:"HasLanguage,omitempty"`
	GpsPoints        map[string]GpsInfo           `json:"GpsPoints,omitempty"`
	OdhActive        bool                         `json:"OdhActive"`
	WebcamId         string                       `json:"WebcamId,omitempty"`
	Webcamurl        string                       `json:"Webcamurl,omitempty"`
}

type WebCamProperties struct {
	ViewAngleDegree string `json:"ViewAngleDegree,omitempty"`
	HasVR           bool   `json:"HasVR"`
	ViewerType      string `json:"ViewerType,omitempty"`
	WebcamUrl       string `json:"WebcamUrl,omitempty"`
	HtmlEmbed       string `json:"HtmlEmbed,omitempty"`
	ZeroDirection   string `json:"ZeroDirection,omitempty"`
	TourCam         bool   `json:"TourCam"`
}

type Detail struct {
	Title     string `json:"Title,omitempty"`
	IntroText string `json:"IntroText,omitempty"`
	BaseText  string `json:"BaseText,omitempty"`
	Language  string `json:"Language,omitempty"`
}

type ContactInfo struct {
	Region   string `json:"Region,omitempty"`
	Language string `json:"Language,omitempty"`
	LogoUrl  string `json:"LogoUrl,omitempty"`
}

type GpsInfo struct {
	Gpstype               string  `json:"Gpstype"`
	Latitude              float64 `json:"Latitude"`
	Longitude             float64 `json:"Longitude"`
	Altitude              float64 `json:"Altitude,omitempty"`
	AltitudeUnitofMeasure string  `json:"AltitudeUnitofMeasure,omitempty"`
}

type ImageGallery struct {
	Width        int      `json:"Width,omitempty"`
	Height       int      `json:"Height,omitempty"`
	ImageName    string   `json:"ImageName,omitempty"`
	ImageUrl     string   `json:"ImageUrl,omitempty"`
	ImageSource  string   `json:"ImageSource,omitempty"`
	IsInGallery  bool              `json:"IsInGallery"`
	ImageTags    []string          `json:"ImageTags,omitempty"`
	ListPosition int               `json:"ListPosition"`
	ImageDesc    map[string]string `json:"ImageDesc,omitempty"`
	ImageTitle   map[string]string `json:"ImageTitle,omitempty"`
	ImageAltText map[string]string `json:"ImageAltText,omitempty"`
}

type VideoItem struct {
	Url             string  `json:"Url,omitempty"`
	StreamingSource string  `json:"StreamingSource,omitempty"`
	Active          bool    `json:"Active"`
	Resolution      int     `json:"Resolution,omitempty"`
	Definition      string  `json:"Definition,omitempty"`
	Bitrate         int     `json:"Bitrate,omitempty"`
	Duration        float64 `json:"Duration,omitempty"`
	VideoType       string  `json:"VideoType,omitempty"`
}

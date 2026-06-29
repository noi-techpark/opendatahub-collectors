// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package contentmodel

import "time"

// --- ODH Target Schema (WebcamInfo) ---

type WebcamInfo struct {
	Id               string                       `json:"Id"`
	WebcamId         string                       `json:"WebcamId,omitempty"`
	Source           string                       `json:"Source"`
	Active           bool                         `json:"Active"`
	SmgActive        bool                         `json:"SmgActive"`
	OdhActive        bool                         `json:"OdhActive"`
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
}

type WebCamProperties struct {
	ViewAngleDegree string `json:"ViewAngleDegree,omitempty"`
	HasVR           bool   `json:"HasVR"`
	ViewerType      string `json:"ViewerType,omitempty"`
	WebcamUrl       string `json:"WebcamUrl,omitempty"`
}

type Detail struct {
	Title     string   `json:"Title,omitempty"`
	IntroText string   `json:"IntroText,omitempty"`
	BaseText  string   `json:"BaseText,omitempty"`
	Language  string   `json:"Language,omitempty"`
	Keywords  []string `json:"Keywords,omitempty"`
}

type ContactInfo struct {
	Region      string `json:"Region,omitempty"`
	Language    string `json:"Language,omitempty"`
	LogoUrl     string `json:"LogoUrl,omitempty"`
	ZipCode     string `json:"ZipCode,omitempty"`
	City        string `json:"City,omitempty"`
	Area        string `json:"Area,omitempty"`
	CountryCode string `json:"CountryCode,omitempty"`
	CountryName string `json:"CountryName,omitempty"`
	Url         string `json:"Url,omitempty"`
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
	IsInGallery  bool     `json:"IsInGallery"`
	ImageTags    []string `json:"ImageTags,omitempty"`
	ListPosition int      `json:"ListPosition"`
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

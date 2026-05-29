// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package odhmodel

import (
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

// ── Time helper ───────────────────────────────────────────────────────────────

type FlexibleTime struct {
	time.Time
}

func PtrFlexibleTime(t time.Time) *FlexibleTime {
	ft := FlexibleTime{Time: t}
	return &ft
}

func (ft *FlexibleTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	if s == "null" || s == "" || s == "0001-01-01T00:00:00" {
		ft.Time = time.Time{}
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		ft.Time = t
		return nil
	}
	t, err = time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		ft.Time = t
		return nil
	}
	return err
}

// ── WebcamInfo ────────────────────────────────────────────────────────────────

type WebcamInfo struct {
	// Identity
	Id        *string `json:"Id,omitempty"`
	WebcamId  string  `json:"WebcamId,omitempty"`
	Shortname *string `json:"Shortname,omitempty"`
	Source    *string `json:"Source,omitempty"`

	// Status
	Active    bool `json:"Active"`
	SmgActive bool `json:"SmgActive"`
	OdhActive bool `json:"OdhActive"`

	// Timestamps
	FirstImport *FlexibleTime `json:"FirstImport,omitempty"`
	LastChange  *FlexibleTime `json:"LastChange,omitempty"`

	// Language
	HasLanguage []string `json:"HasLanguage,omitempty"`

	// Mapping — same pattern as lifts/slopes
	Mapping map[string]map[string]string `json:"Mapping,omitempty"`

	// Multilingual content
	Detail       map[string]*clib.DetailGeneric `json:"Detail,omitempty"`
	Webcamname   map[string]string              `json:"Webcamname,omitempty"`
	ContactInfos map[string]interface{}         `json:"ContactInfos"` // always emit {}

	// URLs — top-level convenience fields (mirrors of WebCamProperties)
	Webcamurl  *string `json:"Webcamurl,omitempty"`
	Streamurl  *string `json:"Streamurl,omitempty"`
	Previewurl *string `json:"Previewurl,omitempty"`

	// GPS
	GpsInfo   []GpsInfo           `json:"GpsInfo,omitempty"`
	GpsPoints map[string]*GpsInfo `json:"GpsPoints,omitempty"`

	// Media
	ImageGallery []ImageGalleryEntry `json:"ImageGallery,omitempty"`

	// Webcam-specific properties
	WebCamProperties *WebCamProperties `json:"WebCamProperties,omitempty"`

	// Publishing
	PublishedOn []string `json:"PublishedOn"`

	// Tags
	TagIds  []string `json:"TagIds,omitempty"`
	SmgTags []string `json:"SmgTags,omitempty"`

	// License
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`

	// Misc
	ListPosition         *int        `json:"ListPosition,omitempty"`
	RelatedContent       interface{} `json:"RelatedContent,omitempty"`
	VideoItems           interface{} `json:"VideoItems,omitempty"`
	WebcamAssignedOn     interface{} `json:"WebcamAssignedOn,omitempty"`
	AdditionalProperties interface{} `json:"AdditionalProperties,omitempty"`
}

// ── Supporting types ──────────────────────────────────────────────────────────

type GpsInfo struct {
	Default               *bool   `json:"Default,omitempty"`
	Gpstype               string  `json:"Gpstype"`
	Altitude              *int    `json:"Altitude,omitempty"`
	Geometry              *string `json:"Geometry,omitempty"`
	Latitude              float64 `json:"Latitude"`
	Longitude             float64 `json:"Longitude"`
	AltitudeUnitofMeasure string  `json:"AltitudeUnitofMeasure,omitempty"`
}

// WebCamProperties matches the ODH WebCamProperties shape from live API.
type WebCamProperties struct {
	HasVR           *bool   `json:"HasVR,omitempty"`
	TourCam         *bool   `json:"TourCam,omitempty"`
	HtmlEmbed       *string `json:"HtmlEmbed,omitempty"`
	StreamUrl       *string `json:"StreamUrl,omitempty"`
	WebcamUrl       *string `json:"WebcamUrl,omitempty"`
	PreviewUrl      *string `json:"PreviewUrl,omitempty"`
	ViewerType      *string `json:"ViewerType,omitempty"`
	ZeroDirection   *string `json:"ZeroDirection,omitempty"`
	ViewAngleDegree *string `json:"ViewAngleDegree,omitempty"`
}

// ImageGalleryEntry matches the full ODH ImageGallery shape from live API.
type ImageGalleryEntry struct {
	Width         *int              `json:"Width,omitempty"`
	Height        *int              `json:"Height,omitempty"`
	License       *string           `json:"License,omitempty"`
	ValidTo       *string           `json:"ValidTo,omitempty"`
	ImageUrl      string            `json:"ImageUrl"`
	CopyRight     *string           `json:"CopyRight,omitempty"`
	ImageDesc     map[string]string `json:"ImageDesc"` // always emit {}
	ImageName     string            `json:"ImageName,omitempty"`
	ImageTags     []string          `json:"ImageTags,omitempty"`
	ValidFrom     *string           `json:"ValidFrom,omitempty"`
	ImageTitle    map[string]string `json:"ImageTitle"` // always emit {}
	ImageSource   string            `json:"ImageSource,omitempty"`
	IsInGallery   bool              `json:"IsInGallery"`
	ImageAltText  map[string]string `json:"ImageAltText"` // always emit {}
	ListPosition  *int              `json:"ListPosition,omitempty"`
	LicenseHolder *string           `json:"LicenseHolder,omitempty"`
}

type LicenseInfo struct {
	Author        string `json:"Author"`
	License       string `json:"License"`
	ClosedData    bool   `json:"ClosedData"`
	LicenseHolder string `json:"LicenseHolder"`
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

// LicenseInfo corresponds to the Content API license block.
type LicenseInfo struct {
	License       *string `json:"License,omitempty"`
	LicenseHolder *string `json:"LicenseHolder,omitempty"`
	Author        *string `json:"Author,omitempty"`
	ClosedData    bool    `json:"ClosedData"`
}

// Detail is one language of descriptive text.
type Detail struct {
	BaseText       *string `json:"BaseText,omitempty"`
	Title          *string `json:"Title,omitempty"`
	Header         *string `json:"Header,omitempty"`
	IntroText      *string `json:"IntroText,omitempty"`
	AdditionalText *string `json:"AdditionalText,omitempty"`
	GetThereText   *string `json:"GetThereText,omitempty"`
	Language       *string `json:"Language,omitempty"`
}

// ContactInfos is one language of contact data.
type ContactInfos struct {
	Language    *string `json:"Language,omitempty"`
	CompanyName *string `json:"CompanyName,omitempty"`
	Address     *string `json:"Address,omitempty"`
	City        *string `json:"City,omitempty"`
	ZipCode     *string `json:"ZipCode,omitempty"`
	CountryCode *string `json:"CountryCode,omitempty"`
	Email       *string `json:"Email,omitempty"`
	Phonenumber *string `json:"Phonenumber,omitempty"`
	Faxnumber   *string `json:"Faxnumber,omitempty"`
	Url         *string `json:"Url,omitempty"`
}

// ImageGallery is one image. Titles are merged per language.
type ImageGallery struct {
	ImageUrl     *string           `json:"ImageUrl,omitempty"`
	ImageName    *string           `json:"ImageName,omitempty"`
	ImageTitle   map[string]string `json:"ImageTitle,omitempty"`
	CopyRight    *string           `json:"CopyRight,omitempty"`
	License      *string           `json:"License,omitempty"`
	IsInGallery  *bool             `json:"IsInGallery,omitempty"`
	ListPosition *int              `json:"ListPosition,omitempty"`
}

// NamedRef is a region, municipality, district, or tourism association.
type NamedRef struct {
	ID   *string           `json:"Id,omitempty"`
	Name map[string]string `json:"Name,omitempty"`
}

// LocationInfo holds the place hierarchy. Names are merged per language.
type LocationInfo struct {
	RegionInfo       *NamedRef `json:"RegionInfo,omitempty"`
	MunicipalityInfo *NamedRef `json:"MunicipalityInfo,omitempty"`
	TvInfo           *NamedRef `json:"TvInfo,omitempty"`
	DistrictInfo     *NamedRef `json:"DistrictInfo,omitempty"`
}

// GpsInfo is a position. Geometry is WKT with longitude first.
type GpsInfo struct {
	Gpstype               *string  `json:"Gpstype,omitempty"`
	Latitude              *float64 `json:"Latitude,omitempty"`
	Longitude             *float64 `json:"Longitude,omitempty"`
	Altitude              *float64 `json:"Altitude,omitempty"`
	AltitudeUnitofMeasure *string  `json:"AltitudeUnitofMeasure,omitempty"`
	Geometry              *string  `json:"Geometry,omitempty"`
	Default               bool     `json:"Default"`
}

// OperationScheduleTime is one opening slot.
type OperationScheduleTime struct {
	Start     *string `json:"Start,omitempty"`
	End       *string `json:"End,omitempty"`
	Monday    bool    `json:"Monday"`
	Tuesday   bool    `json:"Tuesday"`
	Wednesday bool    `json:"Wednesday"`
	Thursday  bool    `json:"Thursday"`
	Friday    bool    `json:"Friday"`
	Saturday  bool    `json:"Saturday"`
	Sunday    bool    `json:"Sunday"`
	State     int     `json:"State"`
}

// OperationSchedule is a validity period of opening slots.
type OperationSchedule struct {
	Start                 *string                 `json:"Start,omitempty"`
	Stop                  *string                 `json:"Stop,omitempty"`
	Type                  *string                 `json:"Type,omitempty"`
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime,omitempty"`
}

// Generic is the shared Content API header.
// Mapping, including the sync time, is excluded from the change hash.
type Generic struct {
	ID          *string      `json:"Id,omitempty" hash:"ignore"`
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`
	Shortname   *string      `json:"Shortname,omitempty"`
	Active      bool         `json:"Active"`
	SmgActive   *bool        `json:"SmgActive,omitempty"`
	OdhActive   *bool        `json:"OdhActive,omitempty"`
	FirstImport *string      `json:"FirstImport,omitempty" hash:"ignore"`
	LastChange  *string      `json:"LastChange,omitempty" hash:"ignore"`
	HasLanguage []string     `json:"HasLanguage,omitempty" hash:"set"`

	Mapping map[string]map[string]string `json:"Mapping,omitempty" hash:"ignore"`

	Source      *string  `json:"Source,omitempty" hash:"ignore"`
	TagIds      []string `json:"TagIds,omitempty" hash:"set"`
	PublishedOn []string `json:"PublishedOn"`
}

// ODHActivityPoi is one gastronomy in the Content API.
type ODHActivityPoi struct {
	Generic

	Type         *string                 `json:"Type,omitempty"`
	SubType      *string                 `json:"SubType,omitempty"`
	Detail       map[string]Detail       `json:"Detail,omitempty"`
	ContactInfos map[string]ContactInfos `json:"ContactInfos,omitempty"`
	ImageGallery []ImageGallery          `json:"ImageGallery,omitempty"`
	SmgTags      []string                `json:"SmgTags,omitempty" hash:"set"`
	// The local Content API clears LocationInfo when the referenced region does
	// not exist. Keep sending it, and do not let that round-trip mark the POI as changed.
	LocationInfo      *LocationInfo       `json:"LocationInfo,omitempty" hash:"ignore"`
	GpsInfo           []GpsInfo           `json:"GpsInfo,omitempty"`
	OperationSchedule []OperationSchedule `json:"OperationSchedule,omitempty"`
	CustomId          *string             `json:"CustomId,omitempty"`
}

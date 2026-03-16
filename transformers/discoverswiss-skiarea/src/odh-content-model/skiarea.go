// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import "time"

// LicenseInfo corresponds to the LicenseInfo class and ILicenseInfo interface.
type LicenseInfo struct {
	License       *string `json:"License,omitempty"`
	LicenseHolder *string `json:"LicenseHolder,omitempty"`
	Author        *string `json:"Author,omitempty"`
	ClosedData    bool    `json:"ClosedData"`
}

// UpdateInfo is not provided, assuming it's a simple struct or left out for now.
type UpdateInfo struct{}

// Metadata corresponds to the Metadata class and IMetaData interface.
type Metadata struct {
	ID         string      `json:"Id"`
	Type       string      `json:"Type"`
	LastUpdate *time.Time  `json:"LastUpdate,omitempty"`
	Source     string      `json:"Source"`
	Reduced    bool        `json:"Reduced"`
	UpdateInfo *UpdateInfo `json:"UpdateInfo,omitempty"`
}

type DetailGeneric struct {
	BaseText *string `json:"BaseText,omitempty"`
	Title    *string `json:"Title,omitempty"`
	Language *string `json:"Language,omitempty"`
}

// Detail extends DetailGeneric with additional fields
type Detail struct {
	BaseText       *string `json:"BaseText,omitempty"`
	Title          *string `json:"Title,omitempty"`
	Header         *string `json:"Header,omitempty"`
	IntroText      *string `json:"IntroText,omitempty"`
	AdditionalText *string `json:"AdditionalText,omitempty"`
	GetThereText   *string `json:"GetThereText,omitempty"`
	SubHeader      *string `json:"SubHeader,omitempty"`
	Language       *string `json:"Language,omitempty"`
}

// ContactInfos represents contact information
type ContactInfos struct {
	Language    *string `json:"Language,omitempty"`
	CompanyName *string `json:"CompanyName,omitempty"`
	Givenname   *string `json:"Givenname,omitempty"`
	Surname     *string `json:"Surname,omitempty"`
	Address     *string `json:"Address,omitempty"`
	City        *string `json:"City,omitempty"`
	ZipCode     *string `json:"ZipCode,omitempty"`
	CountryCode *string `json:"CountryCode,omitempty"`
	Email       *string `json:"Email,omitempty"`
	Phonenumber *string `json:"Phonenumber,omitempty"`
	Faxnumber   *string `json:"Faxnumber,omitempty"`
	Url         *string `json:"Url,omitempty"`
	LogoUrl     *string `json:"LogoUrl,omitempty"`
}

// ImageGallery represents an image in the gallery
type ImageGallery struct {
	ImageUrl     *string           `json:"ImageUrl,omitempty"`
	ImageName    *string           `json:"ImageName,omitempty"`
	ImageTitle   map[string]string `json:"ImageTitle,omitempty"`
	ImageDesc    map[string]string `json:"ImageDesc,omitempty"`
	CopyRight    *string           `json:"CopyRight,omitempty"`
	License      *string           `json:"License,omitempty"`
	ImageSource  *string           `json:"ImageSource,omitempty"`
	Width        *int              `json:"Width,omitempty"`
	Height       *int              `json:"Height,omitempty"`
	IsInGallery  *bool             `json:"IsInGallery,omitempty"`
	ListPosition *int              `json:"ListPosition,omitempty"`
}

// RegionInfo represents region information
type RegionInfo struct {
	ID   *string           `json:"Id,omitempty"`
	Name map[string]string `json:"Name,omitempty"`
}

// MunicipalityInfo represents municipality information
type MunicipalityInfo struct {
	ID   *string           `json:"Id,omitempty"`
	Name map[string]string `json:"Name,omitempty"`
}

// TvInfo represents tourism association information
type TvInfo struct {
	ID   *string           `json:"Id,omitempty"`
	Name map[string]string `json:"Name,omitempty"`
}

// DistrictInfo represents district information
type DistrictInfo struct {
	ID   *string           `json:"Id,omitempty"`
	Name map[string]string `json:"Name,omitempty"`
}

// LocationInfo represents location information
type LocationInfo struct {
	RegionInfo       *RegionInfo       `json:"RegionInfo,omitempty"`
	MunicipalityInfo *MunicipalityInfo `json:"MunicipalityInfo,omitempty"`
	TvInfo           *TvInfo           `json:"TvInfo,omitempty"`
	DistrictInfo     *DistrictInfo     `json:"DistrictInfo,omitempty"`
}

// RelatedContent corresponds to the RelatedContent class.
type RelatedContent struct {
	ID   *string `json:"Id,omitempty"`
	Type *string `json:"Type,omitempty"`
	Self *string `json:"Self,omitempty"`
}

type GpsInfo struct {
	Gpstype               *string  `json:"Gpstype,omitempty"`
	Latitude              *float64 `json:"Latitude,omitempty"`
	Longitude             *float64 `json:"Longitude,omitempty"`
	Altitude              *float64 `json:"Altitude,omitempty"`
	AltitudeUnitofMeasure *string  `json:"AltitudeUnitofMeasure,omitempty"`
	Geometry              *string  `json:"Geometry,omitempty"`
	Default               bool     `json:"Default"`
}

// OperationScheduleTime represents a time slot in an operation schedule
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

// OperationSchedule represents a scheduled operation period
type OperationSchedule struct {
	OperationscheduleName map[string]string       `json:"OperationscheduleName,omitempty"`
	Start                 *string                 `json:"Start,omitempty"`
	Stop                  *string                 `json:"Stop,omitempty"`
	Type                  *string                 `json:"Type,omitempty"`
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime,omitempty"`
}

// Generic corresponds to the Generic class.
// It is intended to be embedded (inlined) in other structs.
type Generic struct {
	ID          *string      `json:"Id,omitempty" hash:"ignore"`
	Meta        *Metadata    `json:"_Meta,omitempty" hash:"ignore"`
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`
	Shortname   *string      `json:"Shortname,omitempty"`
	Active      bool         `json:"Active"`
	FirstImport *string      `json:"FirstImport,omitempty" hash:"ignore"`
	LastChange  *string      `json:"LastChange,omitempty" hash:"ignore"`
	HasLanguage []string     `json:"HasLanguage,omitempty" hash:"set"`

	Mapping map[string]map[string]string `json:"Mapping,omitempty" hash:"ignore"`

	Source *string            `json:"Source,omitempty" hash:"ignore"`
	TagIds []string           `json:"TagIds,omitempty" hash:"set"`
	Geo    map[string]GpsInfo `json:"Geo,omitempty"`
}

// SkiArea corresponds to the OpenDataHub SkiArea model.
type SkiArea struct {
	Generic // Embedded struct for inlining fields

	Detail            map[string]Detail       `json:"Detail,omitempty"`
	ContactInfos      map[string]ContactInfos `json:"ContactInfos,omitempty"`
	ImageGallery      []ImageGallery          `json:"ImageGallery,omitempty"`
	SmgTags           []string                `json:"SmgTags,omitempty" hash:"set"`
	LocationInfo      *LocationInfo           `json:"LocationInfo,omitempty"`
	SkiRegionName     map[string]string       `json:"SkiRegionName,omitempty"`
	GpsInfo           []GpsInfo               `json:"GpsInfo,omitempty"`
	TotalSlopeKm      *string                 `json:"TotalSlopeKm,omitempty"`
	SlopeKmBlue       *string                 `json:"SlopeKmBlue,omitempty"`
	SlopeKmRed        *string                 `json:"SlopeKmRed,omitempty"`
	SlopeKmBlack      *string                 `json:"SlopeKmBlack,omitempty"`
	LiftCount         *string                 `json:"LiftCount,omitempty"`
	AltitudeFrom      *int                    `json:"AltitudeFrom,omitempty"`
	AltitudeTo        *int                    `json:"AltitudeTo,omitempty"`
	SkiAreaMapURL     *string                 `json:"SkiAreaMapURL,omitempty"`
	CustomId          *string                 `json:"CustomId,omitempty"`
	OperationSchedule []OperationSchedule     `json:"OperationSchedule,omitempty"`
}

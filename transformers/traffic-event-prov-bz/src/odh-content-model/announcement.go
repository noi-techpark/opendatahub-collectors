// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import "time"

// LicenseInfo corresponds to the C# LicenseInfo class and ILicenseInfo interface.
type LicenseInfo struct {
	License       *string `json:"License,omitempty"`
	LicenseHolder *string `json:"LicenseHolder,omitempty"`
	Author        *string `json:"Author,omitempty"`
	ClosedData    bool    `json:"ClosedData"`
}

// UpdateInfo is not provided, assuming it's a simple struct or left out for now.
// For now, it will be an empty struct placeholder.
type UpdateInfo struct{}

// Metadata corresponds to the C# Metadata class and IMetaData interface.
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

// RelatedContent corresponds to the C# RelatedContent class.
type RelatedContent struct {
	ID   *string `json:"Id,omitempty"`
	Type *string `json:"Type,omitempty"`
	Self *string `json:"Self,omitempty"` // Computed property, but including it just in case it's serialized
}

type ProviderProvinceBz struct {
	Id       string    `json:"Id"`
	SyncTime time.Time `json:"SyncTime" hash:"ignore"`
}

type GpsInfo struct {
	Gpstype               *string  `json:"gpstype,omitempty"`
	Latitude              *float64 `json:"latitude,omitempty"`
	Longitude             *float64 `json:"longitude,omitempty"`
	Altitude              *float64 `json:"altitude,omitempty"`
	AltitudeUnitofMeasure *string  `json:"altitudeUnitofMeasure,omitempty"`
	Geometry              *string  `json:"geometry,omitempty"`
	Default               bool     `json:"default"`
}

// Generic corresponds to the C# Generic class.
// It is intended to be embedded (inlined) in other structs.
type Generic struct {
	ID          *string      `json:"Id,omitempty" hash:"ignore"`
	Meta        *Metadata    `json:"_Meta,omitempty" hash:"ignore"`
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`
	Shortname   *string      `json:"Shortname,omitempty"`
	Active      bool         `json:"Active"`
	FirstImport *time.Time   `json:"FirstImport,omitempty" hash:"ignore"`
	LastChange  *time.Time   `json:"LastChange,omitempty" hash:"ignore"`
	HasLanguage []string     `json:"HasLanguage,omitempty" hash:"set"`

	// need to make typing explicit because map[string]any is not properly hashed
	Mapping struct {
		ProviderProvinceBz ProviderProvinceBz `json:"ProviderProvinceBz"`
	} `json:"Mapping"`

	// need to make typing explicit because map[string]any is not properly hashed
	// AdditionalProperties struct {
	// 	RoadIncidentProperties RoadIncidentProperties `json:"RoadIncidentProperties"`
	// } `json:"AdditionalProperties,omitempty"`

	Source *string            `json:"Source,omitempty" hash:"ignore"`
	TagIds []string           `json:"TagIds,omitempty" hash:"set"`
	Geo    map[string]GpsInfo `json:"Geo,omitempty"`
}

// Announcement corresponds to the C# Announcement class, embedding Generic for inlining.
type Announcement struct {
	Generic // Embedded struct for inlining fields

	StartTime      *time.Time                `json:"StartTime,omitempty" `
	EndTime        *time.Time                `json:"EndTime,omitempty"`
	Detail         map[string]*DetailGeneric `json:"Detail,omitempty"`
	RelatedContent []*RelatedContent         `json:"RelatedContent,omitempty" hash:"set"`
}

// // RoadIncidentProperties corresponds to the main JSON object.
// type RoadIncidentProperties struct {
// 	// Note the struct tags are now PascalCase to match the C# properties
// 	RoadsInvolved        []RoadInvolved `json:"RoadsInvolved" hash:"set"`
// 	ExpectedDelayMinutes *int           `json:"ExpectedDelayMinutes"`
// 	ExpectedDelayString  *string        `json:"ExpectedDelayString"`
// }

// // RoadInvolved represents an item in the 'RoadsInvolved' array.
// type RoadInvolved struct {
// 	Name  *string    `json:"Name"`
// 	Code  *string    `json:"Code"`
// 	Lanes []LaneInfo `json:"Lanes"`
// }

// // LaneInfo represents an item in the 'Lanes' array.
// type LaneInfo struct {
// 	// Lane is a pointer for optionality (like C#'s int?)
// 	Lane      *int              `json:"Lane"`
// 	LaneName  map[string]string `json:"LaneName"`
// 	Direction *string           `json:"Direction"`
// }

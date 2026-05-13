// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import (
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

type ProviderR3GIS struct {
	Id             string    `json:"Id"`
	RemoteProvider string    `json:"RemoteProvider"`
	SyncTime       time.Time `json:"SyncTime" hash:"ignore"`
}

// Generic corresponds to the C# Generic class.
// It is intended to be embedded (inlined) in other structs.
type Generic struct {
	ID          *string          `json:"Id,omitempty" hash:"ignore"`
	Meta        *clib.Metadata   `json:"_Meta,omitempty" hash:"ignore"`
	LicenseInfo *clib.LicenseInfo `json:"LicenseInfo,omitempty"`
	Shortname   *string          `json:"Shortname,omitempty"`
	Active      bool             `json:"Active"`
	FirstImport *time.Time       `json:"FirstImport,omitempty" hash:"ignore"`
	LastChange  *time.Time       `json:"LastChange,omitempty" hash:"ignore"`
	HasLanguage []string         `json:"HasLanguage,omitempty" hash:"set"`

	// need to make typing explicit because map[string]any is not properly hashed
	Mapping struct {
		ProviderR3GIS ProviderR3GIS `json:"ProviderR3GIS"`
	} `json:"Mapping"`

	// need to make typing explicit because map[string]any is not properly hashed
	AdditionalProperties struct {
		UrbanGreenProperties UrbanGreenProperties `json:"UrbanGreenProperties"`
	} `json:"AdditionalProperties,omitempty"`

	Source *string               `json:"Source,omitempty" hash:"ignore"`
	TagIds []string              `json:"TagIds,omitempty" hash:"set"`
	Geo    map[string]clib.GpsInfo `json:"Geo,omitempty"`
}

// UrbanGreen corresponds to the C# UrbanGreen class, embedding Generic for inlining.
type UrbanGreen struct {
	Generic // Embedded struct for inlining fields

	PutOnSite       *time.Time                     `json:"PutOnSite,omitempty"`
	RemovedFromSite *time.Time                     `json:"RemovedFromSite,omitempty"`
	Detail          map[string]*clib.DetailGeneric `json:"Detail,omitempty"`

	GreenCode        string `json:"GreenCode,omitempty"`
	GreenCodeType    string `json:"GreenCodeType,omitempty"`
	GreenCodeSubtype string `json:"GreenCodeSubtype,omitempty"`
	GreenCodeVersion string `json:"GreenCodeVersion,omitempty"`
}

// UrbanGreenProperties corresponds to the main JSON object.
type UrbanGreenProperties struct {
	Taxonomy map[string]string `json:"Taxonomy"`
}

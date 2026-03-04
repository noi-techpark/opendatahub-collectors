// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import (
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

type ProviderA22BrennerLEC struct {
	Id       string    `json:"Id"`
	SyncTime time.Time `json:"SyncTime" hash:"ignore"`
	Limite   string    `json:"Limite"`
}

// Generic corresponds to the C# Generic class.
type Generic struct {
	ID          *string           `json:"Id,omitempty" hash:"ignore"`
	Meta        *clib.Metadata    `json:"_Meta,omitempty" hash:"ignore"`
	LicenseInfo *clib.LicenseInfo `json:"LicenseInfo,omitempty"`
	Shortname   *string           `json:"Shortname,omitempty"`
	Active      bool              `json:"Active"`
	FirstImport *time.Time        `json:"FirstImport,omitempty" hash:"ignore"`
	LastChange  *time.Time        `json:"LastChange,omitempty" hash:"ignore"`
	HasLanguage []string          `json:"HasLanguage,omitempty" hash:"set"`

	Mapping struct {
		ProviderA22BrennerLEC ProviderA22BrennerLEC `json:"ProviderA22BrennerLEC"`
	} `json:"Mapping"`

	Source *string                 `json:"Source,omitempty" hash:"ignore"`
	TagIds []string                `json:"TagIds,omitempty" hash:"set"`
	Geo    map[string]clib.GpsInfo `json:"Geo,omitempty"`
}

// Announcement corresponds to the C# Announcement class.
type Announcement struct {
	Generic

	StartTime      *time.Time                     `json:"StartTime,omitempty"`
	EndTime        *time.Time                     `json:"EndTime,omitempty"`
	Detail         map[string]*clib.DetailGeneric  `json:"Detail,omitempty"`
	RelatedContent []*clib.RelatedContent          `json:"RelatedContent,omitempty" hash:"set"`
}

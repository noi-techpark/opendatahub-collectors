// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import (
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

type ProviderA22 struct {
	Id                string    `json:"Id"`
	SyncTime          time.Time `json:"SyncTime" hash:"ignore"`
	FasciaOraria      string    `json:"FasciaOraria,omitempty"`
	Idcorsia          string    `json:"Idcorsia,omitempty"`
	Iddirezione       string    `json:"Iddirezione,omitempty"`
	MetroInizio       string    `json:"MetroInizio,omitempty"`
	MetroFine         string    `json:"MetroFine,omitempty"`
	Idsottotipoevento string    `json:"Idsottotipoevento,omitempty"`
}

// Generic corresponds to the C# Generic class.
// It is intended to be embedded (inlined) in other structs.
type Generic struct {
	ID          *string           `json:"Id,omitempty" hash:"ignore"`
	Meta        *clib.Metadata    `json:"_Meta,omitempty" hash:"ignore"`
	LicenseInfo *clib.LicenseInfo `json:"LicenseInfo,omitempty"`
	Shortname   *string           `json:"Shortname,omitempty"`
	Active      bool              `json:"Active"`
	FirstImport *time.Time        `json:"FirstImport,omitempty" hash:"ignore"`
	LastChange  *time.Time        `json:"LastChange,omitempty" hash:"ignore"`
	HasLanguage []string          `json:"HasLanguage,omitempty" hash:"set"`

	// need to make typing explicit because map[string]any is not properly hashed
	Mapping struct {
		ProviderA22 ProviderA22 `json:"ProviderA22"`
	} `json:"Mapping"`

	Source *string                 `json:"Source,omitempty" hash:"ignore"`
	TagIds []string                `json:"TagIds,omitempty" hash:"set"`
	Geo    map[string]clib.GpsInfo `json:"Geo,omitempty"`
}

// Announcement corresponds to the C# Announcement class, embedding Generic for inlining.
type Announcement struct {
	Generic // Embedded struct for inlining fields

	StartTime      *time.Time                     `json:"StartTime,omitempty"`
	EndTime        *time.Time                     `json:"EndTime,omitempty"`
	Detail         map[string]*clib.DetailGeneric `json:"Detail,omitempty"`
	RelatedContent []*clib.RelatedContent         `json:"RelatedContent,omitempty" hash:"set"`
}

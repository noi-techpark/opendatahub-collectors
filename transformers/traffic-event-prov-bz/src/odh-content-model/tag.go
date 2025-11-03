// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import "time"

type SmgTags struct {
}

type Tag struct {
	////////////////////
	/// C# SmgTags fields
	ID *string `json:"Id,omitempty"` // Nullable string in C#

	TagName map[string]string `json:"TagName"` // Non-nullable initialized property

	ValidForEntity []string `json:"ValidForEntity"` // Non-nullable initialized property

	FirstImport *time.Time `json:"FirstImport,omitempty" hash:"ignore"` // Nullable DateTime in C#
	LastChange  *time.Time `json:"LastChange,omitempty" hash:"ignore"`  // Nullable DateTime in C#

	// LicenseInfo
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty" hash:"ignore"` // Nullable object

	Mapping map[string]map[string]string `json:"Mapping"` // Non-nullable initialized property

	PublishDataWithTagOn map[string]bool `json:"PublishDataWithTagOn,omitempty"` // Nullable IDictionary<string, bool>

	PublishedOn []string `json:"PublishedOn,omitempty"` // Nullable ICollection<string>
	////////////////////
	/// C# TagLinked

	Meta *Metadata `json:"_Meta,omitempty" hash:"ignore"`

	Types []string `json:"Types"` // Non-nullable initialized property

	Source string `json:"Source"`

	Active bool `json:"Active"`

	Description map[string]string `json:"Description"` // Non-nullable initialized property
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

type RawData struct {
	De []SiagMuseum `json:"de"`
	En []SiagMuseum `json:"en"`
	It []SiagMuseum `json:"it"`
}

type SiagMuseum struct {
	System   SiagSystem   `json:"system"`
	Elements SiagElements `json:"elements"`
}

type SiagSystem struct {
	Id           string `json:"id"`       // UUID
	Codename     string `json:"codename"` // stable slug → ODH Id
	Language     string `json:"language"` // "de" | "it" | "en"
	Type         string `json:"type"`
	LastModified string `json:"last_modified"`
	WorkflowStep string `json:"workflow_step"` // "published" = active
}

type SiagElements struct {
	Id                SiagNumberField   `json:"id"` // numeric, nullable
	Title             SiagTextField     `json:"title"`
	Description       SiagRichTextField `json:"description"` // HTML
	Street            SiagTextField     `json:"street"`
	Municipality      SiagTextField     `json:"municipality"`
	ZipCode           SiagTextField     `json:"zip_code"`
	GeoCoordX         SiagTextField     `json:"geocoordinate_x"` // longitude as string
	GeoCoordY         SiagTextField     `json:"geocoordinate_y"` // latitude as string
	Phone             SiagTextField     `json:"phone"`
	Phone2            SiagTextField     `json:"phone_2"`
	Email             SiagTextField     `json:"e_mail"`
	Web               SiagTextField     `json:"web"`
	OpeningHours      SiagRichTextField `json:"opening_hours"` // HTML
	Fee               SiagRichTextField `json:"fee"`           // HTML
	MainImage         SiagAssetField    `json:"main_image"`
	PhotoGallery      SiagAssetField    `json:"photo_gallery"`
	MuseumCategories  SiagTaxonomyField `json:"museum_categories"`
	MuseumServices    SiagTaxonomyField `json:"museum_services"`
	MuseumOfferings   SiagTaxonomyField `json:"museum_offerings"`
	Districts         SiagTaxonomyField `json:"districts"`
	MuseumAssociation SiagChoiceField   `json:"museum_association"`
	ProvincialMuseum  SiagChoiceField   `json:"provincial_museum"`
	Paramuseum        SiagChoiceField   `json:"paramuseum"`
	Mus               SiagTextField     `json:"mus"` // museum card code e.g. "AUG"
	Url               SiagTextField     `json:"url"` // slug
}

// ── Field type wrappers ───────────────────────────────────────────────────────

type SiagTextField struct {
	Value string `json:"value"`
}

type SiagRichTextField struct {
	Value string `json:"value"` // HTML string
}

// SiagNumberField wraps a numeric value that may be null.
type SiagNumberField struct {
	Value *float64 `json:"value"` // pointer — null for some records
}

type SiagAssetField struct {
	Value []SiagAsset `json:"value"`
}

type SiagAsset struct {
	Name        string `json:"name"`
	Description string `json:"description"` // photographer credit
	Type        string `json:"type"`        // MIME type
	Url         string `json:"url"`
	Width       int    `json:"width"`
	Height      int    `json:"height"`
}

type SiagTaxonomyField struct {
	Value []TaxonomyEntry `json:"value"`
}

// TaxonomyEntry is a single taxonomy term.
// Name contains a pipe-separated multilingual string: "de|it|de|en"
type TaxonomyEntry struct {
	Name     string `json:"name"`     // "Kunst|Arte|Kunst|Art"
	Codename string `json:"codename"` // "art"
}

type SiagChoiceField struct {
	Value []ChoiceEntry `json:"value"`
}

// ChoiceEntry represents a yes/no choice field.
// Codename is "yes" or "no"; Name is a human-readable label used as the
// tag display name when the choice resolves to "yes" (e.g. paramuseum,
// provincial_museum, museum_association).
type ChoiceEntry struct {
	Name     string `json:"name"`
	Codename string `json:"codename"` // "yes" | "no"
}

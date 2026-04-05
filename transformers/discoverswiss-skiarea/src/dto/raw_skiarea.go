// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

import "encoding/json"

// SkiArea represents the raw data structure from DiscoverSwiss API
type SkiArea struct {
	ID                        string               `json:"@id"`
	Identifier                string               `json:"identifier"`
	ApiCrawlerLang            string               `json:"__api_crawler_lang"`
	Type                      string               `json:"type"`
	Name                      string               `json:"name"`
	Description               string               `json:"description,omitempty"`
	DisambiguatingDescription string               `json:"disambiguatingDescription,omitempty"`
	AutoTranslatedData        bool                 `json:"autoTranslatedData"`
	AvailableDataLanguage     []string             `json:"availableDataLanguage,omitempty"`
	Removed                   bool                 `json:"removed"`
	License                   string               `json:"license,omitempty"`
	LastModified              string               `json:"lastModified,omitempty"`
	OsmID                     string               `json:"osm_id,omitempty"`
	Telephone                 string               `json:"telephone,omitempty"`
	URL                       string               `json:"url,omitempty"`
	Address                   *Address             `json:"address,omitempty"`
	Geo                       *GeoCoordinates      `json:"geo,omitempty"`
	Image                     *ImageObject         `json:"image,omitempty"`
	Photo                     []ImageObject        `json:"photo,omitempty"`
	Link                      []Link               `json:"link,omitempty"`
	Category                  []Category           `json:"category,omitempty"`
	ContainedInPlace          []AdministrativeArea `json:"containedInPlace,omitempty"`
	DataGovernance            *DataGovernance      `json:"dataGovernance,omitempty"`

	// Summary and condition objects (complex, pass-through)
	AdditionalProperty  json.RawMessage `json:"additionalProperty,omitempty"`
	CrossCountrySummary json.RawMessage `json:"crossCountrySummary,omitempty"`
	HikingSummary       json.RawMessage `json:"hikingSummary,omitempty"`
	SkiLiftSummary      json.RawMessage `json:"skiLiftSummary,omitempty"`
	SkiSlopeSummary     json.RawMessage `json:"skiSlopeSummary,omitempty"`
	SnowConditions      json.RawMessage `json:"snowConditions,omitempty"`
	SnowConditionsSlope json.RawMessage `json:"snowConditionsSlope,omitempty"`
	SnowboardSummary    json.RawMessage `json:"snowboardSummary,omitempty"`
	TobogganingSummary  json.RawMessage `json:"tobogganingSummary,omitempty"`
	WeatherMountain     json.RawMessage `json:"weatherMountain,omitempty"`
	WeatherValley       json.RawMessage `json:"weatherValley,omitempty"`
	TouristInformation  json.RawMessage `json:"touristInformation,omitempty"`

	// Simple fields
	SeasonStart   string `json:"seasonStart,omitempty"`
	SeasonEnd     string `json:"seasonEnd,omitempty"`
	MinElevation  int    `json:"minElevation,omitempty"`
	MaxElevation  int    `json:"maxElevation,omitempty"`
	HasMap        string `json:"hasMap,omitempty"`
	HasShuttleBus *bool  `json:"hasShuttleBus,omitempty"`

	// Other
	PotentialAction json.RawMessage `json:"potentialAction,omitempty"`
	Logo            json.RawMessage `json:"logo,omitempty"`

	// Sub-entities
	HasSkiLift      []SkiSubEntity `json:"hasSkiLift,omitempty"`
	HasSkiSlope     []SkiSubEntity `json:"hasSkiSlope,omitempty"`
	HasSnowPark     []SkiSubEntity `json:"hasSnowPark,omitempty"`
	HasTobogganing  []SkiSubEntity `json:"hasTobogganing,omitempty"`
	HasCrossCountry []SkiSubEntity `json:"hasCrossCountry,omitempty"`
	HasHiking       []SkiSubEntity `json:"hasHiking,omitempty"`
}

// SkiSubEntity represents an entry in hasSkiLift/hasSkiSlope/hasSnowPark/hasTobogganing
type SkiSubEntity struct {
	Details *SkiSubEntityDetails `json:"details,omitempty"`
}

// SkiSubEntityDetails represents the details of a ski sub-entity (slope, lift, park, toboggan)
// These map to DiscoverSwiss Tour/SkiSlope fields
type SkiSubEntityDetails struct {
	ID                        string               `json:"@id,omitempty"`
	Identifier                string               `json:"identifier,omitempty"`
	Type                      string               `json:"type,omitempty"`
	AdditionalType            string               `json:"additionalType,omitempty"`
	Name                      string               `json:"name,omitempty"`
	Description               string               `json:"description,omitempty"`
	DisambiguatingDescription string               `json:"disambiguatingDescription,omitempty"`
	AvailableDataLanguage     []string             `json:"availableDataLanguage,omitempty"`
	Removed                   bool                 `json:"removed"`
	License                   string               `json:"license,omitempty"`
	LastModified              string               `json:"lastModified,omitempty"`
	Telephone                 string               `json:"telephone,omitempty"`
	FaxNumber                 string               `json:"faxNumber,omitempty"`
	URL                       string               `json:"url,omitempty"`
	Address                   *Address             `json:"address,omitempty"`
	Geo                       *GeoCoordinates      `json:"geo,omitempty"`
	Image                     json.RawMessage      `json:"image,omitempty"`
	Photo                     []ImageObject        `json:"photo,omitempty"`
	Link                      []Link               `json:"link,omitempty"`
	Category                  []Category           `json:"category,omitempty"`
	Tag                       []Tag                `json:"tag,omitempty"`
	ContainedInPlace          []AdministrativeArea `json:"containedInPlace,omitempty"`
	DataGovernance            *DataGovernance      `json:"dataGovernance,omitempty"`

	// Additional properties (e.g. slope label)
	AdditionalProperty []AdditionalProperty `json:"additionalProperty,omitempty"`

	// State / opening
	State               string `json:"state,omitempty"`
	IsAccessibleForFree *bool  `json:"isAccessibleForFree,omitempty"`

	// Tour/slope specific fields
	Length                float64     `json:"length,omitempty"`
	Time                  int         `json:"time,omitempty"`
	Elevation             *Elevation  `json:"elevation,omitempty"`
	Rating                *Rating     `json:"rating,omitempty"`
	Exposition            *Exposition `json:"exposition,omitempty"`
	Highlight             *bool       `json:"highlight,omitempty"`
	GettingThere          string      `json:"gettingThere,omitempty"`
	Parking               string      `json:"parking,omitempty"`
	PublicTransport       string      `json:"publicTransport,omitempty"`
	AdditionalInformation string      `json:"additionalInformation,omitempty"`
	Directions            string      `json:"directions,omitempty"`
	Equipment             string      `json:"equipment,omitempty"`
	SafetyGuidelines      string      `json:"safetyGuidelines,omitempty"`
	TextTeaser            string      `json:"textTeaser,omitempty"`
	TitleTeaser           string      `json:"titleTeaser,omitempty"`

	// Opening hours
	OpeningHoursSpecification []OpeningHoursSpec `json:"openingHoursSpecification,omitempty"`

	// Video
	Video []VideoObject `json:"video,omitempty"`

	// Alternate name
	AlternateName string `json:"alternateName,omitempty"`

	// Duration in ISO 8601
	Duration string `json:"duration,omitempty"`

	// Difficulties
	Difficulties *Difficulties `json:"difficulties,omitempty"`

	// Extra fields preserved in Mapping.Data
	LocatedAt          json.RawMessage `json:"locatedAt,omitempty"`
	StateTimestamp     string          `json:"stateTimestamp,omitempty"`
	PotentialAction    json.RawMessage `json:"potentialAction,omitempty"`
	LengthOpen         float64         `json:"lengthOpen,omitempty"`
	AutoTranslatedData bool            `json:"autoTranslatedData"`
}

// ParseImages parses the Image field which can be a single object or an array.
func (s SkiSubEntityDetails) ParseImages() []ImageObject {
	if len(s.Image) == 0 {
		return nil
	}
	// Try array first
	var arr []ImageObject
	if err := json.Unmarshal(s.Image, &arr); err == nil {
		return arr
	}
	// Try single object
	var single ImageObject
	if err := json.Unmarshal(s.Image, &single); err == nil {
		return []ImageObject{single}
	}
	return nil
}

// Difficulties contains difficulty ratings for a tour/slope
type Difficulties struct {
	Difficulty []DifficultyEntry `json:"difficulty,omitempty"`
}

// DifficultyEntry represents a single difficulty rating
type DifficultyEntry struct {
	ID    string `json:"id,omitempty"`
	Name  string `json:"name,omitempty"`
	Type  string `json:"type,omitempty"`
	Value string `json:"value,omitempty"`
}

// AdditionalProperty represents a key-value property from DiscoverSwiss.
// Value is json.RawMessage because it can be a string or a number.
type AdditionalProperty struct {
	PropertyID string          `json:"propertyId"`
	Value      json.RawMessage `json:"value"`
	ValueType  string          `json:"valueType,omitempty"`
}

// ValueString returns the value as a string, unquoting if necessary.
func (a AdditionalProperty) ValueString() string {
	s := string(a.Value)
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	return s
}

// Tag represents a tag with id, name, and type
type Tag struct {
	ID             string `json:"id,omitempty"`
	Identifier     string `json:"identifier,omitempty"`
	Name           string `json:"name,omitempty"`
	Type           string `json:"type,omitempty"`
	AdditionalType string `json:"additionalType,omitempty"`
}

// Elevation represents altitude/elevation data for a tour/slope
type Elevation struct {
	MaxAltitude  int `json:"maxAltitude,omitempty"`
	MinAltitude  int `json:"minAltitude,omitempty"`
	Ascent       int `json:"ascent,omitempty"`
	Descent      int `json:"descent,omitempty"`
	Differential int `json:"differential,omitempty"`
}

// Rating represents difficulty/quality ratings
type Rating struct {
	Difficulty          int `json:"difficulty,omitempty"`
	Technique           int `json:"technique,omitempty"`
	Condition           int `json:"condition,omitempty"`
	QualityOfExperience int `json:"qualityOfExperience,omitempty"`
	Landscape           int `json:"landscape,omitempty"`
}

// Exposition represents cardinal direction exposure
type Exposition struct {
	NN bool `json:"nn,omitempty"`
	NE bool `json:"ne,omitempty"`
	EE bool `json:"ee,omitempty"`
	SE bool `json:"se,omitempty"`
	SS bool `json:"ss,omitempty"`
	SW bool `json:"sw,omitempty"`
	WW bool `json:"ww,omitempty"`
	NW bool `json:"nw,omitempty"`
}

// OpeningHoursSpec represents opening hours specification
type OpeningHoursSpec struct {
	Name         string `json:"name,omitempty"`
	Opens        string `json:"opens,omitempty"`
	Closes       string `json:"closes,omitempty"`
	DayOfWeek    string `json:"dayOfWeek,omitempty"`
	ValidFrom    string `json:"validFrom,omitempty"`
	ValidThrough string `json:"validThrough,omitempty"`
}

// VideoObject represents a video
type VideoObject struct {
	ContentURL string `json:"contentUrl,omitempty"`
	Name       string `json:"name,omitempty"`
	Caption    string `json:"caption,omitempty"`
}

// Address represents postal address information
type Address struct {
	AddressCountry  string `json:"addressCountry,omitempty"`
	AddressLocality string `json:"addressLocality,omitempty"`
	PostalCode      string `json:"postalCode,omitempty"`
	StreetAddress   string `json:"streetAddress,omitempty"`
	Email           string `json:"email,omitempty"`
	Telephone       string `json:"telephone,omitempty"`
}

// GeoCoordinates represents geographical coordinates
type GeoCoordinates struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation,omitempty"`
}

// ImageObject represents an image with metadata
type ImageObject struct {
	ID              string          `json:"@id"`
	Identifier      string          `json:"identifier"`
	Name            string          `json:"name,omitempty"`
	Type            string          `json:"type"`
	AdditionalType  string          `json:"additionalType,omitempty"`
	ContentURL      string          `json:"contentUrl,omitempty"`
	CopyrightNotice string          `json:"copyrightNotice,omitempty"`
	Caption         string          `json:"caption,omitempty"`
	Width           string          `json:"width,omitempty"`
	Height          string          `json:"height,omitempty"`
	License         string          `json:"license,omitempty"`
	DataGovernance  *DataGovernance `json:"dataGovernance,omitempty"`
}

// Link represents a hyperlink with type
type Link struct {
	Type string `json:"type"`
	URL  string `json:"url"`
}

// Category represents a categorization
type Category struct {
	ID         string `json:"@id"`
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
	Type       string `json:"type"`
}

// AdministrativeArea represents a geographical or administrative area
type AdministrativeArea struct {
	ID             string `json:"@id"`
	Identifier     string `json:"identifier"`
	Name           string `json:"name"`
	Type           string `json:"type"`
	AdditionalType string `json:"additionalType,omitempty"`
}

// DataGovernance represents data provenance and licensing information
type DataGovernance struct {
	Origin   []Origin `json:"origin,omitempty"`
	Provider *Partner `json:"provider,omitempty"`
	Source   *Partner `json:"source,omitempty"`
}

// Origin represents the origin of data
type Origin struct {
	Created      string   `json:"created,omitempty"`
	LastModified string   `json:"lastModified,omitempty"`
	Datasource   string   `json:"datasource,omitempty"`
	License      string   `json:"license,omitempty"`
	SourceID     string   `json:"sourceId,omitempty"`
	Provider     *Partner `json:"provider,omitempty"`
	Source       *Partner `json:"source,omitempty"`
}

// WeatherEntry represents a single weather forecast entry from DiscoverSwiss
type WeatherEntry struct {
	Date        string  `json:"date"`
	Icon        int     `json:"icon"`
	LastUpdate  string  `json:"lastUpdate"`
	Temperature float64 `json:"temperature"`
}

// QuantitativeValue represents a measurement with unit (e.g. snow height in cm)
type QuantitativeValue struct {
	UnitCode string `json:"unitCode"`
	UnitText string `json:"unitText"`
	Value    string `json:"value"`
}

// SnowCondition represents snow conditions from DiscoverSwiss
type SnowCondition struct {
	FreshFallenSnow QuantitativeValue `json:"freshFallenSnow"`
	LastSnowfall    string            `json:"lastSnowfall"`
	MaxSnowHeight   QuantitativeValue `json:"maxSnowHeight"`
}

// Partner represents a data provider or source organization
type Partner struct {
	Acronym    string       `json:"acronym,omitempty"`
	Identifier string       `json:"identifier,omitempty"`
	Name       string       `json:"name,omitempty"`
	Type       string       `json:"type,omitempty"`
	Link       []Link       `json:"link,omitempty"`
	Logo       *ImageObject `json:"logo,omitempty"`
}

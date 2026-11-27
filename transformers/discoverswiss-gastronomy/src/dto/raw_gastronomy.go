// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

import (
	"encoding/json"
	"strings"
)

// Gastronomy is one DiscoverSwiss food establishment in one language.
// The crawler publishes one of these per message.
type Gastronomy struct {
	AtID                      string               `json:"@id"`
	ApiCrawlerLang            string               `json:"__api_crawler_lang"`
	AdditionalType            string               `json:"additionalType"`
	Address                   *Address             `json:"address"`
	AutoTranslatedData        bool                 `json:"autoTranslatedData"`
	AvailableDataLanguage     []string             `json:"availableDataLanguage"`
	Category                  []Category           `json:"category"`
	ContainedInPlace          []AdministrativeArea `json:"containedInPlace"`
	DataGovernance            *DataGovernance      `json:"dataGovernance"`
	Description               string               `json:"description"`
	DisambiguatingDescription string               `json:"disambiguatingDescription"`
	Directions                string               `json:"directions"`
	FaxNumber                 string               `json:"faxNumber"`
	Geo                       *Geo                 `json:"geo"`
	GettingThere              string               `json:"gettingThere"`
	Identifier                string               `json:"identifier"`
	Image                     *Image               `json:"image"`
	LastModified              string               `json:"lastModified"`
	License                   string               `json:"license"`
	Name                      string               `json:"name"`
	OpeningHoursSpecification []OpeningHoursSpec   `json:"openingHoursSpecification"`
	Photo                     []Image              `json:"photo"`
	Removed                   bool                 `json:"removed"`
	Telephone                 string               `json:"telephone"`
	TextTeaser                string               `json:"textTeaser"`
	TitleTeaser               string               `json:"titleTeaser"`
	Type                      string               `json:"type"`
	URL                       string               `json:"url"`
	AdditionalProperty        []Property           `json:"additionalProperty"`
}

type Address struct {
	AddressCountry  string `json:"addressCountry"`
	AddressLocality string `json:"addressLocality"`
	Email           string `json:"email"`
	PostalCode      string `json:"postalCode"`
	StreetAddress   string `json:"streetAddress"`
	Telephone       string `json:"telephone"`
}

type Category struct {
	Identifier string `json:"identifier"`
	Name       string `json:"name"`
}

type AdministrativeArea struct {
	AdditionalType string `json:"additionalType"`
	Identifier     string `json:"identifier"`
	Name           string `json:"name"`
}

type DataGovernance struct {
	Author *Person  `json:"author"`
	Origin []Origin `json:"origin"`
}

type Person struct {
	FamilyName string `json:"familyName"`
}

type Origin struct {
	Created  string `json:"created"`
	SourceID string `json:"sourceId"`
}

type Geo struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Elevation float64 `json:"elevation"`
}

type Image struct {
	Caption         string `json:"caption"`
	ContentURL      string `json:"contentUrl"`
	CopyrightNotice string `json:"copyrightNotice"`
	Name            string `json:"name"`
}

type OpeningHoursSpec struct {
	DayOfWeek    string `json:"dayOfWeek"`
	Opens        string `json:"opens"`
	Closes       string `json:"closes"`
	ValidFrom    string `json:"validFrom"`
	ValidThrough string `json:"validThrough"`
}

type Property struct {
	PropertyID string     `json:"propertyId"`
	Value      FlexString `json:"value"`
}

// FlexString accepts a JSON string, number, or boolean. DiscoverSwiss uses all three.
type FlexString string

func (s *FlexString) UnmarshalJSON(data []byte) error {
	if string(data) == "null" {
		return nil
	}
	if len(data) > 0 && data[0] == '"' {
		var value string
		if err := json.Unmarshal(data, &value); err != nil {
			return err
		}
		*s = FlexString(value)
		return nil
	}
	*s = FlexString(strings.TrimSpace(string(data)))
	return nil
}

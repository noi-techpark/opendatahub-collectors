// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import (
	"fmt"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

type FlexibleTime struct {
	time.Time
}

func PtrFlexibleTime(t time.Time) *FlexibleTime {
	ft := FlexibleTime{Time: t}
	return &ft
}

func (ft *FlexibleTime) UnmarshalJSON(b []byte) error {
	s := strings.Trim(string(b), "\"")
	if s == "null" || s == "" || s == "0001-01-01T00:00:00" {
		ft.Time = time.Time{}
		return nil
	}

	// 1. Try RFC3339 (Standard with 'Z' or offset)
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		ft.Time = t
		return nil
	}

	// 2. Try RFC3339Nano (High precision with 'Z' or offset)
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		ft.Time = t
		return nil
	}

	// 3. Fallback for formats without 'Z' (Common in some ODH responses)
	// We use a custom layout that handles up to 9 fractional digits
	layouts := []string{
		"2006-01-02T15:04:05.999999999",
		"2006-01-02T15:04:05",
	}

	for _, layout := range layouts {
		t, err = time.Parse(layout, s)
		if err == nil {
			ft.Time = t
			return nil
		}
	}

	return fmt.Errorf("could not parse time %s: %w", s, err)
}

// DetailGeneric extends clib.DetailGeneric with additional fields the ODH API supports
// but which are not yet in the SDK struct.
type DetailGeneric struct {
	clib.DetailGeneric
	Header    *string `json:"Header"`
	SubHeader *string `json:"SubHeader"`
	IntroText *string `json:"IntroText"`
}

type ODHActivityPoi struct {
	Generic

	Detail               map[string]*DetailGeneric     `json:"Detail"`
	ContactInfos         map[string]*ContactInfo       `json:"ContactInfos"`
	AdditionalContact    []AdditionalContact           `json:"AdditionalContact"`
	ImageGallery         []ImageGalleryEntry           `json:"ImageGallery"`
	PoiProperty          map[string][]PoiPropertyEntry `json:"PoiProperty"`
	PoiServices          []string                      `json:"PoiServices"`
	AdditionalProperties *AdditionalProperties         `json:"AdditionalProperties,omitempty"`

	SmgActive           bool     `json:"SmgActive"`
	PublishedOn         []string `json:"PublishedOn"`
	SyncUpdateMode      string   `json:"SyncUpdateMode,omitempty"`
	SyncSourceInterface string   `json:"SyncSourceInterface,omitempty"`
	HasFreeEntrance     bool     `json:"HasFreeEntrance"`
}

// Generic matches the pattern used across transformers in the monorepo.
type Generic struct {
	ID          *string                      `json:"Id,omitempty"`
	Meta        *clib.Metadata               `json:"_Meta,omitempty"`
	LicenseInfo *LicenseInfo                 `json:"LicenseInfo,omitempty"`
	Shortname   *string                      `json:"Shortname,omitempty"`
	Active      bool                         `json:"Active"`
	FirstImport *FlexibleTime                `json:"FirstImport,omitempty"`
	LastChange  *FlexibleTime                `json:"LastChange,omitempty"`
	HasLanguage []string                     `json:"HasLanguage"`
	Mapping     map[string]map[string]string `json:"Mapping,omitempty"`
	Source      *string                      `json:"Source,omitempty"`
	TagIds      []string                     `json:"TagIds"`
	GpsInfo     []GpsData                    `json:"GpsInfo"`
	SmgTags     []string                     `json:"SmgTags"` // legacy field — must be filled for now
}

type GpsData struct {
	Gpstype   *string `json:"Gpstype"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
}

type LicenseInfo struct {
	Author        string `json:"Author"`
	License       string `json:"License"`
	ClosedData    bool   `json:"ClosedData"`
	LicenseHolder string `json:"LicenseHolder"`
}

type ContactInfo struct {
	Language    string `json:"Language"`
	Email       string `json:"Email,omitempty"`
	Phonenumber string `json:"Phonenumber,omitempty"`
	Url         string `json:"Url,omitempty"`
	Address     string `json:"Address,omitempty"`
	City        string `json:"City,omitempty"`
	ZipCode     string `json:"ZipCode,omitempty"`
	CountryCode string `json:"CountryCode,omitempty"`
	CountryName string `json:"CountryName,omitempty"`
	CompanyName string `json:"CompanyName,omitempty"`
	LogoUrl     string `json:"LogoUrl,omitempty"`
	Area        string `json:"Area,omitempty"`
}

// AdditionalContact holds importer contact data per language.
type AdditionalContact struct {
	Type        string       `json:"Type"`
	Description string       `json:"Description,omitempty"`
	ContactInfo *ContactInfo `json:"ContactInfos,omitempty"`
}

// ImageGalleryEntry — multilingual image metadata.
type ImageGalleryEntry struct {
	ImageUrl      string            `json:"ImageUrl"`
	ImageName     string            `json:"ImageName,omitempty"`
	ImageDesc     map[string]string `json:"ImageDesc"`
	ImageTitle    map[string]string `json:"ImageTitle"`
	ImageAltText  map[string]string `json:"ImageAltText"`
	CopyRight     string            `json:"CopyRight,omitempty"`
	License       string            `json:"License,omitempty"`
	ImageSource   string            `json:"ImageSource,omitempty"`
	ImageLicence  string            `json:"ImageLicence,omitempty"`
	LicenseHolder string            `json:"LicenseHolder,omitempty"`
	IsInGallery   bool              `json:"IsInGallery"`
	ListPosition  int               `json:"ListPosition"`
	Width         int               `json:"Width,omitempty"`
	Height        int               `json:"Height,omitempty"`
}

// PoiPropertyEntry is a single key-value entry in PoiProperty.
type PoiPropertyEntry struct {
	Name  string `json:"Name"`
	Value string `json:"Value"`
}

type AdditionalProperties struct {
	SiagMuseumDataProperties           *SiagMuseumDataProperties           `json:"SiagMuseumDataProperties,omitempty"`
	SuedtirolWeinCompanyDataProperties *SuedtirolWeinCompanyDataProperties `json:"SuedtirolWeinCompanyDataProperties,omitempty"`
}

type SiagMuseumDataProperties struct {
	Entry        map[string]string `json:"Entry,omitempty"`
	OpeningTimes map[string]string `json:"OpeningTimes,omitempty"`
	Supporter    map[string]string `json:"Supporter,omitempty"`
}

// SuedtirolWeinCompanyDataProperties holds structured wine company data
// with multilingual maps for text fields and booleans for flags.
type SuedtirolWeinCompanyDataProperties struct {
	H1          map[string]string `json:"H1,omitempty"`
	H2          map[string]string `json:"H2,omitempty"`
	Quote       map[string]string `json:"Quote,omitempty"`
	QuoteAuthor map[string]string `json:"QuoteAuthor,omitempty"`

	OpeningTimesWineShop    map[string]string `json:"OpeningtimesWineshop,omitempty"`
	OpeningTimesGuides      map[string]string `json:"OpeningtimesGuides,omitempty"`
	OpeningTimesGastronomie map[string]string `json:"OpeningtimesGastronomie,omitempty"`
	CompanyHoliday          map[string]string `json:"CompanyHoliday,omitempty"`

	Wines []string `json:"Wines,omitempty"`

	HasVisits                  bool  `json:"HasVisits"`
	HasOvernights              bool  `json:"HasOvernights"`
	HasBiowine                 bool  `json:"HasBiowine"`
	HasAccommodation           *bool `json:"HasAccommodation"` // nullable — source field hasaccomodation may be absent
	HasOnlineshop              bool  `json:"HasOnlineshop"`
	HasDeliveryservice         bool  `json:"HasDeliveryservice"`
	HasDirectSales             bool  `json:"HasDirectSales"`
	IsVinumHotel               bool  `json:"IsVinumHotel"`
	IsAnteprima                bool  `json:"IsAnteprima"`
	IsWineStories              bool  `json:"IsWineStories"`
	IsWineSummit               bool  `json:"IsWineSummit"`
	IsSparklingWineassociation bool  `json:"IsSparklingWineassociation"`
	IsWinery                   bool  `json:"IsWinery"`
	IsWineryAssociation        bool  `json:"IsWineryAssociation"`
	IsSkyalpsPartner           bool  `json:"IsSkyalpsPartner"`

	OnlineShopurl      *string `json:"OnlineShopurl"`
	DeliveryServiceUrl *string `json:"DeliveryServiceUrl"`

	SocialsInstagram *string `json:"SocialsInstagram"`
	SocialsFacebook  *string `json:"SocialsFacebook"`
	SocialsLinkedIn  *string `json:"SocialsLinkedIn"`
	SocialsPinterest *string `json:"SocialsPinterest"`
	SocialsTiktok    *string `json:"SocialsTiktok"`
	SocialsYoutube   *string `json:"SocialsYoutube"`
	SocialsTwitter   *string `json:"SocialsTwitter"`

	H1SparklingWineproducer          *string `json:"H1SparklingWineproducer"`
	H2SparklingWineproducer          *string `json:"H2SparklingWineproducer"`
	ImageSparklingWineproducer       *string `json:"ImageSparklingWineproducer"`
	DescriptionSparklingWineproducer *string `json:"DescriptionSparklingWineproducer"`
}

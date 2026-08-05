// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import (
	"encoding/json"
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

// MarshalJSON ensures time is formatted safely for .NET/Elasticsearch APIs.
// Truncating to milliseconds prevents 400 Bad Request crashes on nanosecond precision.
func (ft FlexibleTime) MarshalJSON() ([]byte, error) {
	if ft.Time.IsZero() {
		return []byte(`null`), nil
	}
	return []byte(`"` + ft.Time.Truncate(time.Millisecond).Format(time.RFC3339) + `"`), nil
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

// FlexibleMap handles fields that can be either a plain string or a multilingual map.
type FlexibleMap map[string]string

func (fm *FlexibleMap) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}

	var m map[string]string
	if err := json.Unmarshal(b, &m); err == nil {
		*fm = m
		return nil
	}

	var s string
	if err := json.Unmarshal(b, &s); err == nil {
		if s == "" {
			return nil
		}
		*fm = map[string]string{"de": s}
		return nil
	}

	return fmt.Errorf("could not unmarshal flexible map from %s", string(b))
}

type DetailGeneric struct {
	clib.DetailGeneric
	// Added omitempty to prevent "Header": null which fails validation
	Header    *string `json:"Header,omitempty"`
	SubHeader *string `json:"SubHeader,omitempty"`
	IntroText *string `json:"IntroText,omitempty"`
}

type ODHActivityPoi struct {
	Generic

	Detail               map[string]*DetailGeneric      `json:"Detail"`
	ContactInfos         map[string]*ContactInfo        `json:"ContactInfos"`
	AdditionalContact    map[string][]AdditionalContact `json:"AdditionalContact,omitempty"`
	ImageGallery         []ImageGalleryEntry            `json:"ImageGallery"`
	PoiProperty          map[string][]PoiPropertyEntry  `json:"PoiProperty"`
	PoiServices          []string                       `json:"PoiServices"`
	AdditionalProperties *AdditionalProperties          `json:"AdditionalProperties,omitempty"`

	SmgActive           bool     `json:"SmgActive"`
	OdhActive           bool     `json:"OdhActive"`
	PublishedOn         []string `json:"PublishedOn"`
	SyncUpdateMode      string   `json:"SyncUpdateMode,omitempty"`
	SyncSourceInterface string   `json:"SyncSourceInterface,omitempty"`
	HasFreeEntrance     bool     `json:"HasFreeEntrance"`
}

type Metadata struct {
	ID         string        `json:"Id"`
	Type       string        `json:"Type"`
	LastUpdate *FlexibleTime `json:"LastUpdate,omitempty"`
	Source     string        `json:"Source"`
	Reduced    bool          `json:"Reduced"`
}

type Generic struct {
	ID          *string                      `json:"Id,omitempty"`
	Meta        *Metadata                    `json:"_Meta,omitempty"`
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
	SmgTags     []string                     `json:"SmgTags"`
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

type AdditionalContact struct {
	Type        string       `json:"Type"`
	Description string       `json:"Description,omitempty"`
	ContactInfo *ContactInfo `json:"ContactInfos,omitempty"`
}

type ImageGalleryEntry struct {
	ImageUrl      string            `json:"ImageUrl"`
	ImageName     string            `json:"ImageName,omitempty"`
	ImageDesc     map[string]string `json:"ImageDesc,omitempty"`
	ImageTitle    map[string]string `json:"ImageTitle,omitempty"`
	ImageAltText  map[string]string `json:"ImageAltText,omitempty"`
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

type SuedtirolWeinCompanyDataProperties struct {
	H1          FlexibleMap `json:"H1,omitempty"`
	H2          FlexibleMap `json:"H2,omitempty"`
	Quote       FlexibleMap `json:"Quote,omitempty"`
	QuoteAuthor FlexibleMap `json:"QuoteAuthor,omitempty"`

	Slogan   FlexibleMap `json:"Slogan,omitempty"`
	FarmName FlexibleMap `json:"FarmName,omitempty"`

	OpeningTimesWineShop    FlexibleMap `json:"OpeningtimesWineshop,omitempty"`
	OpeningTimesGuides      FlexibleMap `json:"OpeningtimesGuides,omitempty"`
	OpeningTimesGastronomie FlexibleMap `json:"OpeningtimesGastronomie,omitempty"`
	CompanyHoliday          FlexibleMap `json:"CompanyHoliday,omitempty"`

	Wines []string `json:"Wines,omitempty"`

	HasVisits                  bool  `json:"HasVisits"`
	HasOvernights              bool  `json:"HasOvernights"`
	HasBiowine                 bool  `json:"HasBiowine"`
	HasAccommodation           *bool `json:"HasAccommodation,omitempty"`
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

	OnlineShopurl      FlexibleMap `json:"OnlineShopurl,omitempty"`
	DeliveryServiceUrl FlexibleMap `json:"DeliveryServiceUrl,omitempty"`

	SocialsInstagram string `json:"SocialsInstagram,omitempty"`
	SocialsFacebook  string `json:"SocialsFacebook,omitempty"`
	SocialsLinkedIn  string `json:"SocialsLinkedIn,omitempty"`
	SocialsPinterest string `json:"SocialsPinterest,omitempty"`
	SocialsTiktok    string `json:"SocialsTiktok,omitempty"`
	SocialsYoutube   string `json:"SocialsYoutube,omitempty"`
	SocialsTwitter   string `json:"SocialsTwitter,omitempty"`

	H1SparklingWineproducer          FlexibleMap `json:"H1SparklingWineproducer,omitempty"`
	H2SparklingWineproducer          FlexibleMap `json:"H2SparklingWineproducer,omitempty"`
	ImageSparklingWineproducer       FlexibleMap `json:"ImageSparklingWineproducer,omitempty"`
	DescriptionSparklingWineproducer FlexibleMap `json:"DescriptionSparklingWineproducer,omitempty"`
}

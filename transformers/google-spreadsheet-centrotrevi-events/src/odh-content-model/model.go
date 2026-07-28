// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package odhmodel

type EventLinked struct {
	Active               bool                         `json:"Active"`
	ContactInfos         map[string]ContactInfos      `json:"ContactInfos,omitempty"`
	DateBegin            string                       `json:"DateBegin,omitempty"`
	DateEnd              string                       `json:"DateEnd,omitempty"`
	Detail               map[string]Detail            `json:"Detail,omitempty"`
	EventAdditionalInfos map[string]map[string]string `json:"EventAdditionalInfos,omitempty"`
	EventBooking         map[string]any               `json:"EventBooking,omitempty"`
	EventDate            []EventDate                  `json:"EventDate,omitempty"`
	EventProperty        map[string]any               `json:"EventProperty,omitempty"`
	FirstImport          string                       `json:"FirstImport,omitempty"`
	GpsInfo              []GpsInfo                    `json:"GpsInfo,omitempty"`
	Id                   string                       `json:"Id"`
	ImageGallery         []ImageGalleryItem           `json:"ImageGallery,omitempty"`
	LicenseInfo          map[string]any               `json:"LicenseInfo,omitempty"`
	LocationInfo         map[string]any               `json:"LocationInfo,omitempty"`
	OdhActive            bool                         `json:"OdhActive,omitempty"`
	OrganizerInfos       map[string]ContactInfos      `json:"OrganizerInfos,omitempty"`
	OrgRID               string                       `json:"OrgRID,omitempty"`
	Shortname            string                       `json:"Shortname,omitempty"`
	Source               string                       `json:"Source,omitempty"`
	TagIds               []string                     `json:"TagIds,omitempty"`
	Topics               []map[string]any             `json:"Topics,omitempty"`
	VenueIds             []string                     `json:"VenueIds,omitempty"`
	Mapping              map[string]map[string]string `json:"Mapping,omitempty"`
}

type Detail struct {
	BaseText  string `json:"BaseText,omitempty"`
	Language  string `json:"Language,omitempty"`
	Title     string `json:"Title,omitempty"`
	SubHeader string `json:"SubHeader,omitempty"`
}

type EventDate struct {
	Active bool   `json:"Active"`
	Begin  string `json:"Begin,omitempty"`
	End    string `json:"End,omitempty"`
	From   string `json:"From,omitempty"`
	To     string `json:"To,omitempty"`
}

type ContactInfos struct {
	Address     string `json:"Address,omitempty"`
	City        string `json:"City,omitempty"`
	CompanyName string `json:"CompanyName,omitempty"`
	CountryCode string `json:"CountryCode,omitempty"`
	CountryName string `json:"CountryName,omitempty"`
	Email       string `json:"Email,omitempty"`
	Language    string `json:"Language,omitempty"`
	Phonenumber string `json:"Phonenumber,omitempty"`
	Url         string `json:"Url,omitempty"`
	ZipCode     string `json:"ZipCode,omitempty"`
}

type GpsInfo struct {
	Gpstype   string  `json:"Gpstype"`
	Latitude  float64 `json:"Latitude"`
	Longitude float64 `json:"Longitude"`
}

type ImageGalleryItem struct {
	ImageName   string            `json:"ImageName"`
	ImageUrl    string            `json:"ImageUrl"`
	Width       int               `json:"Width"`
	Height      int               `json:"Height"`
	ImageSource string            `json:"ImageSource"`
	ImageTitle  map[string]string `json:"ImageTitle,omitempty"`
}

type VenueV2 struct {
	Id              string                       `json:"Id"`
	Active          *bool                        `json:"Active,omitempty"`
	Shortname       string                       `json:"Shortname"`
	Detail          map[string]any               `json:"Detail,omitempty"`
	Mapping         map[string]map[string]string `json:"Mapping,omitempty"`
	MaxCapacity     *int                         `json:"MaxCapacity,omitempty"`
	RoomDetails     []VenueRoomDetailsV2         `json:"RoomDetails,omitempty"`
	ContactInfos    map[string]any               `json:"ContactInfos,omitempty"`
	LocationInfo    map[string]any               `json:"LocationInfo,omitempty"`
}

type VenueRoomDetailsV2 struct {
	Id                  string                       `json:"Id"`
	Shortname           string                       `json:"Shortname"`
	Detail              map[string]DetailGeneric     `json:"Detail"`
	Active              bool                         `json:"Active"`
	VenueRoomProperties *VenueRoomProperties         `json:"VenueRoomProperties,omitempty"`
	MaxCapacity         *int                         `json:"MaxCapacity,omitempty"`
	Mapping             map[string]map[string]string `json:"Mapping,omitempty"`
}

type DetailGeneric struct {
	Title    string `json:"Title"`
	Language string `json:"Language"`
}

type VenueRoomProperties struct {
	SquareMeters *float64 `json:"SquareMeters,omitempty"`
}

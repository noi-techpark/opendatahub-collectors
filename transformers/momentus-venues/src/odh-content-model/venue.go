// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package odhcontentmodel

type VenueV2 struct {
	Id                   string                       `json:"Id"`
	Active               *bool                        `json:"Active"`
	Shortname            string                       `json:"Shortname"`
	Detail               map[string]any               `json:"Detail"`
	Tags                 []any                        `json:"Tags"`
	TagIds               []string                     `json:"TagIds"`
	Mapping              map[string]map[string]string `json:"Mapping"`
	Placement            any                          `json:"Placement"`
	MaxCapacity          *int                         `json:"MaxCapacity"`
	ImageGallery         []any                        `json:"ImageGallery"`
	VenueRoomProperties  any                          `json:"VenueRoomProperties"`
	RoomDetails          []VenueRoomDetailsV2         `json:"RoomDetails"`
	ContactInfos         map[string]any               `json:"ContactInfos"`
	DistanceInfo         any                          `json:"DistanceInfo"`
	LocationInfo         map[string]any               `json:"LocationInfo"`
	RelatedContent       []any                        `json:"RelatedContent"`
	OperationSchedule    any                          `json:"OperationSchedule"`
	AdditionalProperties any                          `json:"AdditionalProperties"`
}

type VenueRoomDetailsV2 struct {
	Id                  string                           `json:"Id"`
	Shortname           string                           `json:"Shortname"`
	Detail              map[string]DetailGeneric         `json:"Detail"`
	Active              bool                             `json:"Active"`
	VenueRoomProperties *VenueRoomProperties             `json:"VenueRoomProperties"`
	MaxCapacity         *int                             `json:"MaxCapacity"`
	Mapping             map[string]map[string]string     `json:"Mapping"`
}

type DetailGeneric struct {
	Title    string `json:"Title"`
	Language string `json:"Language"`
}

type VenueRoomProperties struct {
	SquareMeters *float64 `json:"SquareMeters,omitempty"`
}

// Momentus models we receive from the Crawler
type MomentusRoom struct {
	Id                 string   `json:"id"`
	Name               string   `json:"name"`
	Group              string   `json:"group"`
	IsActive           bool     `json:"isActive"`
	SquareFootage      *float64 `json:"squareFootage"`
	MaxCapacity        *int     `json:"maxCapacity"`
	IsComboRoom        bool     `json:"isComboRoom"`
	SubRoomIds         []string `json:"subRoomIds"`
	ConflictingRoomIds []string `json:"conflictingRoomIds"`
	ItemCode           string   `json:"itemCode"`
}

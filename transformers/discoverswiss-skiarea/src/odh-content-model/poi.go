// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

// ODHActivityPoiType represents a type classification for the POI
type ODHActivityPoiType struct {
	ID   *string `json:"Id,omitempty"`
	Type *string `json:"Type,omitempty"`
	Key  *string `json:"Key,omitempty"`
}

// Tag represents a tag in ODH
type Tag struct {
	ID     *string `json:"Id,omitempty"`
	Source *string `json:"Source,omitempty"`
	Type   *string `json:"Type,omitempty"`
	Name   *string `json:"Name,omitempty"`
}

// Ratings represents the rating fields of an activity/POI
type Ratings struct {
	Difficulty *string `json:"Difficulty,omitempty"`
	Technique  *string `json:"Technique,omitempty"`
	Stamina    *string `json:"Stamina,omitempty"`
	Experience *string `json:"Experience,omitempty"`
	Landscape  *string `json:"Landscape,omitempty"`
}

// Exposition represents cardinal direction exposure as array of strings
// e.g. ["N","S","E","W","NE","SE","SW","NW"]
type Exposition []string

// ODHActivityPoi corresponds to the OpenDataHub ODHActivityPoi model.
// This is the target format for ski lifts, slopes, snow parks, and tobogganing runs.
type ODHActivityPoi struct {
	Generic // Embedded struct for inlining fields

	Type    *string `json:"Type,omitempty"`
	SubType *string `json:"SubType,omitempty"`
	PoiType *string `json:"PoiType,omitempty"`

	Detail       map[string]Detail       `json:"Detail,omitempty"`
	ContactInfos map[string]ContactInfos `json:"ContactInfos,omitempty"`
	ImageGallery []ImageGallery          `json:"ImageGallery,omitempty"`
	SmgTags      []string                `json:"SmgTags,omitempty" hash:"set"`
	LocationInfo *LocationInfo           `json:"LocationInfo,omitempty"`

	// GPS and track data
	GpsInfo  []GpsInfo  `json:"GpsInfo,omitempty"`
	GpsTrack []GpsTrack `json:"GpsTrack,omitempty"`

	// Altitude and distance
	AltitudeHighestPoint *float64 `json:"AltitudeHighestPoint,omitempty"`
	AltitudeLowestPoint  *float64 `json:"AltitudeLowestPoint,omitempty"`
	AltitudeSumUp        *float64 `json:"AltitudeSumUp,omitempty"`
	AltitudeSumDown      *float64 `json:"AltitudeSumDown,omitempty"`
	AltitudeDifference   *float64 `json:"AltitudeDifference,omitempty"`
	DistanceLength       *float64 `json:"DistanceLength,omitempty"`
	DistanceDuration     *float64 `json:"DistanceDuration,omitempty"`

	// Ratings
	Ratings          *Ratings   `json:"Ratings,omitempty"`
	Difficulty       *string    `json:"Difficulty,omitempty"`
	ExpositionValues Exposition `json:"Exposition,omitempty"`

	// Status
	IsOpen          *bool `json:"IsOpen,omitempty"`
	IsPrepared      *bool `json:"IsPrepared,omitempty"`
	IsWithLigth     *bool `json:"IsWithLigth,omitempty"`
	HasFreeEntrance *bool `json:"HasFreeEntrance,omitempty"`
	HasRentals      *bool `json:"HasRentals,omitempty"`
	LiftAvailable   *bool `json:"LiftAvailable,omitempty"`
	FeetClimb       *bool `json:"FeetClimb,omitempty"`
	RunToValley     *bool `json:"RunToValley,omitempty"`
	BikeTransport   *bool `json:"BikeTransport,omitempty"`
	Highlight       *bool `json:"Highlight,omitempty"`

	// Capacity
	MaxSeatingCapacity *int `json:"MaxSeatingCapacity,omitempty"`

	// Tags and categories
	Tags                []Tag                `json:"Tags,omitempty"`
	ODHActivityPoiTypes []ODHActivityPoiType `json:"ODHActivityPoiTypes,omitempty"`

	// Operation schedule
	OperationSchedule []OperationSchedule `json:"OperationSchedule,omitempty"`

	// Custom / external IDs
	CustomId *string `json:"CustomId,omitempty"`

	// Area references (AreaId is an array of strings in ODH API)
	AreaId []string `json:"AreaId,omitempty"`
}

// GpsTrack represents a GPS track reference
type GpsTrack struct {
	GpxTrackUrl  *string           `json:"GpxTrackUrl,omitempty"`
	GpxTrackDesc map[string]string `json:"GpxTrackDesc,omitempty"`
	Type         *string           `json:"Type,omitempty"`
}

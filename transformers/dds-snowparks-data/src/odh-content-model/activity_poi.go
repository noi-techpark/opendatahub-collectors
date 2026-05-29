// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package odhmodel

import (
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

// ── Time helper ───────────────────────────────────────────────────────────────

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
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		ft.Time = t
		return nil
	}
	t, err = time.Parse("2006-01-02T15:04:05", s)
	if err == nil {
		ft.Time = t
		return nil
	}
	return err
}

// ── ODHActivityPoi ────────────────────────────────────────────────────────────

type ODHActivityPoi struct {
	Generic

	// Multilingual content
	Detail               map[string]*clib.DetailGeneric `json:"Detail,omitempty"`
	ContactInfos         map[string]interface{}         `json:"ContactInfos"` // always emit {}
	AdditionalPoiInfos   map[string]*AdditionalPoiInfo  `json:"AdditionalPoiInfos,omitempty"`
	AdditionalProperties map[string]interface{}         `json:"AdditionalProperties"` // always emit {}
	PoiProperty          map[string]interface{}         `json:"PoiProperty"`          // always emit {}
	ImageGallery         []ImageGalleryEntry            `json:"ImageGallery,omitempty"`

	// LocationInfo: no omitempty on the field itself so &LocationInfo{} emits
	// {"TvInfo":null,...} not {} — matches old API shape.
	LocationInfo *LocationInfo `json:"LocationInfo,omitempty"`

	// Sync metadata
	SmgActive           bool     `json:"SmgActive"`
	OdhActive           bool     `json:"OdhActive"`
	PublishedOn         []string `json:"PublishedOn"`
	SyncUpdateMode      string   `json:"SyncUpdateMode,omitempty"`
	SyncSourceInterface string   `json:"SyncSourceInterface,omitempty"`

	// Classification
	Type     *string `json:"Type,omitempty"`
	SubType  *string `json:"SubType,omitempty"`
	CustomId string  `json:"CustomId,omitempty"`

	// Numeric fields
	DistanceLength       *float64 `json:"DistanceLength,omitempty"`
	DistanceDuration     *float64 `json:"DistanceDuration,omitempty"`
	AltitudeLowestPoint  *int     `json:"AltitudeLowestPoint,omitempty"`
	AltitudeHighestPoint *int     `json:"AltitudeHighestPoint,omitempty"`
	AltitudeDifference   *int     `json:"AltitudeDifference,omitempty"`
	AltitudeSumUp        *int     `json:"AltitudeSumUp,omitempty"`
	AltitudeSumDown      *int     `json:"AltitudeSumDown,omitempty"`

	// FIX: *bool — nil serializes as null (old API has null for slopes, not false)
	BikeTransport *bool  `json:"BikeTransport"`
	Number        string `json:"Number,omitempty"`

	// Nullable flags — old API has these as null, not set by DSS importer
	WayNumber        *string  `json:"WayNumber,omitempty"`
	Difficulty       *string  `json:"Difficulty,omitempty"`
	Ratings          *Ratings `json:"Ratings,omitempty"`
	Exposition       *string  `json:"Exposition,omitempty"`
	IsOpen           bool     `json:"IsOpen"`
	IsPrepared       *bool    `json:"IsPrepared,omitempty"`
	IsWithLigth      *bool    `json:"IsWithLigth,omitempty"` // ODH typo — preserved
	HasRentals       *bool    `json:"HasRentals,omitempty"`
	RunToValley      *bool    `json:"RunToValley,omitempty"`
	FeetClimb        *bool    `json:"FeetClimb,omitempty"`
	LiftAvailable    *bool    `json:"LiftAvailable,omitempty"`
	Highlight        *bool    `json:"Highlight,omitempty"`
	CopyrightChecked *bool    `json:"CopyrightChecked,omitempty"`
	HasFreeEntrance  *bool    `json:"HasFreeEntrance,omitempty"`

	// GPS
	GpsTrack  []GpsTrack          `json:"GpsTrack,omitempty"`
	GpsPoints map[string]*GpsInfo `json:"GpsPoints,omitempty"`

	// Schedule
	OperationSchedule []OperationSchedule `json:"OperationSchedule,omitempty"`
}

// ── Generic ───────────────────────────────────────────────────────────────────

type Generic struct {
	ID          *string                      `json:"Id,omitempty"`
	Shortname   *string                      `json:"Shortname,omitempty"`
	Active      bool                         `json:"Active"`
	Source      *string                      `json:"Source,omitempty"`
	FirstImport *FlexibleTime                `json:"FirstImport,omitempty"`
	LastChange  *FlexibleTime                `json:"LastChange,omitempty"`
	HasLanguage []string                     `json:"HasLanguage,omitempty"`
	Mapping     map[string]map[string]string `json:"Mapping,omitempty"`
	TagIds      []string                     `json:"TagIds,omitempty"`
	SmgTags     []string                     `json:"SmgTags,omitempty"`
	GpsInfo     []GpsInfo                    `json:"GpsInfo,omitempty"`
	LicenseInfo *LicenseInfo                 `json:"LicenseInfo,omitempty"`
}

// ── Supporting types ──────────────────────────────────────────────────────────

type LicenseInfo struct {
	Author        string `json:"Author"`
	License       string `json:"License"`
	ClosedData    bool   `json:"ClosedData"`
	LicenseHolder string `json:"LicenseHolder"`
}

// GpsInfo matches the full ODH shape.
type GpsInfo struct {
	Default               *bool    `json:"Default,omitempty"`
	Gpstype               string   `json:"Gpstype"`
	Altitude              *float64 `json:"Altitude,omitempty"`
	Geometry              *string  `json:"Geometry,omitempty"`
	Latitude              float64  `json:"Latitude"`
	Longitude             float64  `json:"Longitude"`
	AltitudeUnitofMeasure string   `json:"AltitudeUnitofMeasure,omitempty"`
}

// GpsTrack matches old API — a slice of track objects.
type GpsTrack struct {
	Id           *string                `json:"Id"`
	Type         string                 `json:"Type"`
	Format       string                 `json:"Format"`
	GpxTrackUrl  string                 `json:"GpxTrackUrl"`
	GpxTrackDesc map[string]interface{} `json:"GpxTrackDesc"`
}

// OperationSchedule — OperationscheduleName has lowercase 's' matching old API exactly.
type OperationSchedule struct {
	Stop                  string                  `json:"Stop,omitempty"`
	Type                  string                  `json:"Type,omitempty"`
	Start                 string                  `json:"Start,omitempty"`
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime,omitempty"`
	OperationscheduleName map[string]string       `json:"OperationscheduleName"` // lowercase 's' — ODH API shape
}

// OperationScheduleTime includes all fields from old API including ODH typos.
type OperationScheduleTime struct {
	End       string `json:"End"`
	Start     string `json:"Start"`
	State     int    `json:"State"`
	Friday    bool   `json:"Friday"`
	Monday    bool   `json:"Monday"`
	Sunday    bool   `json:"Sunday"`
	Tuesday   bool   `json:"Tuesday"`
	Saturday  bool   `json:"Saturday"`
	Thursday  bool   `json:"Thursday"`
	Timecode  int    `json:"Timecode"`
	Thuresday bool   `json:"Thuresday"` // ODH typo — preserved
	Wednesday bool   `json:"Wednesday"`
}

// AdditionalPoiInfo holds per-language category classification.
type AdditionalPoiInfo struct {
	Novelty    string   `json:"Novelty"`
	PoiType    *string  `json:"PoiType"`
	SubType    *string  `json:"SubType"`
	Language   string   `json:"Language"`
	MainType   *string  `json:"MainType"`
	Categories []string `json:"Categories"`
}

type ImageGalleryEntry struct {
	ImageUrl    string `json:"ImageUrl"`
	ImageName   string `json:"ImageName,omitempty"`
	IsInGallery bool   `json:"IsInGallery"`
}

// Ratings holds difficulty and other numeric ratings.
// C# parser sets Ratings.Difficulty = parseddifficulty alongside the Difficulty field.
type Ratings struct {
	Stamina    *string `json:"Stamina,omitempty"`
	Landscape  *string `json:"Landscape,omitempty"`
	Technique  *string `json:"Technique,omitempty"`
	Difficulty *string `json:"Difficulty,omitempty"`
	Experience *string `json:"Experience,omitempty"`
}

// LocationInfo is set by the ODH pipeline from GpsInfo — transformer emits it empty.
// Sub-fields have NO omitempty so &LocationInfo{} produces:
// {"TvInfo":null,"AreaInfo":null,...} not {} — matches old API.
type LocationInfo struct {
	TvInfo           *LocationRef `json:"TvInfo"`
	AreaInfo         *LocationRef `json:"AreaInfo"`
	RegionInfo       *LocationRef `json:"RegionInfo"`
	DistrictInfo     *LocationRef `json:"DistrictInfo"`
	MunicipalityInfo *LocationRef `json:"MunicipalityInfo"`
}

type LocationRef struct {
	Id   string            `json:"Id,omitempty"`
	Name map[string]string `json:"Name,omitempty"`
	Self string            `json:"Self,omitempty"`
}

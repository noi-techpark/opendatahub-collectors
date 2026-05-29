// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package odhmodel

import (
	"encoding/json"
	"fmt"
	"strconv"
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
	ContactInfos         map[string]interface{}         `json:"ContactInfos"` // always emit {} even if empty
	AdditionalPoiInfos   map[string]*AdditionalPoiInfo  `json:"AdditionalPoiInfos,omitempty"`
	AdditionalProperties map[string]interface{}         `json:"AdditionalProperties"` // always emit {}
	PoiProperty          map[string]interface{}         `json:"PoiProperty"`          // always emit {}
	ImageGallery         []ImageGalleryEntry            `json:"ImageGallery,omitempty"`
	LocationInfo         *LocationInfo                  `json:"LocationInfo,omitempty"`

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

	// Lift-specific numeric fields
	DistanceLength       *float64 `json:"DistanceLength,omitempty"`
	DistanceDuration     *float64 `json:"DistanceDuration,omitempty"`
	AltitudeLowestPoint  *float64 `json:"AltitudeLowestPoint,omitempty"`
	AltitudeHighestPoint *float64 `json:"AltitudeHighestPoint,omitempty"`
	AltitudeDifference   *float64 `json:"AltitudeDifference,omitempty"`
	AltitudeSumUp        *float64 `json:"AltitudeSumUp,omitempty"`
	AltitudeSumDown      *float64 `json:"AltitudeSumDown,omitempty"`
	BikeTransport        *bool    `json:"BikeTransport"`
	Number               string   `json:"Number,omitempty"`

	// Nullable flags — left nil, not set by DSS importer
	WayNumber        *string `json:"WayNumber,omitempty"`
	Difficulty       *string `json:"Difficulty,omitempty"`
	Exposition       *string `json:"Exposition,omitempty"`
	IsOpen           bool    `json:"IsOpen"`
	IsPrepared       *bool   `json:"IsPrepared,omitempty"`
	IsWithLigth      *bool   `json:"IsWithLigth,omitempty"` // ODH typo — preserved
	HasRentals       *bool   `json:"HasRentals,omitempty"`
	RunToValley      *bool   `json:"RunToValley,omitempty"`
	FeetClimb        *bool   `json:"FeetClimb,omitempty"`
	LiftAvailable    *bool   `json:"LiftAvailable,omitempty"`
	Highlight        *bool   `json:"Highlight,omitempty"`
	CopyrightChecked *bool   `json:"CopyrightChecked,omitempty"`
	HasFreeEntrance  *bool   `json:"HasFreeEntrance,omitempty"`

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

// GpsInfo matches the full ODH shape including extra fields.
// The API returns Latitude and Longitude as strings; custom unmarshaling converts them to floats.
type GpsInfo struct {
	Default               *bool    `json:"Default,omitempty"`
	Gpstype               string   `json:"Gpstype"`
	Altitude              *float64 `json:"Altitude,omitempty"`
	Geometry              *string  `json:"Geometry,omitempty"`
	Latitude              float64  `json:"Latitude"`
	Longitude             float64  `json:"Longitude"`
	AltitudeUnitofMeasure string   `json:"AltitudeUnitofMeasure,omitempty"`
}

// UnmarshalJSON handles the case where Latitude and Longitude come from the API as strings.
func (g *GpsInfo) UnmarshalJSON(b []byte) error {
	var raw struct {
		Default               *bool       `json:"Default"`
		Gpstype               string      `json:"Gpstype"`
		Altitude              *float64    `json:"Altitude"`
		Geometry              *string     `json:"Geometry"`
		Latitude              interface{} `json:"Latitude"`  // Can be string or float
		Longitude             interface{} `json:"Longitude"` // Can be string or float
		AltitudeUnitofMeasure string      `json:"AltitudeUnitofMeasure"`
	}

	if err := json.Unmarshal(b, &raw); err != nil {
		return err
	}

	g.Default = raw.Default
	g.Gpstype = raw.Gpstype
	g.Altitude = raw.Altitude
	g.Geometry = raw.Geometry
	g.AltitudeUnitofMeasure = raw.AltitudeUnitofMeasure

	// Convert latitude from interface{} (can be string or float64)
	switch v := raw.Latitude.(type) {
	case string:
		lat, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid latitude: %v", err)
		}
		g.Latitude = lat
	case float64:
		g.Latitude = v
	case nil:
		g.Latitude = 0
	default:
		return fmt.Errorf("latitude has unexpected type: %T", v)
	}

	// Convert longitude from interface{} (can be string or float64)
	switch v := raw.Longitude.(type) {
	case string:
		lon, err := strconv.ParseFloat(v, 64)
		if err != nil {
			return fmt.Errorf("invalid longitude: %v", err)
		}
		g.Longitude = lon
	case float64:
		g.Longitude = v
	case nil:
		g.Longitude = 0
	default:
		return fmt.Errorf("longitude has unexpected type: %T", v)
	}

	return nil
}

// GpsTrack matches the old API: a slice of track objects.
type GpsTrack struct {
	Id           *string                `json:"Id"`
	Type         string                 `json:"Type"`
	Format       string                 `json:"Format"`
	GpxTrackUrl  string                 `json:"GpxTrackUrl"`
	GpxTrackDesc map[string]interface{} `json:"GpxTrackDesc"`
}

// OperationSchedule — field name matches old API exactly (lowercase 's' in OperationscheduleName).
type OperationSchedule struct {
	Stop                  string                  `json:"Stop,omitempty"`
	Type                  string                  `json:"Type,omitempty"`
	Start                 string                  `json:"Start,omitempty"`
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime,omitempty"`
	OperationscheduleName map[string]string       `json:"OperationscheduleName"` // lowercase 's' — matches old API
}

// OperationScheduleTime includes all fields from the old API including ODH typos.
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

// LocationInfo is populated by the ODH pipeline from GpsInfo — not set by transformer.
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

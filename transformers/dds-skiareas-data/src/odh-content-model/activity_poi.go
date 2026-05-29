// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package odhmodel

import (
	"encoding/json"
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

// ── SkiAreaPage — paginated list response from ODH ────────────────────────────
// ODH SkiArea requires &pagenumber=1 to return this envelope shape.

type SkiAreaPage struct {
	TotalResults int       `json:"TotalResults"`
	TotalPages   int       `json:"TotalPages"`
	CurrentPage  int       `json:"CurrentPage"`
	Items        []SkiArea `json:"Items"`
}

// ── SkiArea ───────────────────────────────────────────────────────────────────
//
// Design: the transformer only needs to READ a handful of fields and WRITE back
// only OperationSchedule + LastChange + Mapping. Every other field must be
// round-tripped byte-for-byte so the PUT doesn't destroy IDM-managed data.
//
// Strategy:
//   - Declare the fields we actually read or write as typed struct fields.
//   - Capture every other field in `extra` (map[string]json.RawMessage).
//   - Custom Marshal/Unmarshal merges struct fields and extra into one flat JSON object.
//
// This guarantees: Areas, AreaIds, GpsInfo, GpsPoints, Regions, RegionIds,
// DistrictIds, SkiRegionId, SlopeKm*, ImageGallery, LocationInfo, MunicipalityIds,
// TourismvereinIds, TourismAssociations, LiftCount, Latitude, Longitude,
// AltitudeTo, AltitudeFrom, AreaRadius, SkiAreaMapURL, RelatedContent etc.
// are all preserved unchanged on every PUT.

type SkiArea struct {
	// ── Fields we explicitly read or write ───────────────────────────────────

	Id          *string                      `json:"Id,omitempty"`
	Active      bool                         `json:"Active"`
	Source      *string                      `json:"Source,omitempty"`
	Shortname   *string                      `json:"Shortname,omitempty"`
	FirstImport *FlexibleTime                `json:"FirstImport,omitempty"`
	LastChange  *FlexibleTime                `json:"LastChange,omitempty"`
	HasLanguage []string                     `json:"HasLanguage,omitempty"`
	Mapping     map[string]map[string]string `json:"Mapping,omitempty"`

	// Detail: clib.DetailGeneric covers all ODH Detail fields so round-trip is safe.
	Detail map[string]*clib.DetailGeneric `json:"Detail,omitempty"`

	// ContactInfos: json.RawMessage preserves the full rich IDM shape (Url, City,
	// Address, CompanyName, Tax, Vat etc.) without any data loss.
	ContactInfos map[string]json.RawMessage `json:"ContactInfos,omitempty"`

	// OperationSchedule: intentionally replaced on both CREATE and UPDATE.
	OperationSchedule []OperationSchedule `json:"OperationSchedule,omitempty"`

	// SmgActive/OdhActive/PublishedOn: read from existing record on UPDATE,
	// set explicitly only on CREATE.
	SmgActive   bool     `json:"SmgActive"`
	OdhActive   bool     `json:"OdhActive"`
	PublishedOn []string `json:"PublishedOn"`

	// Sync
	SyncUpdateMode      string `json:"SyncUpdateMode,omitempty"`
	SyncSourceInterface string `json:"SyncSourceInterface,omitempty"`

	// LicenseInfo: read from existing record on UPDATE, set only on CREATE.
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`

	// Tags: omitempty so nil is not sent — existing values preserved on UPDATE.
	TagIds  []string `json:"TagIds,omitempty"`
	SmgTags []string `json:"SmgTags,omitempty"`

	// ── Catch-all: every other field from the ODH API ────────────────────────
	// Areas, AreaId, AreaIds, GpsInfo, GpsPoints, Regions, RegionIds,
	// DistrictIds, SkiRegionId, SkiRegion, SlopeKm*, TotalSlopeKm,
	// AltitudeTo, AltitudeFrom, AltitudeUnitofMeasure, AreaRadius,
	// LiftCount, Latitude, Longitude, LocationInfo, ImageGallery,
	// MunicipalityIds, TourismvereinIds, TourismAssociations,
	// SkiAreaMapURL, SkiRegionName, RelatedContent, etc.
	extra map[string]json.RawMessage
}

// knownFields lists every JSON key handled by the typed struct fields above.
// Any key NOT in this set goes into extra during unmarshal.
var knownFields = map[string]bool{
	"Id": true, "Active": true, "Source": true, "Shortname": true,
	"FirstImport": true, "LastChange": true, "HasLanguage": true,
	"Mapping": true, "Detail": true, "ContactInfos": true,
	"OperationSchedule": true, "SmgActive": true, "OdhActive": true,
	"PublishedOn": true, "SyncUpdateMode": true, "SyncSourceInterface": true,
	"LicenseInfo": true, "TagIds": true, "SmgTags": true,
}

// UnmarshalJSON deserializes all known fields into typed struct fields and
// stores everything else in extra for lossless round-tripping.
func (s *SkiArea) UnmarshalJSON(data []byte) error {
	// First pass: decode into a raw map
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Decode known fields using a type alias to avoid recursion
	type Alias SkiArea
	var alias Alias
	// Build a sub-object with only the known keys and unmarshal into alias
	known := make(map[string]json.RawMessage, len(knownFields))
	for k := range knownFields {
		if v, ok := raw[k]; ok {
			known[k] = v
		}
	}
	knownBytes, err := json.Marshal(known)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(knownBytes, &alias); err != nil {
		return err
	}
	*s = SkiArea(alias)

	// Store all unknown fields in extra
	s.extra = make(map[string]json.RawMessage)
	for k, v := range raw {
		if !knownFields[k] {
			s.extra[k] = v
		}
	}
	return nil
}

// MarshalJSON serializes struct fields and extra fields into one flat JSON object,
// with struct fields taking precedence over extra in case of key collision.
func (s SkiArea) MarshalJSON() ([]byte, error) {
	// Start with extra fields as the base
	merged := make(map[string]json.RawMessage, len(s.extra)+len(knownFields))
	for k, v := range s.extra {
		merged[k] = v
	}

	// Marshal each known field and overlay — struct fields win on collision
	type Alias SkiArea
	alias := Alias(s)
	alias.extra = nil // prevent recursion

	knownBytes, err := json.Marshal(alias)
	if err != nil {
		return nil, err
	}
	var knownMap map[string]json.RawMessage
	if err := json.Unmarshal(knownBytes, &knownMap); err != nil {
		return nil, err
	}
	for k, v := range knownMap {
		merged[k] = v
	}

	return json.Marshal(merged)
}

// ── Supporting types ──────────────────────────────────────────────────────────

// OperationSchedule — OperationscheduleName has lowercase 's' — matches ODH API exactly.
type OperationSchedule struct {
	Stop  string `json:"Stop,omitempty"`
	Type  string `json:"Type,omitempty"`
	Start string `json:"Start,omitempty"`
	// OperationScheduleTime must always emit as [] not be omitted — matches old API shape.
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime"`
	OperationscheduleName map[string]string       `json:"OperationscheduleName"`
}

// OperationScheduleTime — for SkiArea we always emit an empty slice.
type OperationScheduleTime struct {
	Start     string `json:"Start,omitempty"`
	End       string `json:"End,omitempty"`
	State     int    `json:"State,omitempty"`
	Timecode  int    `json:"Timecode,omitempty"`
	Monday    bool   `json:"Monday,omitempty"`
	Tuesday   bool   `json:"Tuesday,omitempty"`
	Wednesday bool   `json:"Wednesday,omitempty"`
	Thursday  bool   `json:"Thursday,omitempty"`
	Thuresday bool   `json:"Thuresday,omitempty"` // ODH typo — preserved
	Friday    bool   `json:"Friday,omitempty"`
	Saturday  bool   `json:"Saturday,omitempty"`
	Sunday    bool   `json:"Sunday,omitempty"`
}

// ContactInfo is used ONLY for CREATE path — marshaled into json.RawMessage.
// On UPDATE the existing RawMessage from ODH is preserved unchanged.
type ContactInfo struct {
	Language    string `json:"Language"`
	Email       string `json:"Email,omitempty"`
	Phonenumber string `json:"Phonenumber,omitempty"`
}

type LicenseInfo struct {
	Author        string `json:"Author"`
	License       string `json:"License"`
	ClosedData    bool   `json:"ClosedData"`
	LicenseHolder string `json:"LicenseHolder"`
}

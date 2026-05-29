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

// ── SkiAreaPage — paginated list response from ODH ──────────────────────────
// ODH SkiArea requires &pagenumber=1 to return this envelope shape.

type SkiAreaPage struct {
	TotalResults int       `json:"TotalResults"`
	TotalPages   int       `json:"TotalPages"`
	CurrentPage  int       `json:"CurrentPage"`
	Items        []SkiArea `json:"Items"`
}

// ── SkiArea — full ODH SkiArea entity ────────────────────────────────────────

type SkiArea struct {
	Id          *string                      `json:"Id,omitempty"`
	Active      bool                         `json:"Active"`
	Source      *string                      `json:"Source,omitempty"`
	Shortname   *string                      `json:"Shortname,omitempty"`
	FirstImport *FlexibleTime                `json:"FirstImport,omitempty"`
	LastChange  *FlexibleTime                `json:"LastChange,omitempty"`
	HasLanguage []string                     `json:"HasLanguage,omitempty"`
	Mapping     map[string]map[string]string `json:"Mapping,omitempty"`

	// Multilingual content — on UPDATE we preserve existing Detail entirely.
	// On CREATE we populate de/it/en Title only from DSS name.
	Detail       map[string]*clib.DetailGeneric `json:"Detail,omitempty"`
	ContactInfos map[string]*ContactInfo        `json:"ContactInfos,omitempty"`

	// OperationSchedule — on UPDATE we replace this entirely with summer+winter seasons.
	// On CREATE we populate it from DSS season data.
	OperationSchedule []OperationSchedule `json:"OperationSchedule,omitempty"`

	// Publishing
	SmgActive   bool     `json:"SmgActive"`
	OdhActive   bool     `json:"OdhActive"`
	PublishedOn []string `json:"PublishedOn"`

	// Sync
	SyncUpdateMode      string `json:"SyncUpdateMode,omitempty"`
	SyncSourceInterface string `json:"SyncSourceInterface,omitempty"`

	// License
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`

	// Additional fields present in ODH SkiArea but managed by other systems —
	// preserved as-is on update by fetching the existing record first.
	TagIds  []string `json:"TagIds,omitempty"`
	SmgTags []string `json:"SmgTags,omitempty"`
}

// ── Supporting types ──────────────────────────────────────────────────────────

// OperationSchedule — same shape as lifts/slopes.
// OperationscheduleName has lowercase 's' — matches ODH API exactly.
type OperationSchedule struct {
	Stop  string `json:"Stop,omitempty"`
	Type  string `json:"Type,omitempty"`
	Start string `json:"Start,omitempty"`
	// OperationScheduleTime must always emit as [] not be omitted — matches old API shape.
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime"`
	OperationscheduleName map[string]string       `json:"OperationscheduleName"`
}

// OperationScheduleTime — for SkiArea we always emit an empty slice.
// The struct mirrors the full ODH shape used in lifts/slopes.
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

// ContactInfo holds per-language contact details.
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

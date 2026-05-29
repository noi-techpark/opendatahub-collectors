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

// ── SkiArea — ODH SkiArea entity ─────────────────────────────────────────────
//
// Design note for UPDATE path:
//   We fetch the existing record, replace ONLY OperationSchedule, then PUT back.
//   All other fields are preserved exactly as returned by the API.
//
//   Critically:
//   - ContactInfos uses json.RawMessage so the full rich shape (Url, City, Address,
//     CompanyName, Tax, Vat etc.) is round-tripped without any data loss.
//   - PublishedOn and LicenseInfo are preserved from the existing record — we never
//     overwrite them on UPDATE (only set them on CREATE).

type SkiArea struct {
	Id          *string                      `json:"Id,omitempty"`
	Active      bool                         `json:"Active"`
	Source      *string                      `json:"Source,omitempty"`
	Shortname   *string                      `json:"Shortname,omitempty"`
	FirstImport *FlexibleTime                `json:"FirstImport,omitempty"`
	LastChange  *FlexibleTime                `json:"LastChange,omitempty"`
	HasLanguage []string                     `json:"HasLanguage,omitempty"`
	Mapping     map[string]map[string]string `json:"Mapping,omitempty"`

	// Detail: clib.DetailGeneric has all ODH fields (Header, BaseText, Keywords etc.)
	// so round-trip on UPDATE is safe without data loss.
	Detail map[string]*clib.DetailGeneric `json:"Detail,omitempty"`

	// ContactInfos uses json.RawMessage to preserve the full ODH shape on UPDATE.
	// The existing idm records have rich ContactInfo (Url, City, Address, CompanyName
	// etc.) that our minimal struct would silently drop. RawMessage round-trips it
	// unchanged. On CREATE we marshal our minimal ContactInfo into RawMessage.
	ContactInfos map[string]json.RawMessage `json:"ContactInfos,omitempty"`

	// OperationSchedule — intentionally replaced on both CREATE and UPDATE.
	OperationSchedule []OperationSchedule `json:"OperationSchedule,omitempty"`

	// Publishing — only set on CREATE; on UPDATE the existing value is preserved
	// because the field is populated from the fetched record.
	SmgActive   bool     `json:"SmgActive"`
	OdhActive   bool     `json:"OdhActive"`
	PublishedOn []string `json:"PublishedOn"`

	// Sync
	SyncUpdateMode      string `json:"SyncUpdateMode,omitempty"`
	SyncSourceInterface string `json:"SyncSourceInterface,omitempty"`

	// LicenseInfo — only set on CREATE; on UPDATE preserved from existing record.
	LicenseInfo *LicenseInfo `json:"LicenseInfo,omitempty"`

	// Tags — omitempty so nil slices are not sent, preserving existing values on UPDATE.
	TagIds  []string `json:"TagIds,omitempty"`
	SmgTags []string `json:"SmgTags,omitempty"`
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

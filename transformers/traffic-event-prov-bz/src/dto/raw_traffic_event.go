// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
)

// FlexString unmarshals from a JSON string, number, boolean or null into a
// Go string. The Province of Bolzano feed has changed several fields from
// integers to strings over time (e.g. messageId, messageTypeId); FlexString
// makes the DTO tolerant of either representation so a future type flip does
// not break parsing of the whole batch.
type FlexString string

func (s *FlexString) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*s = ""
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = FlexString(str)
		return nil
	}
	// Number, boolean or any other scalar: keep its textual form.
	*s = FlexString(strings.Trim(string(b), `"`))
	return nil
}

func (s FlexString) String() string { return string(s) }

// FlexFloat unmarshals a coordinate from a JSON number, a numeric string,
// null or an empty string. Valid is false when no usable number was present
// (null, "", or unparseable), so "senza coordinate" events degrade cleanly
// instead of failing the whole feed.
type FlexFloat struct {
	Value float64
	Valid bool
}

func (f *FlexFloat) UnmarshalJSON(b []byte) error {
	s := strings.Trim(strings.TrimSpace(string(b)), `"`)
	if s == "" || s == "null" {
		f.Value, f.Valid = 0, false
		return nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		// Tolerate junk: treat as missing rather than erroring the batch.
		f.Value, f.Valid = 0, false
		return nil
	}
	f.Value, f.Valid = v, true
	return nil
}

// TrafficEvent is one event from the Province of Bolzano traffic feed
// (https://static-verkehr.provinz.bz.it/publications/traffic/traffic.json).
//
// In the current feed every field is a JSON string except the coordinates
// X/Y (numbers); several string fields may be null. Numeric-looking fields
// use FlexString so the DTO keeps parsing if the provider switches a field
// back to an integer.
type TrafficEvent struct {
	JSONFeaturetype string `json:"json_featuretype"`
	PublishDateTime string `json:"publishDateTime"`
	BeginDate       string `json:"beginDate"`
	EndDate         string `json:"endDate"`
	DescriptionDe   string `json:"descriptionDe"`
	DescriptionIt   string `json:"descriptionIt"`
	TycodeValue     string `json:"tycodeValue"`
	TycodeDe        string `json:"tycodeDe"`
	TycodeIt        string `json:"tycodeIt"`
	SubTycodeValue  string `json:"subTycodeValue"`
	SubTycodeDe     string `json:"subTycodeDe"`
	SubTycodeIt     string `json:"subTycodeIt"`
	PlaceDe         string `json:"placeDe"`
	PlaceIt         string `json:"placeIt"`

	ActualMail    FlexString `json:"actualMail"`
	MessageID     FlexString `json:"messageId"`
	MessageStatus FlexString `json:"messageStatus"`

	MessageZoneID     FlexString `json:"messageZoneId"`
	MessageZoneDescDe string     `json:"messageZoneDescDe"`
	MessageZoneDescIt string     `json:"messageZoneDescIt"`

	MessageGradID     FlexString `json:"messageGradId"`
	MessageGradDescDe string     `json:"messageGradDescDe"`
	MessageGradDescIt string     `json:"messageGradDescIt"`

	MessageStreetID             FlexString `json:"messageStreetId"`
	MessageStreetWapDescDe      string     `json:"messageStreetWapDescDe"`
	MessageStreetWapDescIt      string     `json:"messageStreetWapDescIt"`
	MessageStreetInternetDescDe string     `json:"messageStreetInternetDescDe"`
	MessageStreetInternetDescIt string     `json:"messageStreetInternetDescIt"`
	MessageStreetNr             string     `json:"messageStreetNr"`
	MessageStreetHierarchie     FlexString `json:"messageStreetHierarchie"`

	MessageTypeID     FlexString `json:"messageTypeId"`
	MessageTypeDescDe string     `json:"messageTypeDescDe"`
	MessageTypeDescIt string     `json:"messageTypeDescIt"`

	X FlexFloat `json:"X"`
	Y FlexFloat `json:"Y"`
}

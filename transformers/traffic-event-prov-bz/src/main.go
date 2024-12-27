// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
)

var env tr.Env

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	b := bdplib.FromEnv()

	tr.ListenFromEnv(env, func(r *dto.Raw[string]) error {
		dtos := []trafficEvent{}
		if err := json.Unmarshal([]byte(r.Rawdata), &dtos); err != nil {
			return fmt.Errorf("could not unmarshal the raw payload json: %w", err)
		}
		events := []bdplib.Event{}
		for d, _ := range dtos {
			fmt.Print(d)
		}

		b.SyncEvents(events)
		return nil
	})

}

// All this strange UUID stuff is just there to replicate the UUID generation originally done in Java, and thus maintain primary key compatibility.
// Unfortunately, it uses quite particular behavior that is elaborate to replicate.
// In essence, it generates a JSON from a few fields, and then calculates a v5 UUID (with namespace = null)

// TODO: make UUID generation more sane, but make sure to migrate all existing events in DB, an clear up if changing the UUIDs could lead to problems

type UUIDMap struct {
	BeginDate *UUIDDate `json:"beginDate"`
	EndDate   *UUIDDate `json:"endDate"`
	X         float64   `json:"X"`
	Y         float64   `json:"Y"`
}
type UUIDDate struct {
	Year       int    `json:"year"`
	Month      string `json:"month"`
	DayOfWeek  string `json:"dayOfWeek"`
	LeapYear   bool   `json:"leapYear"`
	DayOfMonth int    `json:"dayOfMonth"`
	MonthValue int    `json:"monthValue"`
	Era        string `json:"era"`
	DayOfYear  int    `json:"dayOfYear"`
	Chronology struct {
		CalendarType string `json:"calendarType"`
		ID           string `json:"id"`
	} `json:"chronology"`
}

func toDate(s string) (UUIDDate, error) {
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		return UUIDDate{}, err
	}
	ret := UUIDDate{}
	ret.Year = d.Year()
	ret.Month = strings.ToUpper(d.Month().String())
	ret.DayOfWeek = strings.ToUpper(d.Weekday().String())
	ret.LeapYear = ret.Year%4 == 0 && ret.Year%100 != 0 || ret.Year%400 == 0
	ret.DayOfMonth = d.Day()
	ret.MonthValue = int(d.Month())
	ret.Era = "CE"
	ret.DayOfYear = d.YearDay()
	ret.Chronology.CalendarType = "iso8601"
	ret.Chronology.ID = "ISO"

	return ret, nil
}

func makeUUID(name string) string {
	// Have to do the v5 UUID ourselves, because the one from library does not support a nil namespace. The code is copied from there, sans the namespace part
	hash := sha1.New()
	hash.Write([]byte(name))
	u := uuid.UUID{}
	copy(u[:], hash.Sum(nil))
	u.SetVersion(uuid.V5)
	u.SetVariant(uuid.VariantRFC9562)
	return u.String()
}

func makeUUIDJson(e trafficEvent) (string, error) {
	u := UUIDMap{}
	begin, err := toDate(e.BeginDate)
	if err != nil {
		return "", fmt.Errorf("cannot parse beginDate: %w", err)
	}
	u.BeginDate = &begin
	if e.EndDate != "" {
		end, err := toDate(e.EndDate)
		if err != nil {
			return "", fmt.Errorf("cannot parse endDate: %w", err)
		}
		u.EndDate = &end
	}
	u.X = e.X
	u.Y = e.Y
	jsonBytes, err := json.Marshal(u)
	if err != nil {
		return "", fmt.Errorf("cannot marshal uuid json: %w", err)
	}
	jsonString := string(jsonBytes[:])
	return jsonString, nil
}

type trafficEvent struct {
	JSONFeaturetype             string  `json:"json_featuretype"`
	PublishDateTime             string  `json:"publishDateTime"`
	BeginDate                   string  `json:"beginDate"`
	EndDate                     string  `json:"endDate"`
	DescriptionDe               string  `json:"descriptionDe"`
	DescriptionIt               string  `json:"descriptionIt"`
	TycodeValue                 string  `json:"tycodeValue"`
	TycodeDe                    string  `json:"tycodeDe"`
	TycodeIt                    string  `json:"tycodeIt"`
	SubTycodeValue              string  `json:"subTycodeValue"`
	SubTycodeDe                 string  `json:"subTycodeDe"`
	SubTycodeIt                 string  `json:"subTycodeIt"`
	PlaceDe                     string  `json:"placeDe"`
	PlaceIt                     string  `json:"placeIt"`
	ActualMail                  int     `json:"actualMail"`
	MessageID                   int     `json:"messageId"`
	MessageStatus               int     `json:"messageStatus"`
	MessageZoneID               int     `json:"messageZoneId"`
	MessageZoneDescDe           string  `json:"messageZoneDescDe"`
	MessageZoneDescIt           string  `json:"messageZoneDescIt"`
	MessageGradID               int     `json:"messageGradId"`
	MessageGradDescDe           string  `json:"messageGradDescDe"`
	MessageGradDescIt           string  `json:"messageGradDescIt"`
	MessageStreetID             int     `json:"messageStreetId"`
	MessageStreetWapDescDe      string  `json:"messageStreetWapDescDe"`
	MessageStreetWapDescIt      string  `json:"messageStreetWapDescIt"`
	MessageStreetInternetDescDe string  `json:"messageStreetInternetDescDe"`
	MessageStreetInternetDescIt string  `json:"messageStreetInternetDescIt"`
	MessageStreetNr             string  `json:"messageStreetNr"`
	MessageStreetHierarchie     int     `json:"messageStreetHierarchie"`
	MessageTypeID               int     `json:"messageTypeId"`
	MessageTypeDescDe           string  `json:"messageTypeDescDe"`
	MessageTypeDescIt           string  `json:"messageTypeDescIt"`
	X                           float64 `json:"X"`
	Y                           float64 `json:"Y"`
}

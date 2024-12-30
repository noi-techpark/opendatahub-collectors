// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"crypto/sha1"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/uuid/v5"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkt"
)

var env tr.Env

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	b := bdplib.FromEnv()

	tr.ListenFromEnv(env, func(r *dto.Raw[string]) error {
		dtos, err := unmarshalRawJson(r.Rawdata)
		if err != nil {
			return fmt.Errorf("could not unmarshal the raw payload json: %w", err)
		}
		events := []bdplib.Event{}
		for _, d := range dtos {
			e, err := mapEvent(d)
			if err != nil {
				return err
			}
			e.Origin = b.Origin
			events = append(events, e)
		}

		b.SyncEvents(events)
		return nil
	})
}

func unmarshalRawJson(s string) ([]trafficEvent, error) {
	dtos := []trafficEvent{}
	err := json.Unmarshal([]byte(s), &dtos)
	return dtos, err
}

func mapEvent(d trafficEvent) (bdplib.Event, error) {
	e := bdplib.Event{}
	j, err := makeUUIDJson(d)
	if err != nil {
		return e, err
	}
	uuid := makeUUID(j)
	e.Uuid = uuid
	e.EventSeriesUuid = uuid
	e.Category = fmt.Sprintf("%s_%s | %s_%s", d.TycodeIt, d.SubTycodeIt, d.TycodeDe, d.SubTycodeDe)
	e.Name = strconv.Itoa(d.MessageID)
	e.Description = fmt.Sprintf("%s | %s", d.DescriptionIt, d.DescriptionDe)

	if d.X != nil && d.Y != nil {
		wkt, err := point2WKT(*d.X, *d.Y)
		if err != nil {
			return e, fmt.Errorf("error creating point wkt: %w", err)
		}
		e.WktGeometry = wkt
	}

	beginDate, err := time.Parse(dayDateFormat, d.BeginDate)
	if err != nil {
		return e, fmt.Errorf("error parsing BeginDate (%s): %w", d.BeginDate, err)
	}
	e.EventStart = beginDate.UTC().UnixMilli()

	if d.EndDate != nil && *d.EndDate != "" {
		endDate, err := time.Parse(dayDateFormat, *d.EndDate)
		if err != nil {
			return e, fmt.Errorf("error parsing EndDate (%s): %w", *d.EndDate, err)
		}
		e.EventEnd = endDate.UTC().UnixMilli() + 1 // +1 because we exclude the upper bound.
	}

	e.MetaData = map[string]any{}
	e.MetaData["json_featuretype"] = d.JSONFeaturetype
	e.MetaData["publisherDateTime"] = d.PublishDateTime
	e.MetaData["tycodeValue"] = d.TycodeValue
	e.MetaData["tycodeDe"] = d.TycodeDe
	e.MetaData["tycodeIt"] = d.TycodeIt
	e.MetaData["subTycodeValue"] = d.SubTycodeValue
	e.MetaData["subTycodeDe"] = d.SubTycodeDe
	e.MetaData["subTycodeIt"] = d.SubTycodeIt
	e.MetaData["placeDe"] = d.PlaceDe
	e.MetaData["placeIt"] = d.PlaceIt
	e.MetaData["actualMail"] = d.ActualMail
	e.MetaData["messageId"] = d.MessageID
	e.MetaData["messageStatus"] = d.MessageStatus
	e.MetaData["messageZoneId"] = d.MessageZoneID
	e.MetaData["messageZoneDescDe"] = d.MessageZoneDescDe
	e.MetaData["messageZoneDescIt"] = d.MessageZoneDescIt
	e.MetaData["messageGradId"] = d.MessageGradID
	e.MetaData["messageGradDescDe"] = d.MessageGradDescDe
	e.MetaData["messageGradDescIt"] = d.MessageGradDescIt
	e.MetaData["messageStreetId"] = d.MessageStreetID
	e.MetaData["messageStreetWapDescDe"] = d.MessageStreetWapDescDe
	e.MetaData["messageStreetWapDescIt"] = d.MessageStreetWapDescIt
	e.MetaData["messageStreetInternetDescDe"] = d.MessageStreetInternetDescDe
	e.MetaData["messageStreetInternetDescIt"] = d.MessageStreetInternetDescIt
	e.MetaData["messageStreetNr"] = d.MessageStreetNr
	e.MetaData["messageStreetHierarchie"] = d.MessageStreetHierarchie

	return e, nil
}

type trafficEvent struct {
	JSONFeaturetype             string   `json:"json_featuretype"`
	PublishDateTime             string   `json:"publishDateTime"`
	BeginDate                   string   `json:"beginDate"`
	EndDate                     *string  `json:"endDate"`
	DescriptionDe               string   `json:"descriptionDe"`
	DescriptionIt               string   `json:"descriptionIt"`
	TycodeValue                 string   `json:"tycodeValue"`
	TycodeDe                    string   `json:"tycodeDe"`
	TycodeIt                    string   `json:"tycodeIt"`
	SubTycodeValue              string   `json:"subTycodeValue"`
	SubTycodeDe                 string   `json:"subTycodeDe"`
	SubTycodeIt                 string   `json:"subTycodeIt"`
	PlaceDe                     string   `json:"placeDe"`
	PlaceIt                     string   `json:"placeIt"`
	ActualMail                  int      `json:"actualMail"`
	MessageID                   int      `json:"messageId"`
	MessageStatus               int      `json:"messageStatus"`
	MessageZoneID               int      `json:"messageZoneId"`
	MessageZoneDescDe           string   `json:"messageZoneDescDe"`
	MessageZoneDescIt           string   `json:"messageZoneDescIt"`
	MessageGradID               int      `json:"messageGradId"`
	MessageGradDescDe           string   `json:"messageGradDescDe"`
	MessageGradDescIt           string   `json:"messageGradDescIt"`
	MessageStreetID             int      `json:"messageStreetId"`
	MessageStreetWapDescDe      string   `json:"messageStreetWapDescDe"`
	MessageStreetWapDescIt      string   `json:"messageStreetWapDescIt"`
	MessageStreetInternetDescDe string   `json:"messageStreetInternetDescDe"`
	MessageStreetInternetDescIt string   `json:"messageStreetInternetDescIt"`
	MessageStreetNr             string   `json:"messageStreetNr"`
	MessageStreetHierarchie     int      `json:"messageStreetHierarchie"`
	MessageTypeID               int      `json:"messageTypeId"`
	MessageTypeDescDe           string   `json:"messageTypeDescDe"`
	MessageTypeDescIt           string   `json:"messageTypeDescIt"`
	X                           *float64 `json:"X"`
	Y                           *float64 `json:"Y"`
}

func point2WKT(x float64, y float64) (string, error) {
	p := geom.NewPointFlat(geom.XY, []float64{x, y})
	p.SetSRID(4326)
	return wkt.Marshal(p)
}

// All this strange UUID stuff is just there to replicate the UUID generation originally done in Java, and thus maintain primary key compatibility.
// Unfortunately, it uses quite particular behavior that is elaborate to replicate.
// In essence, it generates a JSON from a few fields, and then calculates a v5 UUID (with namespace = null)

// TODO: make UUID generation more sane, but make sure to migrate all existing events in DB, an clear up if changing the UUIDs could lead to problems

type UUIDMap struct {
	BeginDate *UUIDDate `json:"beginDate"`
	EndDate   *UUIDDate `json:"endDate"`
	X         *float64  `json:"X"`
	Y         *float64  `json:"Y"`
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

const dayDateFormat = "2006-01-02"

func toDate(s string) (UUIDDate, error) {
	d, err := time.Parse(dayDateFormat, s)
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
	if e.EndDate != nil && *e.EndDate != "" {
		end, err := toDate(*e.EndDate)
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

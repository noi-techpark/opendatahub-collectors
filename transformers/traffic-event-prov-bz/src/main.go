// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	"github.com/twpayne/go-geom"
	"github.com/twpayne/go-geom/encoding/wkt"
)

var env tr.Env

const UUID_NS = "traffic-events-prov-bz"

func main() {
	slog.Info("Traffic data collector starting up...")
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	b := bdplib.FromEnv()

	err := tr.ListenFromEnv(env, func(r *dto.Raw[string]) error {
		slog.Info("New message received")
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
	ms.FailOnError(err, "transformer handler failed")
}

func unmarshalRawJson(s string) ([]trafficEvent, error) {
	dtos := []trafficEvent{}
	err := json.Unmarshal([]byte(s), &dtos)
	return dtos, err
}

func mapEvent(d trafficEvent) (bdplib.Event, error) {
	e := bdplib.Event{}
	e.Uuid = makeUUID(toJsonOrPanic(uuidObj(d)))
	e.EventSeriesUuid = makeUUID([]byte(strconv.Itoa(d.MessageID)))
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

	if d.EndDate != "" {
		endDate, err := time.Parse(dayDateFormat, d.EndDate)
		if err != nil {
			return e, fmt.Errorf("error parsing EndDate (%s): %w", d.EndDate, err)
		}
		end := endDate.UTC().UnixMilli() + 1 // +1 because we exclude the upper bound.
		e.EventEnd = &end
	}

	e.MetaData = map[string]any{}
	e.MetaData["json_featuretype"] = d.JSONFeaturetype
	e.MetaData["publishDateTime"] = d.PublishDateTime
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
	EndDate                     string   `json:"endDate"`
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

type UUIDMap struct {
	MessageID               int     `json:"messageId"`
	X                       float64 `json:"x"`
	Y                       float64 `json:"y"`
	BeginDate               string  `json:"beginDate"`
	EndDate                 string  `json:"endDate"`
	TycodeValue             string  `json:"tycodeValue"`
	SubTycodeValue          string  `json:"subTycodeValue"`
	PlaceDe                 string  `json:"placeDe"`
	PlaceIt                 string  `json:"placeIt"`
	MessageStatus           int     `json:"messageStatus"`
	MessageZoneID           int     `json:"messageZoneId"`
	MessageGradID           int     `json:"messageGradId"`
	MessageTypeID           int     `json:"messageTypeId"`
	MessageStreetID         int     `json:"messageStreetId"`
	MessageStreetNr         string  `json:"messageStreetNr"`
	MessageStreetHierarchie int     `json:"messageStreetHierarchie"`
}

const dayDateFormat = "2006-01-02"

// UUID_NAMESPACE := uuid.NewSHA1(uuid.Nil, []byte("traffic-event-prov-bz"))
// where uuid.Nil is an array[16] full of zeroes, not a zero length array
// see corresponding main_test.go/Test_namespace
const UUID_NAMESPACE = "c168cf4d-7fc7-5608-acad-c167f498f096"

func uuidObj(e trafficEvent) UUIDMap {
	return UUIDMap{e.MessageID, *e.X, *e.Y, e.BeginDate, e.EndDate,
		e.TycodeValue, e.SubTycodeValue, e.PlaceDe, e.PlaceIt, e.MessageStatus, e.MessageZoneID, e.MessageGradID, e.MessageTypeID,
		e.MessageStreetID, e.MessageStreetNr, e.MessageStreetHierarchie}
}

func toJsonOrPanic(obj any) []byte {
	// if you ever port this to another language:
	// golang Json creation is deterministic: Always in order or struct fields and no whitespace
	// the hash is a UUID V5, with custom namespace (see the constant's comment on how it was created)
	json, err := json.Marshal(obj)
	if err != nil {
		panic(fmt.Errorf("cannot marshal uuid json: %w", err))
	}
	return json
}

func makeUUID(b []byte) string {
	namespace := uuid.MustParse(UUID_NAMESPACE)
	uuid := uuid.NewSHA1(namespace, b).String()
	return uuid
}

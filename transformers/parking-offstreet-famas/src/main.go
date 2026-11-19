// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env struct {
	tr.Env
	bdplib.BdpEnv
}

type RawRecs []RawRec
type RawRec struct {
	Id   int
	Meta XmlRpcValue
	Data XmlRpcValue
}

type XmlRpcValue struct {
	Int       *int          `xml:"int,omitempty" json:",omitempty"`
	I4        *int          `xml:"i4,omitempty" json:",omitempty"`
	Double    *float64      `xml:"double,omitempty" json:",omitempty"`
	Boolean   *uint8        `xml:"boolean,omitempty" json:",omitempty"`
	String    *string       `xml:"string,omitempty" json:",omitempty"`
	DateTime  *string       `xml:"dateTime.iso8601,omitempty" json:",omitempty"`
	Base64    *[]byte       `xml:"base64,omitempty" json:",omitempty"`
	Struct    *XmlRpcStruct `xml:"struct,omitempty" json:",omitempty"`
	Array     *XmlRpcArray  `xml:"array,omitempty" json:",omitempty"`
	StringRaw *string       `xml:",chardata" json:",omitempty"`
}

type XmlRpcStruct struct {
	Members []XmlRpcStructMember `xml:"member"`
}
type XmlRpcStructMember struct {
	Name  string      `xml:"name"`
	Value XmlRpcValue `xml:"value"`
}

type XmlRpcArray struct {
	Data []XmlRpcValue `xml:"data>value"`
}

const STATIONTYPE = "ParkingStation"
const PERIOD = 300

var occupiedDatatype = bdplib.CreateDataType("occupied", "", "Occupacy of a parking area", "Count")

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	ms.InitWithEnv(context.Background(), "", &enrichEnv)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	b.SyncDataTypes([]bdplib.DataType{occupiedDatatype})

	var err error
	err = startEnrichment(context.Background(), env.Env)
	ms.FailOnError(context.Background(), err, "failed to start the enrichment table")
	defer stopEnrichment()

	listener := tr.NewTr[RawRecs](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[RawRecs]) error {
		return Transform(ctx, b, r)
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, b bdplib.Bdp, r *rdb.Raw[RawRecs]) error {
	stations := []bdplib.Station{}
	recs := b.CreateDataMap()
	for _, raw := range r.Rawdata {
		sCode := strconv.Itoa(raw.Id)
		if raw.Meta.Array == nil {
			slog.Warn("Skipping station because of invalid metadata", "id", sCode, "raw.Meta", raw.Meta)
			continue
		}

		if !hasRecord(sCode) {
			slog.Debug("Skipping station without enrichment record", "scode", sCode)
			continue
		}

		sName := sCode
		if n := raw.Meta.Array.Data[1].String; n != nil && *n != "" {
			sName = *n
		}

		s := bdplib.CreateStation(sCode, sName, STATIONTYPE, 0, 0, b.GetOrigin())

		capacity := *raw.Meta.Array.Data[2].I4

		s.MetaData = map[string]any{
			"capacity": capacity,
		}
		enrichStation(sCode, &s)
		stations = append(stations, s)

		if raw.Data.Struct == nil {
			slog.Warn("Skipping station because it has no record data or wrong format", "scode", sCode, "data", raw.Data)
			continue
		}

		state := members2Map(raw.Data.Struct.Members)

		if _, found := state["faultCode"]; found {
			slog.Warn("Skipping station because it has errors in raw data", "scode", sCode, "faultCode", *state["faultCode"].I4, "faultString", *state["faultString"].String)
			continue
		}

		if *state["StatoComunicazione"].Boolean != 1 &&
			*state["AllarmePostiTotali"].Boolean != 1 &&
			*state["AllarmeInattivita"].Boolean != 1 &&
			*state["AllarmePostiOccupati"].Boolean != 1 {

			free := *state["PostiLiberi"].I4
			// Sometimes we get -1 out of the blue. Probably and API error, just ignore
			if free >= 0 {
				occupied := min(max(capacity-free, 0), capacity) // clamp between 0 and capacity
				timestamp := *state["PostiLiberiTs"].I4 * 1000

				recs.AddRecord(sCode, occupiedDatatype.Name, bdplib.CreateRecord(int64(timestamp), occupied, PERIOD))
			}
		}
	}

	err := b.SyncStations(STATIONTYPE, stations, true, false)
	if err != nil {
		return err
	}
	err = b.PushData(STATIONTYPE, recs)
	if err != nil {
		return err
	}
	return nil
}

func members2Map(members []XmlRpcStructMember) map[string]XmlRpcValue {
	ret := map[string]XmlRpcValue{}
	for _, r := range members {
		ret[r.Name] = r.Value
	}
	return ret
}

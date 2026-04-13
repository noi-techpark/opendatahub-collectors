// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"time"

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

var Whitelist = []int{2343, 2344, 2345, 2350, 2764}

const Vehicle = "ON_DEMAND_VEHICLE"
const Period = 60
const Origin = "smart-taxi-merano"

func contains(whitelist []int, value int) bool {
	for _, item := range whitelist {
		if item == value {
			return true
		}
	}
	return false
}

func mapStatus(status string) string {
	m := map[string]string{
		"1": "FREE",
		"2": "OCCUPIED",
		"3": "AVAILABLE",
	}
	val, ok := m[status]
	if ok {
		return val
	}
	return "undefined status"
}

type payload struct {
	Uid      string `json:"_IdUtente"`
	Nickname string `json:"_Nickname"`
	State    string `json:"_Stato"`
	Lat      string `json:"_Latitudine"`
	Long     string `json:"_Longitudine"`
	Time     string `json:"_OraComunicazione"`
}

type payloadArray []payload

func unmarshalRaw(s string) (payloadArray, error) {
	var p payloadArray
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
	}
	return p, nil
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting smart-taxi-merano transformer...")
	b := bdplib.FromEnv(env.BdpEnv)

	defer tel.FlushOnPanic()

	dtState := bdplib.CreateDataType("state", "", "state", "Instantaneous")
	dtPosition := bdplib.CreateDataType("position", "", "position", "Instantaneous")
	ds := []bdplib.DataType{dtState, dtPosition}
	ms.FailOnError(context.Background(), b.SyncDataTypes(ds), "Error pushing datatypes")

	listener := tr.NewTr[string](context.Background(), env.Env)

	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		slog.Info("New message received")

		rawArray, err := unmarshalRaw(r.Rawdata)
		if err != nil {
			return fmt.Errorf("unable to unmarshal raw payload: %w", err)
		}

		dm := b.CreateDataMap()
		for _, raw := range rawArray {
			num, _ := strconv.Atoi(raw.Uid)
			if !contains(Whitelist, num) {
				continue
			}

			slog.Info("Processing vehicle", "id", raw.Uid)
			lat, _ := strconv.ParseFloat(raw.Lat, 64)
			lon, _ := strconv.ParseFloat(raw.Long, 64)
			sname := fmt.Sprintf("vehicle:%s", raw.Uid)
			s := bdplib.CreateStation(sname, raw.Nickname, Vehicle, lat, lon, Origin)

			if err := b.SyncStations(Vehicle, []bdplib.Station{s}, false, false); err != nil {
				return fmt.Errorf("error syncing stations: %w", err)
			}

			latLongMap := map[string]string{
				"lat": raw.Lat,
				"lon": raw.Long,
			}
			state := mapStatus(raw.State)
			parsedTime, err := time.Parse("02/01/2006 15:04:05", raw.Time)
			if err != nil {
				slog.Error("Error parsing time", "err", err, "raw_time", raw.Time)
			}

			dm.AddRecord(s.Id, dtState.Name, bdplib.CreateRecord(parsedTime.UnixMilli(), state, Period))
			dm.AddRecord(s.Id, dtPosition.Name, bdplib.CreateRecord(parsedTime.UnixMilli(), latLongMap, Period))
		}

		if err := b.PushData(Vehicle, dm); err != nil {
			return fmt.Errorf("error pushing data to bdp: %w", err)
		}

		return nil
	})
	ms.FailOnError(context.Background(), err, "transformer handler failed")
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"

	"github.com/relvacode/iso8601"
	"golang.org/x/exp/maps"
)

const period = 60
const stationtype = "EnvironmentStation"

var env tr.Env

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)

	b := bdplib.FromEnv()

	defer tel.FlushOnPanic()

	dtmap := readDataTypes("datatypes.csv")
	ms.FailOnError(ctx, b.SyncDataTypes("", maps.Values(dtmap)), "error pushing datatypes")

	stations, err := readStationCSV("stations.csv")
	ms.FailOnError(ctx, err, "error loading station csv")
	bdpStations := []bdplib.Station{}
	for _, s := range stations {
		bdpStations = append(bdpStations, map2Bdp(s, b.GetOrigin()))
	}
	ms.FailOnError(ctx, b.SyncStations(stationtype, bdpStations, true, false), "error syncing stations")

	listener := tr.NewTr[payload](ctx, env)

	err = listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[payload]) error {
		payload := mqttPayload{}
		if err := json.Unmarshal([]byte(r.Rawdata.Payload), &payload); err != nil {
			return err
		}

		sensorid := payload.ControlUnitId
		ts := payload.DateTimeAcquisition

		station, err := currentStation(stations, sensorid, ts.Time)
		if err != nil {
			return fmt.Errorf("error mapping station for sensor %s: %w", sensorid, err)
		}

		dm := b.CreateDataMap()

		for _, v := range payload.Resval {
			dt, ok := dtmap[strconv.Itoa(v.Id)]
			if !ok {
				return fmt.Errorf("error mapping data type %d for sensor %s", v.Id, sensorid)
			}
			dm.AddRecord(station.id, dt.Name, bdplib.CreateRecord(ts.UnixMilli(), v.Value, period))
		}

		if err := b.PushData(stationtype, dm); err != nil {
			return fmt.Errorf("error pushing data: %w", err)
		}
		return nil
	})
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

type bdpDataTypeMap map[string]bdplib.DataType

func readDataTypes(path string) bdpDataTypeMap {
	dts := readCsv(path)
	dtm := bdpDataTypeMap{}
	for _, dt := range dts[1:] {
		// in the old data collector, for raw datatypes the unit is always null instead of using the one from CSV. Is this correct?
		dtm[dt[0]] = bdplib.CreateDataType(dt[1], dt[2], dt[3], dt[4])
	}
	return dtm
}

type payload struct {
	MsgId   int
	Topic   string
	Payload string
}

type mqttPayload struct {
	DateTimeAcquisition iso8601.Time
	ControlUnitId       string
	Resval              []struct {
		Id    int
		Value float64
		// ignoring other fields
	}
}

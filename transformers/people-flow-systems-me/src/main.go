// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
)

var env struct {
	tr.Env
	bdplib.BdpEnv
	CRON_AGGR         string
	TS_API_BASE_URL   string
	TS_API_REFERER    string
	ODH_TOKEN_URL     string
	ODH_CLIENT_ID     string
	ODH_CLIENT_SECRET string
}

const STATIONTYPE = "PeopleCounter"
const AGGR_PERIOD = 600
const BASE_PERIOD = 1
const AGGR_LAG = time.Minute * 10 // only sum records older than 10 minutes to avoid incomplete windows

var dtCount = bdplib.CreateDataType("countPeople", "people", "Number of people passing by", "sum")
var dtIn = bdplib.CreateDataType("countPeople", "people", "Person passing in direction In", "instantaneous")
var dtOut = bdplib.CreateDataType("countPeople", "people", "Person passing in direction Out", "instantaneous")

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	n := odhts.NewCustomClient(env.TS_API_BASE_URL, env.ODH_TOKEN_URL, env.TS_API_REFERER)
	n.UseAuth(env.ODH_CLIENT_ID, env.ODH_CLIENT_SECRET)

	b.SyncDataTypes([]bdplib.DataType{dtCount, dtIn, dtOut})

	stations, err := syncStations(b)
	ms.FailOnError(ctx, err, "could not sync stations")

	// start cron for aggregation job
	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON_AGGR, func() {
		ms.FailOnError(ctx, aggregate(ctx, b, n), "aggregation job failed")
	})
	c.Start()

	listener := tr.NewTr[RawType](ctx, env.Env)
	err = listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[RawType]) error {
		recs := b.CreateDataMap()
		// last part of topic is the sensor ID used to map metadata in csv
		parts := strings.Split(r.Rawdata.Topic, "/")
		sensorId := parts[len(parts)-1]
		station, found := stations[sensorId]
		if !found {
			return fmt.Errorf("could not find station metadata for topic %s", r.Rawdata.Topic)
		}

		dt := ""
		switch r.Rawdata.Payload.Data.Direction {
		case "In":
			dt = dtIn.Name
		case "Out":
			dt = dtOut.Name
		default:
			return fmt.Errorf("unknown direction %s", r.Rawdata.Payload.Data.Direction)
		}
		recs.AddRecord(station.ID, dt, bdplib.CreateRecord(r.Rawdata.Payload.Data.Timestamp.UnixMilli(), 1, BASE_PERIOD))

		err := b.PushData(STATIONTYPE, recs)
		if err != nil {
			return err
		}
		return nil

	})
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func syncStations(b bdplib.Bdp) (map[string]SensorData, error) {
	stations, err := readStationCsv("stations.csv")
	if err != nil {
		return nil, fmt.Errorf("could not read stations.csv")
	}

	bdpStations := []bdplib.Station{}
	for _, sd := range stations {
		bdpStations = append(bdpStations, bdplib.CreateStation(sd.ID, sd.Name, STATIONTYPE, sd.Lat, sd.Lon, env.BDP_ORIGIN))
	}
	if err := b.SyncStations(STATIONTYPE, bdpStations, true, false); err != nil {
		return nil, fmt.Errorf("could not sync stations")
	}
	return stations, nil
}

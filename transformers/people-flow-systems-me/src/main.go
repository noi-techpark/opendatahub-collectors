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
	CRON_AGGR            string
	TS_API_BASE_URL      string
	TS_API_REFERER       string
	TS_API_TOKEN_URL     string
	TS_API_CLIENT_ID     string
	TS_API_CLIENT_SECRET string

	// Set this date to enable catchup mode and faster re-elaborate history.
	// Don't use in production
	CATCHUP_UNTIL time.Time `default:"2025-01-01T00:00:00+00:00"`
}

const STATIONTYPE = "PeopleCounter"
const AGGR_PERIOD = 600
const BASE_PERIOD = 1

var dtCount = bdplib.CreateDataType("countPeople", "people", "Number of people passing by within the last PERIOD seconds", "count")
var dtIn = bdplib.CreateDataType("sense_in", "people", "Sensing direction In", "instantaneous")
var dtOut = bdplib.CreateDataType("sense_out", "people", "Sensing direction Out", "instantaneous")

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	n := odhts.NewCustomClient(env.TS_API_BASE_URL, env.TS_API_TOKEN_URL, env.TS_API_REFERER)
	n.UseAuth(env.TS_API_CLIENT_ID, env.TS_API_CLIENT_SECRET)

	b.SyncDataTypes([]bdplib.DataType{dtCount, dtIn, dtOut})

	stations, err := syncStations(b)
	ms.FailOnError(ctx, err, "could not sync stations")

	if env.CRON_AGGR != "" {
		slog.Info("Starting cron scheduler for aggregation. To disable, set schedule to empty", "schedule", env.CRON_AGGR)
		c := cron.New(cron.WithSeconds())
		c.AddFunc(env.CRON_AGGR, func() {
			slog.Info("Starting aggregation job")
			ms.FailOnError(ctx, aggregate(ctx, b, n), "aggregation job failed")
			slog.Info("Aggregation job done")
		})
		c.Start()
	} else {
		slog.Info("Aggregation job diabled. Set a cron schedule to enable")
	}

	listener := tr.NewTr[RawType](ctx, env.Env)
	recs := b.CreateDataMap()
	recs_cnt := 0
	err = listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[RawType]) error {
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

		ts := r.Rawdata.Payload.Data.Timestamp
		recs.AddRecord(station.ID, dt, bdplib.CreateRecord(ts.UnixMilli(), 1, BASE_PERIOD))
		recs_cnt += 1

		if ts.After(env.CATCHUP_UNTIL) || recs_cnt > 5000 {
			err := b.PushData(STATIONTYPE, recs)
			if err != nil {
				return err
			}
			recs = b.CreateDataMap()
			recs_cnt = 0
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

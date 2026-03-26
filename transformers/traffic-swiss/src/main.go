// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationType = "TrafficSensor"
	Origin      = "FEDRO"
	Period      = 600 // 10 minutes in seconds
)

var trafficDataTypes = []bdplib.DataType{
	bdplib.CreateDataType("average-speed-light-vehicles", "km/h", "Average speed light vehicles (10 min mean)", "Mean"),
	bdplib.CreateDataType("average-speed-heavy-vehicles", "km/h", "Average speed heavy vehicles (10 min mean)", "Mean"),
	bdplib.CreateDataType("average-speed", "km/h", "Average speed all vehicles (10 min mean)", "Mean"),
	bdplib.CreateDataType("average-flow-light-vehicles", "veh", "Traffic flow light vehicles (10 min sum)", "Total"),
	bdplib.CreateDataType("average-flow-heavy-vehicles", "veh", "Traffic flow heavy vehicles (10 min sum)", "Total"),
	bdplib.CreateDataType("average-flow", "veh", "Traffic flow all vehicles (10 min sum)", "Total"),
}

var env struct {
	tr.Env
	bdplib.BdpEnv
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting tr-traffic-swiss...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	err := b.SyncDataTypes(trafficDataTypes)
	ms.FailOnError(ctx, err, "failed to sync data types")

	agg := NewAggregator()

	listener := tr.NewTr[string](ctx, env.Env)
	err = listener.Start(ctx, MultiFormatMiddleware[Root](TransformWithBdp(b, agg)))
	ms.FailOnError(ctx, err, "error while listening to queue")
}

// TransformWithBdp returns a Handler that calls Transform with the given BDP client and aggregator.
func TransformWithBdp(bdp bdplib.Bdp, agg *Aggregator) tr.Handler[Root] {
	return func(ctx context.Context, payload *rdb.Raw[Root]) error {
		return Transform(ctx, bdp, agg, payload)
	}
}

// Transform maps the collector payload to ODH BDP API calls.
// Raw 1-minute measurements are aggregated over a 10-sample window before being pushed.
func Transform(ctx context.Context, bdp bdplib.Bdp, agg *Aggregator, payload *rdb.Raw[Root]) error {
	root := payload.Rawdata

	// 1. Build stations
	stations := make([]bdplib.Station, 0, len(root.Stations))
	for _, s := range root.Stations {
		station := bdplib.CreateStation(s.ID, s.ID, StationType, s.Lat, s.Lon, bdp.GetOrigin())
		station.MetaData = make(map[string]interface{}, len(s.Metadata))
		for k, v := range s.Metadata {
			station.MetaData[k] = v
		}
		stations = append(stations, station)
	}

	// 2. Sync stations
	if err := bdp.SyncStations(StationType, stations, true, false); err != nil {
		return err
	}

	// 3. Aggregate raw measurements and push completed windows
	if len(root.Measurements) == 0 {
		return nil
	}

	dataMap := bdp.CreateDataMap()
	for _, m := range root.Measurements {
		aggVal, aggTs, done := agg.Add(m.StationID, m.DataType, m.Value, m.Timestamp)
		if done {
			dataMap.AddRecord(
				m.StationID,
				m.DataType,
				bdplib.CreateRecord(aggTs.UnixMilli(), aggVal, Period),
			)
		}
	}

	if len(dataMap.Branch) == 0 {
		return nil
	}

	return bdp.PushData(StationType, dataMap)
}

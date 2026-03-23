// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
)

var env struct {
	dc.Env
	CRON              string `default:"20 * * * * *"`
	CRON_STATIC       string `default:"0 0 2 * * *"`
	REALTIME_URL      string
	STATIC_URL        string
	AUTH_BEARER_TOKEN string
}

var (
	stationsMu sync.RWMutex
	stations   = make(map[string]StationDTO)
	charIndex  = make(map[string]map[string]string) // stationID → measurementIndex → odhDataType
)

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting dc-traffic-swiss...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)
	aggregator := NewAggregator()

	// Fetch static data once at startup so stations are available immediately.
	runStaticCollection(context.Background(), collector)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON_STATIC, func() {
		runStaticCollection(context.Background(), collector)
	})
	c.AddFunc(env.CRON, func() {
		runRealtimeCollection(context.Background(), collector, aggregator)
	})
	c.Run()
}

func runStaticCollection(ctx context.Context, collector *dc.Dc[dc.EmptyData]) {
	ctx, col := collector.StartCollection(ctx)
	defer col.End(ctx)

	data, err := FetchURL(env.STATIC_URL, "")
	if err != nil {
		slog.Error("static fetch failed", "err", err)
		return
	}

	parsed, chars, err := ParseStaticXML(data)
	if err != nil {
		slog.Error("static XML parse failed", "err", err)
		return
	}

	stationsMu.Lock()
	for _, s := range parsed {
		stations[s.ID] = s
	}
	for id, m := range chars {
		charIndex[id] = m
	}
	stationsMu.Unlock()

	root := Root{Stations: snapshotStations(), Measurements: nil}
	publishRoot(ctx, col, root)
	slog.Info("static collection complete", "stations", len(parsed))
}

func runRealtimeCollection(ctx context.Context, collector *dc.Dc[dc.EmptyData], agg *Aggregator) {
	ctx, col := collector.StartCollection(ctx)
	defer col.End(ctx)

	data, err := FetchSOAP(env.REALTIME_URL, realtimeSoapAction, realtimeSoapBody, env.AUTH_BEARER_TOKEN)
	if err != nil {
		slog.Error("realtime fetch failed", "err", err)
		return
	}

	siteMeasurements, err := ParseRealtimeXML(data)
	if err != nil {
		slog.Error("realtime XML parse failed", "err", err)
		return
	}

	stationsMu.RLock()
	chars := charIndex
	stationsMu.RUnlock()

	var measurements []MeasurementDTO
	for _, sm := range siteMeasurements {
		ts, err := time.Parse(time.RFC3339, sm.TimeDefault)
		if err != nil {
			slog.Warn("bad timestamp in realtime feed", "raw", sm.TimeDefault, "err", err)
			ts = time.Now()
		}

		idxMap, ok := chars[sm.SiteRef.ID]
		if !ok {
			continue
		}

		for _, mv := range sm.Values {
			dt, ok := idxMap[mv.Index]
			if !ok {
				continue
			}

			var value float64
			if strings.Contains(dt, "flow") {
				value = mv.VehicleFlowRate
			} else {
				value = mv.SpeedValue
			}

			aggVal, aggTs, done := agg.Add(sm.SiteRef.ID, dt, value, ts)
			if done {
				measurements = append(measurements, MeasurementDTO{
					StationID: sm.SiteRef.ID,
					DataType:  dt,
					Value:     aggVal,
					Timestamp: aggTs,
				})
			}
		}
	}

	if len(measurements) == 0 {
		return
	}

	stationsMu.RLock()
	allStations := snapshotStations()
	stationsMu.RUnlock()

	root := Root{Stations: allStations, Measurements: measurements}
	publishRoot(ctx, col, root)
	slog.Info("realtime collection: published aggregated measurements", "count", len(measurements))
}

// snapshotStations returns a slice copy of all known stations.
// Caller must NOT hold stationsMu (or must hold at least RLock).
func snapshotStations() []StationDTO {
	result := make([]StationDTO, 0, len(stations))
	for _, s := range stations {
		result = append(result, s)
	}
	return result
}

func publishRoot(ctx context.Context, col *dc.Collection, root Root) {
	jsonBytes, err := json.Marshal(root)
	if err != nil {
		slog.Error("marshal root failed", "err", err)
		return
	}
	if err := col.Publish(ctx, &rdb.RawAny{
		Provider:  env.PROVIDER,
		Timestamp: time.Now(),
		Rawdata:   string(jsonBytes),
	}); err != nil {
		slog.Error("publish failed", "err", err)
	}
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const (
	stationType  = "ParkingStation"
	dataTypeFree = "free"
	period       = 120
	municipality = "valgardena"

	// Station codes are published as "<origin>:<provider id>" and are already
	// live in the timeseries, so the prefix is pinned here rather than taken
	// from BDP_ORIGIN: changing it would orphan every existing station.
	origin = "GARDENA"
)

var env struct {
	tr.Env

	MQ_META_QUEUE    string
	MQ_META_EXCHANGE string `default:"routed"`
	MQ_META_KEY      string
	MQ_META_CLIENT   string
}

type ParkingData struct {
	Uid string `json:"id"`
	// Time is the provider's own clock. It is not used as the record timestamp:
	// the provider pins the minute field (:45, :59) so it lags by up to 15
	// minutes. The collection timestamp is authoritative.
	Time string `json:"timestamp"`
	// Occupancy is deprecated as per email from 25.03.2026, replaced by free_slots which should do the same thing
	Occupancy int `json:"occupancy"`
	Free      int `json:"free_slots"`
}

type ParkingMetadata struct {
	Uid      string  `json:"id"`
	NameDE   string  `json:"name_DE"`
	NameIT   string  `json:"name_IT"`
	Lat      float64 `json:"latitude"`
	Long     float64 `json:"longitude"`
	Capacity int     `json:"capacity"`
	Provider string  `json:"provider"`
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	ms.InitWithEnv(ctx, "", &enrichEnv)
	ms.InitWithEnv(ctx, "", &discoveryEnv)
	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(bdplib.BdpEnv{
		BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
		BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
		BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
		BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
		BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),
		BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
		BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
	})

	ms.FailOnError(ctx, syncDataTypes(b), "failed to sync data types")

	ms.FailOnError(ctx, startEnrichment(ctx, env.Env), "failed to start the enrichment table")
	defer stopEnrichment()
	ms.FailOnError(ctx, startDiscovery(ctx, env.Env), "failed to start the discovery reporter")

	data := tr.NewTr[string](ctx, env.Env)
	meta := tr.NewTr[string](ctx, metaEnv())

	// Start blocks until its AMQP channel closes, which no listener can recover
	// from on its own. Report whichever stops first and let the pod restart onto
	// a fresh connection instead of idling with no consumer.
	stopped := make(chan error, 2)
	go func() {
		stopped <- data.Start(ctx, tr.RawString2JsonMiddleware[ParkingData](transformDataWithBdp(b)))
	}()
	go func() {
		stopped <- meta.Start(ctx, tr.RawString2JsonMiddleware[[]ParkingMetadata](transformMetadataWithBdp(b)))
	}()

	ms.FailOnError(ctx, <-stopped, "queue listener stopped")
}

// metaEnv reuses the broker connection settings of the data queue and overrides
// only what identifies the metadata queue.
func metaEnv() tr.Env {
	e := env.Env
	e.MQ_QUEUE = env.MQ_META_QUEUE
	e.MQ_EXCHANGE = env.MQ_META_EXCHANGE
	e.MQ_KEY = env.MQ_META_KEY
	e.MQ_CLIENT = env.MQ_META_CLIENT
	return e
}

func syncDataTypes(b bdplib.Bdp) error {
	return b.SyncDataTypes([]bdplib.DataType{
		bdplib.CreateDataType(dataTypeFree, "count", "Number of free parking slots", "Instantaneous"),
	})
}

func transformDataWithBdp(bdp bdplib.Bdp) tr.Handler[ParkingData] {
	return func(ctx context.Context, payload *rdb.Raw[ParkingData]) error {
		return TransformData(ctx, bdp, payload)
	}
}

func TransformData(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[ParkingData]) error {
	// The station is withheld by the metadata queue, so it does not exist for
	// the writer to attach a measurement to.
	station := stationId(payload.Rawdata.Uid)
	if enrichEnv.REFTABLE_GATE && !hasRecord(station) {
		logger.Get(ctx).Debug("skipping occupancy of a station without enrichment record", "station", station)
		return nil
	}

	dm := bdp.CreateDataMap()
	dm.AddRecord(station, dataTypeFree,
		bdplib.CreateRecord(payload.Timestamp.UnixMilli(), payload.Rawdata.Free, period))

	if err := bdp.PushData(stationType, dm); err != nil {
		return fmt.Errorf("failed pushing parking occupancy: %w", err)
	}

	logger.Get(ctx).Debug("pushed parking occupancy", "station", station, "free", payload.Rawdata.Free)
	return nil
}

func transformMetadataWithBdp(bdp bdplib.Bdp) tr.Handler[[]ParkingMetadata] {
	return func(ctx context.Context, payload *rdb.Raw[[]ParkingMetadata]) error {
		return TransformMetadata(ctx, bdp, payload)
	}
}

func TransformMetadata(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[[]ParkingMetadata]) error {
	log := logger.Get(ctx)
	stations := make([]bdplib.Station, 0, len(payload.Rawdata))
	for _, m := range payload.Rawdata {
		s := bdplib.CreateStation(stationId(m.Uid), m.NameIT, stationType, m.Lat, m.Long, origin)
		s.MetaData = map[string]interface{}{
			"name_IT":      m.NameIT,
			"name_DE":      m.NameDE,
			"capacity":     m.Capacity,
			"municipality": municipality,
		}

		entry := discoveryEntry(s, "", map[string]any{
			"capacity": m.Capacity,
			"name_de":  m.NameDE,
			"provider": m.Provider,
		})
		// Records are keyed by station code, the control tower's default
		// provider-id path; nothing else on the station carries the uid alone.
		key := s.Id
		entry.Reason = "no-record"
		if hasRecord(key) {
			entry.Reason = ""
		} else if enrichEnv.REFTABLE_GATE {
			log.Warn("withholding station without enrichment record", "id", m.Uid, "name", m.NameIT)
			entry.Status = discovery.StatusWithheld
			observe(key, entry)
			continue
		}
		entry.Status = discovery.StatusPublished
		observe(key, entry)

		enrichStation(key, &s)
		stations = append(stations, s)
	}

	// An empty list is a provider glitch, not an empty car park set.
	reportDiscovery(ctx, len(payload.Rawdata) > 0)

	if err := bdp.SyncStations(stationType, stations, true, false); err != nil {
		return fmt.Errorf("failed syncing parking stations: %w", err)
	}

	logger.Get(ctx).Debug("synced parking stations", "count", len(stations))
	return nil
}

func stationId(id string) string {
	return fmt.Sprintf("%s:%s", origin, id)
}

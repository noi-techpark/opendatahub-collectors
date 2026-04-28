// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	SOURCE      = "fuchtstechnik"
	ID_TEMPLATE = "urn:stations:fuchtstechnik"

	stationType = "ParkingStation"

	dataTypeFree     = "free"
	dataTypeOccupied = "occupied"

	// measurementTimestampLayout matches "2026-03-30 11:07:58" (UTC assumed).
	measurementTimestampLayout = "2006-01-02 15:04:05"

	measurementPeriod = 0
)

var env tr.Env
var stations Stations

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting parking-offstreet-fuchtstechnik transformer...")

	b := bdplib.FromEnv(bdplib.BdpEnv{
		BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
		BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
		BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
		BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
		BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),
		BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
		BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
	})
	defer tel.FlushOnPanic()

	stations = ReadStations("../resources/stations.csv")

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync types")

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[string](context.Background(), env)
	err = listener.Start(context.Background(), Base64Decode(TransformWithBdp(b)))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Base64Decode[P any](h tr.Handler[P]) tr.Handler[string] {
	return func(ctx context.Context, r *rdb.Raw[string]) error {
		pRaw := rdb.Raw[P]{Provider: r.Provider, Timestamp: r.Timestamp}
		decoded, err := base64.StdEncoding.DecodeString(r.Rawdata)
		if err != nil {
			return fmt.Errorf("middleware failed decode base64 rawdata string: %w", err)
		}
		err = json.Unmarshal([]byte(decoded), &pRaw.Rawdata)
		if err != nil {
			return fmt.Errorf("middleware failed parsing rawdata string to json: %w", err)
		}
		return h(ctx, &pRaw)
	}
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[ParkingEvent] {
	return func(ctx context.Context, payload *rdb.Raw[ParkingEvent]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[ParkingEvent]) error {
	event := payload.Rawdata

	slog.Info("Processing parking event",
		"id", event.Id,
		"capacity", event.Capacity,
		"measurements", len(event.Measurements))

	if event.Id == "" {
		return fmt.Errorf("empty event id")
	}

	station := buildStation(bdp, event)

	dataMap := bdp.CreateDataMap()
	for _, m := range event.Measurements {
		ts, err := time.Parse(measurementTimestampLayout, m.Timestamp)
		if err != nil {
			return fmt.Errorf("failed to parse measurement timestamp %q: %w", m.Timestamp, err)
		}
		tsMs := ts.UnixMilli()

		free := m.Availability
		occupied := event.Capacity - m.Availability

		dataMap.AddRecord(station.Id, dataTypeFree, bdplib.CreateRecord(tsMs, free, measurementPeriod))
		dataMap.AddRecord(station.Id, dataTypeOccupied, bdplib.CreateRecord(tsMs, occupied, measurementPeriod))
	}

	if err := bdp.SyncStations(stationType, []bdplib.Station{station}, true, true); err != nil {
		return fmt.Errorf("failed to sync station: %w", err)
	}
	if err := bdp.PushData(stationType, dataMap); err != nil {
		return fmt.Errorf("failed to push data: %w", err)
	}
	return nil
}

func buildStation(bdp bdplib.Bdp, event ParkingEvent) bdplib.Station {
	urn := clib.GenerateID(ID_TEMPLATE, event.Id)

	// Prefer Italian name, fall back to German, then id.
	name := event.NameIT
	if name == "" {
		name = event.NameDE
	}
	if name == "" {
		name = event.Id
	}

	station := bdplib.CreateStation(
		urn, name,
		stationType, event.Latitude, event.Longitude, bdp.GetOrigin())

	// Base metadata: inline event names + capacity + provider id.
	meta := map[string]any{
		"provider_id": event.Id,
		"capacity":    event.Capacity,
	}
	if event.NameIT != "" {
		meta["name_it"] = event.NameIT
	}
	if event.NameDE != "" {
		meta["name_de"] = event.NameDE
	}

	// Merge NeTEx / richer metadata from stations.csv if available.
	if data := stations.GetStationByID(event.Id); data != nil {
		for k, v := range data.ToMetadata() {
			// Do not overwrite inline event values for name_it / name_de.
			if _, exists := meta[k]; !exists {
				meta[k] = v
			}
		}
	}

	station.MetaData = meta
	return station
}

func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(dataTypeFree, "", "Amount of free parking slots", "Instantaneous"),
		bdplib.CreateDataType(dataTypeOccupied, "", "Amount of occupied parking slots", "Instantaneous"),
	}
	return bdp.SyncDataTypes(dataTypes)
}

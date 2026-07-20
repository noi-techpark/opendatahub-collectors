// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const (
	StationTypeParkingStation = "ParkingStation"

	// IDTemplate is the URN prefix for deterministically deriving the BDP
	// station code from the provider's site id via clib.GenerateID (UUIDv5).
	IDTemplate = "urn:parking:affluences"

	// Affluences elaborates a fresh occupancy roughly every 30 minutes; we
	// poll more often and the measurements are idempotent (same provider
	// timestamp overwrites). Keep the declared sampling period constant.
	Period = 600

	DataTypeOccupancy = "occupancy"
	DataTypeFree      = "free"
	DataTypeOpen      = "open"
)

// env: tr.Env carries the MQ/raw-data-bridge wiring, bdplib.BdpEnv the BDP
// writer + OAuth config. Both are populated from environment variables by
// ms.InitWithEnv (BDP_* names bound to the oauth-collector secret at deploy).
var env struct {
	tr.Env
	bdplib.BdpEnv
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	logger.Get(ctx).Info("Starting parking-affluences transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	logger.Get(ctx).Info("Syncing data types on startup")
	ms.FailOnError(ctx, syncDataTypes(b), "failed to sync data types")

	logger.Get(ctx).Info("Starting transformer listener...")
	listener := tr.NewTr[string](ctx, env.Env)
	err := listener.Start(ctx, tr.RawString2JsonMiddleware[AffluencesPayload](TransformWithBdp(b)))
	ms.FailOnError(ctx, err, "error while listening to queue")
}

func syncDataTypes(b bdplib.Bdp) error {
	return b.SyncDataTypes([]bdplib.DataType{
		bdplib.CreateDataType(DataTypeOccupancy, "count", "Number of occupied parking spaces", "Instantaneous"),
		bdplib.CreateDataType(DataTypeFree, "count", "Number of free parking spaces", "Instantaneous"),
		bdplib.CreateDataType(DataTypeOpen, "", "Whether the parking station is currently open (boolean stored as string)", "Instantaneous"),
	})
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[AffluencesPayload] {
	return func(ctx context.Context, payload *rdb.Raw[AffluencesPayload]) error {
		return Transform(ctx, bdp, payload)
	}
}

// Transform maps one merged Affluences poll into ParkingStation master data
// plus occupancy / free / open measurements. Stations are derived from the
// payload on every message (the payload always carries the full site set),
// so they are synced together with the measurements.
func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[AffluencesPayload]) error {
	log := logger.Get(ctx)

	// Sort by id so station lists and measurement order are deterministic
	// (snapshot stability), independent of the order the collector emitted.
	sites := make([]Site, len(payload.Rawdata))
	copy(sites, payload.Rawdata)
	sort.Slice(sites, func(i, j int) bool { return sites[i].ID < sites[j].ID })

	stations := make([]bdplib.Station, 0, len(sites))
	dm := bdp.CreateDataMap()

	for _, site := range sites {
		if site.ID == "" {
			log.Warn("skipping site with empty id")
			continue
		}

		// BDP station code: deterministic UUIDv5 URN derived from the
		// provider site id (never the raw provider id). Same code is used
		// for the station and all its records.
		code := clib.GenerateID(IDTemplate, site.ID)
		stations = append(stations, buildStation(bdp, code, site))

		rt := site.Realtime
		ts := measurementTimestamp(rt.OccupancyDatetimeUTC, payload.Timestamp)

		// "open" is always available; store the boolean as a string.
		dm.AddRecord(code, DataTypeOpen, bdplib.CreateRecord(ts, strconv.FormatBool(rt.Open), Period))

		// occupancy and free need both occupancy and capacity to be present.
		if rt.Occupancy == nil || rt.Capacity == nil {
			log.Warn("realtime occupancy/capacity missing; only pushing 'open'",
				"station", site.ID, "name", site.PrimaryName)
			continue
		}

		occupancy := *rt.Occupancy
		capacity := *rt.Capacity
		free := capacity - occupancy

		// Guard against out-of-range provider values (e.g. occupancy above
		// capacity, or a negative occupancy) so we never publish an
		// impossible count.
		if occupancy < 0 || occupancy > capacity || free < 0 {
			log.Warn("clamping out-of-range occupancy into [0, capacity]",
				"station", site.ID, "occupancy", occupancy, "capacity", capacity)
			occupancy = clampInt(occupancy, capacity)
			free = clampInt(free, capacity)
		}

		dm.AddRecord(code, DataTypeOccupancy, bdplib.CreateRecord(ts, occupancy, Period))
		dm.AddRecord(code, DataTypeFree, bdplib.CreateRecord(ts, free, Period))
	}

	log.Info("Syncing stations and pushing measurements", "stations", len(stations))
	if err := bdp.SyncStations(StationTypeParkingStation, stations, true, false); err != nil {
		return fmt.Errorf("failed to sync stations: %w", err)
	}
	if err := bdp.PushData(StationTypeParkingStation, dm); err != nil {
		return fmt.Errorf("failed to push measurements: %w", err)
	}
	return nil
}

// buildStation maps Affluences site metadata to a flat ParkingStation. The
// station code is the deterministic URN passed in (derived via
// clib.GenerateID); the raw provider site id is kept in metadata for
// traceability.
func buildStation(bdp bdplib.Bdp, code string, site Site) bdplib.Station {
	station := bdplib.CreateStation(
		code,
		site.PrimaryName,
		StationTypeParkingStation,
		site.Location.Coordinates.Latitude,
		site.Location.Coordinates.Longitude,
		bdp.GetOrigin(),
	)

	meta := map[string]any{"provider_id": site.ID}
	if site.Realtime.Capacity != nil {
		meta["capacity"] = *site.Realtime.Capacity
	}
	if site.SecondaryName != "" {
		meta["secondary_name"] = site.SecondaryName
	}
	if site.Timezone != "" {
		meta["timezone"] = site.Timezone
	}
	if addr := buildAddress(site.Location.Address); len(addr) > 0 {
		meta["address"] = addr
	}
	if len(site.Categories) > 0 {
		names := make([]string, 0, len(site.Categories))
		for _, c := range site.Categories {
			names = append(names, c.Name)
		}
		meta["categories"] = names
	}
	if site.PhoneNumber != nil {
		meta["phone_number"] = *site.PhoneNumber
	}
	if site.Email != nil {
		meta["email"] = *site.Email
	}
	if site.URL != nil {
		meta["url"] = *site.URL
	}

	station.MetaData = meta
	return station
}

// buildAddress collects the non-null address fields into a metadata map.
func buildAddress(a Address) map[string]any {
	out := map[string]any{}
	if a.Route != nil {
		out["route"] = *a.Route
	}
	if a.City != nil {
		out["city"] = *a.City
	}
	if a.ZipCode != nil {
		out["zip_code"] = *a.ZipCode
	}
	if a.Region != nil {
		out["region"] = *a.Region
	}
	if a.CountryCode != nil {
		out["country_code"] = *a.CountryCode
	}
	return out
}

// measurementTimestamp uses the provider's own measurement time
// (occupancy_datetime_utc) when available so re-polls between provider
// updates land on the same timestamp (idempotent overwrite), falling back to
// the ingestion time when it is missing or unparseable.
func measurementTimestamp(raw string, fallback time.Time) int64 {
	if raw != "" {
		if t, err := time.Parse(time.RFC3339, raw); err == nil {
			return t.UnixMilli()
		}
	}
	return fallback.UnixMilli()
}

// clampInt constrains v to [0, hi]. If hi is negative it collapses to 0.
func clampInt(v, hi int) int {
	if hi < 0 {
		hi = 0
	}
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}

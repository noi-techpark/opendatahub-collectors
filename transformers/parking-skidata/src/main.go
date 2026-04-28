// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	SOURCE      = "skidata"
	ID_TEMPLATE = "urn:parking:skidata"

	stationTypeParent = "ParkingFacility"
	stationType       = "ParkingStation"

	measurementPeriod = 600
)

var env struct {
	tr.Env

	// Time-series API used to hydrate the in-memory cache at startup.
	// The same OAuth client_id/secret is used for both BDP writes and
	// timeseries reads (mirrors the pattern in people-flow-systems-me).
	TS_API_BASE_URL string `default:""`
	TS_API_TOKEN_URL string `default:""`
	TS_API_REFERER  string `default:"tr-parking-skidata"`
}

var stations Stations
var categories CountingCategories
var cache *Cache
var urnToProviderID map[string]string

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting parking-skidata transformer...")

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

	loadResources("../resources")

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync types")

	slog.Info("Syncing all stations on startup")
	err = syncAllStations(b)
	ms.FailOnError(context.Background(), err, "failed to sync stations")

	cache = NewCache()
	urnToProviderID = buildURNIndex(stations)

	if env.TS_API_BASE_URL != "" {
		ts := odhts.NewCustomClient(env.TS_API_BASE_URL, env.TS_API_TOKEN_URL, env.TS_API_REFERER)
		ts.UseAuth(os.Getenv("ODH_CLIENT_ID"), os.Getenv("ODH_CLIENT_SECRET"))
		datatypes := allDataTypeNames(categories)
		if hErr := hydrateCache(cache, ts, os.Getenv("BDP_ORIGIN"), datatypes, urnToProviderID); hErr != nil {
			// Hydration is best-effort: continue with an empty cache.
			slog.Warn("Cache hydration failed; starting empty", "err", hErr)
		}
	} else {
		slog.Info("TS_API_BASE_URL unset; skipping cache hydration")
	}

	slog.Info("Starting transformer listener...")
	listener := tr.NewTr[ParkingEvent](context.Background(), env.Env)
	err = listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// buildURNIndex maps each known station's URN back to its provider id.
// The cache is keyed by provider id, but BDP returns URNs (the value we
// passed to bdplib.CreateStation). Hydration uses this lookup to convert
// the BDP-side scode into our cache key.
func buildURNIndex(s Stations) map[string]string {
	out := make(map[string]string, len(s))
	for _, row := range s {
		out[clib.GenerateID(ID_TEMPLATE, row.ID)] = row.ID
	}
	return out
}

// loadResources populates the package-level stations and categories
// slices from CSV files under resourcesDir. It always loads the base
// stations.csv and counting_categories.csv, then merges in an optional
// overlay selected by the RESOURCES_OVERLAY env variable. For example,
// RESOURCES_OVERLAY=test appends rows from stations.test.csv and
// counting_categories.test.csv on top of the base files. Unset/empty
// loads only the base CSVs (production behaviour).
func loadResources(resourcesDir string) {
	stations = ReadStations(resourcesDir + "/stations.csv")
	categories = ReadCountingCategories(resourcesDir + "/counting_categories.csv")

	overlay := os.Getenv("RESOURCES_OVERLAY")
	if overlay == "" {
		slog.Info("No RESOURCES_OVERLAY set; loading base CSVs only",
			"stations", len(stations), "categories", len(categories))
		return
	}
	suffix := "." + overlay + ".csv"
	overlayStations := ReadStationsOptional(resourcesDir + "/stations" + suffix)
	overlayCategories := ReadCountingCategoriesOptional(resourcesDir + "/counting_categories" + suffix)
	stations = append(stations, overlayStations...)
	categories = append(categories, overlayCategories...)
	slog.Info("Loaded CSVs with overlay",
		"overlay", overlay,
		"stations", len(stations), "extra_stations", len(overlayStations),
		"categories", len(categories), "extra_categories", len(overlayCategories))
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[ParkingEvent] {
	return func(ctx context.Context, payload *rdb.Raw[ParkingEvent]) error {
		return Transform(ctx, bdp, payload)
	}
}

// Transform handles a single Skidata push event:
//  1. Updates the in-memory cache with the event's per-category value.
//  2. Pushes the per-category measurement on the carpark URN.
//  3. Recomputes and pushes the carpark "overall" (cat-3 if cached, else
//     sum of non-3 categories) on the carpark URN.
//  4. Recomputes and pushes the facility's overall + per-category totals
//     (sum across all carparks of the facility) on the facility URN.
//
// Stations themselves are already synced at startup with full
// per-category capacity metadata, so the per-event handler only emits
// measurements.
func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[ParkingEvent]) error {
	event := payload.Rawdata
	ts := payload.Timestamp.UnixMilli()

	slog.Info("Processing parking event",
		"facilityNr", event.Carpark.FacilityNr,
		"carparkId", event.Carpark.Id,
		"category", event.CountingCategoryId,
		"level", event.Level, "capacity", event.Capacity)

	parentProviderID := fmt.Sprintf("%07d", event.Carpark.FacilityNr)
	childProviderID := fmt.Sprintf("%s_%d", parentProviderID, event.Carpark.Id)

	// Resolve the descriptor for this category. Prefer the CSV row's
	// human-readable name (which carries the slug for unknown ids), but
	// fall back to the event's own name if no CSV row is found.
	row := categories.Find(parentProviderID, event.Carpark.Id, event.CountingCategoryId)
	name := event.Name
	if row != nil {
		name = row.Name
	}
	d := descriptorFor(event.CountingCategoryId, name)

	free := event.Capacity - event.Level
	occupied := event.Level

	parentID := clib.GenerateID(ID_TEMPLATE, parentProviderID)
	childID := clib.GenerateID(ID_TEMPLATE, childProviderID)

	// 1. Update cache.
	cache.Set(childProviderID, d.freeType(), free, ts)
	cache.Set(childProviderID, d.occupiedType(), occupied, ts)

	// 2. Per-category carpark measurement.
	carparkData := bdp.CreateDataMap()
	carparkData.AddRecord(childID, d.freeType(), bdplib.CreateRecord(ts, free, measurementPeriod))
	carparkData.AddRecord(childID, d.occupiedType(), bdplib.CreateRecord(ts, occupied, measurementPeriod))

	// 3. Carpark overall (cat-3 if present, else sum of non-3).
	//    Skip when the event itself is cat 3 — its per-category push
	//    already lands on `free`/`occupied` (same datatype, same value).
	if d.suffix != "" {
		if v, ok := cache.CarparkOverall(childProviderID, "free"); ok {
			carparkData.AddRecord(childID, "free", bdplib.CreateRecord(ts, v, measurementPeriod))
		}
		if v, ok := cache.CarparkOverall(childProviderID, "occupied"); ok {
			carparkData.AddRecord(childID, "occupied", bdplib.CreateRecord(ts, v, measurementPeriod))
		}
	}

	if err := bdp.PushData(stationType, carparkData); err != nil {
		return fmt.Errorf("failed to push carpark data: %w", err)
	}

	// 4. Facility-level aggregates (overall + per-category).
	facilityData := bdp.CreateDataMap()
	facilityData.AddRecord(parentID, "free", bdplib.CreateRecord(ts, cache.FacilityOverall(parentProviderID, "free"), measurementPeriod))
	facilityData.AddRecord(parentID, "occupied", bdplib.CreateRecord(ts, cache.FacilityOverall(parentProviderID, "occupied"), measurementPeriod))
	// Per-category facility totals — skip cat 3 because that's already
	// the overall (would push the same record twice on the parent URN).
	if d.suffix != "" {
		facilityData.AddRecord(parentID, d.freeType(), bdplib.CreateRecord(ts, cache.FacilityPerCategory(parentProviderID, d.freeType()), measurementPeriod))
		facilityData.AddRecord(parentID, d.occupiedType(), bdplib.CreateRecord(ts, cache.FacilityPerCategory(parentProviderID, d.occupiedType()), measurementPeriod))
	}

	if err := bdp.PushData(stationTypeParent, facilityData); err != nil {
		return fmt.Errorf("failed to push facility data: %w", err)
	}
	return nil
}

// syncAllStations sends every known ParkingFacility (parent) and
// ParkingStation (child) to BDP with full per-category capacity/limit
// metadata. Parent stations carry aggregated facility-level capacities
// summed across all their carparks.
func syncAllStations(bdp bdplib.Bdp) error {
	parents := []bdplib.Station{}
	children := []bdplib.Station{}

	// Group station rows by the station_type column written by the
	// sync-stations script (ParkingFacility for parents, ParkingStation
	// for carparks). Rows missing the column are warned and skipped.
	for _, row := range stations {
		switch row.StationType {
		case stationTypeParent:
			parents = append(parents, buildParentStation(bdp, row))
		case stationType:
			child, ok := buildChildStation(bdp, row)
			if !ok {
				continue
			}
			children = append(children, child)
		default:
			slog.Warn("Skipping CSV row with unknown station_type",
				"id", row.ID, "station_type", row.StationType)
		}
	}

	slog.Info("Syncing stations to BDP",
		"parking_facilities", len(parents),
		"parking_stations", len(children))

	if err := bdp.SyncStations(stationTypeParent, parents, true, false); err != nil {
		return fmt.Errorf("sync parents: %w", err)
	}
	if err := bdp.SyncStations(stationType, children, true, false); err != nil {
		return fmt.Errorf("sync children: %w", err)
	}
	return nil
}

// buildParentStation builds a ParkingFacility station with NeTEx
// metadata plus per-category capacity/limits aggregated across all of
// its carparks (static, computed once at startup from the CSVs).
// Live aggregated measurements are NOT emitted by this transformer —
// events arrive per carpark and the transformer is stateless; the
// per-category sum of free/occupied across carparks is computed
// downstream.
func buildParentStation(bdp bdplib.Bdp, row Station) bdplib.Station {
	id := clib.GenerateID(ID_TEMPLATE, row.ID)
	station := bdplib.CreateStation(id, row.Name, stationTypeParent, row.Lat, row.Lon, bdp.GetOrigin())
	meta := row.ToMetadata()
	meta["provider_id"] = row.ID

	// Aggregate per-category capacity / limits across all carparks.
	totals := map[string]int{}
	for _, cat := range categories.ForFacility(row.ID) {
		d := descriptorFor(cat.CountingCategoryId, cat.Name)
		totals[d.metaKey("capacity")] += cat.Capacity
		totals[d.metaKey("occupancy_limit")] += cat.OccupancyLimit
		totals[d.metaKey("free_limit")] += cat.FreeLimit
	}
	for k, v := range totals {
		meta[k] = v
	}

	station.MetaData = meta
	return station
}

func buildChildStation(bdp bdplib.Bdp, row Station) (bdplib.Station, bool) {
	if row.ParentID == "" {
		slog.Warn("Skipping ParkingStation row with empty parent_id", "id", row.ID)
		return bdplib.Station{}, false
	}

	id := clib.GenerateID(ID_TEMPLATE, row.ID)
	parentID := clib.GenerateID(ID_TEMPLATE, row.ParentID)

	station := bdplib.CreateStation(id, row.Name, stationType, row.Lat, row.Lon, bdp.GetOrigin())
	station.ParentStation = parentID

	meta := row.ToMetadata()
	meta["provider_id"] = row.ID
	meta["facility_id"] = row.ParentID
	meta["carpark_id"] = row.CarparkID

	for _, cat := range categories.ForCarpark(row.ParentID, row.CarparkID) {
		d := descriptorFor(cat.CountingCategoryId, cat.Name)
		meta[d.metaKey("capacity")] = cat.Capacity
		meta[d.metaKey("occupancy_limit")] = cat.OccupancyLimit
		meta[d.metaKey("free_limit")] = cat.FreeLimit
	}

	station.MetaData = meta
	return station, true
}

// syncDataTypes registers BDP data types for every category suffix
// observed in counting_categories.csv (plus the standard short_stay /
// subscribers / total trio, in case the CSV is empty during bootstrap).
func syncDataTypes(bdp bdplib.Bdp) error {
	suffixes := map[string]bool{
		"":            true, // total
		"short_stay":  true,
		"subscribers": true,
	}
	for _, cat := range categories {
		d := descriptorFor(cat.CountingCategoryId, cat.Name)
		suffixes[d.suffix] = true
	}

	keys := make([]string, 0, len(suffixes))
	for k := range suffixes {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	dataTypes := []bdplib.DataType{}
	for _, suffix := range keys {
		d := catDescriptor{suffix: suffix}
		var label string
		if suffix == "" {
			label = "parking slots"
		} else {
			label = "'" + strings.ReplaceAll(suffix, "_", " ") + "' parking slots"
		}
		dataTypes = append(dataTypes,
			bdplib.CreateDataType(d.freeType(), "", "Amount of free "+label, "Instantaneous"),
			bdplib.CreateDataType(d.occupiedType(), "", "Amount of occupied "+label, "Instantaneous"),
		)
	}
	return bdp.SyncDataTypes(dataTypes)
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const (
	StationTypeParkingStation  = "ParkingStation"
	StationTypeParkingFacility = "ParkingFacility"

	// The collector polls every 5 minutes (see the api-crawler helm values).
	PERIOD = 300

	DataTypeFree     = "free"
	DataTypeOccupied = "occupied"
)

// spaceTypeSlug maps the SMI ParkingSpaceType enumeration to the suffix used
// for the per-type data types (free_<slug> / occupied_<slug>). The "total"
// entry maps to the base free/occupied types.
var spaceTypeSlug = map[string]string{
	"total":       "",
	"shortterm":   "short_term",
	"longterm":    "long_term",
	"charging":    "charging",
	"handicapped": "handicapped",
	"woman":       "woman",
	"familiy":     "family", // typo in the SMI XSD enumeration
	"family":      "family",
	"extraLarge":  "extra_large",
}

var env struct {
	tr.Env
	bdplib.BdpEnv
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	log := logger.Get(ctx)
	log.Info("Starting parking-swarco transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	ms.FailOnError(ctx, syncDataTypes(b), "failed to sync data types")

	listener := tr.NewTr[string](ctx, env.Env)
	err := listener.Start(ctx, tr.RawString2JsonMiddleware[SwarcoData](TransformWithBdp(b)))
	ms.FailOnError(ctx, err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[SwarcoData] {
	return func(ctx context.Context, payload *rdb.Raw[SwarcoData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[SwarcoData]) error {
	log := logger.Get(ctx)

	static := payload.Rawdata.Static.StaticPOIData
	if len(static) == 0 {
		// A payload with no stations would deactivate the whole origin on
		// sync: refuse it instead, it is a provider glitch.
		return fmt.Errorf("payload contains no static POI data, refusing to process")
	}

	dynByID := map[string]DynamicPOIData{}
	for _, d := range payload.Rawdata.Dynamic.DynamicPOIData {
		dynByID[d.ObjectID] = d
	}

	var facilities, stations []bdplib.Station
	facilityData := bdp.CreateDataMap()
	stationData := bdp.CreateDataMap()

	for _, poi := range static {
		if poi.ObjectID == "" {
			log.Warn("skipping POI without objectID", "name", poi.Name)
			continue
		}
		lat, lon, hasPos := position(poi.Location)
		if !hasPos {
			// A station without coordinates disappears from every bounding-box
			// query while still looking healthy; refuse it and make it visible.
			log.Warn("skipping POI without usable location", "object_id", poi.ObjectID, "name", poi.Name)
			continue
		}

		dyn, hasDyn := dynByID[poi.ObjectID]
		ts := payload.Timestamp
		if hasDyn && !dyn.TimeStamp.IsZero() {
			ts = dyn.TimeStamp
		}
		tsMillis := ts.UnixMilli()

		if len(poi.Areas) > 0 {
			// A POI with areas is a ParkingFacility parent; its areas are the
			// actual ParkingStations.
			parent := bdplib.CreateStation(
				stationCode(poi.ObjectID), poi.Name, StationTypeParkingFacility, lat, lon, bdp.GetOrigin())
			parent.MetaData = poi.ToMetadata()
			facilities = append(facilities, parent)
			if hasDyn {
				addOccupancyRecords(ctx, &facilityData, parent.Id, dyn.OccupancyTotal, tsMillis)
			}

			areaOccupancy := map[string]OccupancyData{}
			for _, ad := range dyn.OccupancyAreas {
				if ad.Occupancy != nil {
					areaOccupancy[ad.ObjectID] = *ad.Occupancy
				}
			}
			for _, area := range poi.Areas {
				if area.ObjectID == "" {
					log.Warn("skipping area without objectID", "facility", poi.ObjectID, "name", area.Name)
					continue
				}
				name := area.Name
				if name == "" {
					name = poi.Name
				}
				child := bdplib.CreateStation(
					stationCode(area.ObjectID), name, StationTypeParkingStation, lat, lon, bdp.GetOrigin())
				child.ParentStation = parent.Id
				child.ParentStationType = StationTypeParkingFacility
				child.MetaData = areaMetadata(area)
				stations = append(stations, child)
				if occ, ok := areaOccupancy[area.ObjectID]; ok {
					addOccupancyRecords(ctx, &stationData, child.Id, []OccupancyData{occ}, tsMillis)
				}
			}
		} else {
			station := bdplib.CreateStation(
				stationCode(poi.ObjectID), poi.Name, StationTypeParkingStation, lat, lon, bdp.GetOrigin())
			station.MetaData = poi.ToMetadata()
			stations = append(stations, station)
			if hasDyn {
				addOccupancyRecords(ctx, &stationData, station.Id, dyn.OccupancyTotal, tsMillis)
			}
		}
	}

	if len(facilities) > 0 {
		if err := bdp.SyncStations(StationTypeParkingFacility, facilities, true, false); err != nil {
			return fmt.Errorf("failed to sync %s stations: %w", StationTypeParkingFacility, err)
		}
	}
	if len(stations) > 0 {
		if err := bdp.SyncStations(StationTypeParkingStation, stations, true, false); err != nil {
			return fmt.Errorf("failed to sync %s stations: %w", StationTypeParkingStation, err)
		}
	}
	if len(facilityData.Branch) > 0 {
		if err := bdp.PushData(StationTypeParkingFacility, facilityData); err != nil {
			return fmt.Errorf("failed to push %s data: %w", StationTypeParkingFacility, err)
		}
	}
	if len(stationData.Branch) > 0 {
		if err := bdp.PushData(StationTypeParkingStation, stationData); err != nil {
			return fmt.Errorf("failed to push %s data: %w", StationTypeParkingStation, err)
		}
	}
	return nil
}

// addOccupancyRecords maps one occupancy entry per parking space type to
// free/occupied records ("total" feeds the base types, other space types the
// suffixed ones). Unknown space types are skipped: their data types were never
// synced, and silently inventing types is worse than a warning.
func addOccupancyRecords(ctx context.Context, dm *bdplib.DataMap, scode string, occupancies []OccupancyData, ts int64) {
	log := logger.Get(ctx)
	for _, occ := range occupancies {
		slug, known := spaceTypeSlug[occ.ParkingSpaceType]
		if !known {
			log.Warn("skipping unknown parking space type", "type", occ.ParkingSpaceType, "scode", scode)
			continue
		}
		freeType, occupiedType := DataTypeFree, DataTypeOccupied
		if slug != "" {
			freeType = DataTypeFree + "_" + slug
			occupiedType = DataTypeOccupied + "_" + slug
		}
		if occ.VacantSpaces != nil && *occ.VacantSpaces >= 0 {
			dm.AddRecord(scode, freeType, bdplib.CreateRecord(ts, *occ.VacantSpaces, PERIOD))
		}
		if occ.OccupiedSpaces != nil && *occ.OccupiedSpaces >= 0 {
			dm.AddRecord(scode, occupiedType, bdplib.CreateRecord(ts, *occ.OccupiedSpaces, PERIOD))
		}
	}
}

func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeFree, "count", "Free parking spots", "Instantaneous"),
		bdplib.CreateDataType(DataTypeOccupied, "count", "Occupied parking spots", "Instantaneous"),
	}
	seen := map[string]bool{}
	slugs := []string{}
	for _, slug := range spaceTypeSlug {
		if slug == "" || seen[slug] {
			continue
		}
		seen[slug] = true
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)
	for _, slug := range slugs {
		label := "'" + strings.ReplaceAll(slug, "_", " ") + "' parking spots"
		dataTypes = append(dataTypes,
			bdplib.CreateDataType(DataTypeFree+"_"+slug, "count", "Free "+label, "Instantaneous"),
			bdplib.CreateDataType(DataTypeOccupied+"_"+slug, "count", "Occupied "+label, "Instantaneous"),
		)
	}
	return bdp.SyncDataTypes(dataTypes)
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const (
	IDTemplate = "urn:bike-boxes:loewenbytes"

	StationType = "BikeParking"

	measurementPeriod = 600
)

// Data type names are shared with the Bicincittà bike-boxes integration
// (transformers/bike-boxes) so that both providers report onto the same
// Open Data Hub measurement types, per the integration spec's instruction
// to reuse existing types where possible.
const (
	DataTypeUsageState        = "usageState"
	DataTypeFree              = "free"
	DataTypeFreeRegularBikes  = "freeSpotsRegularBike"
	DataTypeFreeElectricBikes = "freeSpotsElectricBike"
)

var env tr.Env

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	log := logger.Get(ctx)
	log.Info("Starting bike-boxes-loewenbytes transformer...")

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

	log.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(ctx, err, "failed to sync types")

	log.Info("Starting transformer listener...")
	listener := tr.NewTr[string](ctx, env)
	err = listener.Start(ctx, tr.RawString2JsonMiddleware[RawData](TransformWithBdp(b)))
	ms.FailOnError(ctx, err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[RawData] {
	return func(ctx context.Context, payload *rdb.Raw[RawData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[RawData]) error {
	log := logger.Get(ctx)
	ts := payload.Timestamp.UnixMilli()

	byID := func(stations []Station, id string) *Station {
		for i := range stations {
			if stations[i].StationID == id {
				return &stations[i]
			}
		}
		return nil
	}

	var stations []bdplib.Station
	dm := bdp.CreateDataMap()

	for _, it := range payload.Rawdata.It {
		de := byID(payload.Rawdata.De, it.StationID)
		en := byID(payload.Rawdata.En, it.StationID)
		lld := byID(payload.Rawdata.Lld, it.StationID)

		scode := clib.GenerateID(IDTemplate, it.StationID)

		station := bdplib.CreateStation(scode, it.Name, StationType, it.Latitude, it.Longitude, bdp.GetOrigin())

		names := map[string]any{"it": it.Name}
		if de != nil {
			names["de"] = de.Name
		}
		if en != nil {
			names["en"] = en.Name
		}
		if lld != nil {
			names["lld"] = lld.Name
		}

		meta := map[string]any{
			"provider_id":  it.StationID,
			"locationID":   it.LocationID,
			"locationName": it.LocationName,
			"names":        names,
			"address":      it.Address,
			"type":         mapStationType(it.Type),
			"totalPlaces":  it.TotalPlaces,
			"places":       mapPlaces(it.Places),
		}
		station.MetaData = meta

		stations = append(stations, station)

		addMeasurements(dm, scode, it, ts)

		log.Debug("Processed station", "stationID", it.StationID, "name", it.Name)
	}

	log.Info("Syncing stations and pushing data", "stations", len(stations))

	if err := bdp.SyncStations(StationType, stations, true, false); err != nil {
		return err
	}
	if err := bdp.PushData(StationType, dm); err != nil {
		return err
	}

	return nil
}

func addMeasurements(dm bdplib.DataMap, scode string, s Station, ts int64) {
	dm.AddRecord(scode, DataTypeUsageState, bdplib.CreateRecord(ts, mapStationState(s.State), measurementPeriod))
	dm.AddRecord(scode, DataTypeFree, bdplib.CreateRecord(ts, s.CountFreePlacesAvailable, measurementPeriod))
	dm.AddRecord(scode, DataTypeFreeRegularBikes, bdplib.CreateRecord(ts, s.CountFreePlacesAvailableMuscularBikes, measurementPeriod))
	dm.AddRecord(scode, DataTypeFreeElectricBikes, bdplib.CreateRecord(ts, s.CountFreePlacesAvailableAssistedBikes, measurementPeriod))
}

func mapStationType(t int) string {
	switch t {
	case 4:
		return "veloHub"
	case 5:
		return "bikeBoxGroup"
	default:
		return "unknown"
	}
}

func mapPlaces(places []Place) []map[string]any {
	out := make([]map[string]any, 0, len(places))
	for _, p := range places {
		out = append(out, map[string]any{
			"position": p.Position,
			"type":     mapPlaceType(p.Type),
			"state":    mapPlaceState(p.State),
			"level":    p.Level,
		})
	}
	return out
}

func mapPlaceType(t int) string {
	switch t {
	case 1:
		return "withoutRefill"
	case 2:
		return "withRefill"
	default:
		return "unknown"
	}
}

func mapPlaceState(state int) string {
	switch state {
	case 1:
		return "free"
	case 2:
		return "occupied"
	case 3:
		return "out of service"
	default:
		return "unknown"
	}
}

func mapStationState(state int) string {
	switch state {
	case 1:
		return "in service"
	case 2:
		return "out of service"
	default:
		return "unknown"
	}
}

func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeUsageState, "state", "Usage state", "Instantaneous"),
		bdplib.CreateDataType(DataTypeFree, "count", "Free parking spots", "Instantaneous"),
		bdplib.CreateDataType(DataTypeFreeRegularBikes, "count", "Free parking spots (regular bikes)", "Instantaneous"),
		bdplib.CreateDataType(DataTypeFreeElectricBikes, "count", "Free parking spots (electric bikes)", "Instantaneous"),
	}
	return bdp.SyncDataTypes(dataTypes)
}

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"math"
	"strconv"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationTypeBikeParking    = "BikeParking"
	StationTypeParkingStation = "ParkingStation"

	Origin = "SBB"
	Period = 1800 // 30 minutes in seconds
)

const (
	ParkingFacilityCategoryBike = "BIKE"
	ParkingFacilityCategoryCar  = "CAR"
)

const (
	DataTypePredictedForecastedOccupancy   = "predictedForecastedOccupancy"
	DataTypeCurrentEstimatedOccupancy      = "currentEstimatedOccupancy"
	DataTypeCurrentEstimatedOccupancyLevel = "currentEstimatedOccupancyLevel"
)

// Measurement field names to exclude from parking facility metadata
var measurementFields = map[string]bool{
	DataTypePredictedForecastedOccupancy:   true,
	DataTypeCurrentEstimatedOccupancy:      true,
	DataTypeCurrentEstimatedOccupancyLevel: true,
}

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Swiss parking data transformer...")

	b := bdplib.FromEnv()
	defer tel.FlushOnPanic()

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync data types")

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[string](context.Background(), env)

	err = listener.Start(context.Background(), MultiFormatMiddleware[Root](TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[Root] {
	return func(ctx context.Context, payload *rdb.Raw[Root]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Root]) error {
	slog.Info("Processing Swiss parking data", "features", len(payload.Rawdata.Features))

	ts := payload.Timestamp.UnixMilli()

	bikeStations, carStations, dataMap := processFeatures(bdp, payload.Rawdata, ts)

	slog.Info("Syncing bike parking stations", "count", len(bikeStations))
	err := bdp.SyncStations(StationTypeBikeParking, bikeStations, true, false)
	if err != nil {
		return fmt.Errorf("syncing bike parking stations: %w", err)
	}

	slog.Info("Syncing car parking stations", "count", len(carStations))
	err = bdp.SyncStations(StationTypeParkingStation, carStations, true, false)
	if err != nil {
		return fmt.Errorf("syncing car parking stations: %w", err)
	}

	err = bdp.PushData(StationTypeParkingStation, dataMap)
	if err != nil {
		return fmt.Errorf("pushing car parking measurements: %w", err)
	}

	slog.Info("Swiss parking data transformation completed successfully")
	return nil
}

// processFeatures splits the unified feature collection into bike and car
// parking stations (keyed off the "parkingFacilityCategory" property) and
// collects car parking occupancy measurements.
func processFeatures(bdp bdplib.Bdp, root Root, ts int64) ([]bdplib.Station, []bdplib.Station, bdplib.DataMap) {
	var bikeStations, carStations []bdplib.Station
	dataMap := bdp.CreateDataMap()

	for _, feature := range root.Features {
		props := feature.Properties

		category, _ := props["parkingFacilityCategory"].(string)

		var stationType, codeField string
		switch category {
		case ParkingFacilityCategoryBike:
			stationType, codeField = StationTypeBikeParking, "uic"
		case ParkingFacilityCategoryCar:
			stationType, codeField = StationTypeParkingStation, "didokId"
		default:
			slog.Warn("feature has unknown parkingFacilityCategory", "featureID", feature.ID, "category", category)
			continue
		}

		codeVal, ok := props[codeField]
		if !ok || codeVal == nil {
			slog.Warn("feature missing station code", "featureID", feature.ID, "field", codeField)
			continue
		}
		stationCode := formatCode(codeVal)

		nameVal, ok := props["displayName"]
		if !ok || nameVal == nil {
			slog.Warn("feature missing displayName", "featureID", feature.ID)
			continue
		}
		name := fmt.Sprintf("%v", nameVal)

		lat, lon, err := extractCoordinates(feature.Geometry)
		if err != nil {
			slog.Warn("feature has invalid coordinates", "featureID", feature.ID, "err", err)
			continue
		}

		station := bdplib.CreateStation(fmt.Sprintf("%s:%s", Origin, stationCode), name, stationType, lat, lon, Origin)

		metadata := make(map[string]interface{})
		for k, v := range props {
			if !measurementFields[k] {
				metadata[k] = v
			}
		}
		station.MetaData = metadata

		if category == ParkingFacilityCategoryBike {
			bikeStations = append(bikeStations, station)
			continue
		}

		carStations = append(carStations, station)

		if v, ok := props[DataTypePredictedForecastedOccupancy]; ok && v != nil {
			dataMap.AddRecord(station.Id, DataTypePredictedForecastedOccupancy,
				bdplib.CreateRecord(ts, map[string]any{"predictions": v}, Period))
		}
		if v, ok := props[DataTypeCurrentEstimatedOccupancy]; ok && v != nil {
			dataMap.AddRecord(station.Id, DataTypeCurrentEstimatedOccupancy,
				bdplib.CreateRecord(ts, v, Period))
		}
		if v, ok := props[DataTypeCurrentEstimatedOccupancyLevel]; ok && v != nil {
			dataMap.AddRecord(station.Id, DataTypeCurrentEstimatedOccupancyLevel,
				bdplib.CreateRecord(ts, v, Period))
		}
	}

	return bikeStations, carStations, dataMap
}

// extractCoordinates finds the Point geometry within a GeometryCollection and
// converts its GeoJSON [longitude, latitude] coordinates to (latitude, longitude).
func extractCoordinates(geom GeoJSONGeometryCollection) (float64, float64, error) {
	for _, g := range geom.Geometries {
		if g.Type != "Point" {
			continue
		}
		var coords []float64
		if err := json.Unmarshal(g.Coordinates, &coords); err != nil {
			return 0, 0, fmt.Errorf("invalid point coordinates: %w", err)
		}
		if len(coords) < 2 {
			return 0, 0, fmt.Errorf("point coordinates has less than 2 elements")
		}
		return coords[1], coords[0], nil
	}
	return 0, 0, fmt.Errorf("no point geometry found")
}

// formatCode formats a station code property value as a string. Codes like
// "uic" are decoded from JSON as float64; formatting them with "%v" directly
// would produce scientific notation (e.g. "8.503104e+06"), so whole numbers
// are converted to plain integer form first.
func formatCode(v interface{}) string {
	if f, ok := v.(float64); ok && f == math.Trunc(f) {
		return strconv.FormatInt(int64(f), 10)
	}
	return fmt.Sprintf("%v", v)
}

func syncDataTypes(bdp bdplib.Bdp) error {
	var dataTypes []bdplib.DataType

	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypePredictedForecastedOccupancy, "", "Predicted forecasted occupancy (JSON array with hourly forecasts)", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeCurrentEstimatedOccupancy, "%", "Current estimated occupancy percentage", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeCurrentEstimatedOccupancyLevel, "", "Current estimated occupancy level (LOW, MEDIUM, HIGH)", "Instantaneous"))

	return bdp.SyncDataTypes(dataTypes)
}

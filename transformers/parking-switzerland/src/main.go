// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationTypeBikeParking = "BikeParking"
	StationTypeParkingStation = "ParkingStation"

	Origin = "SBB"
	MeasurementPeriod = 1800 // 30 minutes in seconds
)

const (
	DataTypePredictedForecastedOccupancy  = "predictedForecastedOccupancy"
	DataTypeCurrentEstimatedOccupancy      = "currentEstimatedOccupancy"
	DataTypeCurrentEstimatedOccupancyLevel = "currentEstimatedOccupancyLevel"
)

// Measurement field names to exclude from car parking metadata
var carParkingMeasurementFields = map[string]bool{
	DataTypePredictedForecastedOccupancy:  true,
	DataTypeCurrentEstimatedOccupancy:      true,
	DataTypeCurrentEstimatedOccupancyLevel: true,
}

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Swiss parking data transformer...")

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

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync data types")

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[string](context.Background(), env)

	err = listener.Start(context.Background(),
		tr.RawString2JsonMiddleware[CombinedParkingData](TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[CombinedParkingData] {
	return func(ctx context.Context, payload *rdb.Raw[CombinedParkingData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[CombinedParkingData]) error {
	slog.Info("Processing Swiss parking data",
		"timestamp", payload.Timestamp,
		"bikeFeatures", len(payload.Rawdata.BikeParking.Features),
		"carFeatures", len(payload.Rawdata.CarParking.Features))

	ts := payload.Timestamp.UnixMilli()

	// Process bike parking stations (stations + metadata only, no measurements)
	bikeStations, err := processBikeParking(bdp, payload.Rawdata.BikeParking)
	if err != nil {
		return fmt.Errorf("processing bike parking: %w", err)
	}

	// Process car parking stations and measurements
	carStations, carDataMap, err := processCarParking(bdp, payload.Rawdata.CarParking, ts)
	if err != nil {
		return fmt.Errorf("processing car parking: %w", err)
	}

	// Sync bike parking stations
	slog.Info("Syncing bike parking stations", "count", len(bikeStations))
	err = bdp.SyncStations(StationTypeBikeParking, bikeStations, true, false)
	if err != nil {
		return fmt.Errorf("syncing bike parking stations: %w", err)
	}

	// Sync car parking stations
	slog.Info("Syncing car parking stations", "count", len(carStations))
	err = bdp.SyncStations(StationTypeParkingStation, carStations, true, false)
	if err != nil {
		return fmt.Errorf("syncing car parking stations: %w", err)
	}

	// Push car parking measurements
	err = bdp.PushData(StationTypeParkingStation, carDataMap)
	if err != nil {
		return fmt.Errorf("pushing car parking measurements: %w", err)
	}

	slog.Info("Swiss parking data transformation completed successfully")
	return nil
}

func processBikeParking(bdp bdplib.Bdp, fc GeoJSONFeatureCollection) ([]bdplib.Station, error) {
	var stations []bdplib.Station

	for _, feature := range fc.Features {
		props := feature.Properties

		// Extract station code from properties.source.id
		sourceMap, ok := props["source"].(map[string]interface{})
		if !ok {
			slog.Warn("Bike parking feature missing source map", "featureID", feature.ID)
			continue
		}

		stationCode := fmt.Sprintf("%v", sourceMap["id"])
		name := fmt.Sprintf("%v", sourceMap["name"])

		lat, lon, err := extractCoordinates(feature.Geometry)
		if err != nil {
			slog.Warn("Bike parking feature invalid coordinates", "featureID", feature.ID, "err", err)
			continue
		}

		station := bdplib.CreateStation(stationCode, name, StationTypeBikeParking, lat, lon, Origin)

		// Store all properties as metadata
		metadata := make(map[string]interface{})
		for k, v := range props {
			metadata[k] = v
		}
		station.MetaData = metadata

		stations = append(stations, station)
	}

	return stations, nil
}

func processCarParking(bdp bdplib.Bdp, fc GeoJSONFeatureCollection, ts int64) ([]bdplib.Station, bdplib.DataMap, error) {
	var stations []bdplib.Station
	dataMap := bdp.CreateDataMap()

	for _, feature := range fc.Features {
		props := feature.Properties

		stationCode := fmt.Sprintf("%v", props["didokId"])
		name := fmt.Sprintf("%v", props["displayName"])

		lat, lon, err := extractCoordinates(feature.Geometry)
		if err != nil {
			slog.Warn("Car parking feature invalid coordinates", "featureID", feature.ID, "err", err)
			continue
		}

		station := bdplib.CreateStation(stationCode, name, StationTypeParkingStation, lat, lon, Origin)

		// Store all properties as metadata, except measurement fields
		metadata := make(map[string]interface{})
		for k, v := range props {
			if !carParkingMeasurementFields[k] {
				metadata[k] = v
			}
		}
		station.MetaData = metadata

		stations = append(stations, station)

		// Add measurements (only if values are non-nil)
		if v, ok := props[DataTypePredictedForecastedOccupancy]; ok && v != nil {
			dataMap.AddRecord(stationCode, DataTypePredictedForecastedOccupancy,
				bdplib.CreateRecord(ts, v, MeasurementPeriod))
		}

		if v, ok := props[DataTypeCurrentEstimatedOccupancy]; ok && v != nil {
			// JSON unmarshals numbers as float64
			dataMap.AddRecord(stationCode, DataTypeCurrentEstimatedOccupancy,
				bdplib.CreateRecord(ts, v, MeasurementPeriod))
		}

		if v, ok := props[DataTypeCurrentEstimatedOccupancyLevel]; ok && v != nil {
			dataMap.AddRecord(stationCode, DataTypeCurrentEstimatedOccupancyLevel,
				bdplib.CreateRecord(ts, v, MeasurementPeriod))
		}
	}

	return stations, dataMap, nil
}

// extractCoordinates converts GeoJSON [longitude, latitude] to (latitude, longitude)
func extractCoordinates(geom GeoJSONGeometry) (float64, float64, error) {
	if len(geom.Coordinates) < 2 {
		return 0, 0, fmt.Errorf("coordinates array has less than 2 elements")
	}
	// GeoJSON: [longitude, latitude] → return (latitude, longitude)
	return geom.Coordinates[1], geom.Coordinates[0], nil
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

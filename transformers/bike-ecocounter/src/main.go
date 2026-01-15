// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const (
	StationType       = "BikeCounter"
	StationCodePrefix = "urn:bikecounter:ecocounter"
)

// Data type names based on travelMode
const (
	DataTypeBike       = "vehicle-detection"
	DataTypePedestrian = "countpeople"
	DataTypeCar        = "nr. vehicles"
)

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting bike ecocounter data transformer...")

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

	slog.Info("Syncing data types on startup")
	syncDataTypes(b)

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[string](context.Background(), env)
	err := listener.Start(context.Background(),
		tr.RawString2JsonMiddleware[[]EcocounterSite](TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[[]EcocounterSite] {
	return func(ctx context.Context, payload *rdb.Raw[[]EcocounterSite]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[[]EcocounterSite]) error {
	log := logger.Get(ctx)
	sites := payload.Rawdata
	log.Info("Processing ecocounter data transformation", "sites", len(sites))

	var stations []bdplib.Station
	dataMap := bdp.CreateDataMap()

	for _, site := range sites {
		log.Debug("Processing site", "id", site.ID, "name", site.Name, "directional", site.Directional)

		if site.Directional {
			// Create a station for each direction
			directions := getUniqueDirections(site.Measurements)
			for _, direction := range directions {
				station := createStation(bdp, site, direction)
				stations = append(stations, station)

				// Add measurements for this direction
				addMeasurements(log, dataMap, station.Id, site, direction)
			}
		} else {
			// Create a single station for non-directional sites
			station := createStation(bdp, site, "")
			stations = append(stations, station)

			// Add all measurements
			addMeasurements(log, dataMap, station.Id, site, "")
		}
	}

	log.Info("Syncing stations and pushing data", "stations", len(stations))

	// Sync stations
	err := bdp.SyncStations(StationType, stations, true, false)
	ms.FailOnError(ctx, err, "failed to sync stations")

	// Push measurement data
	err = bdp.PushData(StationType, dataMap)
	ms.FailOnError(ctx, err, "failed to push data")

	log.Info("Ecocounter data transformation completed successfully")
	return nil
}

func getUniqueDirections(measurements []Measurement) []string {
	directionMap := make(map[string]bool)
	for _, m := range measurements {
		if m.Direction != "" {
			directionMap[m.Direction] = true
		}
	}

	directions := make([]string, 0, len(directionMap))
	for dir := range directionMap {
		directions = append(directions, dir)
	}
	return directions
}

func createStation(bdp bdplib.Bdp, site EcocounterSite, direction string) bdplib.Station {
	var stationID string
	var stationName string

	if direction != "" && direction != "undefined" {
		stationID = fmt.Sprintf("%s:%d:%s", StationCodePrefix, site.ID, strings.ToUpper(direction))
		stationName = fmt.Sprintf("%s (%s)", site.Name, direction)
	} else {
		stationID = fmt.Sprintf("%s:%d", StationCodePrefix, site.ID)
		stationName = site.Name
	}

	station := bdplib.CreateStation(
		stationID,
		stationName,
		StationType,
		site.Location.Lat,
		site.Location.Lon,
		bdp.GetOrigin(),
	)

	metadata := createMetadata(site, direction)
	station.MetaData = metadata

	return station
}

func createMetadata(site EcocounterSite, direction string) map[string]interface{} {
	metadata := make(map[string]interface{})

	metadata["siteId"] = site.ID
	metadata["directional"] = site.Directional
	metadata["granularity"] = site.Granularity
	metadata["hasTimestampedData"] = site.HasTimestampedData
	metadata["hasWeather"] = site.HasWeather
	metadata["travelModes"] = site.TravelModes

	if direction != "" {
		metadata["direction"] = direction
	}

	// Store counter information
	if len(site.Counters) > 0 {
		counters := make([]map[string]interface{}, len(site.Counters))
		for i, c := range site.Counters {
			counters[i] = map[string]interface{}{
				"id":               c.ID,
				"installationDate": c.InstallationDate,
				"serial":           c.Serial,
			}
		}
		metadata["counters"] = counters
	}

	return metadata
}

func addMeasurements(log *slog.Logger, dataMap bdplib.DataMap, stationID string, site EcocounterSite, direction string) {
	// Calculate period in seconds from granularity (e.g., "PT1H" = 3600, "PT15M" = 900)
	period := parseGranularityToSeconds(site.Granularity)

	for _, measurement := range site.Measurements {
		// Filter by direction if specified
		if direction != "" && measurement.Direction != direction {
			continue
		}

		dataType := mapTravelModeToDataType(measurement.TravelMode)
		if dataType == "" {
			// log.Warn("Unknown travel mode", "travelMode", measurement.TravelMode, "flowID", measurement.FlowID)
			continue
		}

		for _, dataPoint := range measurement.Data {
			timestamp, err := time.Parse(time.RFC3339, dataPoint.Timestamp)
			if err != nil {
				log.Warn("Failed to parse timestamp", "timestamp", dataPoint.Timestamp, "error", err)
				continue
			}

			dataMap.AddRecord(stationID, dataType, bdplib.CreateRecord(timestamp.UnixMilli(), dataPoint.Counts, period))
		}
	}
}

func mapTravelModeToDataType(travelMode string) string {
	switch strings.ToLower(travelMode) {
	case "bike":
		return DataTypeBike
	case "pedestrian":
		return DataTypePedestrian
	case "car":
		return DataTypeCar
	default:
		return ""
	}
}

func parseGranularityToSeconds(granularity string) uint64 {
	// Parse ISO 8601 duration format like "PT1H", "PT15M", "PT1H30M"
	granularity = strings.TrimPrefix(granularity, "PT")

	seconds := 0

	// Handle hours
	if idx := strings.Index(granularity, "H"); idx != -1 {
		hours, err := strconv.Atoi(granularity[:idx])
		if err == nil {
			seconds += hours * 3600
		}
		granularity = granularity[idx+1:]
	}

	// Handle minutes
	if idx := strings.Index(granularity, "M"); idx != -1 {
		minutes, err := strconv.Atoi(granularity[:idx])
		if err == nil {
			seconds += minutes * 60
		}
		granularity = granularity[idx+1:]
	}

	// Handle seconds
	if idx := strings.Index(granularity, "S"); idx != -1 {
		secs, err := strconv.Atoi(granularity[:idx])
		if err == nil {
			seconds += secs
		}
	}

	// Default to 1 hour if parsing failed
	if seconds == 0 {
		seconds = 3600
	}

	return uint64(seconds)
}

func syncDataTypes(bdp bdplib.Bdp) {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeBike, "count", "Number of bikes detected", "Instantaneous"),
		bdplib.CreateDataType(DataTypePedestrian, "count", "Number of pedestrians detected", "Instantaneous"),
		bdplib.CreateDataType(DataTypeCar, "count", "Number of vehicles detected", "Instantaneous"),
	}

	err := bdp.SyncDataTypes(dataTypes)
	ms.FailOnError(context.Background(), err, "failed to sync data types")
}

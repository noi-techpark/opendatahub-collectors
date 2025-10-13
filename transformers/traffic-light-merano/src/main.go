// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"io"
	"log/slog"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"golang.org/x/text/encoding/charmap"
)

const (
	stationType = "TrafficSensor"
	period      = 600 // 10 minutes in seconds

	dataTypeTransits    = "total-transits nr"
	dataTypeTemperature = "temperature"
)

var env tr.Env
var stationProto StationLookup

// charsetReader handles different character encodings for XML
func charsetReader(charset string, input io.Reader) (io.Reader, error) {
	switch strings.ToLower(charset) {
	case "iso-8859-1", "latin1":
		return charmap.ISO8859_1.NewDecoder().Reader(input), nil
	default:
		return input, nil
	}
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[string] {
	return func(ctx context.Context, payload *rdb.Raw[string]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[string]) error {
	log := logger.Get(ctx)

	// Deserialize XML from string with charset support
	var trafficData TrafficData
	decoder := xml.NewDecoder(bytes.NewReader([]byte(payload.Rawdata)))
	decoder.CharsetReader = charsetReader
	err := decoder.Decode(&trafficData)
	if err != nil {
		log.Error("Failed to unmarshal XML", "error", err)
		return err
	}

	dataMap := bdp.CreateDataMap()

	// Process each sezione (traffic monitoring point)
	for _, sezione := range trafficData.Sezioni {
		// Get station metadata from KML lookup
		stationData := stationProto.GetStationByID(sezione.ID)
		if stationData == nil {
			log.Warn("Station not found in KML", "station_id", sezione.ID)
			continue
		}

		// Get the base midnight timestamp from DAY_0
		baseMidnight := sezione.Day_0.GetBaseMidnightTimestamp()
		if baseMidnight == 0 {
			log.Warn("Failed to parse iso_date", "station_id", sezione.ID)
			continue
		}

		// Process all valid FT (traffic flow) values
		ftValues := sezione.Day_0.FT.GetAllValidValues(baseMidnight)
		for _, tv := range ftValues {
			dataMap.AddRecord(
				stationData.ID,
				dataTypeTransits,
				bdplib.CreateRecord(tv.Timestamp, tv.Value, period),
			)
		}

		// Process all valid T (temperature) values
		tValues := sezione.Day_0.T.GetAllValidValues(baseMidnight)
		for _, tv := range tValues {
			dataMap.AddRecord(
				stationData.ID,
				dataTypeTemperature,
				bdplib.CreateRecord(tv.Timestamp, tv.Value, period),
			)
		}

		log.Debug("Processed station",
			"station_id", sezione.ID,
			"ft_values", len(ftValues),
			"t_values", len(tValues))
	}

	// Push data records
	err = bdp.PushData(stationType, dataMap)
	if err != nil {
		return err
	}

	log.Info("Transformation completed", "sezioni_count", len(trafficData.Sezioni))
	return nil
}

// CreateStationsFromLookup creates BDP stations from the station lookup
func CreateStationsFromLookup(lookup StationLookup, origin string) []bdplib.Station {
	stations := make([]bdplib.Station, 0, len(lookup))

	for _, stationData := range lookup {
		station := bdplib.CreateStation(
			stationData.ID,
			stationData.Name,
			stationType,
			stationData.Lat,
			stationData.Lon,
			origin,
		)
		stations = append(stations, station)
	}

	return stations
}

func SyncDataTypesAndStations(bdp bdplib.Bdp, stations []bdplib.Station) {
	// Sync data types
	var dataTypes []bdplib.DataType

	dataTypes = append(dataTypes, bdplib.CreateDataType(
		dataTypeTransits,
		"",
		"Total number of vehicles passing through the sensor",
		"Instantaneous",
	))

	dataTypes = append(dataTypes, bdplib.CreateDataType(
		dataTypeTemperature,
		"°C",
		"Temperature measured at the sensor location",
		"Instantaneous",
	))

	err := bdp.SyncDataTypes(dataTypes)
	ms.FailOnError(context.Background(), err, "failed to sync data types")

	// Sync stations
	err = bdp.SyncStations(stationType, stations, true, true)
	ms.FailOnError(context.Background(), err, "failed to sync stations")
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting traffic light merano transformer...")

	defer tel.FlushOnPanic()

	// Initialize BDP client
	b := bdplib.FromEnv()

	// Load stations from KML file
	slog.Info("Loading stations from KML file...")
	stationProto = ReadStations("../resources/nodes.kml")
	slog.Info("Stations loaded", "count", len(stationProto))

	// Create BDP stations from lookup
	slog.Info("Creating BDP stations...")
	stations := CreateStationsFromLookup(stationProto, b.GetOrigin())
	slog.Info("Stations created", "count", len(stations))

	// Sync data types and stations
	slog.Info("Syncing data types and stations...")
	SyncDataTypesAndStations(b, stations)
	slog.Info("Sync completed")

	// Start listener
	slog.Info("Starting listener...")
	listener := tr.NewTr[string](context.Background(), env)
	err := listener.Start(context.Background(), TransformWithBdp(b))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

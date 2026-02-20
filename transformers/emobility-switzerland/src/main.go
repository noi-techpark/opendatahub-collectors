// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationType  = "EChargingPlug"
	DataTypeName = "echarging-plug-status-ocpi"
	Period       = 600
)

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting e-mobility Switzerland transformer...")

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
		tr.RawString2JsonMiddleware[Envelope](handleMessage(b)))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(
			DataTypeName,
			"state",
			"Current state of echarging plug according to OICP standard",
			"Instantaneous",
		),
	}
	return bdp.SyncDataTypes(dataTypes)
}

func handleMessage(bdp bdplib.Bdp) tr.Handler[Envelope] {
	return func(ctx context.Context, payload *rdb.Raw[Envelope]) error {
		switch payload.Rawdata.Type {
		case "static":
			return handleStatic(ctx, bdp, payload)
		case "realtime":
			return handleRealtime(ctx, bdp, payload)
		default:
			return fmt.Errorf("unknown envelope type: %s", payload.Rawdata.Type)
		}
	}
}

func handleStatic(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Envelope]) error {
	slog.Info("Processing static data")

	var staticData StaticResponse
	if err := json.Unmarshal(payload.Rawdata.Data, &staticData); err != nil {
		return fmt.Errorf("failed to unmarshal static data: %w", err)
	}

	var stations []bdplib.Station

	for _, operator := range staticData.EVSEData {
		slog.Debug("Processing operator", "operatorID", operator.OperatorID, "records", len(operator.EVSEDataRecord))

		for _, rec := range operator.EVSEDataRecord {
			lat, lon, err := parseGoogleCoords(rec.GeoCoordinates.Google)
			if err != nil {
				slog.Warn("Skipping record with invalid coordinates",
					"chargingStationId", rec.ChargingStationId, "err", err)
				continue
			}

			name := rec.ChargingStationId
			if len(rec.ChargingStationNames) > 0 && rec.ChargingStationNames[0].Value != "" {
				name = rec.ChargingStationNames[0].Value
			}

			station := bdplib.CreateStation(
				rec.ChargingStationId,
				name,
				StationType,
				lat, lon,
				operator.OperatorID,
			)

			meta, err := buildMetadata(rec)
			if err != nil {
				slog.Warn("Failed to build metadata, using empty",
					"chargingStationId", rec.ChargingStationId, "err", err)
				meta = map[string]interface{}{}
			}
			// Add operator info to metadata
			meta["operatorName"] = operator.OperatorName
			station.MetaData = meta

			stations = append(stations, station)
		}
	}

	slog.Info("Syncing stations", "count", len(stations))
	if err := bdp.SyncStations(StationType, stations, true, false); err != nil {
		return fmt.Errorf("error syncing %s stations: %w", StationType, err)
	}

	slog.Info("Static data processing completed", "stations", len(stations))
	return nil
}

func handleRealtime(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Envelope]) error {
	slog.Info("Processing real-time data")

	var statusData StatusResponse
	if err := json.Unmarshal(payload.Rawdata.Data, &statusData); err != nil {
		return fmt.Errorf("failed to unmarshal realtime data: %w", err)
	}

	dataMap := bdp.CreateDataMap()
	ts := payload.Timestamp.UnixMilli()
	recordCount := 0

	for _, operator := range statusData.EVSEStatuses {
		for _, rec := range operator.EVSEStatusRecord {
			dataMap.AddRecord(
				rec.EvseID,
				DataTypeName,
				bdplib.CreateRecord(ts, rec.EVSEStatus, Period),
			)
			recordCount++
		}
	}

	slog.Info("Pushing real-time data", "records", recordCount)
	if err := bdp.PushData(StationType, dataMap); err != nil {
		return fmt.Errorf("error pushing %s data: %w", StationType, err)
	}

	slog.Info("Real-time data processing completed", "records", recordCount)
	return nil
}

// parseGoogleCoords parses the OICP Google coordinate format "lat lng" into float64 values.
func parseGoogleCoords(google string) (float64, float64, error) {
	parts := strings.Fields(google)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinate format: %q", google)
	}
	if parts[0] == "None" || parts[1] == "None" {
		return 0, 0, fmt.Errorf("coordinates are None: %q", google)
	}
	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude %q: %w", parts[0], err)
	}
	lon, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude %q: %w", parts[1], err)
	}
	return lat, lon, nil
}

// buildMetadata creates a metadata map from an EVSE record using a generic approach:
// marshal the entire record to JSON, unmarshal to a map, then remove/rename mapped fields.
func buildMetadata(rec EVSEDataRecord) (map[string]interface{}, error) {
	jsonBytes, err := json.Marshal(rec)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal record: %w", err)
	}

	var meta map[string]interface{}
	if err := json.Unmarshal(jsonBytes, &meta); err != nil {
		return nil, fmt.Errorf("failed to unmarshal to map: %w", err)
	}

	// Remove fields already mapped to ODH core station fields
	delete(meta, "ChargingStationId")
	delete(meta, "GeoCoordinates")

	// Rename Plugs -> outlets per specification
	if plugs, ok := meta["Plugs"]; ok {
		meta["outlets"] = plugs
		delete(meta, "Plugs")
	}

	// Handle ChargingStationNames: keep additional names beyond [0] as metadata
	if names, ok := meta["ChargingStationNames"]; ok {
		nameSlice, isSlice := names.([]interface{})
		if isSlice && len(nameSlice) > 1 {
			meta["additionalNames"] = nameSlice[1:]
		}
		delete(meta, "ChargingStationNames")
	}

	// Clean up null values to avoid cluttering metadata
	for k, v := range meta {
		if v == nil {
			delete(meta, k)
		}
	}

	return meta, nil
}

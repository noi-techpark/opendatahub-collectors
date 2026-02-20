// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationType     = "EChargingPlug"
	DataTypeStatus  = "echarging-plug-status"
	Origin          = "BFE" // Swiss Federal Office of Energy
	Period          = 600   // 10 minutes
)

var env struct {
	tr.Env
	bdplib.BdpEnv
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Swiss e-mobility transformer...")

	b := bdplib.FromEnv(env.BdpEnv)
	defer tel.FlushOnPanic()

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync data types")

	slog.Info("Starting transformer listener...")
	listener := tr.NewTr[Root](context.Background(), env.Env)

	err = listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[Root] {
	return func(ctx context.Context, payload *rdb.Raw[Root]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Root]) error {
	slog.Info("Processing Swiss e-mobility data",
		"timestamp", payload.Timestamp,
		"operators", len(payload.Rawdata.EVSEData),
		"statusOperators", len(payload.Rawdata.EVSEStatuses))

	ts := payload.Timestamp.UnixMilli()

	// Step 1: Process static data (EVSE locations/stations)
	stations, err := processStaticData(payload.Rawdata.EVSEData)
	if err != nil {
		return fmt.Errorf("processing static data: %w", err)
	}

	err = bdp.SyncStations(StationType, stations, false, false)
	if err != nil {
		return fmt.Errorf("syncing stations: %w", err)
	}

	slog.Info("Synced stations", "count", len(stations))

	// Step 2: Process real-time status data
	dataMap := bdp.CreateDataMap()
	statusCount := 0

	// Create status lookup map (flatten operator structure)
	statusByEvseID := make(map[string]string)
	for _, statusOperator := range payload.Rawdata.EVSEStatuses {
		for _, status := range statusOperator.EVSEStatusRecord {
			statusByEvseID[status.EvseID] = status.EvseStatus
		}
	}

	// Push status measurements for all known stations
	for _, station := range stations {
		evseID := station.Id // Station ID is BFE:EvseID

		if status, ok := statusByEvseID[evseID]; ok {
			// Convert OICP status to value
			statusValue := convertStatusToValue(status)
			
			record := bdplib.CreateRecord(ts, statusValue, Period)
			dataMap.AddRecord(evseID, DataTypeStatus, record)
			statusCount++
		}
	}

	err = bdp.PushData(StationType, dataMap)
	if err != nil {
		return fmt.Errorf("pushing data: %w", err)
	}

	slog.Info("Pushed status measurements", "count", statusCount)
	return nil
}

func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(
			DataTypeStatus,
			"state",
			"Current status of echarging plug (OICP standard)",
			"Instantaneous",
		),
	}
	return bdp.SyncDataTypes(dataTypes)
}

func processStaticData(evseOperators []EVSEOperator) ([]bdplib.Station, error) {
	stations := make([]bdplib.Station, 0)

	// Flatten operator structure
	for _, operator := range evseOperators {
		for _, evse := range operator.EVSEDataRecord {
			// Parse coordinates
			lat, lon, err := parseGoogleCoords(evse.GeoCoordinates)
			if err != nil {
				slog.Warn("Skipping EVSE with invalid coordinates",
					"evseID", evse.EvseID, "err", err)
				continue
			}

			// Extract station name
			stationName := extractStationName(evse.ChargingStationNames)
			if stationName == "" {
				stationName = evse.ChargingStationId
			}

			// Build metadata
			metadata := buildMetadata(&evse)

			station := bdplib.Station{
				Id:          evse.EvseID,
				Name:        stationName,
				Latitude:    lat,
				Longitude:   lon,
				Origin:      Origin,
				StationType: StationType,
				MetaData:    metadata,
			}

			stations = append(stations, station)
		}
	}

	return stations, nil
}

func parseGoogleCoords(geo *GeoCoordinate) (float64, float64, error) {
	if geo == nil || geo.Google == "" {
		return 0, 0, fmt.Errorf("missing coordinates")
	}

	parts := strings.Split(geo.Google, " ")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid coordinate format: %s", geo.Google)
	}

	lat, err := strconv.ParseFloat(parts[0], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid latitude: %w", err)
	}

	lon, err := strconv.ParseFloat(parts[1], 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid longitude: %w", err)
	}

	return lat, lon, nil
}

func extractStationName(names []ChargingStationName) string {
	for _, name := range names {
		if name.Lang == "en" || name.Lang == "de" {
			return name.Value
		}
	}
	if len(names) > 0 {
		return names[0].Value
	}
	return ""
}

func buildMetadata(evse *EVSEDataItem) map[string]interface{} {
	metadata := make(map[string]interface{})

	// Core identifiers
	metadata["evseID"] = evse.EvseID
	metadata["chargingStationId"] = evse.ChargingStationId

	// Address information
	if evse.Address != nil {
		if evse.Address.Street != nil {
			metadata["street"] = *evse.Address.Street
		}
		if evse.Address.City != nil {
			metadata["city"] = *evse.Address.City
		}
		if evse.Address.PostalCode != nil {
			metadata["postalCode"] = *evse.Address.PostalCode
		}
		if evse.Address.Country != nil {
			metadata["country"] = *evse.Address.Country
		}
	}

	// Accessibility
	if evse.Accessibility != nil {
		metadata["accessibility"] = *evse.Accessibility
	}
	if evse.IsOpen24Hours != nil {
		metadata["isOpen24Hours"] = *evse.IsOpen24Hours
	}

	// Technical details
	if len(evse.Plugs) > 0 {
		metadata["plugs"] = evse.Plugs
	}
	if len(evse.ChargingFacilities) > 0 {
		facilitiesJSON, _ := json.Marshal(evse.ChargingFacilities)
		metadata["chargingFacilities"] = string(facilitiesJSON)
	}
	if len(evse.AuthenticationModes) > 0 {
		metadata["authenticationModes"] = evse.AuthenticationModes
	}
	if len(evse.PaymentOptions) > 0 {
		metadata["paymentOptions"] = evse.PaymentOptions
	}

	// Energy info
	if evse.RenewableEnergy != nil {
		metadata["renewableEnergy"] = *evse.RenewableEnergy
	}

	// Contact
	if evse.HotlinePhoneNumber != nil {
		metadata["hotlinePhoneNumber"] = *evse.HotlinePhoneNumber
	}

	return metadata
}

func convertStatusToValue(status string) float64 {
	// OICP Status codes mapping to numeric values
	switch status {
	case "Available":
		return 1.0
	case "Occupied":
		return 2.0
	case "Reserved":
		return 3.0
	case "Unknown":
		return 0.0
	case "OutOfService":
		return -1.0
	default:
		return 0.0
	}
}

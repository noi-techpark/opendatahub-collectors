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
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationTypePlug    = "EChargingPlug"
	StationTypeStation = "EChargingStation"
	Origin             = "BFE" // Swiss Federal Office of Energy
	Period             = 600   // 10 minutes
	DataTypeStatus     = "echarging-plug-status-oicp"
)

var dtNumberAvailable = bdplib.DataType{
	Name:        "number-available",
	Description: "number of available vehicles / charging points",
	Rtype:       "Instantaneous",
}
var dtPlugStatus = bdplib.DataType{
	Name:        DataTypeStatus,
	Description: "Current state of echarging plug according to OCPI standard",
	Rtype:       "",
}

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
	listener := tr.NewTr[string](context.Background(), env.Env)

	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware[Root](TransformWithBdp(b)))
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

	// Step 1: Process static data — build parent EChargingStation and child EChargingPlug stations
	plugStations, parentStations, err := processStaticData(payload.Rawdata.EVSEData)
	if err != nil {
		return fmt.Errorf("processing static data: %w", err)
	}

	err = bdp.SyncStations(StationTypeStation, parentStations, false, false)
	if err != nil {
		return fmt.Errorf("syncing parent stations: %w", err)
	}
	slog.Info("Synced parent stations", "count", len(parentStations))

	err = bdp.SyncStations(StationTypePlug, plugStations, false, false)
	if err != nil {
		return fmt.Errorf("syncing plug stations: %w", err)
	}
	slog.Info("Synced plug stations", "count", len(plugStations))

	// Step 2: Build EVSE status lookup
	statusByEvseID := make(map[string]string)
	for _, statusOperator := range payload.Rawdata.EVSEStatuses {
		for _, status := range statusOperator.EVSEStatusRecord {
			statusByEvseID[status.EvseID] = status.EvseStatus
		}
	}

	// Step 3: Push plug-level status measurements
	plugDataMap := bdp.CreateDataMap()
	statusCount := 0
	for _, plug := range plugStations {
		if status, ok := statusByEvseID[plug.Id]; ok {
			record := bdplib.CreateRecord(ts, status, Period)
			plugDataMap.AddRecord(plug.Id, dtPlugStatus.Name, record)
			statusCount++
		}
	}
	err = bdp.PushData(StationTypePlug, plugDataMap)
	if err != nil {
		return fmt.Errorf("pushing plug data: %w", err)
	}
	slog.Info("Pushed plug status measurements", "count", statusCount)

	// Step 4: Push parent-level number-available measurements
	availableByParent := make(map[string]int)
	for _, plug := range plugStations {
		if statusByEvseID[plug.Id] == "Available" {
			availableByParent[plug.ParentStation]++
		}
	}

	stationDataMap := bdp.CreateDataMap()
	for _, parent := range parentStations {
		count := availableByParent[parent.Id]
		record := bdplib.CreateRecord(ts, count, Period)
		stationDataMap.AddRecord(parent.Id, dtNumberAvailable.Name, record)
	}
	err = bdp.PushData(StationTypeStation, stationDataMap)
	if err != nil {
		return fmt.Errorf("pushing station data: %w", err)
	}
	slog.Info("Pushed station number-available measurements", "count", len(parentStations))

	return nil
}

func syncDataTypes(bdp bdplib.Bdp) error {
	return bdp.SyncDataTypes([]bdplib.DataType{dtNumberAvailable, dtPlugStatus})
}

// processStaticData groups EVSEs by ChargingStationId, validates consistency within each group,
// and returns a slice of EChargingPlug stations and a slice of EChargingStation parent stations.
func processStaticData(evseOperators []EVSEOperator) (plugStations []bdplib.Station, parentStations []bdplib.Station, err error) {
	type stationGroup struct {
		evses []EVSEDataItem
	}
	groups := make(map[string]*stationGroup)
	groupOrder := make([]string, 0) // preserve operator order for determinism

	for _, operator := range evseOperators {
		for _, evse := range operator.EVSEDataRecord {
			sid := evse.ChargingStationId
			if _, ok := groups[sid]; !ok {
				groups[sid] = &stationGroup{}
				groupOrder = append(groupOrder, sid)
			}
			groups[sid].evses = append(groups[sid].evses, evse)
		}
	}

	plugStations = make([]bdplib.Station, 0)
	parentStations = make([]bdplib.Station, 0)

	for _, stationID := range groupOrder {
		group := groups[stationID]

		// Find first EVSE with valid coords as the reference for station-level fields
		var refEVSE *EVSEDataItem
		var refLat, refLon float64
		var refName string

		for i := range group.evses {
			evse := &group.evses[i]
			lat, lon, parseErr := parseGoogleCoords(evse.GeoCoordinates)
			if parseErr != nil {
				slog.Warn("Skipping EVSE with invalid coordinates", "evseID", evse.EvseID, "err", parseErr)
				continue
			}
			if refEVSE == nil {
				refEVSE = evse
				refLat, refLon = lat, lon
				refName = extractStationName(evse.ChargingStationNames)
			} else {
				// Warn if EVSEs under the same station report different station-level fields
				name := extractStationName(evse.ChargingStationNames)
				if name != refName {
					slog.Warn("EVSE under same station has different name",
						"stationID", stationID, "evseID", evse.EvseID,
						"expected", refName, "got", name)
				}
				if math.Abs(lat-refLat) > 0.001 || math.Abs(lon-refLon) > 0.001 {
					slog.Warn("EVSE under same station has different coordinates",
						"stationID", stationID, "evseID", evse.EvseID,
						"expectedLat", refLat, "expectedLon", refLon,
						"gotLat", lat, "gotLon", lon)
				}
			}
		}

		if refEVSE == nil {
			slog.Warn("No valid EVSE for station, skipping", "stationID", stationID)
			continue
		}

		stationName := refName
		if stationName == "" {
			stationName = stationID
		}

		parent := bdplib.Station{
			Id:          stationID,
			Name:        stationName,
			Latitude:    refLat,
			Longitude:   refLon,
			Origin:      Origin,
			StationType: StationTypeStation,
			MetaData:    buildStationMetadata(refEVSE),
		}
		parentStations = append(parentStations, parent)

		for i := range group.evses {
			evse := &group.evses[i]
			lat, lon, parseErr := parseGoogleCoords(evse.GeoCoordinates)
			if parseErr != nil {
				continue // already warned above
			}
			plug := bdplib.Station{
				Id:            evse.EvseID,
				Name:          stationName,
				Latitude:      lat,
				Longitude:     lon,
				Origin:        Origin,
				StationType:   StationTypePlug,
				ParentStation: stationID,
				MetaData:      buildPlugMetadata(evse),
			}
			plugStations = append(plugStations, plug)
		}
	}

	return plugStations, parentStations, nil
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

func extractStationName(names ChargingStationNameList) string {
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

// buildStationMetadata returns metadata fields that are common to all EVSEs under a station.
func buildStationMetadata(evse *EVSEDataItem) map[string]interface{} {
	metadata := make(map[string]interface{})

	metadata["chargingStationId"] = evse.ChargingStationId

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

	if evse.Accessibility != nil {
		metadata["accessibility"] = *evse.Accessibility
	}
	if evse.IsOpen24Hours != nil {
		metadata["isOpen24Hours"] = *evse.IsOpen24Hours
	}
	if evse.HotlinePhoneNumber != nil {
		metadata["hotlinePhoneNumber"] = *evse.HotlinePhoneNumber
	}

	return metadata
}

// buildPlugMetadata returns metadata fields that are specific to an individual EVSE/plug.
func buildPlugMetadata(evse *EVSEDataItem) map[string]interface{} {
	metadata := make(map[string]interface{})

	metadata["evseID"] = evse.EvseID

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
	if evse.RenewableEnergy != nil {
		metadata["renewableEnergy"] = *evse.RenewableEnergy
	}

	return metadata
}

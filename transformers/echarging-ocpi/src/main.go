// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
)

const stationTypeLocation = "EChargingStation"
const stationTypePlug = "EChargingPlug"
const period = 600

var dtNumberAvailable = bdplib.DataType{
	Name:        "number-available",
	Description: "number of available vehicles / charging points",
	Rtype:       "Instantaneous",
}
var dtPlugStatus = bdplib.DataType{
	Name:        "echarging-plug-status-ocpi",
	Description: "Current state of echarging plug according to OCPI standard",
	Rtype:       "Instantaneous",
}

func syncDataTypes(b *bdplib.Bdp) {
	failOnError(b.SyncDataTypes(stationTypeLocation, []bdplib.DataType{dtNumberAvailable}), "could not sync data types. aborting...")
	failOnError(b.SyncDataTypes(stationTypePlug, []bdplib.DataType{dtPlugStatus}), "could not sync data types. aborting...")
}

func main() {
	initLogging()

	b := bdplib.FromEnv()

	syncDataTypes(b)

	listen(func(r *raw) error {
		locations, err := unmarshalRaw(r.Rawdata)
		if err != nil {
			return fmt.Errorf("error unmarshalling raw payload to locations struct: %w", err)
		}

		stations := []bdplib.Station{}
		locationData := b.CreateDataMap()
		plugs := []bdplib.Station{}
		plugData := b.CreateDataMap()

		for _, loc := range locations.Data {
			station := bdplib.CreateStation(
				loc.ID,
				loc.Name,
				stationTypeLocation,
				loc.Coordinates.Latitude,
				loc.Coordinates.Longitude,
				b.Origin)

			station.MetaData = map[string]any{
				"country_code":  loc.CountryCode,
				"party_id":      loc.PartyID,
				"publish":       loc.Publish,
				"address":       loc.Address,
				"city":          loc.City,
				"postal_code":   loc.PostalCode,
				"time_zone":     loc.TimeZone,
				"opening_times": loc.OpeningTimes,
				"directions":    loc.Directions,
			}

			stations = append(stations, station)

			numAvailable := 0

			for _, evse := range loc.Evses {
				plug := bdplib.CreateStation(
					evse.UID,
					evse.EvseID,
					stationTypePlug,
					loc.Coordinates.Latitude,
					loc.Coordinates.Longitude,
					b.Origin)

				plug.ParentStation = station.Id

				plug.MetaData = map[string]any{
					"capabilities": evse.Capabilities,
					"connectors":   evse.Connectors,
				}

				plugs = append(plugs, plug)
				if evse.Status == "AVAILABLE" {
					numAvailable++
				}
				plugData.AddRecord(plug.Id, dtPlugStatus.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), evse.Status, period))
			}

			locationData.AddRecord(station.Id, dtNumberAvailable.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), numAvailable, period))
		}

		if err := b.SyncStations(stationTypeLocation, stations, true, true); err != nil {
			return fmt.Errorf("error syncing %s: %w", stationTypeLocation, err)
		}
		if err := b.SyncStations(stationTypePlug, plugs, true, true); err != nil {
			return fmt.Errorf("error syncing %s: %w", stationTypePlug, err)
		}
		if err := b.PushData(stationTypeLocation, locationData); err != nil {
			return fmt.Errorf("error pushing location data: %w", err)
		}
		if err := b.PushData(stationTypePlug, plugData); err != nil {
			return fmt.Errorf("error pushing plug data: %w", err)
		}

		// push all
		return nil
	})
}

func initLogging() {
	logLevel := os.Getenv("LOG_LEVEL")

	level := new(slog.LevelVar)
	level.UnmarshalText([]byte(logLevel))

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("Start logger with level: " + logLevel)
}

func failOnError(err error, msg string) {
	if err != nil {
		slog.Error(msg, "err", err)
		panic(err)
	}
}

func unmarshalRaw(s string) (OCPILocations, error) {
	var p OCPILocations
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return p, fmt.Errorf("error unmarshalling payload json: %w", err)
	}

	return p, nil
}

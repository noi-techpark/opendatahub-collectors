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

const stationtype = "EChargingStation"

func main() {
	initLogging()

	b := bdplib.FromEnv()

	listen(func(r *raw) error {
		locations, err := unmarshalRaw(r.Rawdata)
		if err != nil {
			return fmt.Errorf("error unmarshalling raw payload to locations struct: %w", err)
		}

		stations := []bdplib.Station{}

		for _, loc := range locations.Data {
			stations = append(stations, bdplib.CreateStation(
				loc.ID,
				loc.Name,
				stationtype,
				loc.Coordinates.Latitude,
				loc.Coordinates.Longitude,
				b.Origin))
		}

		if err := b.SyncStations(stationtype, stations, true, false); err != nil {
			return fmt.Errorf("error syncing stations: %w", err)
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

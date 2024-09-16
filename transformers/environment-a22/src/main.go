// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"golang.org/x/exp/maps"
)

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

func main() {
	initLogging()

	b := bdplib.FromEnv()

	dtmap := readDataTypes()
	failOnError(b.SyncDataTypes("", maps.Values(dtmap)), "Error pushing datatypes")

	// load station configs from CSV

	// sync stations

	s := bdplib.CreateStation("id", "name", "type", 46.1, 11.2, b.Origin)
	s.MetaData = map[string]any{
		"keyname": "value",
	}
	failOnError(b.SyncStations("stationtype", []bdplib.Station{s}, true, false), "Error syncing stations")

	listen(func(r *raw) error {
		_, err := unmarshalRaw(r.Rawdata)
		if err != nil {
			return fmt.Errorf("unable to unmarshal raw payload: %w", err)
		}

		// Get matching physical station from config. If not found, reject with error

		dm := b.CreateDataMap()
		// dm.AddRecord(s.Id, dtFree.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), raw.Lots, Period))

		if err := b.PushData("stationtype", dm); err != nil {
			return fmt.Errorf("error pushing data: %w", err)
		}
		return nil
	})
}

type bdpDataTypeMap map[string]bdplib.DataType

func readDataTypes() bdpDataTypeMap {
	dts := readCsv("datatypes.csv")
	dtm := bdpDataTypeMap{}
	for _, dt := range dts[1:] {
		// in the old data collector, for raw datatypes the unit is always null instead of using the one from CSV. Is this correct?
		dtm[dt[0]] = bdplib.CreateDataType(dt[1], dt[2], dt[3], dt[4])
	}
	return dtm
}

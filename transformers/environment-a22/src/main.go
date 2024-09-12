// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
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

	dtFree := bdplib.CreateDataType("free", "", "free", "Instantaneous")
	dtOccupied := bdplib.CreateDataType("occupied", "", "occupied", "Instantaneous")

	ds := []bdplib.DataType{dtFree, dtOccupied}
	failOnError(b.SyncDataTypes("stationtype", ds), "Error pushing datatypes")

	// push data types
	// load station configs from CSV
	// sync stations

	s := bdplib.CreateStation("id", "name", "type", 46.1, 11.2, b.Origin)
	s.MetaData = map[string]any{
		"keyname": "value",
	}
	failOnError(b.SyncStations("stationtype", []bdplib.Station{s}, true, false), "Error syncing stations")

	listen(func(r *raw) error {
		raw, err := unmarshalRaw(r.Rawdata)
		if err != nil {
			return fmt.Errorf("Unable to unmarshal raw payload: %w", err)
		}

		// Get matching physical station from config. If not found, reject with error

		dm := b.CreateDataMap()
		dm.AddRecord(s.Id, dtFree.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), raw.Lots, Period))
		dm.AddRecord(s.Id, dtOccupied.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), tot-raw.Lots, Period))

		if err := b.PushData("stationtype", dm); err != nil {
			return fmt.Errorf("Error pushing data: %w", err)
		}
		return nil
	})
}

type payload struct {
	MsgId   int
	Topic   string
	Payload strMqttPayload
}

// the Payload JSON is a string that we have to first unmarshal
type strMqttPayload mqttPayload

type mqttPayload []struct {
	DateTimeAcquisition time.Time
	ControlUnitId       string
	Resval              []struct {
	}
}

func unmarshalRaw(s string) (payload, error) {
	var p payload
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return p, fmt.Errorf("error unmarshalling payload json: %w", err)
	}

	return p, nil
}

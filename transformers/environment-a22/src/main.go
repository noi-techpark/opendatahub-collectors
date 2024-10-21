// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"time"

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

const period = 60
const stationtype = "EnvironmentStation"

func main() {
	initLogging()

	b := bdplib.FromEnv()

	dtmap := readDataTypes("datatypes.csv")
	failOnError(b.SyncDataTypes("", maps.Values(dtmap)), "error pushing datatypes")

	scfg, err := readStationCSV("stations.csv")
	failOnError(err, "error loading station csv")
	stations, err := compileHistory(scfg)
	failOnError(err, "error compiling station history")
	bdpStations := []bdplib.Station{}
	for _, s := range stations {
		bdpStations = append(bdpStations, map2Bdp(s, b.Origin))
	}
	failOnError(b.SyncStations(stationtype, bdpStations, true, false), "error syncing stations")

	listen(func(r *raw) error {
		payload := r.Rawdata

		sensorid := payload.Payload.ControlUnitId
		ts := payload.Payload.DateTimeAcquisition

		station, err := currentStation(stations, sensorid, ts)
		if err != nil {
			return fmt.Errorf("error mapping station for sensor %s: %w", sensorid, err)
		}

		dm := b.CreateDataMap()

		for _, v := range payload.Payload.Resval {
			dt, ok := dtmap[strconv.Itoa(v.Id)]
			if !ok {
				return fmt.Errorf("error mapping data type %d for sensor %s", v.Id, sensorid)
			}
			dm.AddRecord(station.id, dt.Name, bdplib.CreateRecord(ts.UnixMilli(), v.Value, period))
		}

		if err := b.PushData(stationtype, dm); err != nil {
			return fmt.Errorf("error pushing data: %w", err)
		}
		return nil
	})
}

type bdpDataTypeMap map[string]bdplib.DataType

func readDataTypes(path string) bdpDataTypeMap {
	dts := readCsv(path)
	dtm := bdpDataTypeMap{}
	for _, dt := range dts[1:] {
		// in the old data collector, for raw datatypes the unit is always null instead of using the one from CSV. Is this correct?
		dtm[dt[0]] = bdplib.CreateDataType(dt[1], dt[2], dt[3], dt[4])
	}
	return dtm
}

type payload struct {
	MsgId   int
	Topic   string
	Payload strMqttPayload
}

// the Payload JSON is a string that we have to first unmarshal
type strMqttPayload mqttPayload

type mqttPayload struct {
	DateTimeAcquisition time.Time
	ControlUnitId       string
	Resval              []struct {
		Id    int
		Value float64
		// ignoring other fields
	}
}

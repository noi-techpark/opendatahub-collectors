// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
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
)

var env struct {
	tr.Env
	bdplib.BdpEnv
	VALKEY_ADDR   string
	VALKEY_PASS   string
	VALKEY_DB     int
	VALKEY_PREFIX string
}

type SensorPayload struct {
	Type string
	Data struct {
		Name      string
		Direction string
		Timestamp MaybeTimezoneTime
	}
}

// Payload is a string containing a JSON
func (sp *SensorPayload) UnmarshalJSON(data []byte) error {
	var raw string
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}

	type Alias SensorPayload
	var alias Alias
	if err := json.Unmarshal([]byte(raw), &alias); err != nil {
		return err
	}

	*sp = SensorPayload(alias)
	return nil
}

type MaybeTimezoneTime struct {
	time.Time
}

// Depending on sensor, Timestamp does not have a timezone, so we have to handle both cases
func (ct *MaybeTimezoneTime) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)

	// Try parsing with timezone first (RFC3339 variants)
	if t, err := time.Parse(time.RFC3339, str); err == nil {
		ct.Time = t
		return nil
	}
	if t, err := time.Parse(time.RFC3339Nano, str); err == nil {
		ct.Time = t
		return nil
	}

	// We have no timezone, so assume we're in Italy
	romeLocation, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		return err
	}

	for _, format := range []string{"2006-01-02T15:04:05", "2006-01-02T15:04:05.999999999"} {
		if t, err := time.ParseInLocation(format, str, romeLocation); err == nil {
			ct.Time = t
			return nil
		}
	}

	return fmt.Errorf("unable to parse timestamp: %s", str)
}

type RawType struct {
	Topic   string
	MsgId   int
	Payload SensorPayload
}

const STATIONTYPE = "PeopleCounter"
const PERIOD = 600

var datatype = bdplib.CreateDataType("countPeople", "people", "Number of people passing by", "sum")

const WINDOW_START_KEY = "current_window"

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	b.SyncDataTypes([]bdplib.DataType{datatype})

	stations, err := readStationCsv("stations.csv")
	ms.FailOnError(ctx, err, "could not read stations csv")

	bdpStations := []bdplib.Station{}
	for _, sd := range stations {
		bdpStations = append(bdpStations, bdplib.CreateStation(sd.ID, sd.Name, STATIONTYPE, sd.Lat, sd.Lon, env.BDP_ORIGIN))
	}
	ms.FailOnError(ctx, b.SyncStations(STATIONTYPE, bdpStations, true, false), "could not sync stations")

	// vk := redis.NewClient(&redis.Options{Addr: env.VALKEY_ADDR, Password: env.VALKEY_PASS, DB: env.VALKEY_DB})

	listener := tr.NewTr[RawType](ctx, env.Env)
	err = listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[RawType]) error {

		// check if current msg beyond window range
		// if it is, flush the current window
		// - make the measurement and push it
		// - reset the counter and window start for station

		recs := b.CreateDataMap()

		// last part of topic is the sensor ID used to map metadata in csv
		parts := strings.Split(r.Rawdata.Topic, "/")
		sensorId := parts[len(parts)-1]
		station, found := stations[sensorId]
		if !found {
			return fmt.Errorf("could not find station metadata for topic %s", r.Rawdata.Topic)
		}
		recs.AddRecord(station.ID, datatype.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), 1, PERIOD))
		err := b.PushData(STATIONTYPE, recs)
		if err != nil {
			return err
		}
		return nil

	})
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

type SensorData struct {
	ID     string  `csv:"id"`
	Sensor string  `csv:"sensor"`
	Name   string  `csv:"name"`
	Lat    float64 `csv:"lat"`
	Lon    float64 `csv:"lon"`
}

func readStationCsv(filename string) (map[string]SensorData, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}
	sensorMap := make(map[string]SensorData)
	for i, record := range records[1:] {
		if len(record) != 5 {
			return nil, fmt.Errorf("row %d has %d columns, expected 5", i+2, len(record))
		}

		lat, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid lat value '%s'", i+2, record[3])
		}

		lon, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid lon value '%s'", i+2, record[4])
		}

		sensor := record[1]
		if _, exists := sensorMap[sensor]; exists {
			return nil, fmt.Errorf("duplicate sensor found: %s", sensor)
		}

		sensorMap[sensor] = SensorData{
			ID:     record[0],
			Sensor: sensor,
			Name:   record[2],
			Lat:    lat,
			Lon:    lon,
		}
	}

	return sensorMap, nil
}

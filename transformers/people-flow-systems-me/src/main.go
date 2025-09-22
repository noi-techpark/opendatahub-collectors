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
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/go-timeseries-client/where"
	"github.com/noi-techpark/opendatahub-go-sdk/elab"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env struct {
	tr.Env
	bdplib.BdpEnv
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
const AGGR_PERIOD = 600
const BASE_PERIOD = 1
const AGGR_LAG = time.Minute * 10 // only sum records older than 10 minutes to avoid incomplete windows

var dtCount = bdplib.CreateDataType("countPeople", "people", "Number of people passing by", "sum")
var dtIn = bdplib.CreateDataType("countPeople", "people", "Person passing in direction In", "instantaneous")
var dtOut = bdplib.CreateDataType("countPeople", "people", "Person passing in direction Out", "instantaneous")

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	b.SyncDataTypes([]bdplib.DataType{dtCount, dtIn, dtOut})

	stations, err := readStationCsv("stations.csv")
	ms.FailOnError(ctx, err, "could not read stations csv")

	bdpStations := []bdplib.Station{}
	for _, sd := range stations {
		bdpStations = append(bdpStations, bdplib.CreateStation(sd.ID, sd.Name, STATIONTYPE, sd.Lat, sd.Lon, env.BDP_ORIGIN))
	}
	ms.FailOnError(ctx, b.SyncStations(STATIONTYPE, bdpStations, true, false), "could not sync stations")

	listener := tr.NewTr[RawType](ctx, env.Env)
	err = listener.Start(ctx, func(ctx context.Context, r *rdb.Raw[RawType]) error {
		recs := b.CreateDataMap()
		// last part of topic is the sensor ID used to map metadata in csv
		parts := strings.Split(r.Rawdata.Topic, "/")
		sensorId := parts[len(parts)-1]
		station, found := stations[sensorId]
		if !found {
			return fmt.Errorf("could not find station metadata for topic %s", r.Rawdata.Topic)
		}

		dt := ""
		switch r.Rawdata.Payload.Data.Direction {
		case "In":
			dt = dtIn.Name
		case "Out":
			dt = dtOut.Name
		default:
			return fmt.Errorf("unknown direction %s", r.Rawdata.Payload.Data.Direction)
		}
		recs.AddRecord(station.ID, dt, bdplib.CreateRecord(r.Rawdata.Payload.Data.Timestamp.UnixMilli(), 1, BASE_PERIOD))

		err := b.PushData(STATIONTYPE, recs)
		if err != nil {
			return err
		}
		return nil

	})
	ms.FailOnError(context.Background(), err, "error while listening to queue")

}

// the timestamp of the aggregated record is the end of the AGG_PERIOD long window
func windowTs(ts time.Time) time.Time {
	return ts.Truncate(time.Second * AGGR_PERIOD).Add(time.Second * AGGR_PERIOD)
}

func sumElaboration(ctx context.Context, b bdplib.Bdp, n odhts.C) {
	e := elab.NewElaboration(&n, &b)
	e.StationTypes = append(e.StationTypes, STATIONTYPE)
	e.Filter = where.Eq("sorigin", env.BDP_ORIGIN)
	e.BaseTypes = append(e.BaseTypes, elab.BaseDataType{Name: dtIn.Name, Period: BASE_PERIOD})
	e.BaseTypes = append(e.BaseTypes, elab.BaseDataType{Name: dtOut.Name, Period: BASE_PERIOD})
	e.ElaboratedTypes = append(e.ElaboratedTypes, elab.ElaboratedDataType{Name: dtCount.Name, Period: AGGR_PERIOD, DontSync: true})
	e.StartingPoint = time.Date(2025, 07, 31, 0, 0, 0, 0, time.UTC) // first records came in that day in testing

	is, err := e.RequestState()
	ms.FailOnError(ctx, err, "failed requesting initial elaboration state")

	res := []elab.ElabResult{}

	for scode, st := range is[STATIONTYPE].Stations {
		start := st.Datatypes[dtCount.Name].Periods[AGGR_PERIOD]
		if start.IsZero() {
			start = e.StartingPoint
		}
		end := time.Now().Add(-AGGR_LAG)

		measures, err := e.RequestHistory([]string{STATIONTYPE}, e.StationTypes, []string{dtIn.Name, dtOut.Name}, []elab.Period{BASE_PERIOD}, start, end)
		ms.FailOnError(ctx, err, "failed requesting history for count elaboration station %s from %s to %s", scode, start.String(), end.String())

		// Create contiguous AGGR_PERIOD sized windows, then we count the records for each window
		// Windows may also be empty, we still have to count them as 0
		idx := 0
		curWin := start
		for {
			curWin = curWin.Add(time.Second * AGGR_PERIOD)
			cnt := 0
			for idx < len(measures) {
				meas := measures[idx]
				win := windowTs(meas.Timestamp.Time)
				if win.Equal(curWin) {
					cnt += 1
					idx += 1
				} else if win.After(curWin) {
					// measurement belongs to one of the next windows
					break
				} else {
					ms.FailOnError(ctx, fmt.Errorf("tried to elaborate record at %s before current window %s. This should not be possible", win.String(), curWin.String()), "")
				}
			}
			// To be sure that the data for a window is complete, the end date lags behind Now().
			// But, if we see that there are still measurements left to elaborate for the following windows,
			// we can also assume the window is complete and don't have to respect the lag
			if curWin.After(end) && idx >= len(measures) {
				break
			}
			res = append(res, elab.ElabResult{StationType: STATIONTYPE, StationCode: scode, Timestamp: start, Period: AGGR_PERIOD, DataType: dtCount.Name, Value: cnt})
		}
	}
	ms.FailOnError(ctx, e.PushResults(STATIONTYPE, res), "failed pushing elaboration results")
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

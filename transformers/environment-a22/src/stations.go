// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
)

type station struct {
	id            string
	name          string
	lat           float64
	lon           float64
	latest_sensor string
	history       []Sensorhistory
}

type Sensorhistory struct {
	Sensor_id    string
	Sensor_start time.Time
	Sensor_end   time.Time
}

const (
	CSV_REFNAME      int = 0
	CSV_STATION_CODE int = 1
	CSV_STATION_NAME int = 2
	CSV_LATITUDE     int = 3
	CSV_LONGITUDE    int = 4
	CSV_FIRST_DATE   int = 5
)

const dateOnlyFormat = "2006-01-02"

func (h *Sensorhistory) MarshalJSON() ([]byte, error) {
	var end any
	end = ""
	if !h.Sensor_end.IsZero() {
		end = h.Sensor_end.Format(dateOnlyFormat)
	}
	return json.Marshal(map[string]any{
		"id":    h.Sensor_id,
		"start": h.Sensor_start.Format(dateOnlyFormat),
		"end":   end,
	})
}

func readStationCSV(path string) (map[string]station, error) {
	stationf := readCsv(path)
	stations := map[string]station{}
	dates := []time.Time{}
	for i := CSV_FIRST_DATE; i < len(stationf[0]); i++ {
		dt, err := time.Parse("2006-01-02", stationf[0][i])
		if err != nil {
			return nil, fmt.Errorf("error parsing column header date %s: %w", stationf[0][i], err)
		}
		dates = append(dates, dt)
	}
	for _, st := range stationf[1:] {
		scode := st[CSV_STATION_CODE]
		sname := st[CSV_STATION_NAME]
		lat, err := strconv.ParseFloat(st[CSV_LATITUDE], 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing lat float value %s: %w", st[2], err)
		}
		lon, err := strconv.ParseFloat(st[CSV_LONGITUDE], 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing lon float value %s: %w", st[2], err)
		}
		station := station{id: scode, name: sname, lat: lat, lon: lon}
		prev := ""
		for i, date := range dates {
			cur := st[CSV_FIRST_DATE+i]
			if cur != prev {
				if prev != "" {
					station.history[len(station.history)-1].Sensor_end = date
				}
				if cur != "" {
					station.history = append(station.history, Sensorhistory{Sensor_id: cur, Sensor_start: date})
				}
				prev = cur
			}
		}
		station.latest_sensor = prev
		stations[station.id] = station
	}
	return stations, nil
}

func map2Bdp(s station, origin string) bdplib.Station {
	mapped := bdplib.CreateStation(s.id, s.name, "EnvironmentStation", s.lat, s.lon, origin)

	mapped.MetaData = make(map[string]interface{})

	mapped.MetaData["sensor_id"] = s.latest_sensor
	mapped.MetaData["sensor_history"] = s.history

	return mapped
}

func currentStation(sts map[string]station, sensor string, ts time.Time) (station, error) {
	var ret station
	var latest time.Time
	for _, s := range sts {
		for _, h := range s.history {
			if h.Sensor_id == sensor && (ts.After(h.Sensor_start) || ts.Equal(h.Sensor_start)) && (h.Sensor_end.IsZero() || ts.Before(h.Sensor_end)) && h.Sensor_start.After(latest) {
				ret = s
				latest = h.Sensor_start
			}
		}
	}

	if latest.IsZero() {
		return ret, fmt.Errorf("missing sensor mapping for sensor %s at time %s", sensor, ts)
	}
	return ret, nil
}

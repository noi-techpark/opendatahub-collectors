// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/go-timeseries-writer-client/bdplib"
	"golang.org/x/exp/maps"
)

type stationcfg struct {
	station
	sensor_id    string
	sensor_start time.Time
}

type station struct {
	id      string
	name    string
	lat     float64
	lon     float64
	history []Sensorhistory
}

type Sensorhistory struct {
	Sensor_id    string
	Sensor_start time.Time
	Sensor_end   time.Time
}

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

func readStationCSV(path string) ([]stationcfg, error) {
	stationf := readCsv(path)
	stm := []stationcfg{}
	for _, st := range stationf[1:] {
		// in the old data collector, for raw datatypes the unit is always null instead of using the one from CSV. Is this correct?

		lat, err := strconv.ParseFloat(st[2], 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing lat float value %s: %w", st[2], err)
		}
		lon, err := strconv.ParseFloat(st[3], 64)
		if err != nil {
			return nil, fmt.Errorf("error parsing lon float value %s: %w", st[2], err)
		}
		sensor_start := time.Time{}
		if strings.TrimSpace(st[5]) != "" {
			sensor_start, err = time.Parse("2006.01.02", st[5])
			if err != nil {
				return nil, fmt.Errorf("error parsing sensor starting date string %s: %w", st[5], err)
			}
		}
		stm = append(stm, stationcfg{station: station{id: st[0], name: st[1], lat: lat, lon: lon}, sensor_id: st[4], sensor_start: sensor_start})
	}
	return stm, nil
}

func map2Bdp(s station, origin string) bdplib.Station {
	mapped := bdplib.CreateStation(s.id, s.name, "EnvironmentStation", s.lat, s.lon, origin)

	mapped.MetaData = make(map[string]interface{})

	if len(s.history) > 0 {
		currentSensor := s.history[len(s.history)-1]
		mapped.MetaData["sensor_id"] = currentSensor.Sensor_id
	}
	mapped.MetaData["sensor_history"] = s.history

	return mapped
}

func compileHistory(cfgs []stationcfg) ([]station, error) {
	smap := map[string]station{}
	sort.Slice(cfgs, func(i, j int) bool {
		l := cfgs[i]
		r := cfgs[j]
		if l.id < r.id {
			return true
		} else {
			return l.sensor_start.Before(r.sensor_start)
		}
	})

	for _, cfg := range cfgs {
		s, ok := smap[cfg.id]
		if !ok {
			s = cfg.station
		}

		if len(s.history) > 0 {
			s.history[len(s.history)-1].Sensor_end = cfg.sensor_start
		}
		s.history = append(s.history, Sensorhistory{Sensor_id: cfg.sensor_id, Sensor_start: cfg.sensor_start})

		smap[cfg.id] = s
	}
	return maps.Values(smap), nil
}

func currentStation(sts []station, sensor string, ts time.Time) (station, error) {
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

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"
	"time"

	"gotest.tools/v3/assert"
)

func TestLoadStations(t *testing.T) {
	cfgs, err := readStationCSV("../resources/stations.csv")
	if err != nil {
		t.Error("error loading CSV", err)
	}
	stations, err := compileHistory(cfgs)
	if err != nil {
		t.Error("error compiling station history", err)
	}

	for _, cfg := range stations {
		t.Logf("Station %s", cfg.id)
		for _, h := range cfg.history {
			t.Logf("   %s - %s: %s", h.sensor_start.Format("20060102"), h.sensor_end.Format("20060102"), h.sensor_id)
		}
	}
}

func assertSensorMapping(t *testing.T, sts []station, sensId string, ts time.Time, matchStationId string) {
	s, err := currentStation(sts, sensId, ts)
	assert.NilError(t, err, "failed matching sensor. expected %s", matchStationId)
	assert.Equal(t, s.id, matchStationId)
}

func TestSensorMapping(t *testing.T) {
	now := time.Now()
	sts := []station{
		{id: "t1", history: []Sensorhistory{
			{Sensor_id: "s2", Sensor_start: now.Add(-1 * time.Hour), Sensor_end: now},
			{Sensor_id: "s1", Sensor_start: now, Sensor_end: now.Add(time.Hour)},
			{Sensor_id: "s1", Sensor_start: now.Add(time.Hour), Sensor_end: now.Add(4 * time.Hour)},
		}},
		{id: "t2", history: []Sensorhistory{
			{Sensor_id: "s1", Sensor_start: now.Add(4 * time.Hour)},
		}},
	}

	assertSensorMapping(t, sts, "s1", now, "t1")
	assertSensorMapping(t, sts, "s1", now.Add(time.Hour), "t1")
	assertSensorMapping(t, sts, "s1", now.Add(time.Hour).Add(time.Minute), "t1")
	assertSensorMapping(t, sts, "s1", now.Add(5*time.Hour), "t2")
	_, err := currentStation(sts, "s1", now.Add(-6*time.Hour))
	if err == nil {
		t.Error("expected error, but got none")
	}
	_, err = currentStation(sts, "s2", now.Add(time.Hour))
	if err == nil {
		t.Error("expected error, but got none")
	}
}

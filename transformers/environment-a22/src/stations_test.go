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
	stations, err := readStationCSV("../resources/stations.csv")
	if err != nil {
		t.Error("error loading CSV", err)
	}

	s103 := stations["A22_KM_103-700"]
	defer dumpStationsHist(t, s103)
	assertSensorAt(t, s103, "", time.Date(2018, 03, 02, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ01", time.Date(2020, 03, 02, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ01", time.Date(2021, 03, 02, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ01", time.Date(2021, 05, 24, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "", time.Date(2021, 05, 25, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ01", time.Date(2021, 07, 01, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ02", time.Date(2024, 07, 21, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ14", time.Date(2024, 07, 29, 0, 0, 0, 0, time.UTC))
	assertSensorAt(t, s103, "AIRQ05", time.Date(2029, 07, 29, 0, 0, 0, 0, time.UTC))

	// check if latest tracks correctly
	assert.Equal(t, s103.latest_sensor, "AIRQ14")
	assert.Equal(t, stations["A22_KM_107-800"].latest_sensor, "")
	assert.Equal(t, stations["A22_KM_076-600"].latest_sensor, "")
}

func dumpStationsHist(t *testing.T, s station) {
	if t.Failed() {
		t.Logf("Dumping station history for id = %s", s.id)
		for _, h := range s.history {
			t.Logf("   %s - %s: %s", h.Sensor_start.Format("20060102"), h.Sensor_end.Format("20060102"), h.Sensor_id)
		}
	}
}

func assertSensorAt(t *testing.T, s station, sensor string, ts time.Time) {
	foundSensor := sensorAtTime(s, ts)
	assert.Equal(t, foundSensor, sensor, "Sensor for stations %s at time %s is %s, but expected %s", s.id, ts.String(), foundSensor, sensor)
}

func sensorAtTime(s station, ts time.Time) string {
	for _, h := range s.history {
		if (ts.After(h.Sensor_start) || ts.Equal(h.Sensor_start)) && (h.Sensor_end.IsZero() || ts.Before(h.Sensor_end)) {
			return h.Sensor_id
		}
	}
	return ""
}

func assertFindBySensor(t *testing.T, sts map[string]station, sensor string, ts time.Time, station string) {
	s, err := currentStation(sts, sensor, ts)
	assert.NilError(t, err, "failed matching sensor. expected %s", station)
	assert.Equal(t, s.id, station)
}

func TestSensorMapping(t *testing.T) {
	now := time.Now()
	sts := map[string]station{
		"t1": {id: "t1", history: []Sensorhistory{
			{Sensor_id: "s2", Sensor_start: now.Add(-1 * time.Hour), Sensor_end: now},
			{Sensor_id: "s1", Sensor_start: now, Sensor_end: now.Add(time.Hour)},
			{Sensor_id: "s1", Sensor_start: now.Add(time.Hour), Sensor_end: now.Add(4 * time.Hour)},
		}},
		"t2": {id: "t2", history: []Sensorhistory{
			{Sensor_id: "s1", Sensor_start: now.Add(4 * time.Hour)},
		}},
	}

	assertFindBySensor(t, sts, "s1", now, "t1")
	assertFindBySensor(t, sts, "s1", now.Add(time.Hour), "t1")
	assertFindBySensor(t, sts, "s1", now.Add(time.Hour).Add(time.Minute), "t1")
	assertFindBySensor(t, sts, "s1", now.Add(5*time.Hour), "t2")
	_, err := currentStation(sts, "s1", now.Add(-6*time.Hour))
	if err == nil {
		t.Error("expected error, but got none")
	}
	_, err = currentStation(sts, "s2", now.Add(time.Hour))
	if err == nil {
		t.Error("expected error, but got none")
	}
}

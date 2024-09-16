// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "testing"

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

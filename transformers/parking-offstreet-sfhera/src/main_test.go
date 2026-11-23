// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

var ts = time.Date(2026, 11, 23, 9, 0, 0, 0, time.UTC)

// testdata/out.json was captured from the pre-SDK transformer (direct MongoDB
// reads, go-bdp-client v1.0.0) run on the same payloads, so it proves the
// rewrite changes nothing the BDP receives. An empty table with the gate off
// is that transformer's behaviour.
func TestPayloadsMatchLegacyOutput(t *testing.T) {
	setTable(t, map[string]map[string]any{})
	rec := recordDiscovery(t)

	var want []bdpmock.BdpMockCalls
	require.NoError(t, testsuite.LoadOutput(&want, "testdata/out.json"))
	raws := loadRaws(t)
	require.Len(t, want, len(raws))

	for i, raw := range raws {
		m := mustRun(t, raw)
		bdpmock.CompareBdpMockCalls(t, want[i], m.Requests())

		wantJSON, err := json.Marshal(want[i])
		require.NoError(t, err)
		require.JSONEq(t, string(wantJSON), requestsJSON(t, m))
	}
	require.Len(t, rec.All(), 1)
}

func TestDataTypesMatchLegacyOutput(t *testing.T) {
	var want []bdpmock.BdpMockCalls
	require.NoError(t, testsuite.LoadOutput(&want, "testdata/out.json"))

	m := mustRun(t, loadRaws(t)[0])
	require.Equal(t, want[0].SyncedDataTypes, m.Requests().SyncedDataTypes)
}

func TestStationCodeAndMeasurementTimestamp(t *testing.T) {
	raw := rawOf(`{"uid":"3","park":"P","lat":"46.5","long":"11.3","in":"7","out":"2","floor":"-1","lots":10,"tot":"40","reserved":"0"}`, ts)
	m := mustRun(t, raw)

	s, ok := syncedStation(t, m)
	require.True(t, ok)
	require.Equal(t, "parking-bz:3:-1", s.Id)
	require.Equal(t, -1, s.MetaData["floor"])
	require.Equal(t, 40, s.MetaData["capacity"])

	dm := m.Requests().SyncedData[ParkingStation][0].Branch["parking-bz:3:-1"]
	require.Len(t, dm.Branch, 4)
	for name, v := range map[string]any{"free": 10, "occupied": 30, "entering-vehicles-today": 7, "exiting-vehicles-today": 2} {
		recs := dm.Branch[name].Data
		require.Len(t, recs, 1, name)
		require.Equal(t, v, recs[0].Value, name)
		require.Equal(t, ts.UnixMilli(), recs[0].Timestamp, name)
		require.EqualValues(t, 300, recs[0].Period, name)
	}
}

// A garbled number is not an error: the level is published with zeros, as it
// always has been.
func TestUnparsableNumbersBecomeZero(t *testing.T) {
	raw := rawOf(`{"uid":"3","park":"P","lat":"x","long":"","in":"?","out":"","floor":"2","lots":10,"tot":"n/a"}`, ts)
	m := mustRun(t, raw)

	s, _ := syncedStation(t, m)
	require.Zero(t, s.Latitude)
	require.Zero(t, s.Longitude)
	require.Equal(t, 0, s.MetaData["capacity"])
	dm := m.Requests().SyncedData[ParkingStation][0].Branch["parking-bz:3:2"]
	require.Equal(t, -10, dm.Branch["occupied"].Data[0].Value)
	require.Equal(t, 0, dm.Branch["entering-vehicles-today"].Data[0].Value)
}

func TestUndecodablePayloadWritesNothing(t *testing.T) {
	for name, rawdata := range map[string]string{
		"not base64": "%%%",
		"not json":   "bm90IGpzb24=",
		"wrong type": "eyJsb3RzIjoiMSJ9",
	} {
		t.Run(name, func(t *testing.T) {
			rec := recordDiscovery(t)
			raw := rawOf("", ts)
			raw.Rawdata = rawdata

			m, err := runRaw(t, raw)

			require.Error(t, err)
			require.Empty(t, m.Requests().SyncedStations)
			require.Empty(t, m.Requests().SyncedData)
			require.Empty(t, rec.All())
		})
	}
}

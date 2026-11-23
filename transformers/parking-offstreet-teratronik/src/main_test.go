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

// The provider's own measurement timestamp is ignored: the record's is used.
func TestMeasurementTimestampIsTheRecordsOwn(t *testing.T) {
	raw := rawOf(`{"id":"x","name_IT":"It","name_DE":"De","latitude":46.5,"longitude":11.1,"capacity":40,"measurements":[{"timestamp":"2001-01-01 00:00:00","availability":15}]}`, ts)
	m := mustRun(t, raw)

	s, ok := syncedStation(t, m)
	require.True(t, ok)
	require.Equal(t, "parking-sta-marlengo", s.Id)
	require.Equal(t, "It", s.Name)
	require.Equal(t, "It", s.MetaData["name_IT"])
	require.Equal(t, "De", s.MetaData["name_DE"])
	require.NotContains(t, s.MetaData, "name_it")
	require.Equal(t, "Marling - Marlengo", s.MetaData["municipality"])

	dm := m.Requests().SyncedData[ParkingStation][0].Branch["parking-sta-marlengo"]
	require.Len(t, dm.Branch, 2)
	for name, v := range map[string]any{"free": 15, "occupied": 25} {
		recs := dm.Branch[name].Data
		require.Len(t, recs, 1, name)
		require.Equal(t, v, recs[0].Value, name)
		require.Equal(t, ts.UnixMilli(), recs[0].Timestamp, name)
		require.EqualValues(t, 300, recs[0].Period, name)
	}
}

// Only the first measurement counts, and occupied can go negative: both are
// what the transformer has always done.
func TestFirstMeasurementOnlyAndNoClamping(t *testing.T) {
	raw := rawOf(`{"capacity":10,"name_IT":"It","measurements":[{"availability":12},{"availability":3}]}`, ts)
	m := mustRun(t, raw)

	dm := m.Requests().SyncedData[ParkingStation][0].Branch["parking-sta-marlengo"]
	require.Equal(t, 12, dm.Branch["free"].Data[0].Value)
	require.Equal(t, -2, dm.Branch["occupied"].Data[0].Value)
}

// A payload without measurements used to crash the pod after the station was
// synced; it is now refused before anything is written.
func TestPayloadWithoutMeasurementsWritesNothing(t *testing.T) {
	rec := recordDiscovery(t)

	m, err := runRaw(t, rawOf(`{"capacity":10,"name_IT":"It","measurements":[]}`, ts))

	require.Error(t, err)
	require.Empty(t, m.Requests().SyncedStations)
	require.Empty(t, m.Requests().SyncedData)
	require.Empty(t, rec.All())
}

func TestUndecodablePayloadWritesNothing(t *testing.T) {
	for name, rawdata := range map[string]string{
		"not base64": "%%%",
		"not json":   "bm90IGpzb24=",
		"wrong type": "eyJjYXBhY2l0eSI6IngifQ==",
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

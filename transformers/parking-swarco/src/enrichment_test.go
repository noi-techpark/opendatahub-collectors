// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

const (
	singleFile = "testdata/in.json"
	areasFile  = "testdata/in-areas.json"

	keySingle   = "100308"
	keyFacility = "200001"
	keyAreaA    = "200001-A"
	keyAreaB    = "200001-B"
)

func transformFile(t *testing.T, file string) *bdpmock.BdpMock {
	t.Helper()
	var in SwarcoData
	require.NoError(t, testsuite.LoadInputData(&in, file))
	raw := rdb.Raw[SwarcoData]{Rawdata: in, Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.NoError(t, Transform(context.TODO(), b, &raw))
	return b.(*bdpmock.BdpMock)
}

func stationsOf(mock *bdpmock.BdpMock, stationType string) map[string]bdplib.Station {
	out := map[string]bdplib.Station{}
	for _, call := range mock.Requests().SyncedStations[stationType] {
		for _, s := range call.Stations {
			out[s.Id] = s
		}
	}
	return out
}

func pushedStations(mock *bdpmock.BdpMock) []string {
	var ids []string
	for _, dms := range mock.Requests().SyncedData {
		for _, dm := range dms {
			for id := range dm.Branch {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func requestsJSON(t *testing.T, mock *bdpmock.BdpMock) string {
	t.Helper()
	b, err := json.Marshal(mock.Requests())
	require.NoError(t, err)
	return string(b)
}

// An empty table with the gate off is today's behaviour: both committed
// snapshots still hold.
func TestEmptyTableGateOffMatchesSnapshots(t *testing.T) {
	for _, tc := range []struct{ in, out string }{
		{singleFile, "testdata/out.json"},
		{areasFile, "testdata/out-areas.json"},
	} {
		setTable(t, map[string]map[string]any{})
		mock := transformFile(t, tc.in)

		var want bdpmock.BdpMockCalls
		require.NoError(t, testsuite.LoadOutput(&want, tc.out))
		want.SyncedDataTypes = nil
		bdpmock.CompareBdpMockCalls(t, want, mock.Requests())
	}
}

func TestEnrichmentOverlay(t *testing.T) {
	loadFixtures(t)

	single := stationsOf(transformFile(t, singleFile), StationTypeParkingStation)[stationCode(keySingle)]
	require.Equal(t, "Zumis operator name", single.Name)
	require.InDelta(t, 46.5, single.Latitude, 1e-9)
	require.InDelta(t, 11.3, single.Longitude, 1e-9)
	require.Equal(t, "Parkplatz Zumis", single.MetaData["name_de"])
	require.Equal(t, "Parcheggio Zumis", single.MetaData["name_it"])
	require.Equal(t, "Parcheggio Zumis", single.MetaData["standard_name"])
	require.Equal(t, keySingle, single.MetaData["provider_id"])

	mock := transformFile(t, areasFile)
	facility := stationsOf(mock, StationTypeParkingFacility)[stationCode(keyFacility)]
	require.Equal(t, "Test Facility operator name", facility.Name)
	require.Equal(t, "Parcheggio di prova", facility.MetaData["name_it"])
	require.Contains(t, facility.MetaData, "netex_parking")

	areas := stationsOf(mock, StationTypeParkingStation)
	a := areas[stationCode(keyAreaA)]
	require.Equal(t, "Area Nord operator name", a.Name)
	require.Equal(t, "Area Nord IT", a.MetaData["name_it"])
	require.InDelta(t, 46.7, a.Latitude, 1e-9, "no gps in the record")
	// What the transformation owns survives the overlay.
	require.Equal(t, stationCode(keyFacility), a.ParentStation)
	require.Equal(t, StationTypeParkingFacility, a.ParentStationType)
	require.Equal(t, keyAreaA, a.MetaData["provider_id"])
	require.Equal(t, "60", fmt.Sprint(a.MetaData["capacity"]))

	b := areas[stationCode(keyAreaB)]
	require.Equal(t, "Area Sud", b.Name)
	require.NotContains(t, b.MetaData, "name_it")
}

func TestGateOffPublishesEverythingWithoutRecord(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	mock := transformFile(t, areasFile)

	require.Len(t, stationsOf(mock, StationTypeParkingFacility), 1)
	require.Len(t, stationsOf(mock, StationTypeParkingStation), 2)

	for key, reason := range map[string]string{keyFacility: "", keyAreaA: "", keyAreaB: "no-record"} {
		e, ok := rec.Get(key)
		require.True(t, ok, key)
		require.Equal(t, discovery.StatusPublished, e.Status, key)
		require.Equal(t, reason, e.Reason, key)
	}
}

// A facility is published whole or not at all: one area without a record takes
// the facility and its other areas with it.
func TestGateOnWithholdsTheWholeFacilityWhenAnAreaLacksARecord(t *testing.T) {
	loadFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)

	mock := transformFile(t, areasFile)

	require.Empty(t, stationsOf(mock, StationTypeParkingFacility))
	require.Empty(t, stationsOf(mock, StationTypeParkingStation))
	require.Empty(t, pushedStations(mock), "no measurement for a withheld station")

	want := map[string]string{
		keyAreaB:    "no-record",
		keyFacility: "parent-incomplete",
		keyAreaA:    "parent-incomplete",
	}
	for key, reason := range want {
		e, ok := rec.Get(key)
		require.True(t, ok, key)
		require.Equal(t, discovery.StatusWithheld, e.Status, key)
		require.Equal(t, reason, e.Reason, key)
	}
	a, _ := rec.Get(keyAreaA)
	require.Equal(t, keyFacility, a.Parent)
}

func TestGateOnWithholdsFacilityWithoutRecord(t *testing.T) {
	setTable(t, map[string]map[string]any{
		keyAreaA: {"name": "A"},
		keyAreaB: {"name": "B"},
	})
	setGate(t, true)
	rec := recordDiscovery(t)

	mock := transformFile(t, areasFile)

	require.Empty(t, stationsOf(mock, StationTypeParkingStation))
	f, _ := rec.Get(keyFacility)
	require.Equal(t, discovery.StatusWithheld, f.Status)
	require.Equal(t, "no-record", f.Reason)
	a, _ := rec.Get(keyAreaA)
	require.Equal(t, "parent-incomplete", a.Reason)
}

func TestGateOnPublishesFacilityWhenEveryStationHasARecord(t *testing.T) {
	setTable(t, map[string]map[string]any{
		keyFacility: {"name": "F"},
		keyAreaA:    {"name": "A"},
		keyAreaB:    {"name": "B"},
	})
	setGate(t, true)
	rec := recordDiscovery(t)

	mock := transformFile(t, areasFile)

	require.Len(t, stationsOf(mock, StationTypeParkingFacility), 1)
	require.Len(t, stationsOf(mock, StationTypeParkingStation), 2)
	require.NotEmpty(t, pushedStations(mock))
	for _, key := range []string{keyFacility, keyAreaA, keyAreaB} {
		e, _ := rec.Get(key)
		require.Equal(t, discovery.StatusPublished, e.Status, key)
		require.Empty(t, e.Reason, key)
	}
}

func TestGateOnWithholdsSingleStationWithoutRecord(t *testing.T) {
	setTable(t, map[string]map[string]any{})
	setGate(t, true)
	rec := recordDiscovery(t)

	mock := transformFile(t, singleFile)

	require.Empty(t, stationsOf(mock, StationTypeParkingStation))
	require.Empty(t, pushedStations(mock))
	e, ok := rec.Get(keySingle)
	require.True(t, ok, "a withheld station is still reported")
	require.Equal(t, discovery.StatusWithheld, e.Status)
	require.Equal(t, "no-record", e.Reason)
}

func TestDiscoveryReportsTheProviderView(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	transformFile(t, areasFile)

	require.Equal(t, 1, rec.Completes, "the static list is the full station set")
	require.Equal(t, 1, rec.Flushes)

	// The entry holds the provider's name and position, not the record's.
	f, _ := rec.Get(keyFacility)
	require.Equal(t, "P Test Facility", f.Name)
	require.InDelta(t, 46.7, *f.Lat, 1e-9)
	require.Empty(t, f.Parent)
	require.Equal(t, 100, f.Data["capacity"])

	a, _ := rec.Get(keyAreaA)
	require.Equal(t, "Area Nord", a.Name)
	require.Equal(t, keyFacility, a.Parent)
	require.Equal(t, 60, a.Data["capacity"])
}

func TestRefusedPayloadIsNotReported(t *testing.T) {
	rec := recordDiscovery(t)
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	raw := rdb.Raw[SwarcoData]{Timestamp: time.Now()}
	require.Error(t, Transform(context.TODO(), b, &raw))
	require.Zero(t, rec.Completes)
	require.Zero(t, rec.Flushes)
}

// Adopting a Seed must change nothing: every station built with its Seed as
// the record is the station built without one.
func TestSeedReproducesTheUnenrichedStation(t *testing.T) {
	for _, file := range []string{singleFile, areasFile} {
		rec := recordDiscovery(t)
		setTable(t, map[string]map[string]any{})
		plain := transformFile(t, file)

		seeds := map[string]map[string]any{}
		for key, e := range rec.All() {
			require.NotEmpty(t, e.Seed, key)
			seeds[key] = e.Seed
		}
		require.NotEmpty(t, seeds)

		setTable(t, seeds)
		setGate(t, true)
		seeded := transformFile(t, file)
		setGate(t, false)

		require.JSONEq(t, requestsJSON(t, plain), requestsJSON(t, seeded), file)
	}
}

func TestSeedCarriesParkingKeysOnly(t *testing.T) {
	rec := recordDiscovery(t)
	transformFile(t, areasFile)

	a, _ := rec.Get(keyAreaA)
	require.Equal(t, map[string]any{
		"name": "Area Nord",
		"gps":  map[string]any{"lat": 46.7, "lon": 11.7},
	}, a.Seed)
}

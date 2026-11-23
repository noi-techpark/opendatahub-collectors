// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

func loadPayload(t *testing.T) AffluencesPayload {
	t.Helper()
	var in AffluencesPayload
	require.NoError(t, testsuite.LoadInputData(&in, "testdata/in1.json"))
	return in
}

func syncedStations(mock *bdpmock.BdpMock) map[string]bdplib.Station {
	out := map[string]bdplib.Station{}
	for _, call := range mock.Requests().SyncedStations[StationTypeParkingStation] {
		for _, s := range call.Stations {
			out[s.Id] = s
		}
	}
	return out
}

func requestsJSON(t *testing.T, mock *bdpmock.BdpMock) string {
	t.Helper()
	b, err := json.Marshal(mock.Requests())
	require.NoError(t, err)
	return string(b)
}

// An empty table with the gate off is today's behaviour: the committed
// snapshot still holds.
func TestEmptyTableGateOffMatchesSnapshot(t *testing.T) {
	refTable = reftable.NewStatic(map[string]map[string]any{})
	t.Cleanup(func() { refTable = nil })
	rec := recordDiscovery(t)

	mock := runTransform(t, loadPayload(t))

	var want bdpmock.BdpMockCalls
	require.NoError(t, testsuite.LoadOutput(&want, "testdata/out1.json"))
	// The snapshot starts with the data types sync, which runTransform skips.
	want.SyncedDataTypes = nil
	bdpmock.CompareBdpMockCalls(t, want, mock.Requests())

	for _, id := range []string{siteP1, siteP2} {
		e, ok := rec.Get(id)
		require.True(t, ok)
		require.Equal(t, discovery.StatusPublished, e.Status)
		require.Equal(t, "no-record", e.Reason)
	}
}

func TestEnrichmentOverlay(t *testing.T) {
	loadFixtures(t)

	stations := syncedStations(runTransform(t, loadPayload(t)))

	p1 := stations[scode(siteP1)]
	require.Equal(t, "P1 Passo delle Erbe", p1.Name)
	require.InDelta(t, 46.6818, p1.Latitude, 1e-9)
	require.InDelta(t, 11.8225, p1.Longitude, 1e-9)
	require.Equal(t, "Parkplatz Würzjoch", p1.MetaData["name_de"])
	require.Equal(t, "Parcheggio Passo delle Erbe", p1.MetaData["name_it"])
	require.Equal(t, "Test lad P1", p1.MetaData["name_lad"])
	require.Equal(t, "Parcheggio Passo delle Erbe", p1.MetaData["standard_name"])
	netex, ok := p1.MetaData["netex_parking"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "open", netex["layout"])

	// What the transformation owns survives the overlay.
	require.Equal(t, siteP1, p1.MetaData["provider_id"])
	require.Equal(t, "100", fmt.Sprint(p1.MetaData["capacity"]))

	// A station without a record is untouched.
	p2 := stations[scode(siteP2)]
	require.Equal(t, "Parcheggio Passo delle Erbe - Pe de Börz (P2)", p2.Name)
	require.NotContains(t, p2.MetaData, "name_de")
}

func TestGateOffPublishesStationWithoutRecord(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	mock := runTransform(t, loadPayload(t))

	require.Len(t, syncedStations(mock), 2)
	p1, _ := rec.Get(siteP1)
	require.Equal(t, discovery.StatusPublished, p1.Status)
	require.Empty(t, p1.Reason)
	p2, _ := rec.Get(siteP2)
	require.Equal(t, discovery.StatusPublished, p2.Status)
	require.Equal(t, "no-record", p2.Reason)
}

func TestGateOnWithholdsStationWithoutRecord(t *testing.T) {
	loadFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)

	mock := runTransform(t, loadPayload(t))

	stations := syncedStations(mock)
	require.Len(t, stations, 1)
	require.Contains(t, stations, scode(siteP1))
	_, has := recordValue(mock, scode(siteP2), DataTypeOpen)
	require.False(t, has, "no measurement for a withheld station")
	_, has = recordValue(mock, scode(siteP1), DataTypeOpen)
	require.True(t, has)

	p1, _ := rec.Get(siteP1)
	require.Equal(t, discovery.StatusPublished, p1.Status)
	require.Empty(t, p1.Reason)
	p2, ok := rec.Get(siteP2)
	require.True(t, ok, "a withheld station is still reported")
	require.Equal(t, discovery.StatusWithheld, p2.Status)
	require.Equal(t, "no-record", p2.Reason)
}

func TestGateOnWithEmptyTableWithholdsEverything(t *testing.T) {
	setGate(t, true)
	rec := recordDiscovery(t)

	mock := runTransform(t, loadPayload(t))

	require.Empty(t, syncedStations(mock))
	require.Len(t, rec.All(), 2)
}

func TestDiscoveryReportsTheProviderView(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	runTransform(t, loadPayload(t))

	require.Equal(t, 1, rec.Completes, "the payload is the full site set")
	require.Equal(t, 1, rec.Flushes)

	// The entry holds the provider's name and position, not the record's.
	e, ok := rec.Get(siteP1)
	require.True(t, ok)
	require.Equal(t, "Parcheggio Passo delle Erbe (P1)", e.Name)
	require.InDelta(t, 46.681725, *e.Lat, 1e-9)
	require.InDelta(t, 11.822493, *e.Lon, 1e-9)
	require.Empty(t, e.Parent)
	require.Equal(t, 100, e.Data["capacity"])
	require.NotContains(t, e.Data, "occupancy")
}

// Adopting a Seed must change nothing: the station built with the Seed as its
// record is the station built without one.
func TestSeedReproducesTheUnenrichedStation(t *testing.T) {
	rec := recordDiscovery(t)
	refTable = reftable.NewStatic(map[string]map[string]any{})
	t.Cleanup(func() { refTable = nil })
	plain := runTransform(t, loadPayload(t))

	seeds := map[string]map[string]any{}
	for key, e := range rec.All() {
		require.NotEmpty(t, e.Seed, key)
		seeds[key] = e.Seed
	}
	require.Len(t, seeds, 2)

	refTable = reftable.NewStatic(seeds)
	seeded := runTransform(t, loadPayload(t))

	require.JSONEq(t, requestsJSON(t, plain), requestsJSON(t, seeded))
}

func TestSeedCarriesParkingKeysOnly(t *testing.T) {
	rec := recordDiscovery(t)
	runTransform(t, loadPayload(t))

	e, _ := rec.Get(siteP1)
	require.Len(t, e.Seed, 2)
	require.Equal(t, "Parcheggio Passo delle Erbe (P1)", e.Seed["name"])
	require.Equal(t, map[string]any{"lat": 46.681725, "lon": 11.822493}, e.Seed["gps"])
}

func TestRecordClearingNameKeepsStationName(t *testing.T) {
	refTable = reftable.NewStatic(map[string]map[string]any{siteP1: {"name": ""}})
	t.Cleanup(func() { refTable = nil })

	p1 := syncedStations(runTransform(t, loadPayload(t)))[scode(siteP1)]
	require.Equal(t, "Parcheggio Passo delle Erbe (P1)", p1.Name)
}

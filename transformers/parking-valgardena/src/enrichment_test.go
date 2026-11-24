// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

func transformMetadata(t *testing.T) *bdpmock.BdpMock {
	t.Helper()
	var in []ParkingMetadata
	require.NoError(t, testsuite.LoadInputData(&in, "testdata/in-metadata.json"))
	raw := rdb.Raw[[]ParkingMetadata]{Rawdata: in, Timestamp: fixedTime(t)}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.NoError(t, TransformMetadata(context.TODO(), b, &raw))
	return b.(*bdpmock.BdpMock)
}

func transformData(t *testing.T) *bdpmock.BdpMock {
	t.Helper()
	var in ParkingData
	require.NoError(t, testsuite.LoadInputData(&in, "testdata/in-data.json"))
	raw := rdb.Raw[ParkingData]{Rawdata: in, Timestamp: fixedTime(t)}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.NoError(t, TransformData(context.TODO(), b, &raw))
	return b.(*bdpmock.BdpMock)
}

func synced(mock *bdpmock.BdpMock) map[string]bdplib.Station {
	out := map[string]bdplib.Station{}
	for _, call := range mock.Requests().SyncedStations[stationType] {
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
// snapshots still hold.
func TestEmptyTableGateOffMatchesSnapshots(t *testing.T) {
	setTable(t, map[string]map[string]any{})

	var wantMeta, wantData bdpmock.BdpMockCalls
	require.NoError(t, testsuite.LoadOutput(&wantMeta, "testdata/out-metadata.json"))
	require.NoError(t, testsuite.LoadOutput(&wantData, "testdata/out-data.json"))
	wantMeta.SyncedDataTypes, wantData.SyncedDataTypes = nil, nil

	bdpmock.CompareBdpMockCalls(t, wantMeta, transformMetadata(t).Requests())
	bdpmock.CompareBdpMockCalls(t, wantData, transformData(t).Requests())
}

func TestEnrichmentOverlay(t *testing.T) {
	loadFixtures(t)

	stations := synced(transformMetadata(t))

	sella := stations[stationId("passo_sella")]
	require.Equal(t, "Passo Sella operator name", sella.Name)
	require.InDelta(t, 46.5097, sella.Latitude, 1e-9)
	require.InDelta(t, 11.7577, sella.Longitude, 1e-9)
	require.Equal(t, "Sellajoch Parkplatz", sella.MetaData["name_de"])
	require.Equal(t, "Parcheggio Passo Sella", sella.MetaData["name_it"])
	require.Equal(t, "Test lad Sella", sella.MetaData["name_lad"])
	require.Equal(t, "Parcheggio Passo Sella", sella.MetaData["standard_name"])
	require.Contains(t, sella.MetaData, "netex_parking")
	// What the transformation owns survives, and the provider's own names stay.
	require.Equal(t, "199", fmt.Sprint(sella.MetaData["capacity"]))
	require.Equal(t, "Passo Sella", sella.MetaData["name_IT"])
	require.Equal(t, "valgardena", sella.MetaData["municipality"])

	// A record naming nothing else leaves the position alone.
	cristauta := stations[stationId("cristauta")]
	require.Equal(t, "Cristauta operator name", cristauta.Name)
	require.InDelta(t, 46.566344, cristauta.Latitude, 1e-9)

	require.Equal(t, "Garage Pana", stations[stationId("pana")].Name)
}

func TestGateOffPublishesEverythingWithoutRecord(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	stations := synced(transformMetadata(t))

	require.Len(t, stations, 19)
	require.Len(t, rec.All(), 19)
	e, _ := rec.Get(stationId("pana"))
	require.Equal(t, discovery.StatusPublished, e.Status)
	require.Equal(t, "no-record", e.Reason)
	e, _ = rec.Get(stationId("passo_sella"))
	require.Equal(t, discovery.StatusPublished, e.Status)
	require.Empty(t, e.Reason)
}

func TestGateOnWithholdsStationsWithoutRecord(t *testing.T) {
	loadFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)

	stations := synced(transformMetadata(t))

	require.Len(t, stations, 2)
	require.Contains(t, stations, stationId("passo_sella"))
	require.Contains(t, stations, stationId("cristauta"))

	e, ok := rec.Get(stationId("pana"))
	require.True(t, ok, "a withheld station is still reported")
	require.Equal(t, discovery.StatusWithheld, e.Status)
	require.Equal(t, "no-record", e.Reason)
	e, _ = rec.Get(stationId("cristauta"))
	require.Equal(t, discovery.StatusPublished, e.Status)
}

// The data queue must not push measurements for a station the metadata queue
// withholds.
func TestGateOnSkipsMeasurementsOfWithheldStation(t *testing.T) {
	setGate(t, true)
	setTable(t, map[string]map[string]any{})
	require.Empty(t, transformData(t).Requests().SyncedData)

	loadFixtures(t)
	require.NotEmpty(t, transformData(t).Requests().SyncedData[stationType])
}

func TestGateOffPushesMeasurementsWithoutRecord(t *testing.T) {
	setTable(t, map[string]map[string]any{})
	require.NotEmpty(t, transformData(t).Requests().SyncedData[stationType])
}

func TestDiscoveryReportsTheProviderView(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	transformMetadata(t)

	require.Equal(t, 1, rec.Completes, "the metadata payload is the full station set")
	require.Equal(t, 1, rec.Flushes)

	// The entry holds the provider's name and position, not the record's.
	e, _ := rec.Get(stationId("passo_sella"))
	require.Equal(t, "Passo Sella", e.Name)
	require.InDelta(t, 46.509610662844, *e.Lat, 1e-12)
	require.InDelta(t, 11.757708692675, *e.Lon, 1e-12)
	require.Equal(t, 199, e.Data["capacity"])
	require.Equal(t, "Sellajoch", e.Data["name_de"])
	require.Equal(t, "aesys", e.Data["provider"])
}

func TestEmptyMetadataIsNotAFullSet(t *testing.T) {
	rec := recordDiscovery(t)
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	raw := rdb.Raw[[]ParkingMetadata]{Timestamp: fixedTime(t)}
	require.NoError(t, TransformMetadata(context.TODO(), b, &raw))
	require.Zero(t, rec.Completes)
	require.Equal(t, 1, rec.Flushes)
}

// Adopting a Seed must change nothing: every station built with its Seed as
// the record is the station built without one.
func TestSeedReproducesTheUnenrichedStation(t *testing.T) {
	rec := recordDiscovery(t)
	setTable(t, map[string]map[string]any{})
	plain := transformMetadata(t)

	seeds := map[string]map[string]any{}
	for key, e := range rec.All() {
		require.NotEmpty(t, e.Seed, key)
		seeds[key] = e.Seed
	}
	require.Len(t, seeds, 19)

	setTable(t, seeds)
	setGate(t, true)
	seeded := transformMetadata(t)

	require.JSONEq(t, requestsJSON(t, plain), requestsJSON(t, seeded))
}

func TestSeedCarriesParkingKeysOnly(t *testing.T) {
	rec := recordDiscovery(t)
	transformMetadata(t)

	e, _ := rec.Get(stationId("passo_sella"))
	require.Equal(t, map[string]any{
		"name": "Passo Sella",
		"gps":  map[string]any{"lat": 46.509610662844, "lon": 11.757708692675},
	}, e.Seed)
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/stretchr/testify/require"
)

func TestEnrichmentOverlay(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	s, ok := syncedStation(t, mustRun(t, loadRaws(t)[0]))
	require.True(t, ok)

	require.Equal(t, "Parcheggio Macello", s.Name)
	require.InDelta(t, 46.4962, s.Latitude, 1e-9)
	require.InDelta(t, 11.3678, s.Longitude, 1e-9)
	require.Equal(t, "Parkplatz Schlachthof", s.MetaData["name_de"])
	require.Equal(t, "Parcheggio Macello", s.MetaData["name_it"])
	require.Equal(t, "Macello parking", s.MetaData["name_en"])
	require.Equal(t, "Test lad macello", s.MetaData["name_lad"])
	require.Equal(t, "Parcheggio Macello Bolzano", s.MetaData["standard_name"])
	netex, ok := s.MetaData["netex_parking"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, "underground", netex["layout"])

	// What the transformation owns survives the overlay.
	require.Equal(t, "370", fmt.Sprint(s.MetaData["capacity"]))
	require.Equal(t, "0", fmt.Sprint(s.MetaData["floor"]))
	require.Equal(t, "Bolzano - Bozen", s.MetaData["municipality"])

	e, _ := rec.Get(stationKey)
	require.Equal(t, discovery.StatusPublished, e.Status)
	require.Empty(t, e.Reason)
}

func TestGateOffPublishesStationWithoutRecord(t *testing.T) {
	setTable(t, map[string]map[string]any{"parking-bz:other:0": {"name": "x"}})
	rec := recordDiscovery(t)

	m := mustRun(t, loadRaws(t)[0])

	s, ok := syncedStation(t, m)
	require.True(t, ok)
	require.Equal(t, "TURIST-PARKING", s.Name)
	require.NotContains(t, s.MetaData, "name_de")
	e, _ := rec.Get(stationKey)
	require.Equal(t, discovery.StatusPublished, e.Status)
	require.Equal(t, "no-record", e.Reason)
}

func TestGateOnWithholdsStationWithoutRecord(t *testing.T) {
	setTable(t, map[string]map[string]any{})
	setGate(t, true)
	rec := recordDiscovery(t)

	m := mustRun(t, loadRaws(t)[0])

	require.Empty(t, m.Requests().SyncedStations)
	require.Empty(t, m.Requests().SyncedData)
	e, ok := rec.Get(stationKey)
	require.True(t, ok, "a withheld station is still reported")
	require.Equal(t, discovery.StatusWithheld, e.Status)
	require.Equal(t, "no-record", e.Reason)
	require.Equal(t, 1, rec.Flushes)
}

func TestGateOnPublishesStationWithRecord(t *testing.T) {
	loadFixtures(t)
	setGate(t, true)

	_, ok := syncedStation(t, mustRun(t, loadRaws(t)[0]))
	require.True(t, ok)
}

func TestDiscoveryReportsTheProviderView(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	mustRun(t, loadRaws(t)[0])

	require.Equal(t, 0, rec.Completes, "a payload is one station, not the full set")
	require.Equal(t, 1, rec.Flushes)

	// The entry holds the provider's name and position, not the record's.
	e, ok := rec.Get(stationKey)
	require.True(t, ok)
	require.Equal(t, "TURIST-PARKING", e.Name)
	require.InDelta(t, 46.49623289133699, *e.Lat, 1e-12)
	require.InDelta(t, 11.367794908451254, *e.Lon, 1e-12)
	require.Empty(t, e.Parent)
	require.Equal(t, map[string]any{"capacity": 370, "floor": 0, "uid": "8"}, e.Data)
}

// Adopting a Seed must change nothing: the station built with the Seed as its
// record is the station built without one.
func TestSeedReproducesTheUnenrichedStation(t *testing.T) {
	for _, raw := range loadRaws(t) {
		setTable(t, map[string]map[string]any{})
		rec := recordDiscovery(t)
		plain := mustRun(t, raw)

		e, ok := rec.Get(stationKey)
		require.True(t, ok)
		require.NotEmpty(t, e.Seed)

		setTable(t, map[string]map[string]any{stationKey: storedRecord(t, e.Seed)})
		seeded := mustRun(t, raw)

		require.JSONEq(t, requestsJSON(t, plain), requestsJSON(t, seeded))
	}
}

// storedRecord is what the lake hands back for a seed: JSON with numbers kept
// as literals.
func storedRecord(t *testing.T, seed map[string]any) map[string]any {
	t.Helper()
	b, err := json.Marshal(seed)
	require.NoError(t, err)
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var out map[string]any
	require.NoError(t, d.Decode(&out))
	return out
}

func TestSeedCarriesParkingKeysOnly(t *testing.T) {
	rec := recordDiscovery(t)
	mustRun(t, loadRaws(t)[0])

	e, _ := rec.Get(stationKey)
	require.Len(t, e.Seed, 2)
	require.Equal(t, "TURIST-PARKING", e.Seed["name"])
	require.Equal(t, map[string]any{"lat": 46.49623289133699, "lon": 11.367794908451254}, e.Seed["gps"])
}

func TestRecordClearingNameKeepsStationName(t *testing.T) {
	setTable(t, map[string]map[string]any{stationKey: {"name": ""}})

	s, _ := syncedStation(t, mustRun(t, loadRaws(t)[0]))
	require.Equal(t, "TURIST-PARKING", s.Name)
}

func TestNoTableBehavesAsEmptyTable(t *testing.T) {
	refTable = nil
	m := mustRun(t, loadRaws(t)[0])
	_, ok := syncedStation(t, m)
	require.True(t, ok)
}

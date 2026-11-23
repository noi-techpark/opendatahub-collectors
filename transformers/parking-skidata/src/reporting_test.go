// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/stretchr/testify/require"
)

func recordDiscovery(t *testing.T) *discovery.Recorder {
	t.Helper()
	rec := discovery.NewRecorder()
	discoveryObserver = rec
	t.Cleanup(func() { discoveryObserver = noopObserver{} })
	return rec
}

func setGate(t *testing.T, on bool) {
	t.Helper()
	prev := discoveryEnv.REFTABLE_GATE
	discoveryEnv.REFTABLE_GATE = on
	t.Cleanup(func() { discoveryEnv.REFTABLE_GATE = prev })
}

func named(ev ParkingEvent, carpark string) ParkingEvent {
	ev.Carpark.Name = carpark
	return ev
}

func clibID(providerID string) string { return clib.GenerateID(ID_TEMPLATE, providerID) }

func stationIDs(t *testing.T, b bdplib.Bdp) []string {
	t.Helper()
	var ids []string
	for _, s := range syncedStations_(t, b) {
		ids = append(ids, s.Id)
	}
	return ids
}

func TestDiscoveryReportsEnrichedStationsFromTheRegistry(t *testing.T) {
	b := loadTestFixtures(t)
	rec := recordDiscovery(t)

	feed(t, b, named(event(607440, 1, catTotal, 10, 90, "Totale"), "Parcheggio 1"))

	carpark, ok := rec.Get("0607440_1")
	require.True(t, ok)
	require.Equal(t, discovery.StatusPublished, carpark.Status)
	require.Empty(t, carpark.Reason)
	require.Equal(t, "Parcheggio 1", carpark.Name, "the provider's name, not the record's")
	require.Equal(t, "0607440", carpark.Parent)
	require.Equal(t, map[string]any{"facility_id": "0607440", "carpark_id": 1}, carpark.Data)
	require.Nil(t, carpark.Lat)
	require.Nil(t, carpark.Lon)
	require.Nil(t, carpark.Seed)

	facility, ok := rec.Get("0607440")
	require.True(t, ok)
	require.Equal(t, discovery.StatusPublished, facility.Status)
	require.Empty(t, facility.Parent)

	require.Equal(t, 1, rec.Flushes)
	require.Equal(t, 0, rec.Completes, "events are not a snapshot of the provider's stations")
}

func TestDiscoveryGateOffPublishesUnenrichedStations(t *testing.T) {
	b := loadTestFixtures(t)
	rec := recordDiscovery(t)

	feed(t, b, named(event(999999, 0, catTotal, 10, 50, "Totale"), "Unknown carpark"))

	for _, key := range []string{"0999999", "0999999_0"} {
		e, ok := rec.Get(key)
		require.True(t, ok, key)
		require.Equal(t, discovery.StatusPublished, e.Status, key)
		require.Equal(t, "no-record", e.Reason, key)
	}
	require.Len(t, stationIDs(t, b), 2)
	require.Equal(t, 10, records(t, b, stationType)["occupied"])
}

func TestDiscoveryGateOnWithholdsStationsWithoutRecord(t *testing.T) {
	b := loadTestFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)

	feed(t, b, named(event(999999, 0, catTotal, 10, 50, "Totale"), "Unknown carpark"))

	for _, key := range []string{"0999999", "0999999_0"} {
		e, _ := rec.Get(key)
		require.Equal(t, discovery.StatusWithheld, e.Status, key)
		require.Equal(t, "no-record", e.Reason, key)
	}
	carpark, _ := rec.Get("0999999_0")
	require.Equal(t, "0999999", carpark.Parent)
	require.Equal(t, "Unknown carpark", carpark.Name)

	require.Empty(t, stationIDs(t, b), "a withheld station reached BDP")
	require.Empty(t, records(t, b, stationType), "measurements of a withheld station reached BDP")

	// Described stations keep flowing in the same process.
	feed(t, b, named(event(607440, 1, catTotal, 10, 90, "Totale"), "Parcheggio 1"))
	ok, _ := rec.Get("0607440_1")
	require.Equal(t, discovery.StatusPublished, ok.Status)
	require.Contains(t, stationIDs(t, b), clibID("0607440_1"))
	require.Equal(t, 10, records(t, b, stationType)["occupied"])
}

func TestDiscoveryGateOnCarparkNeedsItsFacility(t *testing.T) {
	b := loadTestFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)
	refTable.(*reftable.Static[map[string]any]).Set("0999999_0", map[string]any{"gps": map[string]any{"lat": 46.0, "lon": 11.0}})

	feed(t, b, event(999999, 0, catTotal, 10, 50, "Carpark"))

	carpark, _ := rec.Get("0999999_0")
	require.Equal(t, discovery.StatusWithheld, carpark.Status)
	require.Equal(t, "parent-incomplete", carpark.Reason)
	facility, _ := rec.Get("0999999")
	require.Equal(t, "no-record", facility.Reason)
	require.Empty(t, stationIDs(t, b))
}

func TestDiscoveryEmptyRecordIsNoRecord(t *testing.T) {
	b := loadTestFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)
	refTable.(*reftable.Static[map[string]any]).Set("0999999", map[string]any{})

	feed(t, b, event(999999, 0, catTotal, 10, 50, "Carpark"))

	facility, _ := rec.Get("0999999")
	require.Equal(t, discovery.StatusWithheld, facility.Status)
	require.Equal(t, "no-record", facility.Reason)
}

// A record published later releases the station without waiting for an event.
func TestDiscoveryRecordReleasesAWithheldStation(t *testing.T) {
	b := loadTestFixtures(t)
	setGate(t, true)
	rec := recordDiscovery(t)
	feed(t, b, event(999999, 0, catTotal, 10, 50, "Carpark"))
	require.Empty(t, stationIDs(t, b))
	flushed := rec.Flushes

	static := refTable.(*reftable.Static[map[string]any])
	static.Set("0999999", map[string]any{"gps": map[string]any{"lat": 46.0, "lon": 11.0}})
	static.Set("0999999_0", map[string]any{"gps": map[string]any{"lat": 46.1, "lon": 11.1}})
	resyncAll(t.Context(), b)

	require.ElementsMatch(t, []string{clibID("0999999"), clibID("0999999_0")}, stationIDs(t, b))
	for _, key := range []string{"0999999", "0999999_0"} {
		e, _ := rec.Get(key)
		require.Equal(t, discovery.StatusPublished, e.Status, key)
		require.Empty(t, e.Reason, key)
	}
	require.Equal(t, flushed+1, rec.Flushes, "a resync flushes once")
}

// Nothing the provider states is seedable, so Adopt offers nothing and the
// stations need records written by an operator.
func TestSeedReproducesStationWithoutRecord(t *testing.T) {
	events := []ParkingEvent{
		event(607440, 0, catTotal, 10, 199, "Parcheggio 0"),
		event(607440, 1, catShortStay, 10, 30, "Parcheggio 1"),
		event(404467, 0, catTotal, 10, 601, "Totale"),
		event(999999, 0, catTotal, 10, 50, "Unknown carpark"),
	}
	run := func(t *testing.T, b bdplib.Bdp) map[string]bdplib.Station {
		for _, ev := range events {
			feed(t, b, ev)
		}
		out := map[string]bdplib.Station{}
		for _, s := range syncedStations_(t, b) {
			out[s.Id] = s
		}
		return out
	}

	b := loadTestFixtures(t)
	rec := recordDiscovery(t)
	refTable = reftable.NewStatic(map[string]map[string]any{})
	bare := run(t, b)
	require.Len(t, bare, 7)

	seeds := map[string]map[string]any{}
	for key, e := range rec.All() {
		require.Nil(t, e.Seed, key)
		var asStored map[string]any
		raw, err := json.Marshal(e.Seed)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &asStored))
		seeds[key] = asStored
	}

	b = loadTestFixtures(t)
	refTable = reftable.NewStatic(seeds)
	withSeed := run(t, b)

	// Compared as published: the merge turns numbers into json.Number, which
	// serialises identically.
	require.Equal(t, len(bare), len(withSeed))
	for id, s := range bare {
		want, err := json.Marshal(s)
		require.NoError(t, err)
		got, err := json.Marshal(withSeed[id])
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(got), id)
	}
}

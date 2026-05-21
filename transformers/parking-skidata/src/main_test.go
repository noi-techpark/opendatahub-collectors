// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

// loadTestFixtures wires stations/categories the same way main() does,
// with RESOURCES_OVERLAY=test so the .test.csv overlay (carrying the 0600015
// demo rows) is merged on top of the production CSVs. It also creates an
// empty Cache so per-event aggregations have somewhere to go.
func loadTestFixtures(t *testing.T) {
	t.Helper()
	t.Setenv("RESOURCES_OVERLAY", "test")
	loadResources("../resources")
	cache = NewCache()
}

func TestTransform_Event1(t *testing.T) {
	loadTestFixtures(t)

	var in ParkingEvent
	err := testsuite.LoadInputData(&in, "testdata/in1.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[ParkingEvent]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	// Exercise the same startup flow as main(): data types, then all
	// stations, then the per-event Transform.
	require.Nil(t, syncDataTypes(b))
	require.Nil(t, syncAllStations(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	mock := b.(*bdpmock.BdpMock)
	req := mock.Requests()

	var out bdpmock.BdpMockCalls
	err = testsuite.LoadOutput(&out, "testdata/out1.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out1.json")
		if werr := testsuite.WriteOutput(req, "testdata/out1.json"); werr != nil {
			t.Fatalf("failed to write snapshot: %v", werr)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	bdpmock.CompareBdpMockCalls(t, out, req)
}

// TestSyncAllStations_Snapshot exercises only the startup-sync path
// (data types + every parent/child station with full metadata) against
// the BDP mock, and snapshots the resulting calls into testdata/out_sync.json.
// Useful to inspect the actual SyncDataTypes/SyncStations payloads sent
// to BDP, including aggregated facility-level capacities and per-category
// child metadata.
func TestSyncAllStations_Snapshot(t *testing.T) {
	loadTestFixtures(t)

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	require.Nil(t, syncDataTypes(b))
	require.Nil(t, syncAllStations(b))

	mock := b.(*bdpmock.BdpMock)
	req := mock.Requests()

	var out bdpmock.BdpMockCalls
	err := testsuite.LoadOutput(&out, "testdata/out_sync.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out_sync.json")
		if werr := testsuite.WriteOutput(req, "testdata/out_sync.json"); werr != nil {
			t.Fatalf("failed to write snapshot: %v", werr)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	bdpmock.CompareBdpMockCalls(t, out, req)
}

func TestStations(t *testing.T) {
	// The 0600015 demo rows live in the optional .test.csv overlay, not in
	// the production stations.csv. Merge the two like main() does.
	s := append(
		ReadStations("../resources/stations.csv"),
		ReadStationsOptional("../resources/stations.test.csv")...,
	)

	byID := map[string]*Station{}
	for i := range s {
		byID[s[i].ID] = &s[i]
	}
	require.Nil(t, byID["does-not-exist"])

	parent := byID["0600015"]
	require.NotNil(t, parent)
	require.Equal(t, "0600015", parent.ID)
	require.Equal(t, "ParkingFacility", parent.StationType)
	require.Equal(t, "Parcheggio Demo", parent.Name)
	require.InDelta(t, 46.49067, parent.Lat, 0.00001)

	meta := parent.ToMetadata()
	require.Equal(t, "Bolzano - Bozen", meta["municipality"])
	netex, ok := meta["netex_parking"].(map[string]any)
	require.True(t, ok, "netex_parking should be a nested map")
	require.Equal(t, "urbanParking", netex["type"])
	require.Equal(t, true, netex["charging"])
	require.Equal(t, "noReservations", netex["reservation"])

	child := byID["0600015_0"]
	require.NotNil(t, child)
	require.Equal(t, "0600015_0", child.ID)
	require.Equal(t, "ParkingStation", child.StationType)
	require.Equal(t, "0600015", child.ParentID)
	require.Equal(t, 0, child.CarparkID)
}

func TestCountingCategories(t *testing.T) {
	c := ReadCountingCategories("../resources/counting_categories.csv")

	// 0607242 carpark 0 has the canonical short_stay/subscribers/total trio.
	require.Len(t, c.ForFacility("0607242"), 3)
	require.Len(t, c.ForCarpark("0607242", 0), 3)

	row := c.Find("0607242", 0, 3)
	require.NotNil(t, row)
	require.Equal(t, "Totale", row.Name)
	require.Equal(t, 245, row.Capacity)

	require.Nil(t, c.Find("nonexistent", 0, 1))
}

// TestTransform_Cat2WithPrimedCache exercises the aggregation paths
// that only fire for non-cat-3 events: the per-category push lands on
// free_subscribers/occupied_subscribers, and the overall step copies
// the cached cat-3 value forward to free/occupied. The facility-level
// push includes both the overall AND the per-category sums.
func TestTransform_Cat2WithPrimedCache(t *testing.T) {
	loadTestFixtures(t)
	// Pre-existing cat 3 (Totale) for facility 0600015 carpark 0,
	// and another carpark to exercise facility-level summing.
	cache.Set("0600015_0", "free", 120, 1)
	cache.Set("0600015_0", "occupied", 5, 1)
	cache.Set("0600015_0", "free_short_stay", 80, 1)
	cache.Set("0600015_0", "occupied_short_stay", 4, 1)
	cache.Set("0600015_1", "free", 50, 1)
	cache.Set("0600015_1", "occupied", 10, 1)

	// Synthesize a cat-2 (Abbonati / subscribers) event for carpark 0.
	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)
	raw := rdb.Raw[ParkingEvent]{
		Rawdata: ParkingEvent{
			Capacity:           141,
			Level:              91,
			CountingCategoryId: 2,
			Name:               "Abbonati",
			Carpark:            Carpark{FacilityNr: 600015, Id: 0},
		},
		Timestamp: timestamp,
	}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	mock := b.(*bdpmock.BdpMock)
	req := mock.Requests()

	var out bdpmock.BdpMockCalls
	err = testsuite.LoadOutput(&out, "testdata/out_cat2.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out_cat2.json")
		if werr := testsuite.WriteOutput(req, "testdata/out_cat2.json"); werr != nil {
			t.Fatalf("failed to write snapshot: %v", werr)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

// TestTransform_MultiCategorySequence drives a facility with 3 carparks
// and 6 counting categories (id 1..6) through three events in a row,
// each touching a different category. The cache is pre-hydrated as if
// the transformer had already absorbed earlier events. The snapshot
// proves that:
//
//   - cat 2 (Abbonati) → carpark gets free_subscribers + carpark overall
//     from cat-3 cache; facility gets free + free_subscribers summed.
//   - cat 3 (Totale)   → carpark gets free/occupied only (no duplicate
//     overall push, because cat 3's per-category names ARE free/occupied);
//     facility gets free + occupied summed across carparks (no per-cat
//     facility push because suffix == "").
//   - cat 4 (Meusburger, unknown id) → suffix derived by slugify; carpark
//     gets free_meusburger + carpark overall from cat-3 cache; facility
//     gets free + free_meusburger summed.
//
// The fixture facility 0601336 actually exists in resources/counting_categories.csv
// with 3 carparks and 6 categories.
func TestTransform_MultiCategorySequence(t *testing.T) {
	loadTestFixtures(t)

	// Pre-hydrate the cache as if we had already seen recent measurements.
	// Carpark 0: full coverage of cats 1/2/3/4.
	cache.Set("0601336_0", "free_short_stay", 100, 1)
	cache.Set("0601336_0", "occupied_short_stay", 91, 1)
	cache.Set("0601336_0", "free_subscribers", 80, 1)
	cache.Set("0601336_0", "occupied_subscribers", 70, 1)
	cache.Set("0601336_0", "free", 200, 1)
	cache.Set("0601336_0", "occupied", 161, 1)
	cache.Set("0601336_0", "free_meusburger", 15, 1)
	cache.Set("0601336_0", "occupied_meusburger", 6, 1)
	// Carpark 1: only cat 3 cached.
	cache.Set("0601336_1", "free", 250, 1)
	cache.Set("0601336_1", "occupied", 122, 1)
	// Carpark 2: only cat 3 cached.
	cache.Set("0601336_2", "free", 50, 1)
	cache.Set("0601336_2", "occupied", 35, 1)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	// Events come from a fixture file: cat 2 / cat 3 / cat 4 in order,
	// drawn from facility 0601336. Each event runs through Transform
	// against the same mock and contributes to the snapshot.
	var events []ParkingEvent
	require.Nil(t, testsuite.LoadInputData(&events, "testdata/in_multi.json"))

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, syncDataTypes(b))

	for i, e := range events {
		raw := &rdb.Raw[ParkingEvent]{Rawdata: e, Timestamp: timestamp}
		require.Nil(t, Transform(context.TODO(), b, raw), "event %d failed", i)
	}

	mock := b.(*bdpmock.BdpMock)
	req := mock.Requests()

	var out bdpmock.BdpMockCalls
	err = testsuite.LoadOutput(&out, "testdata/out_multi.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out_multi.json")
		if werr := testsuite.WriteOutput(req, "testdata/out_multi.json"); werr != nil {
			t.Fatalf("failed to write snapshot: %v", werr)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

func TestCarparkOverall_Cat3Wins(t *testing.T) {
	c := NewCache()
	c.Set("0600015_0", "free_short_stay", 80, 1)
	c.Set("0600015_0", "free_subscribers", 40, 1)
	c.Set("0600015_0", "free", 120, 1) // cat 3 — should be returned

	v, ok := c.CarparkOverall("0600015_0", "free")
	require.True(t, ok)
	require.Equal(t, 120, v)
}

func TestCarparkOverall_FallbackSum(t *testing.T) {
	c := NewCache()
	c.Set("0600015_0", "free_short_stay", 80, 1)
	c.Set("0600015_0", "free_subscribers", 40, 1)
	// no cat 3 cached -> overall = sum of the others

	v, ok := c.CarparkOverall("0600015_0", "free")
	require.True(t, ok)
	require.Equal(t, 120, v)
}

func TestFacilityAggregations(t *testing.T) {
	c := NewCache()
	// facility 0600015 has two carparks, _0 and _1
	c.Set("0600015_0", "free", 120, 1)
	c.Set("0600015_0", "free_short_stay", 80, 1)
	c.Set("0600015_0", "occupied", 5, 1)
	c.Set("0600015_1", "free", 50, 1)
	c.Set("0600015_1", "free_short_stay", 30, 1)
	c.Set("0600015_1", "occupied", 10, 1)
	// foreign facility — must NOT be summed in
	c.Set("0700000_0", "free", 999, 1)

	require.Equal(t, 170, c.FacilityOverall("0600015", "free"))
	require.Equal(t, 15, c.FacilityOverall("0600015", "occupied"))
	require.Equal(t, 110, c.FacilityPerCategory("0600015", "free_short_stay"))
}

func TestAllDataTypeNames_Sorted(t *testing.T) {
	c := CountingCategories{
		{CountingCategoryId: 1, Name: "SostaBreve"},
		{CountingCategoryId: 4, Name: "Autobus"},
	}
	names := allDataTypeNames(c)
	// Always at least the standard trio (free/occupied + short_stay + subscribers)
	require.Contains(t, names, "free")
	require.Contains(t, names, "occupied")
	require.Contains(t, names, "free_short_stay")
	require.Contains(t, names, "free_subscribers")
	require.Contains(t, names, "free_autobus")
	require.Contains(t, names, "occupied_autobus")
	// Sorted
	prev := ""
	for _, n := range names {
		require.True(t, n > prev, "names not sorted: %q after %q", n, prev)
		prev = n
	}
}

func TestDescriptorFor(t *testing.T) {
	tests := []struct {
		id           int
		name         string
		wantSuffix   string
		wantFreeType string
		wantMetaCap  string
	}{
		{1, "SostaBreve", "short_stay", "free_short_stay", "capacity_short_stay"},
		{2, "Abbonati", "subscribers", "free_subscribers", "capacity_subscribers"},
		{3, "Totale", "", "free", "capacity"},
		{4, "Autobus", "autobus", "free_autobus", "capacity_autobus"},
		{99, "Nobis Abo", "nobis_abo", "free_nobis_abo", "capacity_nobis_abo"},
	}
	for _, tc := range tests {
		d := descriptorFor(tc.id, tc.name)
		require.Equal(t, tc.wantSuffix, d.suffix, "id=%d", tc.id)
		require.Equal(t, tc.wantFreeType, d.freeType(), "id=%d", tc.id)
		require.Equal(t, tc.wantMetaCap, d.metaKey("capacity"), "id=%d", tc.id)
	}
}

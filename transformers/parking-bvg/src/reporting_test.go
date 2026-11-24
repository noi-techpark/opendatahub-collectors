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
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

const (
	facility    = "Bruneck-Mobilitaetszentrum"
	knownA      = "733b3ee3-ff74-4ec9-b799-cfbd1d909c58"
	unknownArea = "00000000-0000-0000-0000-000000000000"
)

func recordDiscovery(t *testing.T) *discovery.Recorder {
	t.Helper()
	rec := discovery.NewRecorder()
	observer = rec
	t.Cleanup(func() { observer = noopObserver{} })
	return rec
}

func transform(t *testing.T, in CountingAreaList) (map[string]bdplib.Station, error) {
	t.Helper()
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	err := Transform(context.TODO(), b, &rdb.Raw[CountingAreaList]{Rawdata: in, Timestamp: time.Now()})
	out := map[string]bdplib.Station{}
	for _, calls := range b.(*bdpmock.BdpMock).Requests().SyncedStations {
		for _, c := range calls {
			for _, s := range c.Stations {
				out[s.Id] = s
			}
		}
	}
	return out, err
}

func fixtureAreas(t *testing.T) CountingAreaList {
	t.Helper()
	var in CountingAreaList
	require.NoError(t, testsuite.LoadInputData(&in, "testdata/in.json"))
	return in
}

func TestDiscoveryFullPayloadIsPublished(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	published, err := transform(t, fixtureAreas(t))
	require.NoError(t, err)

	all := rec.All()
	require.Len(t, all, 19, "18 areas and their facility")
	require.Len(t, published, 19)
	require.Equal(t, 1, rec.Flushes)
	require.Equal(t, 1, rec.Completes)

	for key, e := range all {
		require.Equal(t, discovery.StatusPublished, e.Status, key)
		require.Empty(t, e.Reason, key)
		require.Nil(t, e.Lat, key)
		require.Nil(t, e.Lon, key)
	}

	f := all[facility]
	require.Empty(t, f.Parent)
	require.Equal(t, facility, f.Name)
	require.Equal(t, map[string]any{
		"capacity":          215,
		"capacity_standard": 194,
		"capacity_disabled": 19,
		"capacity_truck":    2,
	}, f.Data)
}

func TestDiscoveryAreaFacts(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	_, err := transform(t, fixtureAreas(t))
	require.NoError(t, err)

	e, ok := rec.Get(knownA)
	require.True(t, ok)
	require.Equal(t, facility, e.Parent)
	require.Equal(t, "Bruneck-Mobilitaetszentrum_A001_010_STD", e.Name, "the provider's area name, not the record's")
	require.Equal(t, map[string]any{"capacity": 10, "type": "Standard"}, e.Data)
}

func TestDiscoveryUnknownAreaWithholdsItsWholeFacility(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	published, err := transform(t, CountingAreaList{
		countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", knownA, 3),
		countingArea("Bruneck-Mobilitaetszentrum_A019_099_STD", unknownArea, 1),
		// another facility, fully described: must keep flowing
		countingArea("Bruneck-Mobilitaetszentrum_A002_009_STD", "8079ecfc-104e-4d70-9f55-01ca83262e45", 1),
	})
	require.ErrorContains(t, err, "nothing to publish")
	require.Empty(t, published)

	require.Equal(t, 1, rec.Flushes, "the report is flushed even when nothing is published")
	require.Equal(t, 1, rec.Completes)

	missing, _ := rec.Get(unknownArea)
	require.Equal(t, discovery.StatusWithheld, missing.Status)
	require.Equal(t, "no-record", missing.Reason)
	require.Equal(t, facility, missing.Parent)

	sibling, _ := rec.Get(knownA)
	require.Equal(t, discovery.StatusWithheld, sibling.Status)
	require.Equal(t, "parent-incomplete", sibling.Reason)

	fac, _ := rec.Get(facility)
	require.Equal(t, discovery.StatusWithheld, fac.Status)
	require.Equal(t, "parent-incomplete", fac.Reason, "the facility has a record; its area does not")
}

func TestDiscoveryFacilityWithoutRecord(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	published, err := transform(t, CountingAreaList{
		countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", knownA, 3),
		countingArea("Nowhere-Garage_A001_004_STD", unknownArea, 1),
	})
	require.NoError(t, err)
	require.Contains(t, published, facility)
	require.NotContains(t, published, "Nowhere-Garage")

	fac, _ := rec.Get("Nowhere-Garage")
	require.Equal(t, discovery.StatusWithheld, fac.Status)
	require.Equal(t, "no-record", fac.Reason)

	area, _ := rec.Get(unknownArea)
	require.Equal(t, discovery.StatusWithheld, area.Status)
	require.Equal(t, "no-record", area.Reason)
	require.Equal(t, "Nowhere-Garage", area.Parent)

	ok, _ := rec.Get(knownA)
	require.Equal(t, discovery.StatusPublished, ok.Status)
}

func TestDiscoveryGateOffPublishesUnenriched(t *testing.T) {
	loadFixtures(t)
	setGate(t, false)
	rec := recordDiscovery(t)

	published, err := transform(t, CountingAreaList{
		countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", knownA, 3),
		countingArea("Bruneck-Mobilitaetszentrum_A019_099_STD", unknownArea, 1),
		countingArea("Nowhere-Garage_A001_004_STD", "11111111-1111-1111-1111-111111111111", 1),
	})
	require.NoError(t, err)

	for _, id := range []string{facility, "Nowhere-Garage", knownA, unknownArea, "11111111-1111-1111-1111-111111111111"} {
		e, _ := rec.Get(id)
		require.Equal(t, discovery.StatusPublished, e.Status, id)
	}
	for _, id := range []string{unknownArea, "Nowhere-Garage", "11111111-1111-1111-1111-111111111111"} {
		e, _ := rec.Get(id)
		require.Equal(t, "no-record", e.Reason, id)
	}
	k, _ := rec.Get(knownA)
	require.Empty(t, k.Reason)

	bare := published["Bruneck-Mobilitaetszentrum_A019_099_STD"]
	require.Equal(t, "Bruneck-Mobilitaetszentrum_A019_099_STD", bare.Name)
	require.Zero(t, bare.Latitude)
	require.Equal(t, facility, bare.ParentStation)
	require.Contains(t, published, "Nowhere-Garage")

	// The facility aggregates both of its areas, enriched or not.
	require.EqualValues(t, "109", fmt.Sprint(published[facility].MetaData["capacity"]), "facility capacity covers the unknown area too")
}

func TestDiscoveryAreaWithoutCountsIsReported(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	silent := countingArea("Bruneck-Mobilitaetszentrum_A002_009_STD", "8079ecfc-104e-4d70-9f55-01ca83262e45", 0)
	silent.Counts = Counts{}
	published, err := transform(t, CountingAreaList{
		countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", knownA, 3),
		silent,
	})
	require.NoError(t, err)
	require.NotContains(t, published, silent.Name)

	e, ok := rec.Get(silent.ID)
	require.True(t, ok)
	require.Equal(t, discovery.StatusWithheld, e.Status)
	require.Equal(t, "no-data", e.Reason)
	require.Equal(t, facility, e.Parent)
}

func TestDiscoveryLineCrossingIsNotAStation(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	line := countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", knownA, 3)
	line.Type = "line_crossing"
	_, err := transform(t, CountingAreaList{line, countingArea("Bruneck-Mobilitaetszentrum_A002_009_STD", "8079ecfc-104e-4d70-9f55-01ca83262e45", 1)})
	require.NoError(t, err)
	_, ok := rec.Get(knownA)
	require.False(t, ok)
}

func TestDiscoveryEmptyPayloadIsNotASnapshot(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	_, err := transform(t, CountingAreaList{})
	require.Error(t, err)
	require.Equal(t, 0, rec.Completes)
	require.Equal(t, 1, rec.Flushes)
}

// Nothing the provider states is seedable, so Adopt offers nothing and the
// stations need records written by an operator.
func TestSeedReproducesStationWithoutRecord(t *testing.T) {
	loadFixtures(t)
	setGate(t, false)
	rec := recordDiscovery(t)

	refTable = reftable.NewStatic(map[string]map[string]any{})
	bare, err := transform(t, fixtureAreas(t))
	require.NoError(t, err)
	require.Len(t, bare, 19)

	seeded := reftable.NewStatic(map[string]map[string]any{})
	for key, e := range rec.All() {
		require.Nil(t, e.Seed, key)
		var asStored map[string]any
		raw, err := json.Marshal(e.Seed)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &asStored))
		seeded.Set(key, asStored)
	}
	refTable = seeded
	withSeed, err := transform(t, fixtureAreas(t))
	require.NoError(t, err)

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

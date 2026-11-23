// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
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

func recordDiscovery(t *testing.T) *discovery.Recorder {
	t.Helper()
	rec := discovery.NewRecorder()
	observer = rec
	t.Cleanup(func() { observer = noopObserver{} })
	return rec
}

func transform(t *testing.T, in FacilityData) map[string]bdplib.Station {
	t.Helper()
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.NoError(t, Transform(context.TODO(), b, &rdb.Raw[FacilityData]{Rawdata: in, Timestamp: time.Now()}))
	out := map[string]bdplib.Station{}
	for _, calls := range b.(*bdpmock.BdpMock).Requests().SyncedStations {
		for _, c := range calls {
			for _, s := range c.Stations {
				out[s.Id] = s
			}
		}
	}
	return out
}

func fixture(t *testing.T, name string) FacilityData {
	t.Helper()
	var in FacilityData
	require.NoError(t, testsuite.LoadInputData(&in, "../testdata/input/"+name+".json"))
	return in
}

// brokenPayload mixes a facility the table describes fully (609883), one with a
// carpark it does not know (608612_9) and one it has never heard of (999999).
func brokenPayload() FacilityData {
	return FacilityData{
		{FacilityID: 609883, FacilityDetails: []FreePlace{{ParkNo: 0, CountingCategoryNo: 3, Capacity: 10}}},
		{FacilityID: 608612, FacilityDetails: []FreePlace{
			{ParkNo: 0, CountingCategoryNo: 3, Capacity: 10},
			{ParkNo: 9, CountingCategoryNo: 3, Capacity: 7},
		}},
		{FacilityID: 999999, FacilityDetails: []FreePlace{{ParkNo: 0, CountingCategoryNo: 3, Capacity: 5}}},
	}
}

func TestDiscoveryDescribedPayloadIsPublished(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	published := transform(t, fixture(t, "skidata"))

	all := rec.All()
	require.Len(t, all, 6, "two facilities and four carparks")
	require.Len(t, published, 6)
	require.Equal(t, 1, rec.Flushes)
	require.Equal(t, 0, rec.Completes, "the payload is one merchant's facilities, never the provider's full set")

	for code, e := range all {
		require.Equal(t, discovery.StatusPublished, e.Status, code)
		require.Empty(t, e.Reason, code)
		require.Nil(t, e.Lat, code)
		require.Nil(t, e.Lon, code)
		require.Equal(t, code, e.Name, "the transformer names a station after its code")
	}

	carpark := all["607440_1"]
	require.Equal(t, "607440", carpark.Parent)
	require.Equal(t, map[string]any{"capacity": 90, "capacity_short_stay": 30, "capacity_subscribers": 60}, carpark.Data)

	facility := all["607440"]
	require.Empty(t, facility.Parent)
	require.Equal(t, 481, facility.Data["capacity"])
	require.Equal(t, 404, facility.Data["capacity_short_stay"])
	require.Equal(t, 77, facility.Data["capacity_subscribers"])
}

func TestDiscoveryLegacyCodesAreTheKeys(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	transform(t, fixture(t, "mybestparking"))

	parent, ok := rec.Get("105_facility")
	require.True(t, ok)
	require.Equal(t, discovery.StatusPublished, parent.Status)
	child, ok := rec.Get("105")
	require.True(t, ok)
	require.Equal(t, "105_facility", child.Parent)
	require.Len(t, rec.All(), 2)
}

func TestDiscoveryGateOnWithholdsTheWholeFacility(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	published := transform(t, brokenPayload())

	require.Contains(t, published, "609883")
	require.Contains(t, published, "609883_0")
	require.Len(t, published, 2)

	missing, _ := rec.Get("608612_9")
	require.Equal(t, discovery.StatusWithheld, missing.Status)
	require.Equal(t, "no-record", missing.Reason)
	require.Equal(t, "608612", missing.Parent)
	require.Equal(t, map[string]any{"capacity": 7}, missing.Data)

	for _, code := range []string{"608612", "608612_0"} {
		e, _ := rec.Get(code)
		require.Equal(t, discovery.StatusWithheld, e.Status, code)
		require.Equal(t, "parent-incomplete", e.Reason, code)
	}
	sibling, _ := rec.Get("608612_0")
	require.Equal(t, "608612", sibling.Parent)

	for _, code := range []string{"999999", "999999_0"} {
		e, _ := rec.Get(code)
		require.Equal(t, discovery.StatusWithheld, e.Status, code)
		require.Equal(t, "no-record", e.Reason, code)
	}

	ok, _ := rec.Get("609883_0")
	require.Equal(t, discovery.StatusPublished, ok.Status)
	require.Equal(t, 1, rec.Flushes)
}

func TestDiscoveryGateOffPublishesUnenriched(t *testing.T) {
	loadFixtures(t)
	gated := transform(t, brokenPayload())

	setGate(t, false)
	rec := recordDiscovery(t)
	open := transform(t, brokenPayload())

	require.Len(t, open, 7)
	for code, s := range gated {
		require.Equal(t, s, open[code], "stations with a record are unaffected by the gate")
	}

	for _, code := range []string{"608612_9", "999999", "999999_0"} {
		e, _ := rec.Get(code)
		require.Equal(t, discovery.StatusPublished, e.Status, code)
		require.Equal(t, "no-record", e.Reason, code)
	}
	for _, code := range []string{"608612", "608612_0", "609883", "609883_0"} {
		e, _ := rec.Get(code)
		require.Equal(t, discovery.StatusPublished, e.Status, code)
		require.Empty(t, e.Reason, code)
	}

	bare := open["608612_9"]
	require.Equal(t, "608612_9", bare.Name)
	require.Equal(t, "608612", bare.ParentStation)
	require.Zero(t, bare.Latitude)
}

// Nothing the provider states is seedable, so Adopt offers nothing and the
// stations need records written by an operator.
func TestSeedReproducesStationWithoutRecord(t *testing.T) {
	for _, name := range []string{"skidata", "mybestparking"} {
		t.Run(name, func(t *testing.T) {
			loadFixtures(t)
			setGate(t, false)
			rec := recordDiscovery(t)

			refTable = reftable.NewStatic(map[string]map[string]any{})
			bare := transform(t, fixture(t, name))
			require.NotEmpty(t, bare)

			seeded := reftable.NewStatic(map[string]map[string]any{})
			for code, e := range rec.All() {
				require.Nil(t, e.Seed, code)
				var asStored map[string]any
				raw, err := json.Marshal(e.Seed)
				require.NoError(t, err)
				require.NoError(t, json.Unmarshal(raw, &asStored))
				seeded.Set(code, asStored)
			}
			refTable = seeded
			withSeed := transform(t, fixture(t, name))

			// Compared as published: the merge turns numbers into json.Number,
			// which serialises identically.
			require.Equal(t, len(bare), len(withSeed))
			for code, s := range bare {
				want, err := json.Marshal(s)
				require.NoError(t, err)
				got, err := json.Marshal(withSeed[code])
				require.NoError(t, err)
				require.JSONEq(t, string(want), string(got), code)
			}
		})
	}
}

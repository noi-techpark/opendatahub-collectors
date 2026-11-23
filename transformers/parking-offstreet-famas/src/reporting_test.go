// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/stretchr/testify/require"
)

func recordDiscovery(t *testing.T) *discovery.Recorder {
	t.Helper()
	rec := discovery.NewRecorder()
	observer = rec
	t.Cleanup(func() { observer = noopObserver{} })
	return rec
}

func runTransform(t *testing.T) []bdplib.Station {
	t.Helper()
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: "FAMAS"})
	require.NoError(t, Transform(context.TODO(), b, loadRaw(t, "testdata/raw.json")))
	var out []bdplib.Station
	for _, call := range b.(*bdpmock.BdpMock).Requests().SyncedStations[STATIONTYPE] {
		out = append(out, call.Stations...)
	}
	return out
}

func byID(stations []bdplib.Station) map[string]bdplib.Station {
	m := map[string]bdplib.Station{}
	for _, s := range stations {
		m[s.Id] = s
	}
	return m
}

func TestDiscoveryGateOn(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)

	published := byID(runTransform(t))
	all := rec.All()

	require.Len(t, all, 10)
	require.Equal(t, 1, rec.Flushes)
	require.Equal(t, 1, rec.Completes)

	withheld, ok := rec.Get("105")
	require.True(t, ok)
	require.Equal(t, discovery.StatusWithheld, withheld.Status)
	require.Equal(t, "no-record", withheld.Reason)
	require.Equal(t, "P5 - Laurin", withheld.Name, "the provider's name, not the record's")
	require.Equal(t, map[string]any{"capacity": 90}, withheld.Data)
	require.Nil(t, withheld.Lat)
	require.Nil(t, withheld.Lon)
	require.Empty(t, withheld.Parent)
	require.NotContains(t, published, "105")

	for code, e := range all {
		if code == "105" {
			continue
		}
		require.Equal(t, discovery.StatusPublished, e.Status, code)
		require.Empty(t, e.Reason, code)
		require.Contains(t, published, code)
	}
}

// The provider-derived name must not be overwritten by the record's: the
// operator needs to see what the provider sends.
func TestDiscoveryNameIsProviderDerived(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	published := byID(runTransform(t))

	e, _ := rec.Get("103")
	require.Equal(t, "P3 - Piazza Walther", e.Name)
	require.NotEqual(t, e.Name, published["103"].Name, "the fixture's record renames the station")
}

func TestDiscoveryGateOffPublishesUnenriched(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	gated := byID(runTransform(t))

	setGate(t, false)
	open := byID(runTransform(t))

	e, _ := rec.Get("105")
	require.Equal(t, discovery.StatusPublished, e.Status)
	require.Equal(t, "no-record", e.Reason)

	require.Len(t, open, len(gated)+1)
	for code, s := range gated {
		require.Equal(t, s, open[code], "stations with a record are unaffected by the gate")
	}
	unenriched := open["105"]
	require.Equal(t, "P5 - Laurin", unenriched.Name)
	require.Zero(t, unenriched.Latitude)
	require.Equal(t, map[string]any{"capacity": 90}, unenriched.MetaData)
}

// A station that fails at the provider is still in the provider's list, so it is
// reported rather than silently dropped from a snapshot that is declared complete.
func TestDiscoveryReportsInvalidMetadata(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	raw := loadRaw(t, "testdata/raw.json")
	raw.Rawdata = append(raw.Rawdata, RawRec{Id: 999})

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: "FAMAS"})
	require.NoError(t, Transform(context.TODO(), b, raw))

	e, ok := rec.Get("999")
	require.True(t, ok)
	require.Equal(t, discovery.StatusWithheld, e.Status)
	require.Equal(t, "invalid-metadata", e.Reason)
}

func TestDiscoveryEmptyPayloadIsNotASnapshot(t *testing.T) {
	loadFixtures(t)
	rec := recordDiscovery(t)
	raw := loadRaw(t, "testdata/raw.json")
	raw.Rawdata = nil

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: "FAMAS"})
	require.NoError(t, Transform(context.TODO(), b, raw))
	require.Equal(t, 0, rec.Completes)
	require.Equal(t, 1, rec.Flushes)
}

// Adopting the seed of a published entry must change nothing downstream.
func TestSeedReproducesStationWithoutRecord(t *testing.T) {
	loadFixtures(t)
	setGate(t, false)
	rec := recordDiscovery(t)

	refTable = reftable.NewStatic(map[string]map[string]any{})
	bare := byID(runTransform(t))
	require.Len(t, bare, 10)

	seeded := reftable.NewStatic(map[string]map[string]any{})
	for code, e := range rec.All() {
		require.NotNil(t, e.Seed, code)
		// The seed travels through the discovery table as JSON.
		var asStored map[string]any
		raw, err := json.Marshal(e.Seed)
		require.NoError(t, err)
		require.NoError(t, json.Unmarshal(raw, &asStored))
		seeded.Set(code, asStored)
	}
	refTable = seeded
	withSeed := byID(runTransform(t))

	// Compared as published: the merge turns numbers into json.Number, which
	// serialises identically.
	require.Equal(t, len(bare), len(withSeed))
	for code, s := range bare {
		want, err := json.Marshal(s)
		require.NoError(t, err)
		got, err := json.Marshal(withSeed[code])
		require.NoError(t, err)
		require.JSONEq(t, string(want), string(got), code)
	}
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

func loadFixtures(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("testdata/enrichment.json")
	require.NoError(t, err)
	refTable, err = reftable.StaticFromJSON[map[string]any](raw)
	require.NoError(t, err)
	t.Cleanup(func() { refTable = nil })
}

func countingArea(name, providerId string, vehicles int) CountingArea {
	return CountingArea{
		ID:     providerId,
		Name:   name,
		Type:   "counting",
		Counts: Counts{Totals: []Total{{CountVehicle: vehicles}}},
	}
}

func syncedCodes(t *testing.T, calls []bdpmock.BdpMockStationCall) []string {
	t.Helper()
	var codes []string
	for _, call := range calls {
		for _, s := range call.Stations {
			codes = append(codes, s.Id)
		}
	}
	return codes
}

// One payload carries every facility of the provider account, so an area the csv
// does not describe must cost exactly its own facility and leave the rest flowing.
func TestUnknownStationSkipsOnlyItsOwnFacility(t *testing.T) {
	loadFixtures(t)

	in := CountingAreaList{
		// fully described by the csv
		countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", "733b3ee3-ff74-4ec9-b799-cfbd1d909c58", 3),
		countingArea("Bruneck-Mobilitaetszentrum_A002_009_STD", "8079ecfc-104e-4d70-9f55-01ca83262e45", 1),
		// Salurn is back at the provider, with areas the csv has never heard of
		countingArea("Salurn-Bahnhof_A001_004_STD", "00000000-0000-0000-0000-000000000000", 1),
	}

	raw := rdb.Raw[CountingAreaList]{Rawdata: in, Timestamp: time.Now()}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	require.NoError(t, Transform(context.TODO(), b, &raw), "one unknown area may not fail the payload")

	req := b.(*bdpmock.BdpMock).Requests()
	facilities := syncedCodes(t, req.SyncedStations[StationTypeParkingFacility])
	stations := syncedCodes(t, req.SyncedStations[StationTypeParkingStation])

	require.Equal(t, []string{"Bruneck-Mobilitaetszentrum"}, facilities)
	require.Equal(t, []string{
		"Bruneck-Mobilitaetszentrum_A001_010_STD",
		"Bruneck-Mobilitaetszentrum_A002_009_STD",
	}, stations)
}

// A facility is aggregated over its zones, so it is published whole or not at all:
// letting the known siblings through would report free/occupied totals that silently
// cover only part of the car park.
func TestPartlyUnknownFacilityIsSkippedWhole(t *testing.T) {
	loadFixtures(t)

	in := CountingAreaList{
		countingArea("Bruneck-Mobilitaetszentrum_A001_010_STD", "733b3ee3-ff74-4ec9-b799-cfbd1d909c58", 3),
		countingArea("Bruneck-Mobilitaetszentrum_A019_099_STD", "00000000-0000-0000-0000-000000000000", 1),
	}

	raw := rdb.Raw[CountingAreaList]{Rawdata: in, Timestamp: time.Now()}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	require.ErrorContains(t, Transform(context.TODO(), b, &raw), "nothing to publish")

	req := b.(*bdpmock.BdpMock).Requests()
	require.Empty(t, req.SyncedStations, "the known sibling of an unknown area may not leak through")
	require.Empty(t, req.SyncedData)
}

// Publishing nothing is correct when nothing is understood; deactivating every
// station of the origin is not.
func TestNothingKnownLeavesExistingStationsAlone(t *testing.T) {
	loadFixtures(t)

	in := CountingAreaList{
		countingArea("Marlengo_A001_010_STD", "00000000-0000-0000-0000-000000000000", 1),
	}

	raw := rdb.Raw[CountingAreaList]{Rawdata: in, Timestamp: time.Now()}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	err := Transform(context.TODO(), b, &raw)

	require.ErrorContains(t, err, "nothing to publish")
	req := b.(*bdpmock.BdpMock).Requests()
	require.Empty(t, req.SyncedStations, "an unusable payload may not deactivate anything")
	require.Empty(t, req.SyncedData)
}

func Test1(t *testing.T) {
	var in = CountingAreaList{}
	err := testsuite.LoadInputData(&in, "testdata/in.json")
	loadFixtures(t)
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[CountingAreaList]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = testsuite.LoadOutput(&out, "testdata/out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()
	// testsuite.WriteOutput(req, "testdata/out.json")
	bdpmock.CompareBdpMockCalls(t, out, req)
}

// The provider renames an area when its capacity changes ("_A010_006_STD" became
// "_A010_016_STD") but keeps its id, so the record must still be found.
func TestRenamedAreaKeepsItsRecord(t *testing.T) {
	loadFixtures(t)

	in := CountingAreaList{
		countingArea("Bruneck-Mobilitaetszentrum_A010_016_STD", "a10447fb-c4d7-4e05-a90a-2583bfbec88a", 3),
	}
	raw := rdb.Raw[CountingAreaList]{Rawdata: in, Timestamp: time.Now()}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	require.NoError(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()
	stations := req.SyncedStations[StationTypeParkingStation][0].Stations
	require.Len(t, stations, 1)
	require.Equal(t, "Bruneck-Mobilitaetszentrum_A010_016_STD", stations[0].Id)
	require.Equal(t, "Parkplatz Bruneck Mobilitaetszentrum (A010_006_STD)", stations[0].Name)
	require.NotZero(t, stations[0].Latitude)
}

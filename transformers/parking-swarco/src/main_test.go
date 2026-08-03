// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
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

func runSnapshot(t *testing.T, inFile, outFile string) {
	t.Helper()

	var in SwarcoData
	require.Nil(t, testsuite.LoadInputData(&in, inFile))

	timestamp, err := time.Parse("2006-01-02", "2026-01-01")
	require.Nil(t, err)
	raw := rdb.Raw[SwarcoData]{Rawdata: in, Timestamp: timestamp}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	// Exercise the same flow as main(): data types at startup, then Transform.
	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, outFile); err != nil {
		t.Logf("No snapshot found, generating %s", outFile)
		require.Nil(t, testsuite.WriteOutput(req, outFile))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

// Real payload captured from the Swarco SMI endpoint (single ParkingStation,
// no areas).
func TestTransformRealData(t *testing.T) {
	runSnapshot(t, "../testdata/in.json", "../testdata/out.json")
}

// A POI with areas becomes a ParkingFacility parent whose areas are child
// ParkingStations, each with its own occupancy from occupancyAreas.
func TestTransformFacilityWithAreas(t *testing.T) {
	runSnapshot(t, "../testdata/in-areas.json", "../testdata/out-areas.json")
}

// An empty static list would deactivate every station of the origin on sync;
// the transformer must refuse the payload instead.
func TestTransformRefusesEmptyStatic(t *testing.T) {
	raw := rdb.Raw[SwarcoData]{Rawdata: SwarcoData{}, Timestamp: time.Now()}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Error(t, Transform(context.TODO(), b, &raw))
	require.Empty(t, b.(*bdpmock.BdpMock).Requests().SyncedStations)
}

// Station codes must be deterministic (idempotent re-processing) and derived,
// never the raw provider id.
func TestStationCode(t *testing.T) {
	require.Equal(t, stationCode("100308"), stationCode("100308"))
	require.NotEqual(t, stationCode("100308"), stationCode("100309"))
	require.NotEqual(t, "100308", stationCode("100308"))
	require.Contains(t, stationCode("100308"), ID_PREFIX)
}

// A POI without usable coordinates must be skipped, not synced at 0/0 where it
// disappears from every bounding-box query.
func TestSkipsStationWithoutLocation(t *testing.T) {
	in := SwarcoData{
		Static: StaticPOIs{Total: 1, StaticPOIData: []StaticPOIData{
			{ObjectID: "1", Name: "no location"},
		}},
	}
	raw := rdb.Raw[SwarcoData]{Rawdata: in, Timestamp: time.Now()}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, Transform(context.TODO(), b, &raw))
	require.Empty(t, b.(*bdpmock.BdpMock).Requests().SyncedStations)
}

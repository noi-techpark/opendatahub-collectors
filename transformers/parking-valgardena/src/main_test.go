// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
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

// fixedTime pins the record timestamp so snapshots stay deterministic.
func fixedTime(t *testing.T) time.Time {
	ts, err := time.Parse(time.RFC3339, "2026-09-24T08:00:02Z")
	require.Nil(t, err)
	return ts
}

func TestTransformData(t *testing.T) {
	var in ParkingData
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in-data.json"))

	raw := rdb.Raw[ParkingData]{Rawdata: in, Timestamp: fixedTime(t)}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, syncDataTypes(b))
	require.Nil(t, TransformData(context.TODO(), b, &raw))

	assertSnapshot(t, b, "testdata/out-data.json")
}

func TestTransformMetadata(t *testing.T) {
	var in []ParkingMetadata
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in-metadata.json"))

	raw := rdb.Raw[[]ParkingMetadata]{Rawdata: in, Timestamp: fixedTime(t)}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, syncDataTypes(b))
	require.Nil(t, TransformMetadata(context.TODO(), b, &raw))

	assertSnapshot(t, b, "testdata/out-metadata.json")
}

// TestTransformDataIsReplaySafe pins the replay requirement: re-transforming the
// same raw event must write the same point, so the writer overwrites rather than
// appending a second measurement. This holds only while the record timestamp
// comes from the raw event and never from time.Now().
func TestTransformDataIsReplaySafe(t *testing.T) {
	var in ParkingData
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in-data.json"))
	raw := rdb.Raw[ParkingData]{Rawdata: in, Timestamp: fixedTime(t)}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, TransformData(context.TODO(), b, &raw))
	require.Nil(t, TransformData(context.TODO(), b, &raw))

	pushes := b.(*bdpmock.BdpMock).SyncedData[stationType]
	require.Len(t, pushes, 2)
	require.Equal(t, pushes[0], pushes[1])
}

// TestStationIdsAreStable pins the published station codes: they already exist
// in the timeseries, so a change here silently orphans the live stations.
func TestStationIdsAreStable(t *testing.T) {
	require.Equal(t, "GARDENA:cristauta", stationId("cristauta"))
	require.Equal(t, "GARDENA:alpe di siusi", stationId("alpe di siusi"))
}

func assertSnapshot(t *testing.T, b bdplib.Bdp, path string) {
	t.Helper()
	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, path); err != nil {
		t.Logf("No snapshot found, generating %s", path)
		require.Nil(t, testsuite.WriteOutput(req, path))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

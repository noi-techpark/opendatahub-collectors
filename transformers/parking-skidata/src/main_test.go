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

func TestTransform_Event1(t *testing.T) {
	stations = ReadStations("../resources/stations.csv")

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

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

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

func TestStations(t *testing.T) {
	s := ReadStations("../resources/stations.csv")

	require.Nil(t, s.GetStationByID("does-not-exist"))

	parent := s.GetStationByID("600015")
	require.NotNil(t, parent)
	require.Equal(t, "600015", parent.ID)
	require.Equal(t, "Parcheggio Demo", parent.Name)
	require.InDelta(t, 46.49067, parent.Lat, 0.00001)

	meta := parent.ToMetadata()
	require.Equal(t, "Bolzano - Bozen", meta["municipality"])
	netex, ok := meta["netex_parking"].(map[string]any)
	require.True(t, ok, "netex_parking should be a nested map")
	require.Equal(t, "urbanParking", netex["type"])
	require.Equal(t, true, netex["charging"])
	require.Equal(t, "noReservations", netex["reservation"])

	child := s.GetStationByID("600015_0")
	require.NotNil(t, child)
	require.Equal(t, "600015_0", child.ID)
}

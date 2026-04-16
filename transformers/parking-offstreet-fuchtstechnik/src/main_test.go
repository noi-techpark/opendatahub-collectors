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

func TestTransform(t *testing.T) {
	stations = ReadStations("../resources/stations.csv")

	var in ParkingEvent
	err := testsuite.LoadInputData(&in, "testdata/in.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2026-03-30")
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
	err = testsuite.LoadOutput(&out, "testdata/out.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out.json")
		if werr := testsuite.WriteOutput(req, "testdata/out.json"); werr != nil {
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

	station := s.GetStationByID("plan_de_gralba")
	require.NotNil(t, station)
	require.Equal(t, "plan_de_gralba", station.ID)
	require.Equal(t, "Parcheggio Plan de Gralba", station.Name)
	require.InDelta(t, 46.53474648514954, station.Lat, 0.00001)

	meta := station.ToMetadata()
	require.Equal(t, "Selva di Val Gardena", meta["municipality"])
	netex, ok := meta["netex_parking"].(map[string]any)
	require.True(t, ok, "netex_parking should be a nested map")
	require.Equal(t, "parkAndRide", netex["type"])
	require.Equal(t, false, netex["charging"])
	require.Equal(t, "noReservations", netex["reservation"])
}

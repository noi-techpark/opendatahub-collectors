// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

func TestTransform(t *testing.T) {
	raw := rdb.Raw[[]EcocounterSite]{}
	err := testsuite.LoadInputData(&raw.Rawdata, "testdata/in.json")
	require.Nil(t, err)

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

func TestGetUniqueDirections(t *testing.T) {
	measurements := []Measurement{
		{Direction: "in", TravelMode: "bike"},
		{Direction: "out", TravelMode: "bike"},
		{Direction: "in", TravelMode: "pedestrian"},
		{Direction: "out", TravelMode: "pedestrian"},
	}

	directions := getUniqueDirections(measurements)
	require.Len(t, directions, 2)
}

func TestMapTravelModeToDataType(t *testing.T) {
	require.Equal(t, DataTypeBike, mapTravelModeToDataType("bike"))
	require.Equal(t, DataTypePedestrian, mapTravelModeToDataType("pedestrian"))
	require.Equal(t, DataTypeCar, mapTravelModeToDataType("car"))
	require.Equal(t, "", mapTravelModeToDataType("unknown"))
}

func TestParseGranularityToSeconds(t *testing.T) {
	require.Equal(t, uint64(3600), parseGranularityToSeconds("PT1H"))
	require.Equal(t, uint64(900), parseGranularityToSeconds("PT15M"))
	require.Equal(t, uint64(1800), parseGranularityToSeconds("PT30M"))
	require.Equal(t, uint64(5400), parseGranularityToSeconds("PT1H30M"))
	require.Equal(t, uint64(60), parseGranularityToSeconds("PT1M"))
	require.Equal(t, uint64(3600), parseGranularityToSeconds("invalid")) // defaults to 1 hour
}

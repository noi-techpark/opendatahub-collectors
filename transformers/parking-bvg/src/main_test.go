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

func Test1(t *testing.T) {
	var in = CountingAreaList{}
	err := testsuite.LoadInputData(&in, "testdata/in.json")
	StationProto = ReadStations("./resources/stations.csv")
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

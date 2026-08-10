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

func TestTransform(t *testing.T) {
	var in RawData
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in.json"))

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)
	raw := rdb.Raw[RawData]{Rawdata: in, Timestamp: timestamp}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: "loewenbytes"})

	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, "testdata/out.json"); err != nil {
		t.Logf("No snapshot found, generating testdata/out.json")
		require.Nil(t, testsuite.WriteOutput(req, "testdata/out.json"))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

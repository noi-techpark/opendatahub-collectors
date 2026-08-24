// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

const goldenPath = "../testdata/output/out.json"

func Test(t *testing.T) {
	var in = Root{}
	err := testsuite.LoadInputData(&in, "../testdata/input/in.json")
	require.Nil(t, err)

	timestamp, err := time.Parse(time.RFC3339, "2025-04-02T13:00:03+02:00")
	require.Nil(t, err)

	raw := rdb.Raw[Root]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	req := b.(*bdpmock.BdpMock).Requests()

	// Delete the golden file to regenerate it from a run. It then agrees with
	// whatever the code does at that moment, regression included, so read the
	// diff before committing one.
	if _, err := os.Stat(goldenPath); errors.Is(err, os.ErrNotExist) {
		require.Nil(t, testsuite.WriteOutput(req, goldenPath))
		t.Logf("wrote %s from this run; review the diff before committing it", goldenPath)
		return
	}

	var out = bdpmock.BdpMockCalls{}
	require.Nil(t, testsuite.LoadOutput(&out, goldenPath))

	bdpmock.CompareBdpMockCalls(t, out, req)
}

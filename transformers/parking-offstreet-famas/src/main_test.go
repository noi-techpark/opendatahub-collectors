// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

func loadRaw(t *testing.T, path string) *rdb.Raw[RawRecs] {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	var raw rdb.Raw[RawRecs]
	require.NoError(t, json.Unmarshal(b, &raw))
	return &raw
}

// UPDATE_GOLDEN=1 rewrites the expected output from the current code.
func TestSnapshot(t *testing.T) {
	loadFixtures(t)
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: "FAMAS"})

	require.NoError(t, Transform(context.TODO(), b, loadRaw(t, "testdata/raw.json")))

	got := b.(*bdpmock.BdpMock).Requests()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		require.NoError(t, testsuite.WriteOutput(got, "testdata/out.json"))
	}
	var want bdpmock.BdpMockCalls
	require.NoError(t, testsuite.LoadOutput(&want, "testdata/out.json"))
	bdpmock.CompareBdpMockCalls(t, want, got)
}

// FAMAS re-exports stations it received from the Open Data Hub. Publishing them
// would duplicate stations another origin already owns, so only stations with a
// record are published.
func TestStationWithoutRecordIsNotPublished(t *testing.T) {
	loadFixtures(t)
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: "FAMAS"})

	require.NoError(t, Transform(context.TODO(), b, loadRaw(t, "testdata/raw.json")))

	for _, call := range b.(*bdpmock.BdpMock).Requests().SyncedStations[STATIONTYPE] {
		for _, s := range call.Stations {
			require.NotEqual(t, "105", s.Id)
		}
	}
}

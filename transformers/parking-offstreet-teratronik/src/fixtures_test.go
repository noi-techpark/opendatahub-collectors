// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/stretchr/testify/require"
)

const (
	origin     = "STA"
	stationKey = "parking-sta-marlengo"
)

// loadRaws reads the prod payloads exactly as the raw data bridge serves them:
// the provider's JSON body base64-encoded in rawdata.
func loadRaws(t *testing.T) []rdb.Raw[string] {
	t.Helper()
	b, err := os.ReadFile("testdata/prod-raw.json")
	require.NoError(t, err)
	var raws []rdb.Raw[string]
	require.NoError(t, json.Unmarshal(b, &raws))
	require.NotEmpty(t, raws)
	return raws
}

// rawOf wraps a provider JSON body the way rest-push stores it.
func rawOf(body string, ts time.Time) rdb.Raw[string] {
	return rdb.Raw[string]{
		Provider:  "sta/parking-marlengo",
		Timestamp: ts,
		Rawdata:   base64.StdEncoding.EncodeToString([]byte(body)),
	}
}

// loadFixtures installs testdata/enrichment.json, a record for the prod
// station only.
func loadFixtures(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("testdata/enrichment.json")
	require.NoError(t, err)
	refTable, err = reftable.StaticFromJSON[map[string]any](raw)
	require.NoError(t, err)
	t.Cleanup(func() { refTable = nil })
}

func setTable(t *testing.T, records map[string]map[string]any) {
	t.Helper()
	refTable = reftable.NewStatic(records)
	t.Cleanup(func() { refTable = nil })
}

func setGate(t *testing.T, on bool) {
	t.Helper()
	enrichEnv.REFTABLE_GATE = on
	t.Cleanup(func() { enrichEnv.REFTABLE_GATE = false })
}

func recordDiscovery(t *testing.T) *discovery.Recorder {
	t.Helper()
	rec := discovery.NewRecorder()
	discoverer = rec
	t.Cleanup(func() { discoverer = nil })
	return rec
}

// runRaw drives one raw document through the handler main wires up.
func runRaw(t *testing.T, raw rdb.Raw[string]) (*bdpmock.BdpMock, error) {
	t.Helper()
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: origin})
	require.NoError(t, syncDataTypes(b))
	err := tr.RawBase64JsonMiddleware(TransformWithBdp(b))(context.Background(), &raw)
	return b.(*bdpmock.BdpMock), err
}

func mustRun(t *testing.T, raw rdb.Raw[string]) *bdpmock.BdpMock {
	t.Helper()
	m, err := runRaw(t, raw)
	require.NoError(t, err)
	return m
}

func syncedStation(t *testing.T, m *bdpmock.BdpMock) (bdplib.Station, bool) {
	t.Helper()
	calls := m.Requests().SyncedStations[ParkingStation]
	if len(calls) == 0 {
		return bdplib.Station{}, false
	}
	require.Len(t, calls, 1)
	require.Len(t, calls[0].Stations, 1)
	return calls[0].Stations[0], true
}

func requestsJSON(t *testing.T, m *bdpmock.BdpMock) string {
	t.Helper()
	b, err := json.Marshal(m.Requests())
	require.NoError(t, err)
	return string(b)
}

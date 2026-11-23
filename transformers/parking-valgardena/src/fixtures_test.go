// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"testing"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/stretchr/testify/require"
)

// testdata/enrichment.json describes two of the metadata fixture's stations.
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

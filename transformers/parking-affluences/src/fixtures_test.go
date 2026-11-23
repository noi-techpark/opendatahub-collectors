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

const (
	siteP1 = "a5a6294b-36b5-4d57-851d-c66e90024fe5"
	siteP2 = "871cbd18-0dc5-4398-a986-930fc164aad2"
)

// testdata/enrichment.json holds a record for P1 only, so every test sees one
// station with and one without a record.
func loadFixtures(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("testdata/enrichment.json")
	require.NoError(t, err)
	refTable, err = reftable.StaticFromJSON[map[string]any](raw)
	require.NoError(t, err)
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

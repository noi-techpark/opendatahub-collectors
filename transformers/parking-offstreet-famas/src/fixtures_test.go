// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"os"
	"testing"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/stretchr/testify/require"
)

// testdata/enrichment.json holds the records the control tower import publishes
// for the retired stations.csv, so the golden output proves the import changes
// nothing.
func loadFixtures(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("testdata/enrichment.json")
	require.NoError(t, err)
	refTable, err = reftable.StaticFromJSON[map[string]any](raw)
	require.NoError(t, err)
	t.Cleanup(func() { refTable = nil })
}

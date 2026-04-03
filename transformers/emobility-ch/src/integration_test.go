// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// loadTestdata finds and decompresses the single .zst file in testdata/,
// unmarshalling the JSON payload into Root.
func loadTestdata(t *testing.T) Root {
	t.Helper()

	f, err := os.Open("testdata/20260403T075501Z_1c08017961a95263.zst")
	require.NoError(t, err, "open testdata file")
	defer f.Close()

	dec, err := zstd.NewReader(f)
	require.NoError(t, err, "create zstd reader")
	defer dec.Close()

	raw, err := io.ReadAll(dec)
	require.NoError(t, err, "decompress testdata")

	var root Root
	require.NoError(t, json.Unmarshal(raw, &root), "unmarshal Root from testdata")
	return root
}

func TestIntegration_Unmarshal(t *testing.T) {
	root := loadTestdata(t)

	require.NotEmpty(t, root.EVSEData, "expected at least one EVSE operator")
	require.NotEmpty(t, root.EVSEStatuses, "expected at least one EVSE status operator")

	t.Logf("loaded %d EVSE operators, %d status operators", len(root.EVSEData), len(root.EVSEStatuses))
}

func TestIntegration_EVSEDataStructure(t *testing.T) {
	root := loadTestdata(t)

	recordCount := 0
	for opIdx, operator := range root.EVSEData {
		if operator.OperatorID == "" {
			t.Errorf("operator %d: missing OperatorID", opIdx)
		}
		for i, evse := range operator.EVSEDataRecord {
			recordCount++
			if evse.EvseID == "" {
				t.Errorf("operator %d, record %d: missing EvseID", opIdx, i)
			}
			if evse.ChargingStationId == "" {
				t.Errorf("operator %d, record %d: missing ChargingStationId", opIdx, i)
			}
			if evse.GeoCoordinates == nil || evse.GeoCoordinates.Google == "" {
				t.Errorf("operator %d, record %d (%s): missing GeoCoordinates", opIdx, i, evse.EvseID)
				continue
			}
			if _, _, err := parseGoogleCoords(evse.GeoCoordinates); err != nil {
				t.Errorf("operator %d, record %d (%s): invalid coordinates: %v", opIdx, i, evse.EvseID, err)
			}
		}
	}
	t.Logf("validated %d EVSE records across %d operators", recordCount, len(root.EVSEData))
}

func TestIntegration_EVSEStatusStructure(t *testing.T) {
	root := loadTestdata(t)

	validStatuses := map[string]bool{
		"Available": true, "Occupied": true, "Reserved": true,
		"Unknown": true, "OutOfService": true,
	}

	statusCount := 0
	for opIdx, op := range root.EVSEStatuses {
		if op.OperatorID == "" {
			t.Errorf("status operator %d: missing OperatorID", opIdx)
		}
		for i, s := range op.EVSEStatusRecord {
			statusCount++
			if s.EvseID == "" {
				t.Errorf("status operator %d, record %d: missing EvseID", opIdx, i)
			}
			if !validStatuses[s.EvseStatus] {
				t.Errorf("status operator %d, record %d (%s): unexpected status value %q", opIdx, i, s.EvseID, s.EvseStatus)
			}
		}
	}
	t.Logf("validated %d status records across %d operators", statusCount, len(root.EVSEStatuses))
}

func TestIntegration_CoordinateRanges(t *testing.T) {
	root := loadTestdata(t)

	validCount := 0
	for _, operator := range root.EVSEData {
		for _, evse := range operator.EVSEDataRecord {
			if evse.GeoCoordinates == nil {
				continue
			}
			if _, _, err := parseGoogleCoords(evse.GeoCoordinates); err != nil {
				t.Errorf("EvseID %s: failed to parse coords: %v", evse.EvseID, err)
				continue
			}
			validCount++
		}
	}
	t.Logf("parsed %d coordinate records", validCount)
}

func TestIntegration_Transform(t *testing.T) {
	root := loadTestdata(t)

	raw := &rdb.Raw[Root]{
		Rawdata:   root,
		Timestamp: time.Date(2026, 4, 3, 7, 55, 1, 0, time.UTC),
	}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	err := Transform(context.Background(), b, raw)
	require.NoError(t, err, "Transform must not error on real testdata")

	calls := b.(*bdpmock.BdpMock).Requests()

	assert.Contains(t, calls.SyncedStations, StationType, "expected station sync")
	assert.Contains(t, calls.SyncedData, StationType, "expected status data push")

	var totalStations int
	for _, batch := range calls.SyncedStations[StationType] {
		totalStations += len(batch.Stations)
	}
	t.Logf("synced %d stations", totalStations)
	assert.Greater(t, totalStations, 0, "expected at least one station to be synced")
}


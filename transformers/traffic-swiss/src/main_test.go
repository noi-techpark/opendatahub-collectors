// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTransformWithEncoding decodes payload then calls Transform with a BdpMock.
func runTransformWithEncoding(t *testing.T, payload string) *bdpmock.BdpMock {
	t.Helper()
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: Origin})
	root, err := DecodePayload[Root](payload)
	require.NoError(t, err)
	raw := &rdb.Raw[Root]{Rawdata: *root, Timestamp: time.Now()}
	require.NoError(t, Transform(context.Background(), b, raw))
	return b.(*bdpmock.BdpMock)
}

func TestTransform_PlainJSON(t *testing.T) {
	mock := runTransformWithEncoding(t, encodePlainJSON(t, sampleRoot()))
	assert.NotEmpty(t, mock.SyncedStations[StationType])
}

func TestTransform_Base64JSON(t *testing.T) {
	mock := runTransformWithEncoding(t, encodeBase64JSON(t, sampleRoot()))
	assert.NotEmpty(t, mock.SyncedStations[StationType])
}

func TestTransform_GzipBase64JSON(t *testing.T) {
	mock := runTransformWithEncoding(t, encodeGzipBase64JSON(t, sampleRoot()))
	assert.NotEmpty(t, mock.SyncedStations[StationType])
}

func TestTransform_AllEncodingsProduceSameOutput(t *testing.T) {
	root := sampleRoot()
	m1 := runTransformWithEncoding(t, encodePlainJSON(t, root))
	m2 := runTransformWithEncoding(t, encodeBase64JSON(t, root))
	m3 := runTransformWithEncoding(t, encodeGzipBase64JSON(t, root))

	assert.Equal(t, len(m1.SyncedStations[StationType]), len(m2.SyncedStations[StationType]))
	assert.Equal(t, len(m1.SyncedStations[StationType]), len(m3.SyncedStations[StationType]))

	dm1, _ := json.Marshal(m1.SyncedData[StationType])
	dm2, _ := json.Marshal(m2.SyncedData[StationType])
	dm3, _ := json.Marshal(m3.SyncedData[StationType])
	assert.Equal(t, string(dm1), string(dm2))
	assert.Equal(t, string(dm1), string(dm3))
}

func TestTransform_StationFields(t *testing.T) {
	mock := runTransformWithEncoding(t, encodePlainJSON(t, sampleRoot()))

	calls := mock.SyncedStations[StationType]
	require.NotEmpty(t, calls)
	// Find station CH:0002.01
	var station *bdplib.Station
	for _, s := range calls[0].Stations {
		if s.Id == "CH:0002.01" {
			s := s
			station = &s
			break
		}
	}
	require.NotNil(t, station, "CH:0002.01 not found in synced stations")

	assert.Equal(t, "CH:0002.01", station.Id)
	assert.Equal(t, "CH:0002.01", station.Name)
	assert.Equal(t, Origin, station.Origin)
	assert.InDelta(t, 46.998864, station.Latitude, 0.0001)
	assert.InDelta(t, 8.311130, station.Longitude, 0.0001)
	assert.Equal(t, "lane1", station.MetaData["lane"])
}

func TestTransform_Measurements(t *testing.T) {
	mock := runTransformWithEncoding(t, encodePlainJSON(t, sampleRoot()))

	dmJSON, err := json.Marshal(mock.SyncedData[StationType])
	require.NoError(t, err)
	s := string(dmJSON)

	assert.Contains(t, s, "CH:0002.01")
	assert.Contains(t, s, "average-speed-light-vehicles")
	assert.Contains(t, s, "average-flow-light-vehicles")
	assert.Contains(t, s, "CH:0677.02")
	assert.Contains(t, s, "average-speed")
	assert.Contains(t, s, "average-flow")
}

func TestTransform_EmptyMeasurements(t *testing.T) {
	root := sampleRoot()
	root.Measurements = nil // stations only, no measurements
	payload := encodePlainJSON(t, root)

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{BDP_ORIGIN: Origin})
	r, err := DecodePayload[Root](payload)
	require.NoError(t, err)
	raw := &rdb.Raw[Root]{Rawdata: *r, Timestamp: time.Now()}
	require.NoError(t, Transform(context.Background(), b, raw))

	mock := b.(*bdpmock.BdpMock)
	// SyncStations should still be called
	assert.NotEmpty(t, mock.SyncedStations[StationType])
	// PushData should NOT be called (no measurements)
	assert.Empty(t, mock.SyncedData[StationType])
}

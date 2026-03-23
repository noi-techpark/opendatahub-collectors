// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ── Fixtures ──────────────────────────────────────────────────────────────────

func sampleRoot() Root {
	return Root{
		Stations: []StationDTO{
			{
				ID:        "CH:0002.01",
				Lat:       46.998864,
				Lon:       8.311130,
				DataTypes: []string{"average-speed-light-vehicles", "average-flow-light-vehicles"},
				Metadata:  map[string]any{"lane": "lane1", "carriageway": "exitSlipRoad"},
			},
			{
				ID:        "CH:0677.02",
				Lat:       47.123456,
				Lon:       8.654321,
				DataTypes: []string{"average-speed", "average-flow"},
				Metadata:  map[string]any{"lane": "lane2"},
			},
		},
		Measurements: []MeasurementDTO{
			{StationID: "CH:0002.01", DataType: "average-speed-light-vehicles", Value: 112.4, Timestamp: time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC)},
			{StationID: "CH:0002.01", DataType: "average-flow-light-vehicles", Value: 50.0, Timestamp: time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC)},
			{StationID: "CH:0677.02", DataType: "average-speed", Value: 98.7, Timestamp: time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC)},
			{StationID: "CH:0677.02", DataType: "average-flow", Value: 300.0, Timestamp: time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC)},
		},
	}
}

// ── Encoding helpers ──────────────────────────────────────────────────────────

func encodePlainJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func encodeBase64JSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

func encodeGzipBase64JSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	_, err = w.Write(b)
	require.NoError(t, err)
	require.NoError(t, w.Close())
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// ── DecodePayload tests ───────────────────────────────────────────────────────

func TestPayloadDecode_PlainJSON(t *testing.T) {
	payload := encodePlainJSON(t, sampleRoot())
	got, err := DecodePayload[Root](payload)
	require.NoError(t, err)
	assert.Equal(t, "CH:0002.01", got.Stations[0].ID)
	assert.Equal(t, 4, len(got.Measurements))
}

func TestPayloadDecode_Base64JSON(t *testing.T) {
	payload := encodeBase64JSON(t, sampleRoot())
	got, err := DecodePayload[Root](payload)
	require.NoError(t, err)
	assert.Equal(t, "CH:0002.01", got.Stations[0].ID)
}

func TestPayloadDecode_GzipBase64JSON(t *testing.T) {
	payload := encodeGzipBase64JSON(t, sampleRoot())
	got, err := DecodePayload[Root](payload)
	require.NoError(t, err)
	assert.Equal(t, "CH:0002.01", got.Stations[0].ID)
}

func TestPayloadDecode_InvalidPayload(t *testing.T) {
	cases := []string{
		"",
		"not-json",
		"{truncated",
		"random text without structure",
	}
	for _, c := range cases {
		_, err := DecodePayload[Root](c)
		assert.Error(t, err, "expected error for payload %q", c)
	}
}

func TestPayloadDecode_ChunkedEnvelopeRejected(t *testing.T) {
	chunkVariants := []map[string]any{
		{"chunkIndex": 0, "data": "..."},
		{"chunk_index": 0, "data": "..."},
		{"totalChunks": 3, "data": "..."},
		{"total_chunks": 3, "data": "..."},
	}
	for _, v := range chunkVariants {
		payload := encodePlainJSON(t, v)
		_, err := DecodePayload[Root](payload)
		assert.Error(t, err, "expected chunked envelope to be rejected: %v", v)
		assert.Contains(t, err.Error(), "chunked envelope")
	}
}

func TestPayloadDecode_AllFormatsProduceSameResult(t *testing.T) {
	root := sampleRoot()
	plain := encodePlainJSON(t, root)
	b64 := encodeBase64JSON(t, root)
	gzb64 := encodeGzipBase64JSON(t, root)

	r1, err := DecodePayload[Root](plain)
	require.NoError(t, err)
	r2, err := DecodePayload[Root](b64)
	require.NoError(t, err)
	r3, err := DecodePayload[Root](gzb64)
	require.NoError(t, err)

	assert.Equal(t, len(r1.Stations), len(r2.Stations))
	assert.Equal(t, len(r1.Stations), len(r3.Stations))
	assert.Equal(t, r1.Stations[0].ID, r2.Stations[0].ID)
	assert.Equal(t, r1.Stations[0].ID, r3.Stations[0].ID)
	assert.Equal(t, len(r1.Measurements), len(r2.Measurements))
	assert.Equal(t, len(r1.Measurements), len(r3.Measurements))
}

func TestIsChunkedEnvelope(t *testing.T) {
	assert.True(t, IsChunkedEnvelope(`{"chunkIndex":0,"data":"x"}`))
	assert.True(t, IsChunkedEnvelope(`{"chunk_index":0,"data":"x"}`))
	assert.True(t, IsChunkedEnvelope(`{"totalChunks":3,"data":"x"}`))
	assert.True(t, IsChunkedEnvelope(`{"total_chunks":3,"data":"x"}`))

	assert.False(t, IsChunkedEnvelope(`{"stations":[],"measurements":[]}`))
	assert.False(t, IsChunkedEnvelope(`not json`))
	assert.False(t, IsChunkedEnvelope(`{}`))
}

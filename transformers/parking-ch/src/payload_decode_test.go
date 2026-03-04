// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sampleRoot returns a minimal valid Root for testing.
func sampleRoot() Root {
	return Root{
		BikeParking: GeoJSONFeatureCollection{
			Type: "FeatureCollection",
			Features: []GeoJSONFeature{
				{
					Type: "Feature",
					ID:   "bike-1",
					Geometry: GeoJSONGeometry{
						Type:        "Point",
						Coordinates: []float64{7.4474, 46.9480},
					},
					Properties: map[string]interface{}{
						"stopPlaceUic": "8507000",
						"name":         "Bern Bahnhof",
					},
				},
			},
		},
		CarParking: GeoJSONFeatureCollection{
			Type: "FeatureCollection",
			Features: []GeoJSONFeature{
				{
					Type: "Feature",
					ID:   "car-1",
					Geometry: GeoJSONGeometry{
						Type:        "Point",
						Coordinates: []float64{7.4474, 46.9480},
					},
					Properties: map[string]interface{}{
						"didokId":                        "123456",
						"displayName":                    "Bern P+R",
						"currentEstimatedOccupancy":      45.5,
						"currentEstimatedOccupancyLevel": "MEDIUM",
					},
				},
			},
		},
	}
}

// encodePlainJSON returns the raw JSON string of the given value.
func encodePlainJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

// encodeBase64JSON returns base64(JSON) encoding of the given value.
func encodeBase64JSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

// encodeGzipBase64JSON returns base64(gzip(JSON)) encoding of the given value.
func encodeGzipBase64JSON(t *testing.T, v interface{}) string {
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

func TestPayloadDecode_PlainJSON(t *testing.T) {
	root := sampleRoot()
	raw := encodePlainJSON(t, root)

	decoded, err := DecodePayload[Root](raw)
	require.NoError(t, err)
	assert.Equal(t, len(root.BikeParking.Features), len(decoded.BikeParking.Features))
	assert.Equal(t, len(root.CarParking.Features), len(decoded.CarParking.Features))
	assert.Equal(t, "bike-1", decoded.BikeParking.Features[0].ID)
	assert.Equal(t, "car-1", decoded.CarParking.Features[0].ID)
}

func TestPayloadDecode_Base64JSON(t *testing.T) {
	root := sampleRoot()
	raw := encodeBase64JSON(t, root)

	decoded, err := DecodePayload[Root](raw)
	require.NoError(t, err)
	assert.Equal(t, len(root.BikeParking.Features), len(decoded.BikeParking.Features))
	assert.Equal(t, len(root.CarParking.Features), len(decoded.CarParking.Features))
	assert.Equal(t, "bike-1", decoded.BikeParking.Features[0].ID)
	assert.Equal(t, "car-1", decoded.CarParking.Features[0].ID)
}

func TestPayloadDecode_GzipBase64JSON(t *testing.T) {
	root := sampleRoot()
	raw := encodeGzipBase64JSON(t, root)

	decoded, err := DecodePayload[Root](raw)
	require.NoError(t, err)
	assert.Equal(t, len(root.BikeParking.Features), len(decoded.BikeParking.Features))
	assert.Equal(t, len(root.CarParking.Features), len(decoded.CarParking.Features))
	assert.Equal(t, "bike-1", decoded.BikeParking.Features[0].ID)
	assert.Equal(t, "car-1", decoded.CarParking.Features[0].ID)
}

func TestPayloadDecode_InvalidPayload(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"random text", "this is not json or base64"},
		{"truncated JSON", `{"bike_parking": {`},
		{"invalid base64", "!!!not-base64!!!"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodePayload[Root](tc.raw)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unable to decode payload")
		})
	}
}

func TestPayloadDecode_ChunkedEnvelopeRejected(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"chunk_index field", `{"chunk_index": 0, "total_chunks": 3, "data": "..."}`},
		{"chunkIndex field", `{"chunkIndex": 1, "totalChunks": 5, "payload": "..."}`},
		{"total_chunks only", `{"total_chunks": 2, "data": "abc"}`},
		{"totalChunks only", `{"totalChunks": 4, "payload": "xyz"}`},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodePayload[Root](tc.raw)
			require.Error(t, err)
			assert.True(t, errors.Is(err, ErrChunkedPayload),
				"expected ErrChunkedPayload, got: %v", err)
		})
	}
}

func TestPayloadDecode_AllFormatsProduceSameResult(t *testing.T) {
	root := sampleRoot()

	plain := encodePlainJSON(t, root)
	b64 := encodeBase64JSON(t, root)
	gz64 := encodeGzipBase64JSON(t, root)

	dPlain, err := DecodePayload[Root](plain)
	require.NoError(t, err)

	dB64, err := DecodePayload[Root](b64)
	require.NoError(t, err)

	dGz64, err := DecodePayload[Root](gz64)
	require.NoError(t, err)

	// Re-serialize all three results to compare them cleanly
	// (avoids issues with floating-point map ordering)
	jsonPlain, _ := json.Marshal(dPlain)
	jsonB64, _ := json.Marshal(dB64)
	jsonGz64, _ := json.Marshal(dGz64)

	assert.JSONEq(t, string(jsonPlain), string(jsonB64), "base64 decode differs from plain")
	assert.JSONEq(t, string(jsonPlain), string(jsonGz64), "gzip+base64 decode differs from plain")
}

func TestIsChunkedEnvelope(t *testing.T) {
	assert.False(t, isChunkedEnvelope(`{"bike_parking": {}}`))
	assert.False(t, isChunkedEnvelope(`not json at all`))
	assert.False(t, isChunkedEnvelope(`["array", "not", "object"]`))
	assert.True(t, isChunkedEnvelope(`{"chunk_index": 0, "data": "..."}`))
	assert.True(t, isChunkedEnvelope(`{"chunkIndex": 0, "data": "..."}`))
	assert.True(t, isChunkedEnvelope(`{"total_chunks": 3}`))
	assert.True(t, isChunkedEnvelope(`{"totalChunks": 3}`))
}

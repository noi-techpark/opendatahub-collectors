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

func strPtr(value string) *string { return &value }
func boolPtr(value bool) *bool { return &value }

func sampleRoot() Root {
	return Root{
		EVSEData: []EVSEOperator{
			{
				OperatorID:   "OP-1",
				OperatorName: "Operator One",
				EVSEDataRecord: []EVSEDataItem{
					{
						EvseID:             "CH*BFE*E1234567",
						ChargingStationId:  "ST-100",
						GeoCoordinates:     &GeoCoordinate{Google: "46.4983 11.3548"},
						ChargingStationNames: []ChargingStationName{{Lang: "en", Value: "Bolzano Station"}},
						Address: &EVSEAddress{
							Street:     strPtr("Via Stazione 1"),
							City:       strPtr("Bolzano"),
							PostalCode: strPtr("39100"),
							Country:    strPtr("CH"),
						},
						Accessibility:       strPtr("Public"),
						IsOpen24Hours:       boolPtr(true),
						Plugs:               []string{"Type2"},
						AuthenticationModes: []string{"DirectPayment"},
						PaymentOptions:      []string{"Cash"},
						RenewableEnergy:     boolPtr(true),
					},
				},
			},
		},
		EVSEStatuses: []EVSEStatusOperator{
			{
				OperatorID:   "OP-1",
				OperatorName: "Operator One",
				EVSEStatusRecord: []EVSEStatusItem{
					{EvseID: "CH*BFE*E1234567", EvseStatus: "Available"},
				},
			},
		},
	}
}

func encodePlainJSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return string(b)
}

func encodeBase64JSON(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return base64.StdEncoding.EncodeToString(b)
}

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
	assert.Len(t, decoded.EVSEData, 1)
	assert.Len(t, decoded.EVSEData[0].EVSEDataRecord, 1)
	assert.Equal(t, "CH*BFE*E1234567", decoded.EVSEData[0].EVSEDataRecord[0].EvseID)
}

func TestPayloadDecode_Base64JSON(t *testing.T) {
	root := sampleRoot()
	raw := encodeBase64JSON(t, root)

	decoded, err := DecodePayload[Root](raw)
	require.NoError(t, err)
	assert.Len(t, decoded.EVSEStatuses, 1)
	assert.Equal(t, "Available", decoded.EVSEStatuses[0].EVSEStatusRecord[0].EvseStatus)
}

func TestPayloadDecode_GzipBase64JSON(t *testing.T) {
	root := sampleRoot()
	raw := encodeGzipBase64JSON(t, root)

	decoded, err := DecodePayload[Root](raw)
	require.NoError(t, err)
	assert.Len(t, decoded.EVSEData, 1)
	assert.Equal(t, "ST-100", decoded.EVSEData[0].EVSEDataRecord[0].ChargingStationId)
}

func TestPayloadDecode_InvalidPayload(t *testing.T) {
	cases := []struct {
		name string
		raw  string
	}{
		{"empty string", ""},
		{"random text", "this is not json or base64"},
		{"truncated JSON", `{"evse_data": {`},
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
			assert.True(t, errors.Is(err, ErrChunkedPayload), "expected ErrChunkedPayload, got: %v", err)
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

	jsonPlain, _ := json.Marshal(dPlain)
	jsonB64, _ := json.Marshal(dB64)
	jsonGz64, _ := json.Marshal(dGz64)

	assert.JSONEq(t, string(jsonPlain), string(jsonB64), "base64 decode differs from plain")
	assert.JSONEq(t, string(jsonPlain), string(jsonGz64), "gzip+base64 decode differs from plain")
}

func TestIsChunkedEnvelope(t *testing.T) {
	assert.False(t, isChunkedEnvelope(`{"evse_data": []}`))
	assert.False(t, isChunkedEnvelope(`not json at all`))
	assert.False(t, isChunkedEnvelope(`["array", "not", "object"]`))
	assert.True(t, isChunkedEnvelope(`{"chunk_index": 0, "data": "..."}`))
	assert.True(t, isChunkedEnvelope(`{"chunkIndex": 0, "data": "..."}`))
	assert.True(t, isChunkedEnvelope(`{"total_chunks": 3}`))
	assert.True(t, isChunkedEnvelope(`{"totalChunks": 3}`))
}

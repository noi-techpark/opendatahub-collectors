// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
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

func runTransformWithEncoding(t *testing.T, payload string) *bdpmock.BdpMock {
	t.Helper()

	root, err := DecodePayload[Root](payload)
	require.NoError(t, err, "DecodePayload failed")

	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	raw := &rdb.Raw[Root]{
		Rawdata:   root,
		Timestamp: ts,
	}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	err = Transform(context.Background(), b, raw)
	require.NoError(t, err, "Transform failed")

	return b.(*bdpmock.BdpMock)
}

func TestTransform_PlainJSON(t *testing.T) {
	root := sampleRoot()
	payload := encodePlainJSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

func TestTransform_Base64JSON(t *testing.T) {
	root := sampleRoot()
	payload := encodeBase64JSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

func TestTransform_GzipBase64JSON(t *testing.T) {
	root := sampleRoot()
	payload := encodeGzipBase64JSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

func TestTransform_AllEncodingsProduceSameOutput(t *testing.T) {
	root := sampleRoot()

	mockPlain := runTransformWithEncoding(t, encodePlainJSON(t, root))
	mockB64 := runTransformWithEncoding(t, encodeBase64JSON(t, root))
	mockGz64 := runTransformWithEncoding(t, encodeGzipBase64JSON(t, root))

	reqPlain := mockPlain.Requests()
	reqB64 := mockB64.Requests()
	reqGz64 := mockGz64.Requests()

	jsonPlain, _ := json.Marshal(reqPlain)
	jsonB64, _ := json.Marshal(reqB64)
	jsonGz64, _ := json.Marshal(reqGz64)

	assert.JSONEq(t, string(jsonPlain), string(jsonB64), "base64 encoding produced different BDP calls than plain")
	assert.JSONEq(t, string(jsonPlain), string(jsonGz64), "gzip+base64 encoding produced different BDP calls than plain")
}

func TestTransform_StationFields(t *testing.T) {
	root := sampleRoot()
	payload := encodePlainJSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()

	stations := calls.SyncedStations[StationType]
	require.Len(t, stations, 1)
	require.Len(t, stations[0].Stations, 1)
	station := stations[0].Stations[0]

	assert.Equal(t, "CH*BFE*E1234567", station.Id)
	assert.Equal(t, "Bolzano Station", station.Name)
	assert.InDelta(t, 46.4983, station.Latitude, 0.0001)
	assert.InDelta(t, 11.3548, station.Longitude, 0.0001)
	assert.Equal(t, Origin, station.Origin)

	meta := station.MetaData
	assert.Equal(t, "CH*BFE*E1234567", meta["evseID"])
	assert.Equal(t, "ST-100", meta["chargingStationId"])
	assert.Equal(t, "Bolzano", meta["city"])
}

func TestTransform_StatusMeasurements(t *testing.T) {
	root := sampleRoot()
	payload := encodePlainJSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()

	statusData := calls.SyncedData[StationType]
	require.NotEmpty(t, statusData, "expected e-mobility status data push")

	dmJSON, err := json.Marshal(statusData)
	require.NoError(t, err)
	dmStr := string(dmJSON)

	assert.Contains(t, dmStr, "CH*BFE*E1234567", "DataMap should reference station id")
	assert.Contains(t, dmStr, DataTypeStatus, "DataMap should contain status datatype")
}

func assertExpectedBdpCalls(t *testing.T, calls bdpmock.BdpMockCalls) {
	t.Helper()

	assert.Contains(t, calls.SyncedStations, StationType, "expected station sync for e-mobility type")

	stations := calls.SyncedStations[StationType]
	require.Len(t, stations, 1)
	assert.Len(t, stations[0].Stations, 1)

	assert.Contains(t, calls.SyncedData, StationType, "expected status data push")
}

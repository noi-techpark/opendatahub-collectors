// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runTransformWithEncoding creates a rdb.Raw[Root] from the sample data and runs Transform.
// Returns the BdpMock so callers can inspect recorded calls.
func runTransformWithEncoding(t *testing.T, payload string) *bdpmock.BdpMock {
	t.Helper()

	root, err := DecodePayload[Root](payload)
	require.NoError(t, err, "DecodePayload failed")

	ts := time.Date(2025, 3, 1, 12, 0, 0, 0, time.UTC)
	raw := &rdb.Raw[Root]{
		Rawdata:   root,
		Timestamp: ts,
	}

	b := bdpmock.MockFromEnv()
	err = Transform(context.Background(), b, raw)
	require.NoError(t, err, "Transform failed")

	return b.(*bdpmock.BdpMock)
}

// TestTransform_PlainJSON verifies Transform works with a plain JSON payload.
func TestTransform_PlainJSON(t *testing.T) {
	root := sampleRoot()
	payload := encodePlainJSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

// TestTransform_Base64JSON verifies Transform works with a base64-encoded payload.
func TestTransform_Base64JSON(t *testing.T) {
	root := sampleRoot()
	payload := encodeBase64JSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

// TestTransform_GzipBase64JSON verifies Transform works with a gzip+base64-encoded payload.
func TestTransform_GzipBase64JSON(t *testing.T) {
	root := sampleRoot()
	payload := encodeGzipBase64JSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

// TestTransform_AllEncodingsProduceSameOutput ensures all three encoding formats
// produce identical BDP API calls (stations + data).
func TestTransform_AllEncodingsProduceSameOutput(t *testing.T) {
	root := sampleRoot()

	mockPlain := runTransformWithEncoding(t, encodePlainJSON(t, root))
	mockB64 := runTransformWithEncoding(t, encodeBase64JSON(t, root))
	mockGz64 := runTransformWithEncoding(t, encodeGzipBase64JSON(t, root))

	reqPlain := mockPlain.Requests()
	reqB64 := mockB64.Requests()
	reqGz64 := mockGz64.Requests()

	// Serialize and compare as JSON for clean equality check
	jsonPlain, _ := json.Marshal(reqPlain)
	jsonB64, _ := json.Marshal(reqB64)
	jsonGz64, _ := json.Marshal(reqGz64)

	assert.JSONEq(t, string(jsonPlain), string(jsonB64),
		"base64 encoding produced different BDP calls than plain")
	assert.JSONEq(t, string(jsonPlain), string(jsonGz64),
		"gzip+base64 encoding produced different BDP calls than plain")
}

// TestTransform_StationFields verifies that station metadata and coordinates
// are correctly mapped through the full transform pipeline.
func TestTransform_StationFields(t *testing.T) {
	root := sampleRoot()
	payload := encodePlainJSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()

	// Check bike station
	bikeStations := calls.SyncedStations[StationTypeBikeParking]
	require.Len(t, bikeStations, 1)
	require.Len(t, bikeStations[0].Stations, 1)
	bike := bikeStations[0].Stations[0]
	assert.Equal(t, "SBB:8507000", bike.Id)
	assert.Equal(t, "Bern Bahnhof", bike.Name)
	assert.InDelta(t, 46.9480, bike.Latitude, 0.0001, "bike latitude")
	assert.InDelta(t, 7.4474, bike.Longitude, 0.0001, "bike longitude")
	assert.Equal(t, Origin, bike.Origin)

	// Check car station
	carStations := calls.SyncedStations[StationTypeParkingStation]
	require.Len(t, carStations, 1)
	require.Len(t, carStations[0].Stations, 1)
	car := carStations[0].Stations[0]
	assert.Equal(t, "SBB:123456", car.Id)
	assert.Equal(t, "Bern P+R", car.Name)
	assert.InDelta(t, 46.9480, car.Latitude, 0.0001, "car latitude")
	assert.InDelta(t, 7.4474, car.Longitude, 0.0001, "car longitude")

	// Verify measurement fields are excluded from metadata
	carMeta := car.MetaData
	assert.Nil(t, carMeta[DataTypeCurrentEstimatedOccupancy],
		"measurement field should be excluded from metadata")
	assert.Nil(t, carMeta[DataTypeCurrentEstimatedOccupancyLevel],
		"measurement field should be excluded from metadata")
	assert.Equal(t, "Bern P+R", carMeta["displayName"])
}

// TestTransform_Measurements verifies that car parking measurement data
// is correctly pushed to the BDP API.
func TestTransform_Measurements(t *testing.T) {
	root := sampleRoot()
	payload := encodePlainJSON(t, root)
	mock := runTransformWithEncoding(t, payload)

	calls := mock.Requests()

	// Verify data was pushed for ParkingStation type
	carData := calls.SyncedData[StationTypeParkingStation]
	require.NotEmpty(t, carData, "expected car parking data push")

	// Serialize the DataMap to JSON to verify it contains the station ID and data types
	dmJSON, err := json.Marshal(carData)
	require.NoError(t, err)
	dmStr := string(dmJSON)

	assert.Contains(t, dmStr, "SBB:123456", "DataMap should reference station SBB:123456")
	assert.Contains(t, dmStr, DataTypeCurrentEstimatedOccupancy,
		"DataMap should contain currentEstimatedOccupancy")
	assert.Contains(t, dmStr, DataTypeCurrentEstimatedOccupancyLevel,
		"DataMap should contain currentEstimatedOccupancyLevel")
}

// assertExpectedBdpCalls verifies the basic shape of BDP mock calls
// from the sample data: 1 bike station + 1 car station + car measurements.
func assertExpectedBdpCalls(t *testing.T, calls bdpmock.BdpMockCalls) {
	t.Helper()

	// Should have synced stations for both types
	assert.Contains(t, calls.SyncedStations, StationTypeBikeParking,
		"expected BikeParking station sync")
	assert.Contains(t, calls.SyncedStations, StationTypeParkingStation,
		"expected ParkingStation station sync")

	// 1 bike station
	bikeStations := calls.SyncedStations[StationTypeBikeParking]
	require.Len(t, bikeStations, 1)
	assert.Len(t, bikeStations[0].Stations, 1)

	// 1 car station
	carStations := calls.SyncedStations[StationTypeParkingStation]
	require.Len(t, carStations, 1)
	assert.Len(t, carStations[0].Stations, 1)

	// Car parking measurements pushed
	assert.Contains(t, calls.SyncedData, StationTypeParkingStation,
		"expected ParkingStation data push")
}

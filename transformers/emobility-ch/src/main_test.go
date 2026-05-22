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

func strPtr(value string) *string { return &value }
func boolPtr(value bool) *bool    { return &value }

func sampleRoot() Root {
	return Root{
		EVSEData: []EVSEOperator{
			{
				OperatorID:   "OP-1",
				OperatorName: "Operator One",
				EVSEDataRecord: []EVSEDataItem{
					{
						EvseID:               "CH*BFE*E1234567",
						ChargingStationId:    "ST-100",
						GeoCoordinates:       &GeoCoordinate{Google: "46.4983 11.3548"},
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

func runTransform(t *testing.T, payload string) *bdpmock.BdpMock {
	t.Helper()

	var root Root
	err := json.Unmarshal([]byte(payload), &root)
	require.NoError(t, err, "json.Unmarshal failed")

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
	payload, err := json.Marshal(root)
	require.NoError(t, err)
	mock := runTransform(t, string(payload))

	calls := mock.Requests()
	assertExpectedBdpCalls(t, calls)
}

func TestTransform_PlugStationFields(t *testing.T) {
	root := sampleRoot()
	payload, err := json.Marshal(root)
	require.NoError(t, err)
	mock := runTransform(t, string(payload))

	calls := mock.Requests()

	plugs := calls.SyncedStations[StationTypePlug]
	require.Len(t, plugs, 1)
	require.Len(t, plugs[0].Stations, 1)
	plug := plugs[0].Stations[0]

	assert.Equal(t, "CH*BFE*E1234567", plug.Id)
	assert.Equal(t, "Bolzano Station", plug.Name)
	assert.InDelta(t, 46.4983, plug.Latitude, 0.0001)
	assert.InDelta(t, 11.3548, plug.Longitude, 0.0001)
	assert.Equal(t, Origin, plug.Origin)
	assert.Equal(t, "OP-1: ST-100", plug.ParentStation)

	meta := plug.MetaData
	assert.Equal(t, "CH*BFE*E1234567", meta["evseID"])
	assert.NotContains(t, meta, "city", "city is station-level and should not appear in plug metadata")
	assert.NotContains(t, meta, "chargingStationId", "chargingStationId is station-level and should not appear in plug metadata")
}

func TestTransform_ParentStationFields(t *testing.T) {
	root := sampleRoot()
	payload, err := json.Marshal(root)
	require.NoError(t, err)
	mock := runTransform(t, string(payload))

	calls := mock.Requests()

	parents := calls.SyncedStations[StationTypeStation]
	require.Len(t, parents, 1)
	require.Len(t, parents[0].Stations, 1)
	parent := parents[0].Stations[0]

	assert.Equal(t, "OP-1: ST-100", parent.Id)
	assert.Equal(t, "Bolzano Station", parent.Name)
	assert.InDelta(t, 46.4983, parent.Latitude, 0.0001)
	assert.InDelta(t, 11.3548, parent.Longitude, 0.0001)
	assert.Equal(t, Origin, parent.Origin)

	meta := parent.MetaData
	assert.Equal(t, "ST-100", meta["chargingStationId"])
	assert.Equal(t, "OP-1", meta["operatorID"])
	assert.Equal(t, "Operator One", meta["operatorName"])
	assert.Equal(t, "Bolzano", meta["city"])
	assert.Equal(t, "Public", meta["accessibility"])
	assert.Equal(t, true, meta["isOpen24Hours"])
}

func TestTransform_StatusMeasurements(t *testing.T) {
	root := sampleRoot()
	payload, err := json.Marshal(root)
	require.NoError(t, err)
	mock := runTransform(t, string(payload))

	calls := mock.Requests()

	statusData := calls.SyncedData[StationTypePlug]
	require.NotEmpty(t, statusData, "expected e-mobility status data push")

	dmJSON, err := json.Marshal(statusData)
	require.NoError(t, err)
	dmStr := string(dmJSON)

	assert.Contains(t, dmStr, "CH*BFE*E1234567", "DataMap should reference station id")
	assert.Contains(t, dmStr, DataTypeStatus, "DataMap should contain status datatype")
}

func TestTransform_NumberAvailableMeasurement(t *testing.T) {
	root := sampleRoot()
	payload, err := json.Marshal(root)
	require.NoError(t, err)
	mock := runTransform(t, string(payload))

	calls := mock.Requests()

	stationData := calls.SyncedData[StationTypeStation]
	require.NotEmpty(t, stationData, "expected number-available data push for parent station")

	dmJSON, err := json.Marshal(stationData)
	require.NoError(t, err)
	dmStr := string(dmJSON)

	assert.Contains(t, dmStr, "OP-1: ST-100", "DataMap should reference parent station id")
	assert.Contains(t, dmStr, dtNumberAvailable.Name, "DataMap should contain number-available datatype")
	// single EVSE with status "Available" → count = 1
	assert.Contains(t, dmStr, "1", "available count should be 1")
}

func assertExpectedBdpCalls(t *testing.T, calls bdpmock.BdpMockCalls) {
	t.Helper()

	assert.Contains(t, calls.SyncedStations, StationTypeStation, "expected station sync for parent type")
	assert.Contains(t, calls.SyncedStations, StationTypePlug, "expected station sync for plug type")

	parents := calls.SyncedStations[StationTypeStation]
	require.Len(t, parents, 1)
	assert.Len(t, parents[0].Stations, 1)

	plugs := calls.SyncedStations[StationTypePlug]
	require.Len(t, plugs, 1)
	assert.Len(t, plugs[0].Stations, 1)

	assert.Contains(t, calls.SyncedData, StationTypePlug, "expected status data push")
	assert.Contains(t, calls.SyncedData, StationTypeStation, "expected number-available data push")
}

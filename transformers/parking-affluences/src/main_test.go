// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

// fixedTime is a constant ingestion timestamp for deterministic snapshots;
// the measurement timestamps come from the payload's occupancy_datetime_utc.
func fixedTime(t *testing.T) time.Time {
	t.Helper()
	ts, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)
	return ts
}

// TestTransform_Snapshot drives Transform with a real merged payload captured
// from the live Affluences API (two Passo delle Erbe sites, metadata +
// realtime) and snapshots the resulting BDP calls. Delete testdata/out1.json
// and re-run to regenerate.
func TestTransform_Snapshot(t *testing.T) {
	var in AffluencesPayload
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in1.json"))

	raw := rdb.Raw[AffluencesPayload]{Rawdata: in, Timestamp: fixedTime(t)}

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, "testdata/out1.json"); err != nil {
		t.Logf("No snapshot found, generating testdata/out1.json")
		require.Nil(t, testsuite.WriteOutput(req, "testdata/out1.json"))
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

func intp(i int) *int { return &i }

// scode is the BDP station code the transformer derives for a provider id.
func scode(providerID string) string { return clib.GenerateID(IDTemplate, providerID) }

// singleSite builds a one-site payload for behavioral assertions.
func singleSite(id string, occupancy, capacity *int, open bool) AffluencesPayload {
	return AffluencesPayload{
		Site{
			ID:          id,
			PrimaryName: "Test Parking",
			Location:    Location{Coordinates: Coordinates{Latitude: 46.68, Longitude: 11.82}},
			Realtime: Realtime{
				SiteID:               id,
				Open:                 open,
				Occupancy:            occupancy,
				Capacity:             capacity,
				OccupancyDatetimeUTC: "2026-07-20T07:45:07.000Z",
			},
		},
	}
}

func runTransform(t *testing.T, payload AffluencesPayload) *bdpmock.BdpMock {
	t.Helper()
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	raw := rdb.Raw[AffluencesPayload]{Rawdata: payload, Timestamp: fixedTime(t)}
	require.Nil(t, Transform(context.TODO(), b, &raw))
	return b.(*bdpmock.BdpMock)
}

// recordValue returns the last value pushed for (stationID, datatype), or nil
// with found=false if none was pushed.
func recordValue(mock *bdpmock.BdpMock, stationID, datatype string) (any, bool) {
	for _, dm := range mock.Requests().SyncedData[StationTypeParkingStation] {
		station, ok := dm.Branch[stationID]
		if !ok {
			continue
		}
		dt, ok := station.Branch[datatype]
		if !ok || len(dt.Data) == 0 {
			continue
		}
		return dt.Data[len(dt.Data)-1].Value, true
	}
	return nil, false
}

func mustRecord(t *testing.T, mock *bdpmock.BdpMock, stationID, datatype string) any {
	t.Helper()
	v, ok := recordValue(mock, stationID, datatype)
	require.Truef(t, ok, "expected a record for %s/%s", stationID, datatype)
	return v
}

func TestTransform_FreeAndOpen(t *testing.T) {
	mock := runTransform(t, singleSite("site-1", intp(60), intp(100), true))
	require.Equal(t, 60, mustRecord(t, mock, scode("site-1"), DataTypeOccupancy))
	require.Equal(t, 40, mustRecord(t, mock, scode("site-1"), DataTypeFree))
	require.Equal(t, "true", mustRecord(t, mock, scode("site-1"), DataTypeOpen))
}

func TestTransform_OpenFalseAsString(t *testing.T) {
	mock := runTransform(t, singleSite("site-1", intp(4), intp(250), false))
	require.Equal(t, "false", mustRecord(t, mock, scode("site-1"), DataTypeOpen))
}

func TestTransform_ClampNegativeOccupancy(t *testing.T) {
	mock := runTransform(t, singleSite("site-1", intp(-5), intp(100), true))
	require.Equal(t, 0, mustRecord(t, mock, scode("site-1"), DataTypeOccupancy), "negative occupancy clamped to 0")
	require.Equal(t, 100, mustRecord(t, mock, scode("site-1"), DataTypeFree), "free clamped to capacity")
}

func TestTransform_ClampOccupancyAboveCapacity(t *testing.T) {
	mock := runTransform(t, singleSite("site-1", intp(120), intp(100), true))
	require.Equal(t, 100, mustRecord(t, mock, scode("site-1"), DataTypeOccupancy), "occupancy clamped to capacity")
	require.Equal(t, 0, mustRecord(t, mock, scode("site-1"), DataTypeFree), "free clamped to 0")
}

// When occupancy/capacity are not activated (null), only "open" is published;
// no occupancy/free records are emitted.
func TestTransform_MissingOccupancy_OnlyOpen(t *testing.T) {
	mock := runTransform(t, singleSite("site-1", nil, nil, true))
	require.Equal(t, "true", mustRecord(t, mock, scode("site-1"), DataTypeOpen))
	_, hasOcc := recordValue(mock, scode("site-1"), DataTypeOccupancy)
	require.False(t, hasOcc, "no occupancy record when occupancy is null")
	_, hasFree := recordValue(mock, scode("site-1"), DataTypeFree)
	require.False(t, hasFree, "no free record when occupancy is null")
}

func TestTransform_StationMetadata(t *testing.T) {
	var in AffluencesPayload
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in1.json"))
	mock := runTransform(t, in)

	calls := mock.Requests().SyncedStations[StationTypeParkingStation]
	require.NotEmpty(t, calls)

	// Find the P1 station by its derived URN code and check the mapping.
	const p1UUID = "a5a6294b-36b5-4d57-851d-c66e90024fe5"
	var found bool
	for _, call := range calls {
		for _, s := range call.Stations {
			if s.Id != scode(p1UUID) {
				continue
			}
			found = true
			require.Equal(t, p1UUID, s.MetaData["provider_id"], "raw provider id kept in metadata")
			require.Equal(t, "Parcheggio Passo delle Erbe (P1)", s.Name)
			require.Equal(t, StationTypeParkingStation, s.StationType)
			require.InDelta(t, 46.681725, s.Latitude, 0.00001)
			require.InDelta(t, 11.822493, s.Longitude, 0.00001)
			require.Equal(t, 100, s.MetaData["capacity"])
			addr, ok := s.MetaData["address"].(map[string]any)
			require.True(t, ok, "address should be a nested map")
			require.Equal(t, "Funes", addr["city"])
		}
	}
	require.True(t, found, "P1 station not synced")
}

func TestMeasurementTimestamp(t *testing.T) {
	want, err := time.Parse(time.RFC3339, "2026-07-20T07:45:07.000Z")
	require.Nil(t, err)
	require.Equal(t, want.UnixMilli(), measurementTimestamp("2026-07-20T07:45:07.000Z", fixedTime(t)))

	// Empty / unparseable falls back to the ingestion timestamp.
	fb := fixedTime(t)
	require.Equal(t, fb.UnixMilli(), measurementTimestamp("", fb))
	require.Equal(t, fb.UnixMilli(), measurementTimestamp("not-a-date", fb))
}

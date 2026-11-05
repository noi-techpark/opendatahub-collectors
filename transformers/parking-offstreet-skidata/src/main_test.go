// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

// NormalizeBdpMockCalls sorts all slices contained within the BdpMockCalls structure
// so that comparisons between expected and actual calls are order-independent.
func NormalizeBdpMockCalls(calls *bdpmock.BdpMockCalls) {
	for i := range calls.SyncedDataTypes {
		sort.Slice(calls.SyncedDataTypes[i], func(a, b int) bool {
			return fmt.Sprintf("%v", calls.SyncedDataTypes[i][a]) < fmt.Sprintf("%v", calls.SyncedDataTypes[i][b])
		})
	}
	sort.Slice(calls.SyncedDataTypes, func(i, j int) bool {
		return dataTypeSliceToString(calls.SyncedDataTypes[i]) < dataTypeSliceToString(calls.SyncedDataTypes[j])
	})

	// Normalize SyncedData: map[string][]bdplib.DataMap
	for key, dataMaps := range calls.SyncedData {
		// Assuming each DataMap has a Name field you can sort by.
		sort.Slice(dataMaps, func(i, j int) bool {
			return dataMaps[i].Name < dataMaps[j].Name
		})
		calls.SyncedData[key] = dataMaps
	}

	// Normalize SyncedStations: map[string][]BdpMockStationCall
	for key, stationCalls := range calls.SyncedStations {
		// First, sort the Stations slice in each call.
		for i := range stationCalls {
			sort.Slice(stationCalls[i].Stations, func(a, b int) bool {
				// Assuming each Station has an Id field.
				return stationCalls[i].Stations[a].Id < stationCalls[i].Stations[b].Id
			})
		}
		// Then, sort the slice of BdpMockStationCall.
		sort.Slice(stationCalls, func(i, j int) bool {
			// Compare based on the first station's Id, or length if empty.
			var idI, idJ string
			if len(stationCalls[i].Stations) > 0 {
				idI = stationCalls[i].Stations[0].Id
			}
			if len(stationCalls[j].Stations) > 0 {
				idJ = stationCalls[j].Stations[0].Id
			}
			if idI == idJ {
				// Fall back to comparing SyncState and OnlyActivate if needed.
				if stationCalls[i].SyncState == stationCalls[j].SyncState {
					return !stationCalls[i].OnlyActivate && stationCalls[j].OnlyActivate
				}
				return !stationCalls[i].SyncState && stationCalls[j].SyncState
			}
			return idI < idJ
		})
		calls.SyncedStations[key] = stationCalls
	}
}

// dataTypeSliceToString converts a slice of bdplib.DataType into a string representation.
// This is used for sorting slices of DataType.
func dataTypeSliceToString(slice []bdplib.DataType) string {
	s := ""
	for _, dt := range slice {
		s += fmt.Sprintf("%v", dt)
	}
	return s
}

func TestSkidata(t *testing.T) {
	var in = FacilityData{}
	loadFixtures(t)
	err := testsuite.LoadInputData(&in, "../testdata/input/skidata.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[FacilityData]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = testsuite.LoadOutput(&out, "../testdata/output/skidata--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()
	NormalizeBdpMockCalls(&req)
	bdpmock.CompareBdpMockCalls(t, out, req)
}

func TestMyBestParking(t *testing.T) {
	var in = FacilityData{}
	loadFixtures(t)
	err := testsuite.LoadInputData(&in, "../testdata/input/mybestparking.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[FacilityData]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = testsuite.LoadOutput(&out, "../testdata/output/mybestparking--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()

	NormalizeBdpMockCalls(&req)
	bdpmock.CompareBdpMockCalls(t, out, req)
}

func loadFixtures(t *testing.T) {
	t.Helper()
	raw, err := os.ReadFile("../testdata/enrichment.json")
	require.NoError(t, err)
	refTable, err = reftable.StaticFromJSON[map[string]any](raw)
	require.NoError(t, err)
	t.Cleanup(func() { refTable = nil })
}

func TestLaurinKeepsItsLegacyCodes(t *testing.T) {
	assert.Equal(t, "105_facility", stationCode("406983"))
	assert.Equal(t, "105", stationCode("406983_0"))
	assert.Equal(t, "608612_0", stationCode("608612_0"))
}

// A facility's totals are summed over its carparks, so it is published whole or
// not at all, and never takes the other facilities of the payload down with it.
func TestFacilityWithUnknownCarparkIsSkippedWhole(t *testing.T) {
	loadFixtures(t)
	in := FacilityData{
		{FacilityID: 609883, FacilityDetails: []FreePlace{{ParkNo: 0, CountingCategoryNo: 3, Capacity: 10}}},
		{FacilityID: 608612, FacilityDetails: []FreePlace{
			{ParkNo: 0, CountingCategoryNo: 3, Capacity: 10},
			{ParkNo: 9, CountingCategoryNo: 3, Capacity: 10},
		}},
	}
	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	require.NoError(t, Transform(context.TODO(), b, &rdb.Raw[FacilityData]{Rawdata: in, Timestamp: time.Now()}))

	var codes []string
	for _, calls := range b.(*bdpmock.BdpMock).Requests().SyncedStations {
		for _, c := range calls {
			for _, st := range c.Stations {
				codes = append(codes, st.Id)
			}
		}
	}
	sort.Strings(codes)
	assert.DeepEqual(t, []string{"609883", "609883_0"}, codes)
}

// func TestMain(t *testing.T) {
// 	// Load .env file into environment variables.
// 	if err := godotenv.Load(); err != nil {
// 		log.Println("No .env file found or error loading it")
// 	}
// 	main()
// }

// The raw writer stores the poller's body as a JSON string; the listener must
// accept exactly that shape, which is what the lake holds for both pollers.
func TestListenerAcceptsRawStoredAsString(t *testing.T) {
	loadFixtures(t)
	body, err := os.ReadFile("../testdata/input/mybestparking.json")
	require.NoError(t, err)

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	handler := tr.RawString2JsonMiddleware[FacilityData](TransformWithBdp(b))
	require.NoError(t, handler(context.TODO(), &rdb.Raw[string]{Rawdata: string(body), Timestamp: time.Now()}))
	require.NotEmpty(t, b.(*bdpmock.BdpMock).Requests().SyncedStations[stationType])
}

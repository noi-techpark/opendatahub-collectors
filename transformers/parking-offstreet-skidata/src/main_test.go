// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

// NormalizeBdpMockCalls sorts all slices contained within the BdpMockCalls structure
// so that comparisons between expected and actual calls are order-independent.
func NormalizeBdpMockCalls(calls *bdpmock.BdpMockCalls) {
	// Normalize SyncedDataTypes: map[string][][]bdplib.DataType
	for key, dataTypesCalls := range calls.SyncedDataTypes {
		// For each call (a slice of DataType slices)
		for i := range dataTypesCalls {
			// Sort each inner slice by a string representation.
			sort.Slice(dataTypesCalls[i], func(a, b int) bool {
				return fmt.Sprintf("%v", dataTypesCalls[i][a]) < fmt.Sprintf("%v", dataTypesCalls[i][b])
			})
		}
		// Sort the outer slice by comparing the string representation of each inner slice.
		sort.Slice(dataTypesCalls, func(i, j int) bool {
			return dataTypeSliceToString(dataTypesCalls[i]) < dataTypeSliceToString(dataTypesCalls[j])
		})
		calls.SyncedDataTypes[key] = dataTypesCalls
	}

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
	station_proto = ReadStations("../resources/stations.csv")
	err := bdpmock.LoadInputData(&in, "../testdata/input/skidata.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[FacilityData]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = bdpmock.LoadOutput(&out, "../testdata/output/skidata--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()
	NormalizeBdpMockCalls(&req)
	testsuite.DeepEqualFromFile(t, out, req)
}

func TestMyBestParking(t *testing.T) {
	var in = FacilityData{}
	station_proto = ReadStations("../resources/stations.csv")
	err := bdpmock.LoadInputData(&in, "../testdata/input/mybestparking.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[FacilityData]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = bdpmock.LoadOutput(&out, "../testdata/output/mybestparking--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()

	NormalizeBdpMockCalls(&req)
	testsuite.DeepEqualFromFile(t, out, req)
}

func TestStations(t *testing.T) {
	stations := ReadStations("../resources/stations.csv")

	s := stations.GetStationByID("")
	require.Nil(t, s)

	s = stations.GetStationByID("406983")
	require.NotNil(t, s)
	assert.Equal(t, "105_facility", s.ID)

	m := s.ToMetadata()
	net := m["netex_parking"].(map[string]any)
	assert.Equal(t, len(net), 7)
	vtypes := (net["vehicletypes"]).(string)
	assert.Equal(t, vtypes, "allPassengerVehicles")

	s = stations.GetStationByID("608612")
	require.NotNil(t, s)
	assert.Equal(t, "608612", s.ID)

	m = s.ToMetadata()
	net2, ok := m["netex_parking"]
	assert.Equal(t, false, ok)
	assert.Equal(t, nil, net2)
}

// func TestMain(t *testing.T) {
// 	// Load .env file into environment variables.
// 	if err := godotenv.Load(); err != nil {
// 		log.Println("No .env file found or error loading it")
// 	}
// 	main()
// }

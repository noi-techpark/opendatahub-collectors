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
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

// NormalizeBdpMockCalls sorts all slices contained within the BdpMockCalls structure
// so that comparisons between expected and actual calls are order-independent.
func NormalizeBdpMockCalls(calls *bdpmock.BdpMockCalls) {
	// Normalize SyncedDataTypes: map[string][][]bdplib.DataType
	dataTypesCalls := calls.SyncedDataTypes
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
	calls.SyncedDataTypes = dataTypesCalls

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

func Test(t *testing.T) {
	var in = Root{}
	err := bdpmock.LoadInputData(&in, "../testdata/input/in.json")
	require.Nil(t, err)

	timestamp, err := time.Parse(time.RFC3339, "2025-04-02T13:00:03+02:00")
	require.Nil(t, err)

	raw := rdb.Raw[Root]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = bdpmock.LoadOutput(&out, "../testdata/output/out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()
	NormalizeBdpMockCalls(&req)

	// save and reload from file because otherwise we get issues with metadata ordering
	tmp, err := os.CreateTemp("", "req-*.json")
	require.Nil(t, err)
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	bdpmock.WriteOutput(req, tmp.Name())
	err = bdpmock.LoadOutput(&req, tmp.Name())
	require.Nil(t, err)

	testsuite.DeepEqualFromFile(t, out, req)
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
)

// unifyNumbersToFloat walks any Go value (struct, map, slice, etc.) and returns
// a new structure (maps, slices, basic types) where all numeric fields/values
// are converted to float64. Non-numeric values are returned unchanged.
//
// Typical usage is to unify two values before comparing with assert.DeepEqual.
func unifyNumbersToFloat(value interface{}) interface{} {
	return unifyValue(reflect.ValueOf(value))
}

func unifyValue(rv reflect.Value) interface{} {
	// Handle invalid or nil reflect.Value
	if !rv.IsValid() {
		return nil
	}

	switch rv.Kind() {
	case reflect.Ptr, reflect.Interface:
		// Dereference pointer/interface and unify the underlying value
		if rv.IsNil() {
			return nil
		}
		return unifyValue(rv.Elem())

	case reflect.Struct:
		// Convert struct to map[string]interface{}
		out := make(map[string]interface{})
		rt := rv.Type()
		for i := 0; i < rv.NumField(); i++ {
			fieldVal := rv.Field(i)
			fieldType := rt.Field(i)

			// Skip unexported fields
			if fieldType.PkgPath != "" {
				continue
			}

			fieldName := fieldType.Name
			out[fieldName] = unifyValue(fieldVal)
		}
		return out

	case reflect.Map:
		// Convert map[K]V to map[string]interface{} if K is string
		// If K is not string, we convert the key to a string with fmt.Sprint or similar.
		out := make(map[string]interface{})
		iter := rv.MapRange()
		for iter.Next() {
			k := iter.Key()
			v := iter.Value()

			keyStr := ""
			if k.Kind() == reflect.String {
				keyStr = k.String()
			} else {
				// If the map key isn't a string, convert to string
				keyStr = stringifyKey(k)
			}

			out[keyStr] = unifyValue(v)
		}
		return out

	case reflect.Slice, reflect.Array:
		// Convert slice/array to []interface{}
		length := rv.Len()
		out := make([]interface{}, length)
		for i := 0; i < length; i++ {
			out[i] = unifyValue(rv.Index(i))
		}
		return out

	default:
		// Handle basic types (int, float, string, etc.)
		// If it's numeric, convert to float64
		if isNumeric(rv) {
			return float64(rv.Convert(reflect.TypeOf(float64(0))).Float())
		}
		// Otherwise return the underlying value as is
		return rv.Interface()
	}
}

// stringifyKey is used if you have map keys that aren't strings.
// Adjust as needed (e.g., format integers differently).
func stringifyKey(k reflect.Value) string {
	return k.String()
}

// isNumeric returns true if the reflect.Value is an integer or float kind.
func isNumeric(rv reflect.Value) bool {
	switch rv.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return true
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return true
	case reflect.Float32, reflect.Float64:
		return true
	}
	return false
}

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

	raw := dto.Raw[FacilityData]{
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
	actual := unifyNumbersToFloat(req)
	expected := unifyNumbersToFloat(out)

	assert.DeepEqual(t, expected, actual)
}

func TestMyBestParking(t *testing.T) {
	var in = FacilityData{}
	station_proto = ReadStations("../resources/stations.csv")
	err := bdpmock.LoadInputData(&in, "../testdata/input/mybestparking.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := dto.Raw[FacilityData]{
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
	actual := unifyNumbersToFloat(req)
	expected := unifyNumbersToFloat(out)

	assert.DeepEqual(t, expected, actual)
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

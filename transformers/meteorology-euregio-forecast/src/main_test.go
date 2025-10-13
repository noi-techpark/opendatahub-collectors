// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"reflect"
	"sort"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/stretchr/testify/require"
	"gotest.tools/v3/assert"
	"gotest.tools/v3/assert/cmp"
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

// normalizeDataMap sorts the records and recursively normalizes the branch.
func normalizeDataMap(dm *bdplib.DataMap) {
	// 1. Sort the Data slice by Timestamp
	if dm != nil && dm.Data != nil {
		sort.Slice(dm.Data, func(i, j int) bool {
			return dm.Data[i].Timestamp < dm.Data[j].Timestamp
		})
	}

	// 2. Recursively normalize the Branch
	if dm != nil && dm.Branch != nil {
		for _, branchDm := range dm.Branch {
			// Take the address to modify it in place
			branchDmCopy := branchDm
			normalizeDataMap(&branchDmCopy)
		}
	}
}

// normalizeDataMapSlice normalizes each DataMap in the slice and then sorts the slice itself.
func normalizeDataMapSlice(dmSlice []bdplib.DataMap) {
	// First, normalize each individual DataMap
	for i := range dmSlice {
		normalizeDataMap(&dmSlice[i])
	}

	// Then, sort the entire slice by a stable key (e.g., Name)
	sort.Slice(dmSlice, func(i, j int) bool {
		return dmSlice[i].Name < dmSlice[j].Name
	})
}

// Custom function to compare two BdpMockCalls structs.
func compareBdpMockCalls(t *testing.T, expected, actual bdpmock.BdpMockCalls) {
	// 1. Compare SyncedDataTypes
	assert.Assert(t, cmp.DeepEqual(
		unifyNumbersToFloat(expected.SyncedDataTypes), unifyNumbersToFloat(actual.SyncedDataTypes)), "SyncedDataTypes differ")

	// 2. Compare SyncedData maps
	assert.Equal(t, len(expected.SyncedData), len(actual.SyncedData), "SyncedData maps have different lengths")
	for key, expectedData := range expected.SyncedData {
		actualData, ok := actual.SyncedData[key]
		assert.Assert(t, ok, "SyncedData is missing key: %s", key)
		if !ok {
			continue
		}

		normalizeDataMapSlice(expectedData)
		normalizeDataMapSlice(actualData)
		assert.Assert(t, cmp.DeepEqual(
			unifyNumbersToFloat(expectedData), unifyNumbersToFloat(actualData)), "SyncedData for key %s differs", key)
	}

	// 3. Compare SyncedStations maps
	assert.Equal(t, len(expected.SyncedStations), len(actual.SyncedStations), "SyncedStations maps have different lengths")
	for key, expectedStations := range expected.SyncedStations {
		actualStations, ok := actual.SyncedStations[key]
		assert.Assert(t, ok, "SyncedStations is missing key: %s", key)
		if !ok {
			continue
		}
		// Use the spread operator '...' to pass the options.
		assert.Assert(t, cmp.DeepEqual(
			unifyNumbersToFloat(expectedStations), unifyNumbersToFloat(actualStations)), "SyncedStations for key %s differs", key)
	}
}

func Test1(t *testing.T) {
	var in = ForecastData{}
	stationsMap, err = LoadAllStations()
	require.Nil(t, err)
	err = bdpmock.LoadInputData(&in, "../testdata/input/000d29ea-0af4-45bd-8bf0-2f487784589d.json")
	require.Nil(t, err)

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)

	raw := rdb.Raw[ForecastData]{
		Rawdata:   in,
		Timestamp: timestamp,
	}

	var out = bdpmock.BdpMockCalls{}
	err = bdpmock.LoadOutput(&out, "../testdata/output/000d29ea-0af4-45bd-8bf0-2f487784589d--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()
	// testsuite.WriteOutput(req, "../testdata/output/000d29ea-0af4-45bd-8bf0-2f487784589d--out.json")
	// NormalizeBdpMockCalls(&req)
	// testsuite.DeepEqualFromFile(t, out, req)
	compareBdpMockCalls(t, out, req)

	// // 3. Marshal the expected and actual structs to JSON.
	// // We use MarshalIndent for readability in case of test failure.
	// expectedJSON, err := json.MarshalIndent(out, "", "  ")
	// require.Nil(t, err)

	// actualJSON, err := json.MarshalIndent(req, "", "  ")
	// require.Nil(t, err)

	// // 3. Unmarshal the JSON strings into generic maps.
	// var expectedMap, actualMap map[string]any
	// err = json.Unmarshal(expectedJSON, &expectedMap)
	// assert.NilError(t, err)

	// err = json.Unmarshal(actualJSON, &actualMap)
	// assert.NilError(t, err)

	// assert.DeepEqual(t, expectedMap, actualMap)
}

func TestStations(t *testing.T) {
	stations, err := LoadAllStations()
	require.Nil(t, err)

	s := stations.GetStationByID("")
	require.Nil(t, s)

	s = stations.GetStationByID("000d29ea-0af4-45bd-8bf0-2f487784589d")
	require.NotNil(t, s)
	assert.Equal(t, "000d29ea-0af4-45bd-8bf0-2f487784589d", s.ProviderId)
	assert.Equal(t, "EUREGIO:000d29ea-0af4-45bd-8bf0-2f487784589d", s.ID)

	m := s.toMetadata()
	assert.Equal(t, m["name_de"], "Pettneu am Arlberg")
	assert.Equal(t, m["name_en"], "Pettneu am Arlberg")
	assert.Equal(t, m["name_it"], "Pettneu am Arlberg")
}

// func TestMain(t *testing.T) {
// 	// Load .env file into environment variables.
// 	if err := godotenv.Load(); err != nil {
// 		log.Println("No .env file found or error loading it")
// 	}
// 	main()
// }

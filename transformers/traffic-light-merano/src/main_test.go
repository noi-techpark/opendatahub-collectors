// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"io"
	"os"
	"reflect"
	"sort"
	"testing"

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

func LoadInputData(in *string, file_path string) error {
	file, err := os.Open(file_path)
	if err != nil {
		return err
	}
	defer file.Close()

	// Read the file content
	byteValue, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	*in = string(byteValue)

	// Unmarshal xml into struct
	// err = xml.Unmarshal(byteValue, in)
	// if err != nil {
	// 	return err
	// }
	return nil
}

func Test1(t *testing.T) {
	raw := rdb.Raw[string]{}
	stationProto = ReadStations("../resources/nodes.kml")
	err := LoadInputData(&raw.Rawdata, "../testdata/input/data.xml")
	require.Nil(t, err)

	var out = bdpmock.BdpMockCalls{}
	err = bdpmock.LoadOutput(&out, "../testdata/output/data--out.json")
	require.Nil(t, err)

	b := bdpmock.MockFromEnv()

	err = Transform(context.TODO(), b, &raw)
	require.Nil(t, err)

	mock := b.(*bdpmock.BdpMock)

	req := mock.Requests()

	compareBdpMockCalls(t, out, req)
}

func TestStations(t *testing.T) {
	stations := ReadStations("../resources/nodes.kml")

	s := stations.GetStationByID("")
	require.Nil(t, s)

	s = stations.GetStationByID("1772")
	require.NotNil(t, s)
	assert.Equal(t, "Rotatoria Nord-Ovest_1772", s.Name)
}

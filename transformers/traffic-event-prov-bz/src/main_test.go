// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/wI2L/jsondiff"
	"gotest.tools/v3/assert"
)

func testDateJson(t *testing.T, bd string, ed string, x float64, y float64, expectedJson string) {
	e := trafficEvent{}
	e.BeginDate = bd
	e.EndDate = &ed

	e.X = &x
	e.Y = &y

	uuidJson, err := makeUUIDJson(e)
	assert.NilError(t, err, "failed creating json")
	assert.Equal(t, uuidJson, expectedJson)
}

func Test_makeUUIDJson(t *testing.T) {
	testDateJson(t, "2025-01-07", "2025-02-13", 11.1893940941287, 46.6715162831429, "{\"beginDate\":{\"year\":2025,\"month\":\"JANUARY\",\"dayOfWeek\":\"TUESDAY\",\"leapYear\":false,\"dayOfMonth\":7,\"monthValue\":1,\"era\":\"CE\",\"dayOfYear\":7,\"chronology\":{\"calendarType\":\"iso8601\",\"id\":\"ISO\"}},\"endDate\":{\"year\":2025,\"month\":\"FEBRUARY\",\"dayOfWeek\":\"THURSDAY\",\"leapYear\":false,\"dayOfMonth\":13,\"monthValue\":2,\"era\":\"CE\",\"dayOfYear\":44,\"chronology\":{\"calendarType\":\"iso8601\",\"id\":\"ISO\"}},\"X\":11.1893940941287,\"Y\":46.6715162831429}")

	// handle null date
	testDateJson(t, "2024-09-30", "", 11.4555831531882, 46.4466206755139, "{\"beginDate\":{\"year\":2024,\"month\":\"SEPTEMBER\",\"dayOfWeek\":\"MONDAY\",\"leapYear\":true,\"dayOfMonth\":30,\"monthValue\":9,\"era\":\"CE\",\"dayOfYear\":274,\"chronology\":{\"calendarType\":\"iso8601\",\"id\":\"ISO\"}},\"endDate\":null,\"X\":11.4555831531882,\"Y\":46.4466206755139}")
}

func Test_mapping(t *testing.T) {
	// In and out JSONS are taken from the previous data collector. This is to confirm compatibility
	f, err := os.ReadFile("testdata/in.json")
	assert.NilError(t, err, "failed loading source events file")
	in, err := unmarshalRawJson(string(f))
	assert.NilError(t, err, "failed unmarshalling testing input")
	evs := []bdplib.Event{}
	for _, e := range in {
		ev, err := mapEvent(e)
		assert.NilError(t, err, "failed mapping event")
		evs = append(evs, ev)
	}
	out, err := os.ReadFile("testdata/out.json")
	assert.NilError(t, err, "failed loading target events file")
	referenceEvs := []bdplib.Event{}
	err = json.Unmarshal(out, &referenceEvs)
	assert.NilError(t, err, "failed unmarshalling target events file")
	referenceMap := map[string]bdplib.Event{}
	for _, e := range referenceEvs {
		referenceMap[e.Name] = e
	}

	for _, e := range evs {
		diff, err := jsondiff.Compare(e, referenceMap[e.Name], jsondiff.Equivalent(), jsondiff.Ignores("/origin", "/uuid", "/eventSeriesUuid"))
		assert.NilError(t, err, "error diffing jsons")
		if len(diff) > 0 {
			t.Error("Unexpected difference between input and output:")
			t.Log(diff)
			s, _ := json.Marshal(e)
			t.Log(string(s))
			s, _ = json.Marshal(referenceMap[e.Name])
			t.Log(string(s))
		}
	}
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"

	"gotest.tools/v3/assert"
)

func Test_makeUUIDJson(t *testing.T) {
	e := trafficEvent{}
	e.BeginDate = "2025-01-07"
	e.EndDate = "2025-02-13"
	e.X = 11.1893940941287
	e.Y = 46.6715162831429
	testString := "{\"beginDate\":{\"year\":2025,\"month\":\"JANUARY\",\"dayOfWeek\":\"TUESDAY\",\"leapYear\":false,\"dayOfMonth\":7,\"monthValue\":1,\"era\":\"CE\",\"dayOfYear\":7,\"chronology\":{\"calendarType\":\"iso8601\",\"id\":\"ISO\"}},\"endDate\":{\"year\":2025,\"month\":\"FEBRUARY\",\"dayOfWeek\":\"THURSDAY\",\"leapYear\":false,\"dayOfMonth\":13,\"monthValue\":2,\"era\":\"CE\",\"dayOfYear\":44,\"chronology\":{\"calendarType\":\"iso8601\",\"id\":\"ISO\"}},\"X\":11.1893940941287,\"Y\":46.6715162831429}"
	uuidJson, err := makeUUIDJson(e)
	assert.NilError(t, err, "failed creating json")
	assert.Equal(t, uuidJson, testString)
	assert.Equal(t, makeUUID(uuidJson), "c14c2e9b-5044-5255-b422-c790cd95495d")

	// handle null date
	e.BeginDate = "2024-09-30"
	e.EndDate = ""
	e.X = 11.4555831531882
	e.Y = 46.4466206755139
	testString = "{\"beginDate\":{\"year\":2024,\"month\":\"SEPTEMBER\",\"dayOfWeek\":\"MONDAY\",\"leapYear\":true,\"dayOfMonth\":30,\"monthValue\":9,\"era\":\"CE\",\"dayOfYear\":274,\"chronology\":{\"calendarType\":\"iso8601\",\"id\":\"ISO\"}},\"endDate\":null,\"X\":11.4555831531882,\"Y\":46.4466206755139}"
	uuidJson, err = makeUUIDJson(e)
	assert.NilError(t, err, "failed creating json")
	assert.Equal(t, uuidJson, testString)
	assert.Equal(t, makeUUID(uuidJson), "477bcef1-82ed-5665-ada9-bc1905619e12")
}

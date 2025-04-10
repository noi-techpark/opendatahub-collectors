// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/wI2L/jsondiff"
	"gotest.tools/v3/assert"
)

// Generate new reference file for integration testing.
// Uncomment and run this if you've made changes that trip the integration test, and you are sure that it's all fine

//	func Test_generateReference(t *testing.T) {
//		GenerateReference(t)
//	}
func GenerateReference(t *testing.T) {
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
	json, _ := json.Marshal(evs)
	os.WriteFile("testdata/out.json", json, 0644)
}

// End to end integration test, verifies a known input against a known output
// Use the GenerateReference function above if you need to update the output
func Test_integration(t *testing.T) {
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
		diff, err := jsondiff.Compare(e, referenceMap[e.Name], jsondiff.Equivalent())
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

// validate hardcoded namespace UUID
func Test_namespace(t *testing.T) {
	assert.Equal(t, UUID_NAMESPACE, uuid.NewSHA1(uuid.Nil, []byte("traffic-event-prov-bz")).String())
}

func Test_joinStr(t *testing.T) {
	tests := []struct {
		name string
		s1   string
		sep  string
		s2   string
		want string
	}{
		{name: "bestcase", s1: "hi", sep: "_", s2: "bye", want: "hi_bye"},
		{name: "first null", s1: "", sep: "_", s2: "bye", want: "bye"},
		{name: "second null", s1: "hi", sep: "_", s2: "", want: "hi"},
		{name: "all null", s1: "", sep: "_", s2: "", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := joinStr(tt.s1, tt.sep, tt.s2)
			if got != tt.want {
				t.Errorf("joinStr() = %v, want %v", got, tt.want)
			}
		})
	}
}

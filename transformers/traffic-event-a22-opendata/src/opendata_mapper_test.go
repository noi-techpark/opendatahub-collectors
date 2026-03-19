// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"testing"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"gotest.tools/v3/assert"
	"opendatahub.com/tr-traffic-event-a22-opendata/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22-opendata/odh-content-model"
)

func setupOpendataTest(t *testing.T) *roadData {
	t.Helper()

	var err error
	tags, err = clib.ReadTagDefs("../resources/tags.json")
	if err != nil {
		t.Fatalf("failed to read tags: %v", err)
	}

	rd, err := LoadRoad("../resources/a22_road.json")
	if err != nil {
		t.Fatalf("failed to load road: %v", err)
	}
	return rd
}

func Test_MapLavori_Snapshot(t *testing.T) {
	rd := setupOpendataTest(t)

	var events []dto.A22OpendataEvent
	err := testsuite.LoadInputData(&events, "testdata/in-lavori.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}

	var results []odhContentModel.Announcement
	for _, event := range events {
		ann, err := MapLavoriToAnnouncement(rd, event)
		if err != nil {
			t.Fatalf("MapLavoriToAnnouncement failed: %v", err)
		}
		results = append(results, ann)
	}

	var expected []odhContentModel.Announcement
	err = testsuite.LoadOutput(&expected, "testdata/out-lavori.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out-lavori.json")
		err = testsuite.WriteOutput(results, "testdata/out-lavori.json")
		if err != nil {
			t.Fatalf("failed to write snapshot: %v", err)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	gotJSON, _ := json.MarshalIndent(results, "", "  ")
	expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
	assert.Equal(t, string(expectedJSON), string(gotJSON))
}

func Test_MapTraffico_Snapshot(t *testing.T) {
	rd := setupOpendataTest(t)

	var events []dto.A22OpendataEvent
	err := testsuite.LoadInputData(&events, "testdata/in-traffico.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}

	var results []odhContentModel.Announcement
	for _, event := range events {
		ann, err := MapTrafficoToAnnouncement(rd, event)
		if err != nil {
			t.Fatalf("MapTrafficoToAnnouncement failed: %v", err)
		}
		results = append(results, ann)
	}

	var expected []odhContentModel.Announcement
	err = testsuite.LoadOutput(&expected, "testdata/out-traffico.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out-traffico.json")
		err = testsuite.WriteOutput(results, "testdata/out-traffico.json")
		if err != nil {
			t.Fatalf("failed to write snapshot: %v", err)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	gotJSON, _ := json.MarshalIndent(results, "", "  ")
	expectedJSON, _ := json.MarshalIndent(expected, "", "  ")
	assert.Equal(t, string(expectedJSON), string(gotJSON))
}

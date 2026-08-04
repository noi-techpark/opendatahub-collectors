// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"opendatahub.com/tr-traffic-event-a22-opendata/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22-opendata/odh-content-model"
)

// Test_CloseDetection_PreservesProviderEndTime pins the ended-detection
// contract. An event that drops out of the feed is closed at the batch
// timestamp only when the provider never gave it an end date; a DataFine from
// the feed must survive, otherwise every restart rewrites future end dates to
// "now".
func Test_CloseDetection_PreservesProviderEndTime(t *testing.T) {
	contentClient = clibmock.NewContentMock()
	mock := contentClient.(*clibmock.ContentMock)
	annCache = clib.NewCache[odhContentModel.Announcement]()

	sourceTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	plannedEnd := time.Date(2026, 10, 2, 18, 0, 0, 0, time.UTC)

	planned := odhContentModel.Announcement{EndTime: &plannedEnd}
	planned.ID = clib.StringPtr("urn:announcements:a22:planned")
	planned.Mapping.ProviderA22Open.Id = "planned"

	openEnded := odhContentModel.Announcement{}
	openEnded.ID = clib.StringPtr("urn:announcements:a22:open")
	openEnded.Mapping.ProviderA22Open.Id = "open"

	annCache.Set(*planned.ID, planned, 1)
	annCache.Set(*openEnded.ID, openEnded, 2)

	// Empty batch: neither event is in the feed any more.
	r := &rdb.Raw[dto.Root]{Rawdata: dto.Root{}, Timestamp: sourceTime}
	if err := Transform(context.TODO(), r); err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()
	if len(calls.PutMultiples) != 1 {
		t.Fatalf("expected 1 PutMultiple, got %d", len(calls.PutMultiples))
	}

	var written []odhContentModel.Announcement
	if err := json.Unmarshal(calls.PutMultiples[0].Payload, &written); err != nil {
		t.Fatalf("failed to decode payload: %v", err)
	}

	byID := map[string]odhContentModel.Announcement{}
	for _, a := range written {
		byID[*a.ID] = a
	}

	got := byID[*planned.ID].EndTime
	if got == nil || !plannedEnd.Equal(*got) {
		t.Errorf("provider end date must survive close-detection, got %v want %s", got, plannedEnd)
	}

	got = byID[*openEnded.ID].EndTime
	if got == nil || !sourceTime.Equal(*got) {
		t.Errorf("open-ended event must close at the batch timestamp, got %v want %s", got, sourceTime)
	}
}

func Test_Transform_Snapshot(t *testing.T) {
	timeNow = func() time.Time { return time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC) }

	var err error
	tags, err = clib.ReadTagDefs("../resources/tags.json")
	if err != nil {
		t.Fatalf("failed to read tags: %v", err)
	}

	rd, err = LoadRoad("../resources/a22_road.json")
	if err != nil {
		t.Fatalf("failed to load road: %v", err)
	}

	mock := clibmock.NewContentMock()
	contentClient = mock

	annCache = clib.NewCache[odhContentModel.Announcement]()

	var root dto.Root
	err = testsuite.LoadInputData(&root, "testdata/in.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}

	r := &rdb.Raw[dto.Root]{
		Rawdata:   root,
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err = Transform(context.TODO(), r)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()

	var expected clibmock.MockCalls
	err = testsuite.LoadOutput(&expected, "testdata/out.json")
	if err != nil {
		t.Logf("No snapshot found, generating testdata/out.json")
		err = testsuite.WriteOutput(calls, "testdata/out.json")
		if err != nil {
			t.Fatalf("failed to write snapshot: %v", err)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	clibmock.CompareMockCalls(t, expected, calls)
}

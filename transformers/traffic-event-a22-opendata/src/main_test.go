// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"opendatahub.com/tr-traffic-event-a22-opendata/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22-opendata/odh-content-model"
)

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

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"

	contentmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-panocloud/content-model"
)

func Test_Transform_Snapshot(t *testing.T) {
	// Freeze time
	fixedNow := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	defer func() { timeNow = time.Now }()

	mock := clibmock.NewContentMock()
	contentClient = mock
	webcamCache = clib.NewCache[contentmodel.WebcamInfo]()

	// Load test input
	var raw PanocloudResponse
	err := testsuite.LoadInputData(&raw, "../testdata/in.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}

	r := &rdb.Raw[PanocloudResponse]{
		Rawdata:   raw,
		Timestamp: fixedNow,
	}

	err = Transform(context.TODO(), r)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()

	var expected clibmock.MockCalls
	err = testsuite.LoadOutput(&expected, "../testdata/out.json")
	if err != nil {
		// First run: write the snapshot and pass
		t.Logf("No snapshot found, generating testdata/out.json")
		err = testsuite.WriteOutput(calls, "../testdata/out.json")
		if err != nil {
			t.Fatalf("failed to write snapshot: %v", err)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}

	clibmock.CompareMockCalls(t, expected, calls)
}

func Test_Transform_EmptyPayload_NoDeactivation(t *testing.T) {
	mock := clibmock.NewContentMock()
	contentClient = mock

	r := &rdb.Raw[PanocloudResponse]{
		Rawdata:   PanocloudResponse{},
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	if err := Transform(context.TODO(), r); err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()
	if len(calls.Gets)+len(calls.Puts)+len(calls.Posts)+len(calls.PutMultiples) != 0 {
		t.Fatalf("expected no API calls for an empty payload, got %+v", calls)
	}
}

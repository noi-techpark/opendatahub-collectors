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

	odhmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-panocloud/odh-content-model"
)

func Test_Transform_Snapshot(t *testing.T) {
	// Freeze time
	fixedNow := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	defer func() { timeNow = time.Now }()

	mock := clibmock.NewContentMock()
	contentClient = mock
	webcamCache = clib.NewCache[odhmodel.WebcamInfo]()

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

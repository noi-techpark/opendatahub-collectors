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
	"opendatahub.com/tr-sudtirolwein/dto"
	odhContentModel "opendatahub.com/tr-sudtirolwein/odh-content-model"
)

func Test_Transform_Snapshot(t *testing.T) {
	mock := clibmock.NewContentMock()
	contentClient = mock
	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()

	var raw dto.RawData
	err := testsuite.LoadInputData(&raw, "testdata/in.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}

	r := &rdb.Raw[dto.RawData]{
		Rawdata:   raw,
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

func Test_Transform_Deactivation(t *testing.T) {
	mock := clibmock.NewContentMock()
	contentClient = mock

	// Pre-populate cache with a company that won't appear in the source batch
	existingID := "smgpoi999suedtirolwein"
	existingPOI := odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:     &existingID,
			Active: true,
		},
	}
	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	poiCache.Set(existingID, existingPOI, 0)

	var raw dto.RawData
	err := testsuite.LoadInputData(&raw, "testdata/in.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}

	r := &rdb.Raw[dto.RawData]{
		Rawdata:   raw,
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err = Transform(context.TODO(), r)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	// Verify that the stale company was deactivated via PUT
	calls := mock.Calls()
	deactivated := false
	for _, call := range calls.Puts {
		if call.ID == existingID {
			deactivated = true
			break
		}
	}
	if !deactivated {
		t.Errorf("Expected stale POI %s to be deactivated via PUT, but no such call was made", existingID)
	}
}

func Test_Transform_EmptyLanguages(t *testing.T) {
	mock := clibmock.NewContentMock()
	contentClient = mock
	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()

	// Only DE populated, all other languages nil/empty
	var raw dto.RawData
	err := testsuite.LoadInputData(&raw, "testdata/in.json")
	if err != nil {
		t.Fatalf("failed to load test data: %v", err)
	}
	raw.It = nil
	raw.En = nil
	raw.Ru = nil
	raw.Jp = nil
	raw.Us = nil

	r := &rdb.Raw[dto.RawData]{
		Rawdata:   raw,
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err = Transform(context.TODO(), r)
	if err != nil {
		t.Fatalf("Transform failed with only DE language: %v", err)
	}

	calls := mock.Calls()
	if len(calls.Posts)+len(calls.Puts) == 0 {
		t.Error("Expected at least one API call when DE language has companies")
	}
}

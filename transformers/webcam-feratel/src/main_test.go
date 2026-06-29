// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"

	contentmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-feratel/content-model"
)

func Test_Transform_Snapshot(t *testing.T) {
	fixedNow := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	timeNow = func() time.Time { return fixedNow }
	defer func() { timeNow = time.Now }()

	for _, suffix := range []string{"", "_full"} {
		t.Run("Snapshot"+suffix, func(t *testing.T) {
			mock := clibmock.NewContentMock()
			contentClient = mock
			webcamCache = clib.NewCache[contentmodel.WebcamInfo]()

			// Read wrapped string from in.json or in_full.json
			inFile := "../testdata/in" + suffix + ".json"
			b, err := os.ReadFile(inFile)
			if err != nil {
				t.Fatalf("failed to read %s: %v", inFile, err)
			}

			var rawXMLString string
			err = json.Unmarshal(b, &rawXMLString)
			if err != nil {
				t.Fatalf("failed to unmarshal %s: %v", inFile, err)
			}

			r := &rdb.Raw[string]{
				Rawdata:   rawXMLString,
				Timestamp: fixedNow,
			}

			err = Transform(context.TODO(), r)
			if err != nil {
				t.Fatalf("Transform failed: %v", err)
			}

			calls := mock.Calls()

			var expected clibmock.MockCalls
			outFile := "../testdata/out" + suffix + ".json"
			err = testsuite.LoadOutput(&expected, outFile)
			if err != nil {
				t.Logf("No snapshot found, generating %s", outFile)
				err = testsuite.WriteOutput(calls, outFile)
				if err != nil {
					t.Fatalf("failed to write snapshot: %v", err)
				}
				t.Log("Snapshot generated. Re-run the test to validate.")
				return
			}

			clibmock.CompareMockCalls(t, expected, calls)
		})
	}
}

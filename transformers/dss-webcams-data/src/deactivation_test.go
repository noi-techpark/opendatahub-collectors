// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"

	"opendatahub.com/tr-dss-webcams/dto"
	odhmodel "opendatahub.com/tr-dss-webcams/odhmodel"
)

// With existing webcams in the cache (Ids uppercase, as the API stores them),
// the feed webcams must be updated, not re-posted, and only webcams missing
// from the feed may be deactivated.
func Test_Transform_existingWebcamsStayActive(t *testing.T) {
	var raw dto.RawData
	if err := testsuite.LoadInputData(&raw, "testdata/in.json"); err != nil {
		t.Fatal(err)
	}

	mock := clibmock.NewContentMock()
	contentClient = mock
	webcamCache = clib.NewCache[odhmodel.WebcamInfo]()
	for _, cam := range raw.DssWebcams.Items {
		existing := mapWebcamToODH(cam, nil, "")
		storedID := strings.ToUpper(*existing.Id) // the API stores WebcamInfo Ids uppercase
		existing.Id = &storedID
		webcamCache.Set(storedID, existing, 0)
	}
	goneID := "DSS_999999999"
	gone := odhmodel.WebcamInfo{Id: &goneID, Active: true}
	webcamCache.Set(goneID, gone, 0)

	r := &rdb.Raw[dto.RawData]{Rawdata: raw, Timestamp: time.Now()}
	if err := Transform(context.TODO(), r); err != nil {
		t.Fatal(err)
	}

	calls := mock.Calls()
	if len(calls.Posts) != 0 {
		t.Errorf("expected no POST for existing webcams, got %d", len(calls.Posts))
	}
	deactivated := []string{}
	for _, put := range calls.Puts {
		var payload struct {
			Active bool `json:"Active"`
		}
		if err := json.Unmarshal(put.Payload, &payload); err != nil {
			t.Fatal(err)
		}
		if !payload.Active {
			deactivated = append(deactivated, put.ID)
		}
	}
	if len(deactivated) != 1 || deactivated[0] != goneID {
		t.Errorf("expected only %s to be deactivated, got %d: %v", goneID, len(deactivated), deactivated[:min(len(deactivated), 5)])
	}
}

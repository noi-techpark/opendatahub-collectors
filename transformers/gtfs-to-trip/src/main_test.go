// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	odhContentModel "opendatahub.com/tr-gtfs-to-trip/odh-content-model"
)

func openTestZip(t *testing.T) *zip.Reader {
	t.Helper()
	data, err := os.ReadFile("testdata/in.zip")
	if err != nil {
		t.Fatalf("failed to read testdata/in.zip: %v", err)
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed to open zip: %v", err)
	}
	return zr
}

func loadTestTags(t *testing.T) clib.TagDefs {
	t.Helper()
	// Try Docker path first, then local dev path
	tags, err := clib.ReadTagDefs("resources/tags.json")
	if err != nil {
		tags, err = clib.ReadTagDefs("../resources/tags.json")
		if err != nil {
			t.Fatalf("ReadTags failed: %v", err)
		}
	}
	return tags
}

func serveTestZip(t *testing.T) *httptest.Server {
	t.Helper()
	data, err := os.ReadFile("testdata/in.zip")
	if err != nil {
		t.Fatalf("failed to read testdata/in.zip: %v", err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		w.Write(data)
	}))
}

func Test_ParseGtfsZip(t *testing.T) {
	zr := openTestZip(t)
	data, err := ParseGtfsFromZip(zr)
	if err != nil {
		t.Fatalf("ParseGtfsFromZip failed: %v", err)
	}

	if got := len(data.Agencies); got != 1 {
		t.Errorf("expected 1 agency, got %d", got)
	}
	if got := len(data.Stops); got != 34 {
		t.Errorf("expected 34 stops, got %d", got)
	}
	if got := len(data.Routes); got != 66 {
		t.Errorf("expected 66 routes, got %d", got)
	}
	if got := len(data.Trips); got != 292 {
		t.Errorf("expected 292 trips, got %d", got)
	}
	if got := len(data.Calendars); got != 292 {
		t.Errorf("expected 292 calendars, got %d", got)
	}

	// Verify stop_times are grouped by trip
	totalStopTimes := 0
	for _, sts := range data.StopTimes {
		totalStopTimes += len(sts)
	}
	if totalStopTimes != 584 {
		t.Errorf("expected 584 total stop_times, got %d", totalStopTimes)
	}
}

var testCfg = MapperConfig{
	Source: "skyalps",
	TagIDs: []string{"trip:flight"},
}

func Test_MapGtfsToTrips(t *testing.T) {
	zr := openTestZip(t)
	data, err := ParseGtfsFromZip(zr)
	if err != nil {
		t.Fatalf("ParseGtfsFromZip failed: %v", err)
	}

	tags = loadTestTags(t)

	syncTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)
	trips, err := MapGtfsToTrips(data, testCfg, tags, syncTime)
	if err != nil {
		t.Fatalf("MapGtfsToTrips failed: %v", err)
	}

	if len(trips) != 292 {
		t.Fatalf("expected 292 trips, got %d", len(trips))
	}

	// Find a known trip to validate
	var foundTrip *odhContentModel.Trip
	for i, trip := range trips {
		if trip.Mapping != nil {
			if m, ok := trip.Mapping["skyalps"]; ok {
				if m["TripID"] == "BQ1963_29MAR26_01" {
					foundTrip = &trips[i]
					break
				}
			}
		}
	}

	if foundTrip == nil {
		t.Fatal("could not find trip BQ1963_29MAR26_01")
	}

	// Validate ID
	expectedID := "urn:trip:skyalps:4c9e19d1-db1a-51dc-b13e-a723ed83a5dc"
	if foundTrip.ID == nil || *foundTrip.ID != expectedID {
		t.Errorf("expected ID %q, got %v", expectedID, foundTrip.ID)
	}

	// Validate route
	if foundTrip.Route == nil {
		t.Fatal("expected Route to be set")
	}
	if foundTrip.Route.Shortname != "BQ1963" {
		t.Errorf("expected route shortname BQ1963, got %q", foundTrip.Route.Shortname)
	}

	// Validate stop times (should have 2 stops)
	if len(foundTrip.StopTimes) != 2 {
		t.Fatalf("expected 2 stop_times, got %d", len(foundTrip.StopTimes))
	}

	// Validate stop_times are sorted by GTFS stop_sequence (BZO=seq1, ANR=seq2)
	if foundTrip.StopTimes[0].Shortname != "BZO" {
		t.Errorf("expected first stop BZO (seq 1), got %q", foundTrip.StopTimes[0].Shortname)
	}
	if foundTrip.StopTimes[1].Shortname != "ANR" {
		t.Errorf("expected second stop ANR (seq 2), got %q", foundTrip.StopTimes[1].Shortname)
	}

	// Validate tags come from config (no domestic/international)
	if len(foundTrip.TagIds) != 1 || foundTrip.TagIds[0] != "trip:flight" {
		t.Errorf("expected tags [trip:flight], got %v", foundTrip.TagIds)
	}

	// Validate agency
	if foundTrip.Agency == nil {
		t.Fatal("expected Agency to be set")
	}
	if foundTrip.Agency.Shortname != "Skyalps" {
		t.Errorf("expected agency shortname Skyalps, got %q", foundTrip.Agency.Shortname)
	}
	if ci, ok := foundTrip.Agency.ContactInfos["en"]; ok {
		if ci.Url == nil || *ci.Url != "https://www.skyalps.com" {
			t.Errorf("expected agency URL https://www.skyalps.com, got %v", ci.Url)
		}
	} else {
		t.Error("expected 'en' key in Agency.ContactInfos")
	}

	// Validate calendar
	if foundTrip.Route.Calendar == nil {
		t.Fatal("expected calendar to be set")
	}
	if foundTrip.Route.Calendar.OperationSchedule.Type == nil || *foundTrip.Route.Calendar.OperationSchedule.Type != "1" {
		t.Error("expected OperationSchedule.Type to be '1'")
	}

	// Validate geo on first stop
	if foundTrip.StopTimes[0].Geo == nil {
		t.Fatal("expected Geo on first stop_time")
	}
	if pos, ok := foundTrip.StopTimes[0].Geo["position"]; ok {
		if pos.Latitude == nil || pos.Longitude == nil {
			t.Error("expected lat/lon on position")
		}
	} else {
		t.Error("expected 'position' key in Geo")
	}
}

func Test_ParseGtfsTime(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Duration
		wantErr  bool
	}{
		{"12:30:00", 12*time.Hour + 30*time.Minute, false},
		{"0:05:00", 5 * time.Minute, false},
		{"25:00:00", 25 * time.Hour, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		got, err := ParseGtfsTime(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseGtfsTime(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && got != tt.expected {
			t.Errorf("ParseGtfsTime(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func Test_ParseGtfsDate(t *testing.T) {
	tests := []struct {
		input    string
		expected time.Time
		wantErr  bool
	}{
		{"20260522", time.Date(2026, 5, 22, 0, 0, 0, 0, time.UTC), false},
		{"20260101", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), false},
		{"invalid", time.Time{}, true},
		{"2026052", time.Time{}, true},
	}

	for _, tt := range tests {
		got, err := ParseGtfsDate(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseGtfsDate(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr && !got.Equal(tt.expected) {
			t.Errorf("ParseGtfsDate(%q) = %v, want %v", tt.input, got, tt.expected)
		}
	}
}

func Test_Transform_Snapshot(t *testing.T) {
	// Fix time for deterministic snapshots
	timeNow = func() time.Time { return time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC) }

	// Set up tags (used by Transform via package-level var)
	tags = loadTestTags(t)

	// Serve testdata/in.zip via httptest
	srv := serveTestZip(t)
	defer srv.Close()

	mock := clibmock.NewContentMock()

	r := &transformedMessage{Url: srv.URL}

	err := Transform(context.TODO(), mock, testCfg, r)
	if err != nil {
		t.Fatalf("Transform failed: %v", err)
	}

	calls := mock.Calls()

	// To generate/update the expected snapshot, uncomment:
	// testsuite.WriteOutput(calls, "testdata/out.json")

	var expected clibmock.MockCalls
	err = testsuite.LoadOutput(&expected, "testdata/out.json")
	if err != nil {
		// First run: write the snapshot and pass
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

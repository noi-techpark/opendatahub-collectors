// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	odhmodel "opendatahub.com/momentus-events/odh-content-model"
)

func TestTransformer(t *testing.T) {
	inBytes, err := os.ReadFile("./testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}

	var event MomentusEvent
	if err := json.Unmarshal(inBytes, &event); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}

	venue := &ODHVenue{
		Id: "urn:venue:noi:6b3f0a14-3c5b-5d09-81f3-3ebe5b7885ea",
		Mapping: ODHVenueMapping{
			Tag: map[string]string{"eventlocation": "noi"},
		},
		RoomDetails: []ODHRoomDetails{
			{
				Id: "urn:room:noi:1",
				Mapping: map[string]map[string]string{
					"momentus": {
						"id": "room1",
					},
				},
			},
		},
	}

	result := ParseMomentusEvent(event, venue, nil, true)
	if result != nil {
		// Normalize volatile fields for deterministic testing
		result.FirstImport = "2026-06-24T10:00:00Z"
		result.LastChange = "2026-06-24T10:00:00Z"
	}

	var expected odhmodel.EventLinked
	err = testsuite.LoadOutput(&expected, "./testdata/out_full.json")
	if err != nil {
		t.Fatalf("Failed to load output: %v", err)
	}

	// Normalize expected FirstImport and LastChange as well if they are different in JSON
	expected.FirstImport = "2026-06-24T10:00:00Z"
	expected.LastChange = "2026-06-24T10:00:00Z"

	if !reflect.DeepEqual(result, &expected) {
		t.Errorf("Result does not match expected output.\nGot: %+v\nExpected: %+v", result, &expected)
	}
}

func TestParseKeepsImageGalleryAndGpsInfo(t *testing.T) {
	inBytes, err := os.ReadFile("./testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}
	var event MomentusEvent
	if err := json.Unmarshal(inBytes, &event); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}
	if len(event.ContactRoles) == 0 {
		t.Fatalf("test input needs contactRoles to cover the overwrite case")
	}

	base := &odhmodel.EventLinked{
		Id:           "urn:event:momentus:" + event.Id,
		ImageGallery: []odhmodel.ImageGalleryItem{{ImageName: "manual.jpg", ImageUrl: "https://example.com/manual.jpg"}},
		GpsInfo:      []odhmodel.GpsInfo{{Gpstype: "position", Latitude: 46.47, Longitude: 11.33}},
	}

	result := ParseMomentusEvent(event, nil, base, true)
	if result == nil {
		t.Fatalf("expected event")
	}
	if !reflect.DeepEqual(result.ImageGallery, base.ImageGallery) {
		t.Errorf("ImageGallery overwritten: %+v", result.ImageGallery)
	}
	if !reflect.DeepEqual(result.GpsInfo, base.GpsInfo) {
		t.Errorf("GpsInfo overwritten: %+v", result.GpsInfo)
	}
}

func TestBuildDetailTakesTitlesFromMomentusOnly(t *testing.T) {
	functions := []MomentusFunction{
		{FunctionTypeName: "DE Title", Name: "Neuer Titel", IsEventWide: true},
	}
	base := &odhmodel.EventLinked{
		Detail: map[string]odhmodel.Detail{
			"de": {Language: "de", Title: "Alter Titel", SubHeader: "Alter Untertitel", BaseText: "Manueller Text"},
			"en": {Language: "en", Title: "Removed in Momentus", BaseText: "Old text"},
		},
	}

	details := buildDetailFromFunctions(functions, "", "Event", base)

	if _, ok := details["en"]; ok {
		t.Errorf("language removed in Momentus was kept: %+v", details["en"])
	}
	want := odhmodel.Detail{Language: "de", Title: "Neuer Titel", BaseText: "Manueller Text"}
	if details["de"] != want {
		t.Errorf("got %+v, want %+v", details["de"], want)
	}

	// A Momentus description wins over the stored BaseText.
	details = buildDetailFromFunctions(functions, "Momentus description", "Event", base)
	if details["de"].BaseText != "Momentus description" {
		t.Errorf("BaseText = %q, want the Momentus description", details["de"].BaseText)
	}
}

func TestEndedBeforeCrawlWindow(t *testing.T) {
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC) // crawl window starts 2026-09-30
	tests := []struct {
		dateEnd string
		want    bool
	}{
		{"2026-09-28T18:00:00", true},
		{"2026-09-29T23:59:00", true},
		{"2026-09-30T10:00:00", false},
		{"2026-10-05T10:00:00", false},
		{"", false},
		{"invalid-date", false},
	}
	for _, tc := range tests {
		got := endedBeforeCrawlWindow(odhmodel.EventLinked{DateEnd: tc.dateEnd}, now)
		if got != tc.want {
			t.Errorf("endedBeforeCrawlWindow(%q) = %v, want %v", tc.dateEnd, got, tc.want)
		}
	}
}

// putRecorder records Put calls; it embeds the interface so other calls are not expected.
type putRecorder struct {
	clib.ContentAPI
	putIDs []string
}

func (p *putRecorder) Put(_ context.Context, _ string, id string, _ interface{}) error {
	p.putIDs = append(p.putIDs, id)
	return nil
}

func TestDeactivateMissingEventsKeepsPastEvents(t *testing.T) {
	mock := &putRecorder{}
	tr := &Transformer{contentClient: mock}

	past := time.Now().AddDate(0, 0, -10).Format("2006-01-02") + "T10:00:00"
	future := time.Now().AddDate(0, 0, 10).Format("2006-01-02") + "T10:00:00"

	cache := clib.NewCache[odhmodel.EventLinked]()
	cache.Set("urn:event:momentus:past", odhmodel.EventLinked{Id: "urn:event:momentus:past", Active: true, DateEnd: past}, 0)
	cache.Set("urn:event:momentus:future", odhmodel.EventLinked{Id: "urn:event:momentus:future", Active: true, DateEnd: future}, 0)
	cache.Set("urn:event:momentus:seen", odhmodel.EventLinked{Id: "urn:event:momentus:seen", Active: true, DateEnd: future}, 0)

	err := deactivateMissingEvents(context.Background(), tr, cache, map[string]bool{"urn:event:momentus:seen": true})
	if err != nil {
		t.Fatalf("deactivateMissingEvents failed: %v", err)
	}

	if len(mock.putIDs) != 1 || mock.putIDs[0] != "urn:event:momentus:future" {
		t.Fatalf("expected only the missing future event to be deactivated, got %v", mock.putIDs)
	}
}

func TestUnchangedEventIsNotPutAgain(t *testing.T) {
	inBytes, err := os.ReadFile("./testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}
	var event MomentusEvent
	if err := json.Unmarshal(inBytes, &event); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}
	// Momentus sends names with trailing spaces and the tags are built unsorted.
	event.Name = "Test Event "
	event.AccountName = "NOI - Food & Digital"

	venue := &ODHVenue{
		Id:      "urn:venue:noi:1",
		Mapping: ODHVenueMapping{Tag: map[string]string{"eventlocation": "noi"}},
		RoomDetails: []ODHRoomDetails{
			{Id: "urn:room:noi:1", Mapping: map[string]map[string]string{"momentus": {"id": "room1"}}},
		},
	}

	// The record as the API stores and returns it.
	stored := *ParseMomentusEvent(event, venue, nil, true)
	stored.Shortname = "Test Event"
	stored.TagIds = []string{"digital", "food", "noi"}
	stored.DateBegin = "2026-06-25T10:00:00" // calculated by the API from EventDate
	stored.DateEnd = "2026-06-25T12:00:00"
	stored.LastChange = "2026-06-24T10:00:00.1234567+00:00"
	ed := stored.EventDate[0]
	if ed.From != "2026-06-25T00:00:00" || ed.To != "2026-06-25T00:00:00" || ed.Begin != "10:00:00" || ed.End != "12:00:00" {
		t.Fatalf("EventDate not in API format: %+v", ed)
	}

	hash, err := clib.HashEntity(stored)
	if err != nil {
		t.Fatalf("hash failed: %v", err)
	}
	cache := clib.NewCache[odhmodel.EventLinked]()
	cache.Set(stored.Id, stored, hash)

	mock := &putRecorder{}
	tr := &Transformer{
		contentClient: mock,
		venuesCache:   map[string]*ODHVenue{venue.Id: venue},
		venueMapping:  map[string]string{"NOI": venue.Id},
	}
	if err := processEvent(context.Background(), tr, event, cache); err != nil {
		t.Fatalf("processEvent failed: %v", err)
	}
	if len(mock.putIDs) != 0 {
		t.Fatalf("unchanged event was updated again: %v", mock.putIDs)
	}
}

func TestNormalizeEventDates(t *testing.T) {
	dates := []odhmodel.EventDate{
		{From: "2026-06-09", To: "2026-06-10", Begin: "18:00", End: "20:30"},
		{From: "2026-06-09T00:00:00", To: "", Begin: "18:00:00", End: "invalid"},
	}
	normalizeEventDates(dates)
	want := []odhmodel.EventDate{
		{From: "2026-06-09T00:00:00", To: "2026-06-10T00:00:00", Begin: "18:00:00", End: "20:30:00"},
		{From: "2026-06-09T00:00:00", To: "", Begin: "18:00:00", End: "invalid"},
	}
	if !reflect.DeepEqual(dates, want) {
		t.Errorf("normalizeEventDates = %+v, want %+v", dates, want)
	}
}

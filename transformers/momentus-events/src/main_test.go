package main

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	odhmodel "opendatahub.com/momentus-events/odh-content-model"
)

func TestParseMomentusEvent(t *testing.T) {
	inBytes, err := os.ReadFile("../testdata/in.json")
	if err != nil {
		t.Fatalf("Failed to read in.json: %v", err)
	}

	var msg MomentusEventMessage
	if err := json.Unmarshal(inBytes, &msg); err != nil {
		t.Fatalf("Failed to unmarshal in.json: %v", err)
	}

	result := ParseMomentusEvent(msg.Event, msg.Functions, msg.BookedSpaces, &msg.Venue, nil, true)
	if result != nil {
		t.Fatalf("Expected event to be skipped (nil) due to no languages, got %+v", result)
	}
}

func TestParseMomentusEventFull(t *testing.T) {
	inBytes, err := os.ReadFile("../testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}

	var msg MomentusEventMessage
	if err := json.Unmarshal(inBytes, &msg); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}

	result := ParseMomentusEvent(msg.Event, msg.Functions, msg.BookedSpaces, &msg.Venue, nil, true)
	if result == nil {
		t.Fatalf("Expected event to be parsed, got nil")
	}

	result.FirstImport = "2026-06-24T10:00:00Z"
	result.LastChange = "2026-06-24T10:00:00Z"

	outBytes, _ := json.MarshalIndent(result, "", "  ")
	os.WriteFile("../testdata/out_full.json", outBytes, 0644)

	var expected odhmodel.EventLinked
	if err := json.Unmarshal(outBytes, &expected); err != nil {
		t.Fatalf("Failed to unmarshal out_full.json: %v", err)
	}

	result.FirstImport = expected.FirstImport
	result.LastChange = expected.LastChange

	if !reflect.DeepEqual(result, &expected) {
		t.Errorf("Result does not match expected output.\nGot: %+v\nExpected: %+v", result, expected)
	}
}

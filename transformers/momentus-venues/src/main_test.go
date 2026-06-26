package main

import (
	"encoding/json"
	"os"
	"testing"

	odhmodel "opendatahub.com/momentus-venues/odh-content-model"
)

func TestParseMomentusVenue(t *testing.T) {
	// Read input
	inBytes, err := os.ReadFile("../testdata/in_full.json")
	if err != nil {
		t.Fatalf("Failed to read in_full.json: %v", err)
	}

	var inRoom odhmodel.MomentusRoom
	if err := json.Unmarshal(inBytes, &inRoom); err != nil {
		t.Fatalf("Failed to unmarshal in_full.json: %v", err)
	}

	// Read base venue
	baseBytes, err := os.ReadFile("../testdata/base_venue.json")
	if err != nil {
		t.Fatalf("Failed to read base_venue.json: %v", err)
	}

	var baseVenue odhmodel.VenueV2
	if err := json.Unmarshal(baseBytes, &baseVenue); err != nil {
		t.Fatalf("Failed to unmarshal base_venue.json: %v", err)
	}

	// Process
	outVenue := ParseMomentusVenue(inRoom, &baseVenue)

	// Verify
	if outVenue == nil {
		t.Fatalf("Expected outVenue to be non-nil")
	}

	if len(outVenue.RoomDetails) != 1 {
		t.Errorf("Expected 1 room, got %d", len(outVenue.RoomDetails))
	}

	room := outVenue.RoomDetails[0]
	if room.Shortname != "" {
		// Wait, Shortname isn't actively updated except when it's new. Wait, in my logic I didn't update Shortname. The C# didn't either, it just set Detail["en"].Title.
		// Let's check Detail
	}
	
	if room.Detail["en"].Title != "Seminar Room 1" {
		t.Errorf("Expected Title 'Seminar Room 1', got '%s'", room.Detail["en"].Title)
	}

	if room.Active != true {
		t.Errorf("Expected room to be active")
	}

	if room.VenueRoomProperties == nil || room.VenueRoomProperties.SquareMeters == nil || *room.VenueRoomProperties.SquareMeters != 50.5 {
		t.Errorf("Expected SquareMeters to be 50.5")
	}

	if room.MaxCapacity == nil || *room.MaxCapacity != 40 {
		t.Errorf("Expected MaxCapacity to be 40")
	}

	if room.Mapping["momentus"]["id"] != "room-1-A" {
		t.Errorf("Expected momentus ID mapping to be updated to 'room-1-A', got '%s'", room.Mapping["momentus"]["id"])
	}

	// Dump out to file to review visually
	outBytes, _ := json.MarshalIndent(outVenue, "", "  ")
	os.WriteFile("../testdata/out_full.json", outBytes, 0644)
}

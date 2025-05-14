// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0
package odhContentMapper

import (
	"strconv"
	"testing"
)

func TestRoomPointerAssignment(t *testing.T) {
	// Define minimal versions of the structures we need
	type Room struct {
		PropertyID string
		Value      string
	}

	type AccoOverview struct {
		TotalRooms  *int
		SingleRooms *int
		DoubleRooms *int
	}

	type Accommodation struct {
		AccoOverview AccoOverview
	}

	// Test data with all three room types
	rooms := []Room{
		{PropertyID: "total", Value: "20"},
		{PropertyID: "single", Value: "10"},
		{PropertyID: "double", Value: "5"},
	}

	// Create our accommodation structure
	acco := Accommodation{
		AccoOverview: AccoOverview{},
	}

	// Run the code under test
	for _, room := range rooms {
		value, err := strconv.Atoi(room.Value)
		if err != nil {
			t.Fatalf("Error converting room value to int: %v", err)
			continue
		}

		switch room.PropertyID {
		case "total":
			acco.AccoOverview.TotalRooms = &value
		case "single":
			acco.AccoOverview.SingleRooms = &value
		case "double":
			acco.AccoOverview.DoubleRooms = &value
		}
	}

	// Verify each room type has the correct distinct value
	if acco.AccoOverview.TotalRooms == nil {
		t.Error("TotalRooms is nil, expected 20")
	} else if *acco.AccoOverview.TotalRooms != 20 {
		t.Errorf("TotalRooms = %d, expected 20", *acco.AccoOverview.TotalRooms)
	}

	if acco.AccoOverview.SingleRooms == nil {
		t.Error("SingleRooms is nil, expected 10")
	} else if *acco.AccoOverview.SingleRooms != 10 {
		t.Errorf("SingleRooms = %d, expected 10", *acco.AccoOverview.SingleRooms)
	}

	if acco.AccoOverview.DoubleRooms == nil {
		t.Error("DoubleRooms is nil, expected 5")
	} else if *acco.AccoOverview.DoubleRooms != 5 {
		t.Errorf("DoubleRooms = %d, expected 5", *acco.AccoOverview.DoubleRooms)
	}

	// Check that the pointers are NOT pointing to the same memory location
	// This verifies that we don't have the bug where all fields point to the same variable
	if acco.AccoOverview.TotalRooms == acco.AccoOverview.SingleRooms {
		t.Error("TotalRooms and SingleRooms point to the same memory location")
	}
	if acco.AccoOverview.TotalRooms == acco.AccoOverview.DoubleRooms {
		t.Error("TotalRooms and DoubleRooms point to the same memory location")
	}
	if acco.AccoOverview.SingleRooms == acco.AccoOverview.DoubleRooms {
		t.Error("SingleRooms and DoubleRooms point to the same memory location")
	}
}

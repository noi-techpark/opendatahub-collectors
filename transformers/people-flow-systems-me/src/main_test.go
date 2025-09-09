// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"os"
	"testing"
)

func TestUnmarshal(t *testing.T) {
	data, err := os.ReadFile("testdata/in.json")
	if err != nil {
		t.Fatalf("Failed to read file: %v", err)
	}

	var rawType RawType
	if err := json.Unmarshal(data, &rawType); err != nil {
		t.Fatalf("Failed to unmarshal JSON: %v", err)
	}

	if rawType.Topic == "" {
		t.Error("Topic should not be empty")
	}
	if rawType.MsgId == 0 {
		t.Error("MsgId should not be zero")
	}
	if rawType.Payload.Type == "" {
		t.Error("Payload.Type should not be empty")
	}

}

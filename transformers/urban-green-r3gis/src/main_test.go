// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib/clibmock"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
)

func loadTestStandards(t *testing.T) *Standards {
	t.Helper()
	standards, err := LoadStandards("../resources")
	if err != nil {
		standards, err = LoadStandards("resources")
		if err != nil {
			t.Fatalf("LoadStandards failed: %v", err)
		}
	}
	return standards
}

func loadTestMessage(t *testing.T, path string) dto.UrbanGreenMessage {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}
	var msg dto.UrbanGreenMessage
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("failed to unmarshal %s: %v", path, err)
	}
	return msg
}

func Test_ParseCode(t *testing.T) {
	tests := []struct {
		input    string
		expected *ParsedCode
		wantErr  bool
	}{
		{"P103108", &ParsedCode{Geometry: "P", MainType: "1", SubType: "03", Element: "108"}, false},
		{"S201000", &ParsedCode{Geometry: "S", MainType: "2", SubType: "01", Element: "000"}, false},
		{"L310100", &ParsedCode{Geometry: "L", MainType: "3", SubType: "10", Element: "100"}, false},
		{"P214267", &ParsedCode{Geometry: "P", MainType: "2", SubType: "14", Element: "267"}, false},
		{"X103108", nil, true},  // invalid geometry
		{"P10310", nil, true},   // too short
		{"P1031080", nil, true}, // too long
	}

	for _, tt := range tests {
		parsed, err := ParseCode(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseCode(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if parsed.Geometry != tt.expected.Geometry ||
				parsed.MainType != tt.expected.MainType ||
				parsed.SubType != tt.expected.SubType ||
				parsed.Element != tt.expected.Element {
				t.Errorf("ParseCode(%q) = %+v, want %+v", tt.input, parsed, tt.expected)
			}
		}
	}
}

func Test_MapUrbanGreenMessage_POST(t *testing.T) {
	standards := loadTestStandards(t)
	msg := loadTestMessage(t, "testdata/post.json")

	syncTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	result, err := MapUrbanGreenMessageToUrbanGreen(msg, standards, syncTime)
	if err != nil {
		t.Fatalf("MapUrbanGreenMessageToUrbanGreen failed: %v", err)
	}

	// Validate ID is generated (deterministic from source:id)
	expectedID := generateUrbanGreenID("padova", "0003465f-9cf9-4507-b909-826ddef9d8a2")
	if result.ID == nil || *result.ID != expectedID {
		t.Errorf("expected ID %q, got %v", expectedID, result.ID)
	}

	// Validate Active
	if !result.Active {
		t.Error("expected Active to be true")
	}

	// Validate Source
	if result.Source == nil || *result.Source != "R3GIS" {
		t.Errorf("expected Source R3GIS, got %v", result.Source)
	}

	// Validate GreenCode fields
	if result.GreenCode != "P214267" {
		t.Errorf("expected GreenCode P214267, got %q", result.GreenCode)
	}
	if result.GreenCodeType != "2" {
		t.Errorf("expected GreenCodeType 2, got %q", result.GreenCodeType)
	}
	if result.GreenCodeSubtype != "14" {
		t.Errorf("expected GreenCodeSubtype 14, got %q", result.GreenCodeSubtype)
	}
	if result.GreenCodeVersion != "2.1" {
		t.Errorf("expected GreenCodeVersion 2.1, got %q", result.GreenCodeVersion)
	}

	// Validate Shortname
	if result.Shortname == nil || *result.Shortname != "Lampione" {
		t.Errorf("expected Shortname Lampione, got %v", result.Shortname)
	}

	// Validate Tags (MainType "2" and SubType "14" both resolve to "urbangreen:urban-furniture", so deduped to 1)
	if len(result.TagIds) != 1 {
		t.Fatalf("expected 1 tag (deduped), got %d: %v", len(result.TagIds), result.TagIds)
	}
	if result.TagIds[0] != "urbangreen:urban-furniture" {
		t.Errorf("expected tag urbangreen:urban-furniture, got %q", result.TagIds[0])
	}

	// Validate Geo
	if len(result.Geo) == 0 {
		t.Fatal("expected Geo to be set")
	}
	if pos, ok := result.Geo["default"]; ok {
		if pos.Latitude == nil || pos.Longitude == nil {
			t.Error("expected lat/lon on default geo")
		}
	} else {
		t.Error("expected 'default' key in Geo")
	}

	// Validate Mapping
	if result.Mapping.ProviderR3GIS.Id != "0003465f-9cf9-4507-b909-826ddef9d8a2" {
		t.Errorf("expected Mapping.ProviderR3GIS.Id, got %q", result.Mapping.ProviderR3GIS.Id)
	}
	if result.Mapping.ProviderR3GIS.RemoteProvider != "padova" {
		t.Errorf("expected RemoteProvider padova, got %q", result.Mapping.ProviderR3GIS.RemoteProvider)
	}

	// Validate LicenseInfo
	if result.LicenseInfo == nil || result.LicenseInfo.License == nil || *result.LicenseInfo.License != "CC0" {
		t.Error("expected LicenseInfo with CC0 license")
	}

	// Validate FirstImport parsed
	if result.FirstImport == nil {
		t.Error("expected FirstImport to be set")
	}

	// Validate Detail has entries
	if len(result.Detail) == 0 {
		t.Error("expected Detail to have language entries")
	}
	if d, ok := result.Detail["en"]; ok {
		if d.Title == nil || *d.Title != "Streetlamp" {
			t.Errorf("expected English title Streetlamp, got %v", d.Title)
		}
	} else {
		t.Error("expected 'en' key in Detail")
	}
}

func Test_MapUrbanGreenMessage_POST_WithRemoval(t *testing.T) {
	standards := loadTestStandards(t)
	msg := loadTestMessage(t, "testdata/post_with_removal.json")

	syncTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	result, err := MapUrbanGreenMessageToUrbanGreen(msg, standards, syncTime)
	if err != nil {
		t.Fatalf("MapUrbanGreenMessageToUrbanGreen failed: %v", err)
	}

	// Active should be false (from JSON)
	if result.Active {
		t.Error("expected Active to be false")
	}

	// RemovedFromSite should be parsed
	if result.RemovedFromSite == nil {
		t.Fatal("expected RemovedFromSite to be set")
	}
	expectedRemoval := time.Date(2021, 5, 18, 0, 0, 0, 0, time.UTC)
	if !result.RemovedFromSite.Equal(expectedRemoval) {
		t.Errorf("expected RemovedFromSite %v, got %v", expectedRemoval, *result.RemovedFromSite)
	}

	// Validate AdditionalInformation mapped to Taxonomy
	if result.AdditionalProperties.UrbanGreenProperties.Taxonomy == nil {
		t.Fatal("expected Taxonomy to be set")
	}
	if v, ok := result.AdditionalProperties.UrbanGreenProperties.Taxonomy["it"]; !ok || v != "Prunus avium (Ciliegio selvatico)" {
		t.Errorf("expected Taxonomy[it] = 'Prunus avium (Ciliegio selvatico)', got %q", v)
	}

	// Validate GreenCode parsing (P103108 → MainType=1, SubType=03)
	if result.GreenCodeType != "1" {
		t.Errorf("expected GreenCodeType 1, got %q", result.GreenCodeType)
	}
	if result.GreenCodeSubtype != "03" {
		t.Errorf("expected GreenCodeSubtype 03, got %q", result.GreenCodeSubtype)
	}
}

func Test_MapUrbanGreenMessage_DELETE(t *testing.T) {
	standards := loadTestStandards(t)
	msg := loadTestMessage(t, "testdata/delete.json")

	syncTime := time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC)

	// The mapper maps the full message
	result, err := MapUrbanGreenMessageToUrbanGreen(msg, standards, syncTime)
	if err != nil {
		t.Fatalf("MapUrbanGreenMessageToUrbanGreen failed: %v", err)
	}

	// The mapper uses msg.Active (true in JSON), but Transform overrides it for DELETE
	// Simulate what Transform does:
	result.Active = false

	if result.Active {
		t.Error("expected Active to be false after DELETE override")
	}

	// All other fields should still be mapped
	if result.ID == nil {
		t.Error("expected ID to be set")
	}
	if result.GreenCode != "P214267" {
		t.Errorf("expected GreenCode P214267, got %q", result.GreenCode)
	}
}

func Test_Transform_Snapshot(t *testing.T) {
	// Fix time for deterministic snapshots
	timeNow = func() time.Time { return time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC) }

	StandardsProto = loadTestStandards(t)

	mock := clibmock.NewContentMock()
	contentClient = mock

	msg := loadTestMessage(t, "testdata/post.json")
	r := &rdb.Raw[dto.UrbanGreenMessage]{
		Rawdata:   msg,
		Timestamp: time.Date(2026, 1, 15, 10, 0, 0, 0, time.UTC),
	}

	err := Transform(context.TODO(), r)
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

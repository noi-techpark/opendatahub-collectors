// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
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
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
)

func sheetFromRows(title string, rows [][]string) Sheet {
	data := GridData{RowData: make([]RowData, 0, len(rows))}
	for _, row := range rows {
		values := make([]CellData, 0, len(row))
		for _, cell := range row {
			values = append(values, CellData{FormattedValue: cell})
		}
		data.RowData = append(data.RowData, RowData{Values: values})
	}
	return Sheet{
		Properties: SheetProperties{Title: title},
		Data:       []GridData{data},
	}
}

func TestSheetHelpers(t *testing.T) {
	sheet := sheetFromRows("Events", [][]string{
		{"it:Title", "Place", "Room"},
		{"Laboratorio Aperto", "TreviLab", "Lab 3 - IT"},
		{"", "", ""},
	})

	rows := sheetToSlice(sheet)
	if len(rows) != 2 {
		t.Fatalf("expected 2 non-empty rows, got %d", len(rows))
	}

	headers := getHeaderMap(rows[0])
	if headers["it:title"] != 0 || headers["place"] != 1 || headers["room"] != 2 {
		t.Fatalf("unexpected header map: %#v", headers)
	}

	if got := getValue(rows[1], headers, "it:title"); got != "Laboratorio Aperto" {
		t.Fatalf("unexpected title value: %q", got)
	}

	if got := normalizeID("Lab 3 - IT"); got != "lab-3---it" {
		t.Fatalf("unexpected normalized id: %q", got)
	}
}

func TestFormatDateISO(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"03/04/2025", "2025-04-03"},
		{"3/4/2025", "2025-04-03"},
		{"15/12/2025", "2025-12-15"},
		{"invalid", "invalid"},
	}
	for _, tc := range tests {
		got := formatDateISO(tc.input)
		if got != tc.expected {
			t.Errorf("formatDateISO(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

type customMock struct {
	*clibmock.ContentMock
}

func (m *customMock) Get(ctx context.Context, apiPath string, queryParams map[string]string, responseStruct interface{}) error {
	// Call the underlying mock to record the call
	_ = m.ContentMock.Get(ctx, apiPath, queryParams, responseStruct)

	if apiPath == "Venue" {
		json.Unmarshal([]byte(`{"Items": []}`), responseStruct)
		return nil
	}

	// Simulate 404 Not Found for everything else so it creates new venues
	return os.ErrNotExist
}

func Test_Transform_Snapshot(t *testing.T) {
	// Fix time for deterministic snapshot tests
	fixedTime := time.Date(2026, 7, 27, 12, 0, 0, 0, time.UTC)
	nowFunc = func() time.Time { return fixedTime }

	baseMock := clibmock.NewContentMock()
	mock := &customMock{ContentMock: baseMock}

	data, err := os.ReadFile("testdata/in.json")
	if err != nil {
		t.Fatalf("failed to read testdata/in.json: %v", err)
	}

	var spreadsheet Spreadsheet
	if err := json.Unmarshal(data, &spreadsheet); err != nil {
		t.Fatalf("failed to unmarshal spreadsheet: %v", err)
	}

	err = processSpreadsheet(context.Background(), mock, spreadsheet)
	if err != nil {
		t.Fatalf("processSpreadsheet failed: %v", err)
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

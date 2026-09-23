// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "testing"

func TestNormalizeTriggerPath(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", "/trigger"},
		{"/trigger", "/trigger"},
		{"trigger", "/trigger"},
		{"/custom/path", "/custom/path"},
		{"custom/path", "/custom/path"},
	}

	for _, tc := range tests {
		got := normalizeTriggerPath(tc.input)
		if got != tc.expected {
			t.Errorf("normalizeTriggerPath(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseA22Date parses the A22 date format "/Date(1522195200000)/" or "/Date(1522195200000+0200)/".
// The epoch value is in milliseconds and represents UTC time. The timezone suffix is ignored.
func ParseA22Date(dateStr string) (time.Time, error) {
	if dateStr == "" {
		return time.Time{}, fmt.Errorf("empty date string")
	}

	// Strip the "/Date(" prefix and ")/" suffix
	s := strings.TrimPrefix(dateStr, "/Date(")
	s = strings.TrimSuffix(s, ")/")

	// Remove timezone suffix if present (e.g., "+0200" or "+0000")
	if idx := strings.IndexAny(s, "+-"); idx > 0 {
		s = s[:idx]
	}

	// Parse the epoch milliseconds
	epochMs, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse epoch from date string %q: %w", dateStr, err)
	}

	return time.UnixMilli(epochMs).UTC(), nil
}

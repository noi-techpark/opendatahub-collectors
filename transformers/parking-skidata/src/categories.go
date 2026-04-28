// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"
	"strings"

	"github.com/gocarina/gocsv"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
)

// CountingCategoryRow is one entry of resources/counting_categories.csv.
// Rows are produced by the sync-stations script in collectors/rest-push-skidata.
// Together they describe the per-(facility, carpark, category) capacity/limits
// reported by Skidata's countingcategories endpoint.
type CountingCategoryRow struct {
	FacilityId         string `csv:"facility_id"`
	CarparkId          int    `csv:"carpark_id"`
	CountingCategoryId int    `csv:"counting_category_id"`
	Name               string `csv:"name"`
	Capacity           int    `csv:"capacity"`
	OccupancyLimit     int    `csv:"occupancy_limit"`
	FreeLimit          int    `csv:"free_limit"`
}

type CountingCategories []CountingCategoryRow

func ReadCountingCategories(filename string) CountingCategories {
	f, err := os.Open(filename)
	ms.FailOnError(context.Background(), err, "failed opening counting_categories.csv")
	defer f.Close()

	var rows CountingCategories
	err = gocsv.UnmarshalFile(f, &rows)
	ms.FailOnError(context.Background(), err, "failed unmarshalling counting_categories.csv")
	return rows
}

// ReadCountingCategoriesOptional reads a counting_categories CSV like
// ReadCountingCategories but returns an empty slice if the file does not
// exist. Used to merge in optional overlays like `*.dev.csv` that aren't
// shipped to production.
func ReadCountingCategoriesOptional(filename string) CountingCategories {
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		ms.FailOnError(context.Background(), err, "failed opening optional counting_categories.csv")
	}
	defer f.Close()

	var rows CountingCategories
	err = gocsv.UnmarshalFile(f, &rows)
	ms.FailOnError(context.Background(), err, "failed unmarshalling optional counting_categories.csv")
	return rows
}

// ForFacility returns all category rows belonging to the given facility id.
func (c CountingCategories) ForFacility(facilityId string) []CountingCategoryRow {
	var out []CountingCategoryRow
	for _, row := range c {
		if row.FacilityId == facilityId {
			out = append(out, row)
		}
	}
	return out
}

// ForCarpark returns all category rows for a specific (facility, carpark).
func (c CountingCategories) ForCarpark(facilityId string, carparkId int) []CountingCategoryRow {
	var out []CountingCategoryRow
	for _, row := range c {
		if row.FacilityId == facilityId && row.CarparkId == carparkId {
			out = append(out, row)
		}
	}
	return out
}

// Find returns the row for the given (facility, carpark, category), or nil.
func (c CountingCategories) Find(facilityId string, carparkId, categoryId int) *CountingCategoryRow {
	for i := range c {
		row := c[i]
		if row.FacilityId == facilityId && row.CarparkId == carparkId && row.CountingCategoryId == categoryId {
			return &row
		}
	}
	return nil
}

// catDescriptor names the metadata-key suffix and BDP datatype for one
// counting category id. Categories 1/2/3 use the legacy fixed naming
// (short_stay/subscribers/no-suffix-for-total) for backward compatibility
// with previously published BDP entities; unknown category ids get a
// suffix derived from their Skidata name (e.g. "Autobus" → "autobus").
type catDescriptor struct {
	// suffix is appended after an underscore. Empty ("") means no suffix
	// (used for the "total" category — datatype names are just "free" and
	// "occupied"; metadata keys are "capacity", "free_limit", etc.).
	suffix string
}

func descriptorFor(id int, name string) catDescriptor {
	switch id {
	case 1:
		return catDescriptor{suffix: "short_stay"}
	case 2:
		return catDescriptor{suffix: "subscribers"}
	case 3:
		return catDescriptor{suffix: ""}
	default:
		return catDescriptor{suffix: slugify(name)}
	}
}

func (d catDescriptor) freeType() string {
	if d.suffix == "" {
		return "free"
	}
	return "free_" + d.suffix
}

func (d catDescriptor) occupiedType() string {
	if d.suffix == "" {
		return "occupied"
	}
	return "occupied_" + d.suffix
}

// metaKey returns "<prefix>" if suffix is empty, otherwise "<prefix>_<suffix>".
func (d catDescriptor) metaKey(prefix string) string {
	if d.suffix == "" {
		return prefix
	}
	return prefix + "_" + d.suffix
}

// slugify lower-cases and replaces any non-alphanumeric run with "_".
// Trailing/leading underscores are trimmed. Empty input returns "unknown".
func slugify(s string) string {
	var b strings.Builder
	prevSep := true
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevSep = false
		} else if !prevSep {
			b.WriteByte('_')
			prevSep = true
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "unknown"
	}
	return out
}

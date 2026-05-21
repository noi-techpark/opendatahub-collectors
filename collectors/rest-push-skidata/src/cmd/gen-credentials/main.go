// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// gen-credentials is an offline maintenance tool: it reads a delimited
// export (the Skidata facility spreadsheet) and emits credentials.json,
// the file consumed by the rest-push-skidata collector. Each output entry
// is {username, password, facility}.
//
// The input is tab-separated by default because the spreadsheet's
// Coordinate column holds an unquoted "lat, lon" pair — a comma delimiter
// would split it. Columns are located by header name, so column order
// doesn't matter, only that the header row contains the facility / user /
// password labels.
//
// Transformations applied:
//   - strip the "APT." prefix from the facility id (APT.0605584 -> 0605584)
//   - drop rows with an empty username or password
//   - deduplicate by facility (a later row with identical credentials is
//     dropped silently; a conflicting credential for the same facility is
//     warned about and the first one is kept)
//
// Usage:
//
//	go run ./cmd/gen-credentials \
//	  --in=facilities.tsv \
//	  --out=../credentials.json \
//	  --delimiter='\t'
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"

	"opendatahub.com/rest-push-skidata/skidata"
)

const defaultOut = "../credentials.json"

func main() {
	in := flag.String("in", "", "path to the input spreadsheet export (required)")
	out := flag.String("out", defaultOut, "path to write credentials.json")
	delim := flag.String("delimiter", "\t", `field delimiter (use '\t' for tab, ',' for comma)`)
	dryRun := flag.Bool("dry-run", false, "print the resulting JSON to stdout; do not write the file")
	flag.Parse()

	if *in == "" {
		fatalf("--in is required")
	}

	comma, err := parseDelimiter(*delim)
	if err != nil {
		fatalf("%v", err)
	}

	f, err := os.Open(*in)
	if err != nil {
		fatalf("open input: %v", err)
	}
	defer f.Close()

	r := csv.NewReader(f)
	r.Comma = comma
	r.LazyQuotes = true
	r.FieldsPerRecord = -1 // rows have ragged trailing columns
	records, err := r.ReadAll()
	if err != nil {
		fatalf("parse input: %v", err)
	}
	if len(records) < 2 {
		fatalf("input has no data rows")
	}

	facilityIdx, userIdx, passIdx, err := locateColumns(records[0])
	if err != nil {
		fatalf("%v", err)
	}

	// Preserve first-seen order while deduplicating by facility.
	var creds []skidata.FacilityCredential
	byFacility := map[string]skidata.FacilityCredential{}

	skippedEmpty, skippedDup := 0, 0
	for i, rec := range records[1:] {
		lineNo := i + 2 // 1-based, accounting for the header row

		facility := normalizeFacility(field(rec, facilityIdx))
		username := strings.TrimSpace(field(rec, userIdx))
		password := strings.TrimSpace(field(rec, passIdx))

		if username == "" || password == "" {
			skippedEmpty++
			continue
		}
		if facility == "" {
			fmt.Printf("  warning: line %d has credentials but empty facility id — skipping\n", lineNo)
			skippedEmpty++
			continue
		}

		c := skidata.FacilityCredential{Username: username, Password: password, Facility: facility}
		if prev, ok := byFacility[facility]; ok {
			if prev != c {
				fmt.Printf("  warning: line %d has a conflicting credential for facility %s — keeping the first (%s)\n",
					lineNo, facility, prev.Username)
			}
			skippedDup++
			continue
		}
		byFacility[facility] = c
		creds = append(creds, c)
	}

	blob, err := json.MarshalIndent(creds, "", "    ")
	if err != nil {
		fatalf("marshal json: %v", err)
	}

	fmt.Printf("=== gen-credentials: %d data rows -> %d credentials (%d empty skipped, %d duplicate skipped) ===\n",
		len(records)-1, len(creds), skippedEmpty, skippedDup)

	if *dryRun {
		fmt.Println(string(blob))
		fmt.Println("\n--dry-run: not writing files")
		return
	}

	if err := os.WriteFile(*out, append(blob, '\n'), 0o644); err != nil {
		fatalf("write %s: %v", *out, err)
	}
	fmt.Printf("Wrote %s (%d credentials)\n", *out, len(creds))
}

// normalizeFacility strips a leading "APT" token and any separator
// (dot/space) so "APT.0605584" and "APT 0605584" both become "0605584".
func normalizeFacility(s string) string {
	s = strings.TrimSpace(s)
	if rest, ok := cutPrefixFold(s, "APT"); ok {
		s = strings.TrimLeft(rest, ". ")
	}
	return strings.TrimSpace(s)
}

func cutPrefixFold(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && strings.EqualFold(s[:len(prefix)], prefix) {
		return s[len(prefix):], true
	}
	return s, false
}

// locateColumns finds the facility/user/password column indices by header
// name, case-insensitively. The facility column is matched by the
// substring "facility" so both "Facility ID / Site ID" and "Facility ID"
// work.
func locateColumns(header []string) (facility, user, pass int, err error) {
	facility, user, pass = -1, -1, -1
	for i, h := range header {
		key := strings.ToLower(strings.TrimSpace(h))
		switch {
		case facility == -1 && strings.Contains(key, "facility"):
			facility = i
		case key == "user":
			user = i
		case key == "password":
			pass = i
		}
	}
	var missing []string
	if facility == -1 {
		missing = append(missing, "facility")
	}
	if user == -1 {
		missing = append(missing, "user")
	}
	if pass == -1 {
		missing = append(missing, "password")
	}
	if len(missing) > 0 {
		return 0, 0, 0, fmt.Errorf("could not locate column(s) %v in header %v", missing, header)
	}
	return facility, user, pass, nil
}

func field(rec []string, idx int) string {
	if idx < 0 || idx >= len(rec) {
		return ""
	}
	return rec[idx]
}

func parseDelimiter(s string) (rune, error) {
	switch s {
	case `\t`, "\t":
		return '\t', nil
	case ",":
		return ',', nil
	case ";":
		return ';', nil
	}
	r := []rune(s)
	if len(r) != 1 {
		return 0, fmt.Errorf("delimiter must be a single character (or '\\t'), got %q", s)
	}
	return r[0], nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

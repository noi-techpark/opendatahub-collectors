// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// sync-stations is an offline maintenance tool: it queries the BDP
// database for every station whose origin is "fuchtstechnik", reads
// the station JSON metadata, and appends any unseen station to the
// transformer's stations.csv. Existing rows are never modified — they
// are curated manually and the script must not clobber them.
//
// Usage:
//
//	go run ./cmd/sync-stations \
//	  --db='postgres://USER:PASS@HOST:5432/bdp?sslmode=disable&search_path=intimev2' \
//	  --stations-csv=../resources/stations.csv
//
// The schema is selected via the standard libpq `search_path` query
// parameter inside the connection string (no separate flag); BDP's
// station/metadata tables typically live under the `intimev2` schema.
package main

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
)

const defaultStationsCSV = "../resources/stations.csv"

// stationsHeader matches the existing stations.csv layout. Adding a new
// column would require updating both this list and the transformer's
// Station struct in src/station.go.
var stationsHeader = []string{
	"id", "facility_id", "name", "municipality",
	"name_en", "name_it", "name_de", "standard_name",
	"netex_type", "netex_vehicletypes", "netex_layout",
	"netex_hazard_prohibited", "netex_charging", "netex_surveillance", "netex_reservation",
	"lat", "lon",
}

// pointprojection is a PostGIS geometry column (stored as EWKB).
// We wrap it in ST_AsText to get a parseable "POINT (lon lat)" string,
// fully-qualified with the `public` schema because PostGIS lives there
// and our search_path is set to `intimev2`.
const query = `
SELECT s.name, public.ST_AsText(s.pointprojection), mt.json
FROM station s
JOIN metadata mt ON mt.id = s.meta_data_id
WHERE s.origin = 'fuchtstechnik'
`

// stationMetadata captures the fields we know how to extract from the
// metadata JSON blob. Only fields the transformer cares about are
// listed; anything extra in the JSON is ignored.
type stationMetadata struct {
	ProviderID   string       `json:"provider_id"`
	NameDe       string       `json:"name_de"`
	NameEn       string       `json:"name_en"`
	NameIt       string       `json:"name_it"`
	StandardName string       `json:"standard_name"`
	Municipality string       `json:"municipality"`
	NetexParking netexParking `json:"netex_parking"`
}

type netexParking struct {
	Type             string `json:"type"`
	VehicleTypes     string `json:"vehicletypes"`
	Layout           string `json:"layout"`
	HazardProhibited *bool  `json:"hazard_prohibited"`
	Charging         *bool  `json:"charging"`
	Surveillance     *bool  `json:"surveillance"`
	Reservation      string `json:"reservation"`
}

func main() {
	dbDSN := flag.String("db", "", "Postgres connection string (e.g. postgres://user:pass@host:5432/bdp?sslmode=disable)")
	stationsCSV := flag.String("stations-csv", defaultStationsCSV, "path to transformer stations.csv")
	dryRun := flag.Bool("dry-run", false, "print intended changes; do not write the CSV")
	flag.Parse()

	if *dbDSN == "" {
		fatalf("--db is required")
	}

	db, err := sql.Open("postgres", *dbDSN)
	if err != nil {
		fatalf("open db: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		fatalf("ping db: %v", err)
	}

	rows, err := db.Query(query)
	if err != nil {
		fatalf("query: %v", err)
	}
	defer rows.Close()

	// Read existing CSV, build a set of existing ids so we can preserve them.
	existingRows, err := readCSV(*stationsCSV, stationsHeader)
	if err != nil {
		fatalf("read stations.csv: %v", err)
	}
	existingIDs := map[string]bool{}
	for _, row := range existingRows {
		existingIDs[row["id"]] = true
	}

	type discovered struct {
		row  map[string]string
		name string // for logging
	}
	var newRows []discovered

	count := 0
	for rows.Next() {
		count++
		var name, pointProj, jsonBlob string
		if err := rows.Scan(&name, &pointProj, &jsonBlob); err != nil {
			fatalf("scan row: %v", err)
		}

		var meta stationMetadata
		if err := json.Unmarshal([]byte(jsonBlob), &meta); err != nil {
			fmt.Printf("  warning: skipping %s — failed to parse metadata json: %v\n", name, err)
			continue
		}
		if meta.ProviderID == "" {
			fmt.Printf("  warning: skipping %s — metadata json has no provider_id\n", name)
			continue
		}
		if existingIDs[meta.ProviderID] {
			continue
		}

		lat, lon, ok := parsePoint(pointProj)
		if !ok {
			fmt.Printf("  warning: %s — could not parse pointprojection %q (using 0,0)\n", name, pointProj)
		}

		row := emptyRow(stationsHeader)
		row["id"] = meta.ProviderID
		row["facility_id"] = meta.ProviderID
		row["name"] = name
		row["name_de"] = meta.NameDe
		row["name_en"] = meta.NameEn
		row["name_it"] = meta.NameIt
		row["standard_name"] = meta.StandardName
		row["municipality"] = meta.Municipality
		row["netex_type"] = meta.NetexParking.Type
		row["netex_vehicletypes"] = meta.NetexParking.VehicleTypes
		row["netex_layout"] = meta.NetexParking.Layout
		row["netex_hazard_prohibited"] = boolStr(meta.NetexParking.HazardProhibited)
		row["netex_charging"] = boolStr(meta.NetexParking.Charging)
		row["netex_surveillance"] = boolStr(meta.NetexParking.Surveillance)
		row["netex_reservation"] = meta.NetexParking.Reservation
		row["lat"] = floatStr(lat)
		row["lon"] = floatStr(lon)

		newRows = append(newRows, discovered{row: row, name: name})
		existingIDs[meta.ProviderID] = true
	}
	if err := rows.Err(); err != nil {
		fatalf("iterate rows: %v", err)
	}

	fmt.Printf("=== fuchtstechnik stations: %d in db, %d new in stations.csv ===\n", count, len(newRows))
	if len(newRows) == 0 {
		fmt.Println("  (no new rows)")
		return
	}
	for _, d := range newRows {
		fmt.Printf("  + %s (provider_id=%s)\n", d.name, d.row["id"])
	}

	if *dryRun {
		fmt.Println("\n--dry-run: not writing files")
		return
	}

	merged := append([]map[string]string{}, existingRows...)
	for _, d := range newRows {
		merged = append(merged, d.row)
	}
	sort.SliceStable(merged, func(i, j int) bool {
		return merged[i]["id"] < merged[j]["id"]
	})

	if err := writeCSV(*stationsCSV, stationsHeader, merged); err != nil {
		fatalf("write stations.csv: %v", err)
	}
	fmt.Printf("\nWrote %s (%d rows)\n", *stationsCSV, len(merged))
}

// parsePoint extracts (lat, lon) from a WKT POINT string like
// "POINT (11.7691835 46.5347464)". Returns ok=false if the format
// doesn't match.
var pointRe = regexp.MustCompile(`(?i)POINT\s*\(\s*(-?\d+(?:\.\d+)?)\s+(-?\d+(?:\.\d+)?)\s*\)`)

func parsePoint(s string) (lat, lon float64, ok bool) {
	m := pointRe.FindStringSubmatch(s)
	if m == nil {
		return 0, 0, false
	}
	// WKT POINT is (longitude latitude).
	lonV, err1 := strconv.ParseFloat(m[1], 64)
	latV, err2 := strconv.ParseFloat(m[2], 64)
	if err1 != nil || err2 != nil {
		return 0, 0, false
	}
	return latV, lonV, true
}

func boolStr(b *bool) string {
	if b == nil {
		return ""
	}
	if *b {
		return "true"
	}
	return "false"
}

func floatStr(f float64) string {
	if f == 0 {
		return "0"
	}
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func readCSV(path string, expectedHeader []string) ([]map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if len(records) == 0 {
		return nil, nil
	}
	header := records[0]
	if !sameHeader(header, expectedHeader) {
		return nil, fmt.Errorf("%s: unexpected header %v (want %v)", path, header, expectedHeader)
	}
	rows := make([]map[string]string, 0, len(records)-1)
	for _, rec := range records[1:] {
		row := map[string]string{}
		for i, col := range header {
			if i < len(rec) {
				row[col] = rec[i]
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func writeCSV(path string, header []string, rows []map[string]string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	w := csv.NewWriter(f)
	if err := w.Write(header); err != nil {
		return err
	}
	for _, row := range rows {
		rec := make([]string, len(header))
		for i, col := range header {
			rec[i] = row[col]
		}
		if err := w.Write(rec); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}

func sameHeader(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimSpace(a[i]) != b[i] {
			return false
		}
	}
	return true
}

func emptyRow(header []string) map[string]string {
	row := make(map[string]string, len(header))
	for _, c := range header {
		row[c] = ""
	}
	return row
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// sync-stations is an offline maintenance tool: it iterates over every
// credential in credentials.json, calls Skidata's getCountingCategories
// endpoint, and reports which credentials work. It then upserts two CSVs
// in the parking-skidata transformer's resources directory:
//
//   - stations.csv: append-only. Adds missing parent (facility) and child
//     (facility_carpark) rows with placeholder NeTEx/coords/names.
//     Existing rows are never modified to preserve curated metadata.
//
//   - counting_categories.csv: full upsert. Inserts new rows and updates
//     capacity/limits/name on existing ones. Rows for facilities that
//     failed this run are kept as-is.
//
// Usage:
//
//	go run ./cmd/sync-stations \
//	  --credentials=../credentials.json \
//	  --base-url=https://car.webhost.skidata.com \
//	  --stations-csv=../../../transformers/parking-skidata/resources/stations.csv \
//	  --categories-csv=../../../transformers/parking-skidata/resources/counting_categories.csv
package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"opendatahub.com/rest-push-skidata/skidata"
)

const (
	defaultBaseURL       = "https://car.webhost.skidata.com"
	defaultCredentials   = "../credentials.json"
	defaultStationsCSV   = "../../../transformers/parking-skidata/resources/stations.csv"
	defaultCategoriesCSV = "../../../transformers/parking-skidata/resources/counting_categories.csv"
)

// stationsHeader matches the existing stations.csv layout. Adding a new
// column would require updating both this list and the transformer's
// Station struct in transformers/parking-skidata/src/station.go.
var stationsHeader = []string{
	"id", "station_type", "parent_id", "carpark_id",
	"name", "municipality",
	"name_en", "name_it", "name_de", "standard_name",
	"netex_type", "netex_vehicletypes", "netex_layout",
	"netex_hazard_prohibited", "netex_charging", "netex_surveillance", "netex_reservation",
	"lat", "lon",
}

// BDP station_type values written by the sync script. Must stay in sync
// with the constants in transformers/parking-skidata/src/main.go.
const (
	stationTypeParkingFacility = "ParkingFacility"
	stationTypeParkingStation  = "ParkingStation"
)

var categoriesHeader = []string{
	"facility_id", "carpark_id", "counting_category_id",
	"name", "capacity", "occupancy_limit", "free_limit",
}

func main() {
	credentialsPath := flag.String("credentials", defaultCredentials, "path to credentials.json")
	baseURL := flag.String("base-url", envOr("SKIDATA_BASE_URL", defaultBaseURL), "Skidata Dynamic Data API base URL")
	stationsCSV := flag.String("stations-csv", defaultStationsCSV, "path to transformer stations.csv")
	categoriesCSV := flag.String("categories-csv", defaultCategoriesCSV, "path to transformer counting_categories.csv")
	dryRun := flag.Bool("dry-run", false, "print intended changes; do not write CSVs")
	flag.Parse()

	creds, err := loadCredentials(*credentialsPath)
	if err != nil {
		fatalf("failed to load credentials: %v", err)
	}
	fmt.Printf("Loaded %d credentials from %s\n", len(creds), *credentialsPath)
	fmt.Printf("Base URL: %s\n\n", *baseURL)

	client := skidata.NewHTTPClient()

	type result struct {
		cred       skidata.FacilityCredential
		categories []skidata.CountingCategory
		err        error
	}
	results := make([]result, 0, len(creds))
	working := 0
	for _, cred := range creds {
		cats, err := skidata.GetCountingCategories(client, *baseURL, cred)
		results = append(results, result{cred, cats, err})
		if err == nil {
			working++
		}
	}

	// Print report
	fmt.Println("=== Credential report ===")
	for _, r := range results {
		if r.err == nil {
			fmt.Printf("  OK   %s — %d categories across %d carparks\n", r.cred.Facility, len(r.categories), countCarparks(r.categories))
		} else {
			fmt.Printf("  FAIL %s — %v\n", r.cred.Facility, r.err)
		}
	}
	fmt.Printf("\n%d/%d credentials working\n\n", working, len(creds))

	// Collect the data harvested from the working credentials only.
	type carparkKey struct {
		facility string
		carpark  int
	}
	seenFacilities := map[string]bool{}
	seenCarparks := map[carparkKey]bool{}
	freshCategories := []skidata.CountingCategory{}
	categoryOwner := map[int]*skidata.FacilityCredential{} // index in freshCategories -> cred

	for i := range results {
		r := &results[i]
		if r.err != nil {
			continue
		}
		seenFacilities[r.cred.Facility] = true
		for _, cat := range r.categories {
			seenCarparks[carparkKey{r.cred.Facility, cat.CarparkId}] = true
			freshCategories = append(freshCategories, cat)
			categoryOwner[len(freshCategories)-1] = &r.cred
		}
	}

	// Upsert stations.csv (append-only).
	stationRows, err := readCSV(*stationsCSV, stationsHeader)
	if err != nil {
		fatalf("failed to read stations.csv: %v", err)
	}
	existingStationIDs := map[string]bool{}
	for _, row := range stationRows {
		existingStationIDs[row["id"]] = true
	}
	newStationRows := []map[string]string{}
	// addStation appends a new row to stations.csv. parentID/carparkID
	// are written only for ParkingStation rows (left blank for facility
	// rows, where they are not meaningful).
	addStation := func(id, stationType, parentID, carparkID string) {
		if existingStationIDs[id] {
			return
		}
		row := emptyRow(stationsHeader)
		row["id"] = id
		row["station_type"] = stationType
		row["parent_id"] = parentID
		row["carpark_id"] = carparkID
		// "name" and "standard_name" are left blank on purpose: they are
		// curated manually in the CSV and the sync script must never
		// overwrite or seed them.
		row["lat"] = "0"
		row["lon"] = "0"
		newStationRows = append(newStationRows, row)
		existingStationIDs[id] = true
		fmt.Printf("  + %s %s\n", stationType, id)
	}

	fmt.Println("=== stations.csv changes ===")
	// Sort facilities for deterministic output.
	facilities := make([]string, 0, len(seenFacilities))
	for f := range seenFacilities {
		facilities = append(facilities, f)
	}
	sort.Strings(facilities)
	for _, fac := range facilities {
		addStation(fac, stationTypeParkingFacility, "", "")
	}
	carparks := make([]carparkKey, 0, len(seenCarparks))
	for k := range seenCarparks {
		carparks = append(carparks, k)
	}
	sort.Slice(carparks, func(i, j int) bool {
		if carparks[i].facility != carparks[j].facility {
			return carparks[i].facility < carparks[j].facility
		}
		return carparks[i].carpark < carparks[j].carpark
	})
	for _, k := range carparks {
		id := fmt.Sprintf("%s_%d", k.facility, k.carpark)
		addStation(id, stationTypeParkingStation, k.facility, strconv.Itoa(k.carpark))
	}
	if len(newStationRows) == 0 {
		fmt.Println("  (no new rows)")
	}
	stationRows = append(stationRows, newStationRows...)
	sort.SliceStable(stationRows, func(i, j int) bool {
		return stationRows[i]["id"] < stationRows[j]["id"]
	})

	// Upsert counting_categories.csv (full upsert).
	categoryRows, err := readCSV(*categoriesCSV, categoriesHeader)
	if err != nil {
		fatalf("failed to read counting_categories.csv: %v", err)
	}
	categoryIndex := map[parsedCatKey]int{} // -> index in categoryRows
	for i, row := range categoryRows {
		k, perr := parseCategoryKey(row)
		if perr != nil {
			fmt.Printf("  warning: skipping malformed category row %d: %v\n", i, perr)
			continue
		}
		categoryIndex[k] = i
	}

	fmt.Println("\n=== counting_categories.csv changes ===")
	added, updated := 0, 0
	for i, cat := range freshCategories {
		owner := categoryOwner[i]
		k := parsedCatKey{owner.Facility, cat.CarparkId, cat.CountingCategoryId}
		desired := map[string]string{
			"facility_id":          owner.Facility,
			"carpark_id":           strconv.Itoa(cat.CarparkId),
			"counting_category_id": strconv.Itoa(cat.CountingCategoryId),
			"name":                 cat.Name,
			"capacity":             strconv.Itoa(cat.Capacity),
			"occupancy_limit":      strconv.Itoa(cat.OccupancyLimit),
			"free_limit":           strconv.Itoa(cat.FreeLimit),
		}
		if idx, ok := categoryIndex[k]; ok {
			if !rowsEqual(categoryRows[idx], desired, categoriesHeader) {
				categoryRows[idx] = desired
				updated++
				fmt.Printf("  ~ %s carpark=%d cat=%d (%s) cap=%d\n", owner.Facility, cat.CarparkId, cat.CountingCategoryId, cat.Name, cat.Capacity)
			}
		} else {
			categoryRows = append(categoryRows, desired)
			categoryIndex[k] = len(categoryRows) - 1
			added++
			fmt.Printf("  + %s carpark=%d cat=%d (%s) cap=%d\n", owner.Facility, cat.CarparkId, cat.CountingCategoryId, cat.Name, cat.Capacity)
		}
	}
	if added == 0 && updated == 0 {
		fmt.Println("  (no changes)")
	}
	sort.SliceStable(categoryRows, func(i, j int) bool {
		ki, _ := parseCategoryKey(categoryRows[i])
		kj, _ := parseCategoryKey(categoryRows[j])
		if ki.facility != kj.facility {
			return ki.facility < kj.facility
		}
		if ki.carpark != kj.carpark {
			return ki.carpark < kj.carpark
		}
		return ki.category < kj.category
	})

	if *dryRun {
		fmt.Println("\n--dry-run: not writing files")
	} else {
		if len(newStationRows) > 0 {
			if err := writeCSV(*stationsCSV, stationsHeader, stationRows); err != nil {
				fatalf("failed to write stations.csv: %v", err)
			}
			fmt.Printf("\nWrote %s\n", *stationsCSV)
		}
		if added > 0 || updated > 0 {
			if err := writeCSV(*categoriesCSV, categoriesHeader, categoryRows); err != nil {
				fatalf("failed to write counting_categories.csv: %v", err)
			}
			fmt.Printf("Wrote %s\n", *categoriesCSV)
		}
	}

	// Non-zero exit if any credential failed.
	if working != len(creds) {
		os.Exit(2)
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadCredentials(path string) ([]skidata.FacilityCredential, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return skidata.ParseCredentials(raw)
}

func countCarparks(cats []skidata.CountingCategory) int {
	seen := map[int]bool{}
	for _, c := range cats {
		seen[c.CarparkId] = true
	}
	return len(seen)
}

// readCSV reads a CSV file into a slice of column→value maps.
// Returns an empty slice if the file does not exist (so we can bootstrap
// a brand-new CSV). Validates that the header matches expectedHeader.
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

func rowsEqual(a, b map[string]string, header []string) bool {
	for _, c := range header {
		if a[c] != b[c] {
			return false
		}
	}
	return true
}

type parsedCatKey struct {
	facility string
	carpark  int
	category int
}

func parseCategoryKey(row map[string]string) (parsedCatKey, error) {
	carpark, err := strconv.Atoi(row["carpark_id"])
	if err != nil {
		return parsedCatKey{}, fmt.Errorf("carpark_id=%q: %w", row["carpark_id"], err)
	}
	category, err := strconv.Atoi(row["counting_category_id"])
	if err != nil {
		return parsedCatKey{}, fmt.Errorf("counting_category_id=%q: %w", row["counting_category_id"], err)
	}
	return parsedCatKey{row["facility_id"], carpark, category}, nil
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "error: "+format+"\n", args...)
	os.Exit(1)
}

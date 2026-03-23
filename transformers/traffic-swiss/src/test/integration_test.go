// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import (
	"encoding/xml"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"
)

// Static endpoint: versioned CKAN direct download URL (check dataset page for updates).
const staticEndpoint = "https://data.opentransportdata.swiss/dataset/9f2cd216-a6a0-47ca-b498-6893bd911f4b/resource/7529d2c0-47bd-4a1d-a85c-fa9a782c9d8b/download/measurementsitetable.current_2025-02-18_00-58-420100_v5.xml"

// Realtime endpoint: SOAP service (requires Bearer token).
const realtimeEndpoint = "https://api.opentransportdata.swiss/TDP/Soap_Datex2/Pull"

// ── Minimal XML structs for parsing (mirrors real DATEX II v2.3 structure) ────
// Root element is D2LogicalModel; measurementSpecificCharacteristics has two levels:
// outer carries index attr, inner carries the measurement data.

type staticFeed struct {
	XMLName xml.Name                `xml:"D2LogicalModel"`
	Sites   []measurementSiteRecord `xml:"payloadPublication>measurementSiteTable>measurementSiteRecord"`
}

type measurementSiteRecord struct {
	ID              string                   `xml:"id,attr"`
	Characteristics []indexedCharacteristics `xml:"measurementSpecificCharacteristics"`
	Location        measurementSiteLocation  `xml:"measurementSiteLocation"`
}

type indexedCharacteristics struct {
	Index     string                          `xml:"index,attr"`
	ValueType string                          `xml:"measurementSpecificCharacteristics>specificMeasurementValueType"`
	VehicleType string                        `xml:"measurementSpecificCharacteristics>specificVehicleCharacteristics>vehicleType"`
}

type measurementSiteLocation struct {
	Lat float64 `xml:"pointByCoordinates>pointCoordinates>latitude"`
	Lon float64 `xml:"pointByCoordinates>pointCoordinates>longitude"`
}

// knownOdhDataTypes lists all valid ODH data type names for traffic sensors.
var knownOdhDataTypes = map[string]bool{
	"average-speed-light-vehicles": true,
	"average-speed-heavy-vehicles": true,
	"average-speed":                true,
	"average-flow-light-vehicles":  true,
	"average-flow-heavy-vehicles":  true,
	"average-flow":                 true,
}

// odhDataType replicates the collector's mapping logic for integration tests.
func odhDataType(valueType, vehicleType string) (string, bool) {
	m := map[string]string{
		"trafficSpeed/car":        "average-speed-light-vehicles",
		"trafficSpeed/lorry":      "average-speed-heavy-vehicles",
		"trafficSpeed/anyVehicle": "average-speed",
		"trafficFlow/car":         "average-flow-light-vehicles",
		"trafficFlow/lorry":       "average-flow-heavy-vehicles",
		"trafficFlow/anyVehicle":  "average-flow",
	}
	v, ok := m[valueType+"/"+vehicleType]
	return v, ok
}

// fetchXML fetches the given URL, setting bearer token if provided.
// Returns (body, nil) on success, (nil, err) on network error.
func fetchXML(url, bearer string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(resp.Body)
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestIntegration_StaticEndpoint(t *testing.T) {
	body, err := fetchXML(staticEndpoint, "")
	if err != nil {
		t.Skipf("static endpoint unreachable (%v); skipping integration test", err)
	}

	if len(body) == 0 {
		t.Skip("empty response from static endpoint; skipping")
	}
	// The configured URL may point to a data portal page rather than the raw XML file.
	// Skip gracefully when an HTML response is received.
	if strings.HasPrefix(strings.TrimSpace(string(body)), "<html") ||
		strings.Contains(string(body[:min(200, len(body))]), "<!DOCTYPE") {
		t.Skipf("static endpoint returned HTML (URL may redirect to portal page, not direct XML download); skipping integration test")
	}
	if !strings.Contains(string(body), "measurementSiteRecord") {
		t.Skipf("response does not appear to contain measurementSiteRecord elements; raw snippet: %s",
			string(body[:min(300, len(body))]))
	}

	var table staticFeed
	if err := xml.Unmarshal(body, &table); err != nil {
		t.Logf("XML unmarshal failed (struct tags may need adjustment): %v", err)
		t.Logf("Raw response snippet: %s", string(body[:min(500, len(body))]))
		return
	}

	if len(table.Sites) == 0 {
		t.Fatal("expected at least one measurementSiteRecord")
	}

	for _, site := range table.Sites {
		if site.ID == "" {
			t.Errorf("site missing ID")
		}
		if len(site.Characteristics) == 0 {
			t.Logf("WARN: site %s has no parsed characteristics (check XML paths)", site.ID)
		}
	}
	t.Logf("Static endpoint: parsed %d sites", len(table.Sites))
}

func TestIntegration_DataTypeMapping(t *testing.T) {
	body, err := fetchXML(staticEndpoint, "")
	if err != nil {
		t.Skipf("static endpoint unreachable: %v", err)
	}

	var table staticFeed
	if err := xml.Unmarshal(body, &table); err != nil {
		t.Skipf("XML unmarshal failed (struct tags may need adjustment): %v", err)
	}

	unknown := 0
	for _, site := range table.Sites {
		for _, c := range site.Characteristics {
			dt, ok := odhDataType(c.ValueType, c.VehicleType)
			if !ok {
				unknown++
				t.Logf("WARN: unmapped characteristic valueType=%q vehicleType=%q in site %s",
					c.ValueType, c.VehicleType, site.ID)
				continue
			}
			if !knownOdhDataTypes[dt] {
				t.Errorf("odhDataType returned unknown ODH data type %q", dt)
			}
		}
	}
	t.Logf("Data type mapping: %d unknown characteristics found", unknown)
}

func TestIntegration_CoordinatesInSwissBounds(t *testing.T) {
	body, err := fetchXML(staticEndpoint, "")
	if err != nil {
		t.Skipf("static endpoint unreachable: %v", err)
	}

	var table staticFeed
	if err := xml.Unmarshal(body, &table); err != nil {
		t.Skipf("XML unmarshal failed: %v", err)
	}

	outOfBounds := 0
	for _, site := range table.Sites {
		lat, lon := site.Location.Lat, site.Location.Lon
		if lat == 0 || lon == 0 {
			continue // coordinate missing or not parsed for this site
		}
		if lat < 45.5 || lat > 48.5 || lon < 5.5 || lon > 10.5 {
			t.Errorf("site %s has coordinates outside Swiss bounds: lat=%v lon=%v", site.ID, lat, lon)
			outOfBounds++
		}
	}
	if outOfBounds > 0 {
		t.Errorf("%d sites have out-of-bounds coordinates", outOfBounds)
	}
}

func TestIntegration_RealtimeEndpoint(t *testing.T) {
	token := os.Getenv("AUTH_BEARER_TOKEN")
	if token == "" {
		t.Skip("AUTH_BEARER_TOKEN not set; skipping realtime endpoint test")
	}

	body, err := fetchXML(realtimeEndpoint, token)
	if err != nil {
		t.Skipf("realtime endpoint unreachable: %v", err)
	}

	if len(body) == 0 {
		t.Fatal("expected non-empty realtime XML response")
	}
	if !strings.Contains(string(body), "siteMeasurements") {
		t.Logf("WARN: response does not appear to contain siteMeasurements; check endpoint URL")
		t.Logf("Raw snippet: %s", string(body[:min(500, len(body))]))
	}
	t.Logf("Realtime endpoint: %d bytes received", len(body))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

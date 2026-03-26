// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package test

import (
	"encoding/xml"
	"fmt"
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

const realtimeSoapAction = "http://opentransportdata.swiss/TDP/Soap_Datex2/Pull/v1/pullMeasuredData"

const realtimeSoapBody = `<?xml version="1.0" encoding="UTF-8"?>
<SOAP-ENV:Envelope xmlns:SOAP-ENV="http://schemas.xmlsoap.org/soap/envelope/" xmlns:dx223="http://datex2.eu/schema/2/2_0">
  <SOAP-ENV:Body>
    <dx223:d2LogicalModel modelBaseVersion="2">
      <dx223:exchange>
        <dx223:supplierIdentification>
          <dx223:country>ch</dx223:country>
          <dx223:nationalIdentifier>OTD</dx223:nationalIdentifier>
        </dx223:supplierIdentification>
      </dx223:exchange>
    </dx223:d2LogicalModel>
  </SOAP-ENV:Body>
</SOAP-ENV:Envelope>`

// ── Minimal XML structs for parsing (mirrors real DATEX II v2.3 structure) ────

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
	Index       string `xml:"index,attr"`
	ValueType   string `xml:"measurementSpecificCharacteristics>specificMeasurementValueType"`
	VehicleType string `xml:"measurementSpecificCharacteristics>specificVehicleCharacteristics>vehicleType"`
}

type measurementSiteLocation struct {
	Lat float64 `xml:"pointByCoordinates>pointCoordinates>latitude"`
	Lon float64 `xml:"pointByCoordinates>pointCoordinates>longitude"`
}

// SOAP/realtime XML structs
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	Inner []byte `xml:",innerxml"`
}

type realtimeFeed struct {
	SiteMeasurements []siteMeasurement `xml:"payloadPublication>siteMeasurements"`
}

type siteMeasurement struct {
	SiteRef     string          `xml:"measurementSiteReference>id,attr"`
	TimeDefault string          `xml:"measurementTimeDefault"`
	Values      []measuredValue `xml:"measuredValue"`
}

type measuredValue struct {
	Index           string  `xml:"index,attr"`
	VehicleFlowRate float64 `xml:"measuredValue>basicData>vehicleFlow>vehicleFlowRate"`
	SpeedValue      float64 `xml:"measuredValue>basicData>averageVehicleSpeed>speed"`
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

// fetchXML fetches a URL via GET, setting bearer token if provided.
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
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(body[:min(200, len(body))]))
	}
	return io.ReadAll(resp.Body)
}

// fetchSOAP sends a SOAP 1.1 POST request and returns the raw response body.
func fetchSOAP(endpoint, soapAction, body, bearer string) ([]byte, error) {
	req, err := http.NewRequest(http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/xml; charset=utf-8")
	req.Header.Set("SOAPAction", soapAction)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, string(respBody[:min(200, len(respBody))]))
	}
	return io.ReadAll(resp.Body)
}

// fetchStaticFeed fetches and parses the static DATEX II XML feed.
// Returns (feed, nil) on success, or skips the test on network/parse failure.
func fetchStaticFeed(t *testing.T) *staticFeed {
	t.Helper()
	body, err := fetchXML(staticEndpoint, "")
	if err != nil {
		t.Skipf("static endpoint unreachable (%v); skipping integration test", err)
	}
	if len(body) == 0 {
		t.Skip("empty response from static endpoint; skipping")
	}
	snippet := string(body[:min(300, len(body))])
	if strings.HasPrefix(strings.TrimSpace(snippet), "<html") || strings.Contains(snippet, "<!DOCTYPE") {
		t.Skipf("static endpoint returned HTML (URL may redirect to portal page, not direct XML download)")
	}
	var feed staticFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		t.Fatalf("XML unmarshal failed: %v\nRaw snippet: %s", err, snippet)
	}
	return &feed
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestIntegration_StaticEndpoint(t *testing.T) {
	t.Log("*** Starting traffic-swiss integration test (static feed) ***")

	feed := fetchStaticFeed(t)

	if len(feed.Sites) < 100 {
		t.Fatalf("expected at least 100 measurement sites, got %d", len(feed.Sites))
	}
	t.Logf("Static endpoint: parsed %d sites", len(feed.Sites))

	t.Run("ValidateSiteIDs", func(t *testing.T) {
		prefixes := map[string]int{}
		for _, site := range feed.Sites {
			if site.ID == "" {
				t.Error("found site with empty ID")
				continue
			}
			if !strings.Contains(site.ID, "CH:") {
				t.Errorf("site ID %q does not contain 'CH:'", site.ID)
			}
			// Track ID prefix patterns for informational purposes
			if idx := strings.LastIndex(site.ID, "CH:"); idx >= 0 {
				prefix := site.ID[:idx+3] // e.g. "CH:", "ZH.CH:", "FH.FR.CH:"
				prefixes[prefix]++
			}
		}
		for prefix, count := range prefixes {
			t.Logf("  ID prefix %q: %d sites", prefix, count)
		}
		t.Logf("✓ All %d site IDs are non-empty and contain 'CH:'", len(feed.Sites))
	})

	t.Run("ValidateCoordinatesInSwissBounds", func(t *testing.T) {
		missing, outOfBounds := 0, 0
		for _, site := range feed.Sites {
			lat, lon := site.Location.Lat, site.Location.Lon
			// Treat zero lat or lon as missing (zero-value default, not a real coordinate)
			if lat == 0 || lon == 0 {
				missing++
				continue
			}
			if lat < 45.5 || lat > 48.5 || lon < 5.5 || lon > 10.5 {
				t.Errorf("site %s: coordinates outside Swiss bounds (lat=%v, lon=%v)", site.ID, lat, lon)
				outOfBounds++
			}
		}
		if missing > len(feed.Sites)/2 {
			t.Errorf("more than half of sites have missing coordinates (%d/%d)", missing, len(feed.Sites))
		}
		t.Logf("✓ Coordinates: %d valid, %d missing, %d out of bounds", len(feed.Sites)-missing-outOfBounds, missing, outOfBounds)
	})

	t.Run("ValidateCharacteristics", func(t *testing.T) {
		withChars := 0
		for _, site := range feed.Sites {
			if len(site.Characteristics) > 0 {
				withChars++
			}
		}
		if withChars == 0 {
			t.Error("no sites have parsed characteristics — check XML path tags")
		}
		t.Logf("✓ %d/%d sites have at least one characteristic", withChars, len(feed.Sites))
	})

	t.Log("*** Static feed integration test passed ***")
}

func TestIntegration_DataTypeMapping(t *testing.T) {
	feed := fetchStaticFeed(t)

	unknown, mapped := 0, 0
	unmappedSet := map[string]struct{}{}

	for _, site := range feed.Sites {
		for _, c := range site.Characteristics {
			dt, ok := odhDataType(c.ValueType, c.VehicleType)
			if !ok {
				unknown++
				key := fmt.Sprintf("valueType=%q vehicleType=%q", c.ValueType, c.VehicleType)
				unmappedSet[key] = struct{}{}
				continue
			}
			if !knownOdhDataTypes[dt] {
				t.Errorf("odhDataType returned unknown ODH data type %q", dt)
			}
			mapped++
		}
	}

	if mapped == 0 {
		t.Error("no characteristics were mapped to ODH data types — check odhDataType mapping")
	}
	if len(unmappedSet) > 0 {
		t.Logf("WARN: %d unknown (valueType, vehicleType) combinations found:", len(unmappedSet))
		for k := range unmappedSet {
			t.Logf("  - %s", k)
		}
	}
	t.Logf("✓ Data type mapping: %d mapped, %d unknown characteristics", mapped, unknown)
}

func TestIntegration_RealtimeEndpoint(t *testing.T) {
	token := os.Getenv("AUTH_BEARER_TOKEN")
	if token == "" {
		t.Skip("AUTH_BEARER_TOKEN not set; skipping realtime endpoint test")
	}

	t.Log("*** Starting traffic-swiss integration test (realtime SOAP feed) ***")

	rawBody, err := fetchSOAP(realtimeEndpoint, realtimeSoapAction, realtimeSoapBody, token)
	if err != nil {
		t.Skipf("realtime endpoint unreachable: %v", err)
	}

	if len(rawBody) == 0 {
		t.Fatal("expected non-empty realtime SOAP response")
	}

	// Parse the SOAP envelope to extract the inner XML body
	var env soapEnvelope
	if err := xml.Unmarshal(rawBody, &env); err != nil {
		t.Fatalf("SOAP envelope parse failed: %v\nRaw snippet: %s", err, string(rawBody[:min(500, len(rawBody))]))
	}

	var feed realtimeFeed
	if err := xml.Unmarshal(env.Body.Inner, &feed); err != nil {
		t.Fatalf("realtime payload parse failed: %v", err)
	}

	t.Logf("Realtime endpoint: %d siteMeasurements parsed", len(feed.SiteMeasurements))

	if len(feed.SiteMeasurements) == 0 {
		t.Fatal("expected at least one siteMeasurement in realtime response")
	}

	t.Run("ValidateSiteReferences", func(t *testing.T) {
		for _, sm := range feed.SiteMeasurements {
			if sm.SiteRef == "" {
				t.Error("siteMeasurement has empty siteRef")
			}
			if !strings.HasPrefix(sm.SiteRef, "CH:") {
				t.Errorf("siteRef %q does not start with 'CH:'", sm.SiteRef)
			}
		}
		t.Logf("✓ All site references are non-empty and start with 'CH:'")
	})

	t.Run("ValidateTimestamps", func(t *testing.T) {
		now := time.Now().UTC()
		stale := 0
		for _, sm := range feed.SiteMeasurements {
			if sm.TimeDefault == "" {
				t.Errorf("siteMeasurement %s has empty timestamp", sm.SiteRef)
				continue
			}
			ts, err := time.Parse(time.RFC3339Nano, sm.TimeDefault)
			if err != nil {
				ts, err = time.Parse(time.RFC3339, sm.TimeDefault)
			}
			if err != nil {
				t.Errorf("siteMeasurement %s: unparseable timestamp %q: %v", sm.SiteRef, sm.TimeDefault, err)
				continue
			}
			// Flag timestamps older than 2 hours as stale
			if now.Sub(ts) > 2*time.Hour {
				stale++
			}
		}
		if stale > len(feed.SiteMeasurements)/2 {
			t.Errorf("more than half of siteMeasurements have stale timestamps (>2h old): %d/%d", stale, len(feed.SiteMeasurements))
		}
		t.Logf("✓ Timestamps: %d recent, %d stale (>2h)", len(feed.SiteMeasurements)-stale, stale)
	})

	t.Run("ValidateMeasurementValues", func(t *testing.T) {
		totalValues, negativeValues := 0, 0
		for _, sm := range feed.SiteMeasurements {
			for _, mv := range sm.Values {
				totalValues++
				if mv.VehicleFlowRate < 0 || mv.SpeedValue < 0 {
					negativeValues++
					t.Errorf("siteMeasurement %s index %s: negative value (flow=%v, speed=%v)",
						sm.SiteRef, mv.Index, mv.VehicleFlowRate, mv.SpeedValue)
				}
			}
		}
		t.Logf("✓ Measurement values: %d total, %d negative", totalValues, negativeValues)
	})

	t.Log("*** Realtime feed integration test passed ***")
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

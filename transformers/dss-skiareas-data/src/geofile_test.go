// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"testing"
	"time"

	odhmodel "opendatahub.com/tr-dss-skiareas/odhmodel"
)

// stubGeoFiles serves every regionMap from a local GPX file, without delay.
func stubGeoFiles(path string) *geoFileFetcher {
	return &geoFileFetcher{
		download: func(ctx context.Context, url string) ([]byte, error) { return os.ReadFile(path) },
		ttl:      1 << 62,
	}
}

func readLines(t *testing.T, path string) [][]geoPoint {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines, err := parseGPXLines(data)
	if err != nil {
		t.Fatal(err)
	}
	return lines
}

func Test_linesToWKT(t *testing.T) {
	wkt := linesToWKT(readLines(t, "testdata/regionmap.gpx"))
	if !strings.HasPrefix(wkt, "LINESTRING (11.6398533 46.6099615, 11.6284056 46.6125558,") ||
		!strings.HasSuffix(wkt, ", 11.6399391 46.6099026)") || strings.Count(wkt, ",") != 35 {
		t.Errorf("unexpected LINESTRING: %.120s...", wkt)
	}

	wkt = linesToWKT(readLines(t, "testdata/regionmap_multi.gpx"))
	if !strings.HasPrefix(wkt, "MULTILINESTRING ((11.635508537292482 46.30447470831477,") ||
		!strings.HasSuffix(wkt, ", 11.63547570195979 46.3045359181646))") || strings.Count(wkt, "), (") != 1 {
		t.Errorf("unexpected MULTILINESTRING: %.120s...", wkt)
	}

	if _, err := parseGPXLines([]byte("<html><title>Attention Required! | Cloudflare</title></html>")); err == nil {
		t.Error("expected error for a page without a track")
	}
}

func Test_centroid(t *testing.T) {
	// Unit square (open ring, as in the GPX outlines): centroid is its middle.
	square := [][]geoPoint{{{Lat: 0, Lon: 0}, {Lat: 0, Lon: 1}, {Lat: 1, Lon: 1}, {Lat: 1, Lon: 0}}}
	if c := centroid(square); c != (geoPoint{Lat: 0.5, Lon: 0.5}) {
		t.Errorf("square centroid = %+v", c)
	}

	// Seiser Alm outline: centroid of the real regionMap.
	c := centroid(readLines(t, "testdata/regionmap.gpx"))
	if math.Abs(c.Lat-46.54532) > 1e-4 || math.Abs(c.Lon-11.64178) > 1e-4 {
		t.Errorf("Seiser Alm centroid = %+v", c)
	}

	// Collinear points have no area: fall back to the mean of the points.
	line := [][]geoPoint{{{Lat: 0, Lon: 0}, {Lat: 1, Lon: 1}, {Lat: 2, Lon: 2}}}
	if c := centroid(line); c != (geoPoint{Lat: 1, Lon: 1}) {
		t.Errorf("line centroid = %+v", c)
	}
}

func geoEntry(t *testing.T, geo map[string]json.RawMessage, key string) map[string]any {
	t.Helper()
	var entry map[string]any
	if err := json.Unmarshal(geo[key], &entry); err != nil {
		t.Fatalf("invalid Geo[%s]: %v", key, err)
	}
	return entry
}

func Test_applyRegionGeo(t *testing.T) {
	geoFiles = stubGeoFiles("testdata/regionmap.gpx")
	ctx := context.Background()
	url := "https://www.dolomitisuperski.com/file/?uuidLift=test"

	// DSS-only SkiArea without GpsInfo: position from the centroid becomes the
	// default in GpsInfo and Geo, the track is added without Default.
	area := odhmodel.SkiArea{}
	applyRegionGeo(ctx, &area, url)

	var gpsInfo []map[string]any
	if err := json.Unmarshal(area.GpsInfo, &gpsInfo); err != nil || len(gpsInfo) != 1 {
		t.Fatalf("expected one GpsInfo entry, got %s", area.GpsInfo)
	}
	position := geoEntry(t, area.Geo, "position")
	for _, entry := range []map[string]any{gpsInfo[0], position} {
		if entry["Gpstype"] != "position" || entry["Default"] != true ||
			!strings.HasPrefix(entry["Geometry"].(string), "POINT (11.6417") ||
			math.Abs(entry["Latitude"].(float64)-46.54532) > 1e-4 {
			t.Errorf("unexpected position %v", entry)
		}
	}
	if track := geoEntry(t, area.Geo, "track"); track["Default"] != nil || !strings.HasPrefix(track["Geometry"].(string), "LINESTRING (") {
		t.Errorf("unexpected track %v", track)
	}

	// Earlier DSS run left the track as default: with the new position it is no
	// longer the default, so there is exactly one.
	area = odhmodel.SkiArea{Geo: map[string]json.RawMessage{"track": json.RawMessage(`{"Default":true,"Geometry":"LINESTRING (1 1, 2 2)"}`)}}
	applyRegionGeo(ctx, &area, url)
	if geoEntry(t, area.Geo, "track")["Default"] != nil || geoEntry(t, area.Geo, "position")["Default"] != true {
		t.Errorf("expected position as the only default, got %s / %s", area.Geo["track"], area.Geo["position"])
	}

	// IDM SkiArea with GpsInfo and Geo position: both kept byte for byte, only
	// the track is added (without Default).
	idmPosition := json.RawMessage(`{"Default":true,"Gpstype":"position","Geometry":"POINT (11.6201 46.5402)","Latitude":46.5402,"Longitude":11.6201}`)
	idmGpsInfo := json.RawMessage(`[` + string(idmPosition) + `]`)
	area = odhmodel.SkiArea{GpsInfo: idmGpsInfo, Geo: map[string]json.RawMessage{"position": idmPosition}}
	applyRegionGeo(ctx, &area, url)
	if string(area.GpsInfo) != string(idmGpsInfo) || string(area.Geo["position"]) != string(idmPosition) {
		t.Errorf("IDM GpsInfo/position was changed: %s / %s", area.GpsInfo, area.Geo["position"])
	}
	if track := geoEntry(t, area.Geo, "track"); track["Default"] != nil || len(track) != 1 {
		t.Errorf("track should only have a Geometry, got %v", track)
	}

	// Download error: SkiArea stays unchanged.
	geoFiles = &geoFileFetcher{download: func(ctx context.Context, url string) ([]byte, error) {
		return nil, errors.New("unexpected status 403")
	}}
	area = odhmodel.SkiArea{}
	applyRegionGeo(ctx, &area, url)
	if area.Geo != nil || area.GpsInfo != nil {
		t.Errorf("expected no change after failed download, got %v / %s", area.Geo, area.GpsInfo)
	}
}

func Test_fetch_retryAndPause(t *testing.T) {
	ctx := context.Background()
	calls := 0
	f := &geoFileFetcher{
		download: func(ctx context.Context, url string) ([]byte, error) {
			calls++
			if calls < 3 {
				return nil, errors.New("unexpected status 403")
			}
			return []byte("ok"), nil
		},
		retryDelays: []time.Duration{time.Millisecond, time.Millisecond},
		maxFailures: 2,
		cooldown:    time.Hour,
	}

	// Two 403s, the second retry succeeds.
	if body, err := f.fetch(ctx, "u"); err != nil || string(body) != "ok" || calls != 3 {
		t.Fatalf("expected success on third attempt, got %q, %v after %d calls", body, err, calls)
	}

	// Always blocked: after 2 failed downloads (3 attempts each) downloads pause.
	calls = 0
	f.download = func(ctx context.Context, url string) ([]byte, error) {
		calls++
		return nil, errors.New("unexpected status 403")
	}
	for i := 0; i < 2; i++ {
		if _, err := f.fetch(ctx, "u"); err == nil {
			t.Fatal("expected error")
		}
	}
	if calls != 6 {
		t.Errorf("expected 6 attempts, got %d", calls)
	}
	if _, err := f.fetch(ctx, "u"); err == nil || calls != 6 {
		t.Errorf("expected paused downloads without new attempts, got %v after %d calls", err, calls)
	}
}

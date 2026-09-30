// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

// stubGeoFiles serves every regionMap from a local GPX file, without delay.
func stubGeoFiles(path string) *geoFileFetcher {
	return &geoFileFetcher{
		download: func(ctx context.Context, url string) ([]byte, error) { return os.ReadFile(path) },
		ttl:      1 << 62,
	}
}

func Test_gpxToWKT(t *testing.T) {
	single, err := os.ReadFile("testdata/regionmap.gpx")
	if err != nil {
		t.Fatal(err)
	}
	wkt, err := gpxToWKT(single)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wkt, "LINESTRING (11.6398533 46.6099615, 11.6284056 46.6125558,") ||
		!strings.HasSuffix(wkt, ", 11.6399391 46.6099026)") || strings.Count(wkt, ",") != 35 {
		t.Errorf("unexpected LINESTRING: %.120s...", wkt)
	}

	multi, err := os.ReadFile("testdata/regionmap_multi.gpx")
	if err != nil {
		t.Fatal(err)
	}
	wkt, err = gpxToWKT(multi)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(wkt, "MULTILINESTRING ((11.635508537292482 46.30447470831477,") ||
		!strings.HasSuffix(wkt, ", 11.63547570195979 46.3045359181646))") || strings.Count(wkt, "), (") != 1 {
		t.Errorf("unexpected MULTILINESTRING: %.120s...", wkt)
	}

	if _, err := gpxToWKT([]byte("<html><title>Attention Required! | Cloudflare</title></html>")); err == nil {
		t.Error("expected error for a page without a track")
	}
}

func Test_withRegionTrack(t *testing.T) {
	geoFiles = stubGeoFiles("testdata/regionmap.gpx")
	ctx := context.Background()
	url := "https://www.dolomitisuperski.com/file/?uuidLift=test"

	track := func(geo map[string]json.RawMessage) map[string]any {
		var entry map[string]any
		if err := json.Unmarshal(geo["track"], &entry); err != nil {
			t.Fatalf("invalid track entry: %v", err)
		}
		return entry
	}

	// DSS-only SkiArea without Geo: track becomes the default entry.
	geo := withRegionTrack(ctx, nil, url)
	if entry := track(geo); entry["Default"] != true || !strings.HasPrefix(entry["Geometry"].(string), "LINESTRING (") {
		t.Errorf("unexpected track %v", entry)
	}

	// IDM SkiArea with a default position: position is kept byte for byte,
	// track is added without Default.
	position := json.RawMessage(`{"Default":true,"Gpstype":"position","Geometry":"POINT (11.6201 46.5402)","Latitude":46.5402,"Longitude":11.6201}`)
	geo = withRegionTrack(ctx, map[string]json.RawMessage{"position": position}, url)
	if string(geo["position"]) != string(position) {
		t.Errorf("position was changed: %s", geo["position"])
	}
	if entry := track(geo); entry["Default"] != nil || len(entry) != 1 {
		t.Errorf("track should only have a Geometry, got %v", entry)
	}

	// Download error: Geo stays unchanged.
	geoFiles = &geoFileFetcher{download: func(ctx context.Context, url string) ([]byte, error) {
		return nil, errors.New("unexpected status 403")
	}}
	geo = withRegionTrack(ctx, map[string]json.RawMessage{"position": position}, url)
	if _, ok := geo["track"]; ok || len(geo) != 1 {
		t.Errorf("expected Geo unchanged after failed download, got %v", geo)
	}
}

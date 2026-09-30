// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"opendatahub.com/tr-dss-lift/dto"
)

// stubGeoFiles serves every geoPositionFile from a local KML file, without delay.
func stubGeoFiles(path string) *geoFileFetcher {
	return &geoFileFetcher{
		download: func(ctx context.Context, url string) ([]byte, error) { return os.ReadFile(path) },
		ttl:      1 << 62,
	}
}

func Test_parseKMLPoints(t *testing.T) {
	data, err := os.ReadFile("testdata/geofile.kml")
	if err != nil {
		t.Fatal(err)
	}
	points, err := parseKMLPoints(data)
	if err != nil {
		t.Fatal(err)
	}
	if len(points) != 7 {
		t.Fatalf("expected 7 points, got %d", len(points))
	}
	if points[0] != (geoPoint{Lat: 46.31319228408, Lon: 11.68806792428}) {
		t.Errorf("unexpected first point %+v", points[0])
	}
	if points[6] != (geoPoint{Lat: 46.32401813725, Lon: 11.69809013631}) {
		t.Errorf("unexpected last point %+v", points[6])
	}

	if _, err := parseKMLPoints([]byte("<html><title>Attention Required! | Cloudflare</title></html>")); err == nil {
		t.Error("expected error for a page without coordinates")
	}
}

func Test_withGeoFileFallback(t *testing.T) {
	geoFiles = stubGeoFiles("testdata/geofile.kml")
	ctx := context.Background()
	file := "https://www.dolomitisuperski.com/file/?uuidLift=test"

	// No location: first point = valley station, last point = mountain station.
	lift := withGeoFileFallback(ctx, dto.DssLift{Pid: 1, GeoPositionFile: file})
	if lift.Location == nil || lift.Location.Lat != "46.31319228408" || lift.Location.Lon != "11.68806792428" {
		t.Errorf("unexpected location %+v", lift.Location)
	}
	if lift.LocationMountain == nil || lift.LocationMountain.Lat != "46.32401813725" || lift.LocationMountain.Lon != "11.69809013631" {
		t.Errorf("unexpected mountain location %+v", lift.LocationMountain)
	}

	// Existing mountain station is kept.
	mountain := &dto.DssLocation{Lat: "46.5", Lon: "11.7"}
	lift = withGeoFileFallback(ctx, dto.DssLift{Pid: 2, GeoPositionFile: file, LocationMountain: mountain})
	if lift.LocationMountain != mountain {
		t.Errorf("mountain location was overwritten: %+v", lift.LocationMountain)
	}

	// Existing location: no download, nothing changes.
	valley := &dto.DssLocation{Lat: "46.4", Lon: "11.6"}
	geoFiles.download = func(ctx context.Context, url string) ([]byte, error) {
		t.Error("unexpected download")
		return nil, errors.New("unexpected")
	}
	lift = withGeoFileFallback(ctx, dto.DssLift{Pid: 3, GeoPositionFile: "https://example.com/other", Location: valley})
	if lift.Location != valley || lift.LocationMountain != nil {
		t.Errorf("location was changed: %+v / %+v", lift.Location, lift.LocationMountain)
	}

	// Download error (e.g. Cloudflare 403): lift stays without location.
	geoFiles = &geoFileFetcher{download: func(ctx context.Context, url string) ([]byte, error) {
		return nil, errors.New("unexpected status 403")
	}}
	lift = withGeoFileFallback(ctx, dto.DssLift{Pid: 4, GeoPositionFile: file})
	if lift.Location != nil || lift.LocationMountain != nil {
		t.Errorf("expected no location after failed download, got %+v / %+v", lift.Location, lift.LocationMountain)
	}
}

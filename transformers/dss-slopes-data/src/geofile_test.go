// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"os"
	"testing"

	"opendatahub.com/tr-dss-slopes/dto"
)

// stubGeoFiles serves every geoPositionFile from a local KML file, without delay.
func stubGeoFiles(path string) *geoFileFetcher {
	return &geoFileFetcher{
		download: func(ctx context.Context, url string) ([]byte, error) { return os.ReadFile(path) },
		ttl:      1 << 62,
	}
}

func Test_withGeoFileFallback(t *testing.T) {
	geoFiles = stubGeoFiles("testdata/geofile.kml")
	ctx := context.Background()
	file := "https://www.dolomitisuperski.com/file/?uuidSlope=test"

	// No location: first track point (slope start).
	slope := withGeoFileFallback(ctx, dto.DssSlope{Pid: 1, GeoPositionFile: file})
	if slope.Location == nil || slope.Location.Lat != "46.50772309209" || slope.Location.Lon != "12.01592642647" {
		t.Errorf("unexpected location %+v", slope.Location)
	}

	// Existing location is kept.
	loc := &dto.DssSlopeLocation{Lat: "46.4", Lon: "11.6"}
	slope = withGeoFileFallback(ctx, dto.DssSlope{Pid: 2, GeoPositionFile: file, Location: loc})
	if slope.Location != loc {
		t.Errorf("location was overwritten: %+v", slope.Location)
	}

	// Download error: slope stays without location.
	geoFiles = &geoFileFetcher{download: func(ctx context.Context, url string) ([]byte, error) {
		return nil, errors.New("unexpected status 403")
	}}
	slope = withGeoFileFallback(ctx, dto.DssSlope{Pid: 3, GeoPositionFile: file})
	if slope.Location != nil {
		t.Errorf("expected no location after failed download, got %+v", slope.Location)
	}
}

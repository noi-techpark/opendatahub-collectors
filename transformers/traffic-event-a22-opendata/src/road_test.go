// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

func loadTestRoad(t *testing.T) *roadData {
	t.Helper()
	rd, err := LoadRoad("../resources/a22_road.json")
	if err != nil {
		t.Fatalf("failed to load road: %v", err)
	}
	return rd
}

func parseWKTCoords(t *testing.T, wkt string) [][2]float64 {
	t.Helper()
	inner := wkt[strings.Index(wkt, "(")+1 : len(wkt)-1]
	parts := strings.Split(inner, ", ")
	var coords [][2]float64
	for _, p := range parts {
		var lon, lat float64
		n, err := fmt.Sscanf(p, "%f %f", &lon, &lat)
		if err != nil || n != 2 {
			t.Fatalf("failed to parse coordinate %q: %v", p, err)
		}
		coords = append(coords, [2]float64{lon, lat})
	}
	return coords
}

func assertNear(t *testing.T, name string, got, want, tol float64) {
	t.Helper()
	if math.Abs(got-want) > tol {
		t.Errorf("%s: got %.4f, want ~%.4f (tolerance %.4f)", name, got, want, tol)
	}
}

// TestKmRangeToWKT_Lavori tests with in-lavori.json data: km 5.3 to 10.2
// (Brennero Paese to Vipiteno)
func TestKmRangeToWKT_Lavori(t *testing.T) {
	rd := loadTestRoad(t)
	wkt := rd.KmRangeToWKT(5.3, 10.2)

	if !strings.HasPrefix(wkt, "LINESTRING (") {
		t.Fatalf("expected LINESTRING, got: %.50s", wkt)
	}

	coords := parseWKTCoords(t, wkt)

	// ~4.9 km at 1km spacing -> expect at least 5 vertices
	if len(coords) < 5 {
		t.Errorf("expected at least 5 vertices for 4.9 km, got %d", len(coords))
	}

	// Start should be near km 5.3 (between Brennero and Vipiteno)
	assertNear(t, "start lon", coords[0][0], 11.47, 0.02)
	assertNear(t, "start lat", coords[0][1], 46.97, 0.02)

	// End should be near km 10.2
	assertNear(t, "end lon", coords[len(coords)-1][0], 11.45, 0.02)
	assertNear(t, "end lat", coords[len(coords)-1][1], 46.93, 0.02)

	t.Logf("km 5.3-10.2: %d vertices", len(coords))
}

// TestKmRangeToWKT_Traffico tests with in-traffico.json data: km 311 to 313
// (Carpi to A1 junction, near Modena)
func TestKmRangeToWKT_Traffico(t *testing.T) {
	rd := loadTestRoad(t)
	wkt := rd.KmRangeToWKT(311, 313)

	if !strings.HasPrefix(wkt, "LINESTRING (") {
		t.Fatalf("expected LINESTRING, got: %.50s", wkt)
	}

	coords := parseWKTCoords(t, wkt)

	// ~2 km segment, expect at least 3 vertices
	if len(coords) < 3 {
		t.Errorf("expected at least 3 vertices for 2 km, got %d", len(coords))
	}

	// Start should be near km 311 (southern end)
	assertNear(t, "start lon", coords[0][0], 10.85, 0.02)
	assertNear(t, "start lat", coords[0][1], 44.69, 0.02)

	// End should be near km 313
	assertNear(t, "end lon", coords[len(coords)-1][0], 10.84, 0.02)
	assertNear(t, "end lat", coords[len(coords)-1][1], 44.67, 0.02)

	t.Logf("km 311-313: %d vertices", len(coords))
}

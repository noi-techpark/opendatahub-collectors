// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

type roadPoint struct {
	Lon  float64
	Lat  float64
	Dist float64 // cumulative distance from start in meters
}

type roadData struct {
	Points  []roadPoint
	KmDists []float64 // projected distance for km 0-313
}

func LoadRoad(path string) (*roadData, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read road file: %w", err)
	}

	var raw struct {
		Points  [][3]float64 `json:"points"`
		KmDists []float64    `json:"km_dists"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse road JSON: %w", err)
	}

	rd := &roadData{
		Points:  make([]roadPoint, len(raw.Points)),
		KmDists: raw.KmDists,
	}
	for i, p := range raw.Points {
		rd.Points[i] = roadPoint{Lon: p[0], Lat: p[1], Dist: p[2]}
	}
	return rd, nil
}

// KmToDistance converts a (possibly fractional) km value to cumulative
// distance in meters along the road axis, interpolating between integer
// km markers.
func (rd *roadData) KmToDistance(km float64) float64 {
	maxKm := float64(len(rd.KmDists) - 1)
	if km <= 0 {
		return rd.KmDists[0]
	}
	if km >= maxKm {
		return rd.KmDists[len(rd.KmDists)-1]
	}
	floor := int(km)
	frac := km - float64(floor)
	return rd.KmDists[floor] + frac*(rd.KmDists[floor+1]-rd.KmDists[floor])
}

func (rd *roadData) interpolatePoint(dist float64) roadPoint {
	if dist <= rd.Points[0].Dist {
		return rd.Points[0]
	}
	last := len(rd.Points) - 1
	if dist >= rd.Points[last].Dist {
		return rd.Points[last]
	}

	i := sort.Search(len(rd.Points), func(i int) bool {
		return rd.Points[i].Dist >= dist
	})
	if rd.Points[i].Dist == dist {
		return rd.Points[i]
	}

	p0 := rd.Points[i-1]
	p1 := rd.Points[i]
	t := (dist - p0.Dist) / (p1.Dist - p0.Dist)
	return roadPoint{
		Lon:  p0.Lon + t*(p1.Lon-p0.Lon),
		Lat:  p0.Lat + t*(p1.Lat-p0.Lat),
		Dist: dist,
	}
}

// KmRangeToWKT returns a WKT LINESTRING representing the road segment
// between kmStart and kmEnd on the A22 highway axis.
func (rd *roadData) KmRangeToWKT(kmStart, kmEnd float64) string {
	if kmStart > kmEnd {
		kmStart, kmEnd = kmEnd, kmStart
	}

	distStart := rd.KmToDistance(kmStart)
	distEnd := rd.KmToDistance(kmEnd)

	if distStart >= distEnd {
		p := rd.interpolatePoint(distStart)
		return fmt.Sprintf("POINT (%.4f %.4f)", p.Lon, p.Lat)
	}

	// Find first point index with Dist >= distStart
	iStart := sort.Search(len(rd.Points), func(i int) bool {
		return rd.Points[i].Dist >= distStart
	})
	// Find first point index with Dist > distEnd
	iEnd := sort.Search(len(rd.Points), func(i int) bool {
		return rd.Points[i].Dist > distEnd
	})

	var coords []string

	pStart := rd.interpolatePoint(distStart)
	coords = append(coords, fmt.Sprintf("%.4f %.4f", pStart.Lon, pStart.Lat))

	for i := iStart; i < iEnd; i++ {
		p := rd.Points[i]
		if p.Dist > distStart && p.Dist < distEnd {
			coords = append(coords, fmt.Sprintf("%.4f %.4f", p.Lon, p.Lat))
		}
	}

	pEnd := rd.interpolatePoint(distEnd)
	coords = append(coords, fmt.Sprintf("%.4f %.4f", pEnd.Lon, pEnd.Lat))

	return fmt.Sprintf("LINESTRING (%s)", strings.Join(coords, ", "))
}

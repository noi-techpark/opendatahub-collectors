// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DSS regionMap files (GPX outlines of a ski area) are converted to a WKT
// geometry for SkiArea.Geo["track"]; their centroid is used as position when a
// SkiArea has no GpsInfo. The file endpoint sits behind Cloudflare bot
// protection: it only answers browser-like requests (User-Agent + Sec-Fetch-*)
// and blocks bursts, so downloads are throttled and cached.

type geoPoint struct {
	Lat float64
	Lon float64
}

// regionTrack is a converted regionMap: the outline as WKT and its centroid.
type regionTrack struct {
	WKT    string
	Center geoPoint
}

type cachedGeoFile struct {
	track     regionTrack
	fetchedAt time.Time
}

type geoFileFetcher struct {
	download    func(ctx context.Context, url string) ([]byte, error)
	minInterval time.Duration   // minimum pause between two downloads
	retryDelays []time.Duration // pauses before retrying a failed download
	maxFailures int             // consecutive failed downloads before pausing (0 = never)
	cooldown    time.Duration   // how long downloads pause after maxFailures
	ttl         time.Duration   // how long a converted track is reused

	mu          sync.Mutex
	lastCall    time.Time
	failures    int
	pausedUntil time.Time
	cache       map[string]cachedGeoFile
}

var geoFiles = &geoFileFetcher{
	download:    downloadGeoFile,
	minInterval: 2 * time.Second,
	retryDelays: []time.Duration{3 * time.Second, 10 * time.Second},
	maxFailures: 5,
	cooldown:    30 * time.Minute,
	ttl:         24 * time.Hour,
}

var geoFileHTTPClient = &http.Client{Timeout: 30 * time.Second}

// Track returns the GPX file at url as WKT LINESTRING / MULTILINESTRING plus its centroid.
func (f *geoFileFetcher) Track(ctx context.Context, url string) (regionTrack, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if cached, ok := f.cache[url]; ok && time.Since(cached.fetchedAt) < f.ttl {
		return cached.track, nil
	}

	body, err := f.fetch(ctx, url)
	if err != nil {
		return regionTrack{}, err
	}
	lines, err := parseGPXLines(body)
	if err != nil {
		return regionTrack{}, err
	}
	track := regionTrack{WKT: linesToWKT(lines), Center: centroid(lines)}

	if f.cache == nil {
		f.cache = map[string]cachedGeoFile{}
	}
	f.cache[url] = cachedGeoFile{track: track, fetchedAt: time.Now()}
	return track, nil
}

// fetch downloads url with a minimum pause before every request. A failed
// download (e.g. Cloudflare 403 after a burst of requests from the cluster) is
// retried after retryDelays. After maxFailures consecutive failed downloads the
// fallback downloads pause for cooldown, so a blocked run does not spend its
// time on retries. Callers hold f.mu.
func (f *geoFileFetcher) fetch(ctx context.Context, url string) ([]byte, error) {
	if time.Now().Before(f.pausedUntil) {
		return nil, fmt.Errorf("downloads paused until %s after repeated failures", f.pausedUntil.Format(time.RFC3339))
	}

	var err error
	for attempt := 0; attempt <= len(f.retryDelays); attempt++ {
		wait := f.minInterval - time.Since(f.lastCall)
		if attempt > 0 && f.retryDelays[attempt-1] > wait {
			wait = f.retryDelays[attempt-1]
		}
		if wait > 0 {
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		var body []byte
		body, err = f.download(ctx, url)
		f.lastCall = time.Now()
		if err == nil {
			f.failures = 0
			return body, nil
		}
	}

	f.failures++
	if f.maxFailures > 0 && f.failures >= f.maxFailures {
		f.pausedUntil = time.Now().Add(f.cooldown)
		f.failures = 0
	}
	return nil, err
}

func downloadGeoFile(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (X11; Linux x86_64; rv:130.0) Gecko/20100101 Firefox/130.0")
	req.Header.Set("Sec-Fetch-Dest", "document")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Sec-Fetch-Site", "same-origin")

	resp, err := geoFileHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10<<20))
}

// parseGPXLines returns the GPX track segments (and routes) as point lists.
// Segments with fewer than two points are skipped, as they are not a valid line.
func parseGPXLines(data []byte) ([][]geoPoint, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var lines [][]geoPoint
	var current []geoPoint

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid GPX: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			switch t.Name.Local {
			case "trkseg", "rte":
				current = nil
			case "trkpt", "rtept":
				if point, ok := gpxPoint(t); ok {
					current = append(current, point)
				}
			}
		case xml.EndElement:
			if (t.Name.Local == "trkseg" || t.Name.Local == "rte") && len(current) >= 2 {
				lines = append(lines, current)
			}
		}
	}

	if len(lines) == 0 {
		return nil, errors.New("no track in GPX")
	}
	return lines, nil
}

// linesToWKT: one line becomes a LINESTRING, several a MULTILINESTRING.
func linesToWKT(lines [][]geoPoint) string {
	parts := make([]string, len(lines))
	for i, line := range lines {
		coords := make([]string, len(line))
		for j, p := range line {
			coords[j] = formatCoordinate(p.Lon) + " " + formatCoordinate(p.Lat)
		}
		parts[i] = "(" + strings.Join(coords, ", ") + ")"
	}
	if len(parts) == 1 {
		return "LINESTRING " + parts[0]
	}
	return "MULTILINESTRING (" + strings.Join(parts, ", ") + ")"
}

// centroid returns the area centroid of the outline: every line is closed as a
// ring, several rings are weighted by their area. Lines without an area (e.g.
// all points on one line) fall back to the mean of the points.
func centroid(lines [][]geoPoint) geoPoint {
	var totalArea, sumLat, sumLon float64
	for _, ring := range lines {
		var area, cLon, cLat float64
		for i := range ring {
			a, b := ring[i], ring[(i+1)%len(ring)]
			cross := a.Lon*b.Lat - b.Lon*a.Lat
			area += cross
			cLon += (a.Lon + b.Lon) * cross
			cLat += (a.Lat + b.Lat) * cross
		}
		area /= 2
		if math.Abs(area) < 1e-12 {
			continue
		}
		weight := math.Abs(area)
		totalArea += weight
		sumLon += weight * cLon / (6 * area)
		sumLat += weight * cLat / (6 * area)
	}
	if totalArea > 0 {
		return geoPoint{Lat: sumLat / totalArea, Lon: sumLon / totalArea}
	}

	var n float64
	var mean geoPoint
	for _, line := range lines {
		for _, p := range line {
			mean.Lat += p.Lat
			mean.Lon += p.Lon
			n++
		}
	}
	return geoPoint{Lat: mean.Lat / n, Lon: mean.Lon / n}
}

// gpxPoint returns the coordinate of a trkpt/rtept element.
func gpxPoint(el xml.StartElement) (geoPoint, bool) {
	var lat, lon string
	for _, attr := range el.Attr {
		switch attr.Name.Local {
		case "lat":
			lat = attr.Value
		case "lon":
			lon = attr.Value
		}
	}
	latF, latErr := strconv.ParseFloat(lat, 64)
	lonF, lonErr := strconv.ParseFloat(lon, 64)
	if latErr != nil || lonErr != nil || !isFinite(latF) || !isFinite(lonF) {
		return geoPoint{}, false
	}
	return geoPoint{Lat: latF, Lon: lonF}, true
}

func formatCoordinate(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

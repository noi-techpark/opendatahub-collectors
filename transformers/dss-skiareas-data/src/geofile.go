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
// geometry for SkiArea.Geo["track"]. The file endpoint sits behind Cloudflare
// bot protection: it only answers browser-like requests (User-Agent +
// Sec-Fetch-*) and blocks bursts, so downloads are throttled and cached.

type cachedGeoFile struct {
	wkt       string
	fetchedAt time.Time
}

type geoFileFetcher struct {
	download    func(ctx context.Context, url string) ([]byte, error)
	minInterval time.Duration // minimum pause between two downloads
	ttl         time.Duration // how long a converted track is reused

	mu       sync.Mutex
	lastCall time.Time
	cache    map[string]cachedGeoFile
}

var geoFiles = &geoFileFetcher{
	download:    downloadGeoFile,
	minInterval: 1500 * time.Millisecond,
	ttl:         24 * time.Hour,
}

var geoFileHTTPClient = &http.Client{Timeout: 30 * time.Second}

// TrackWKT returns the GPX file at url as a WKT LINESTRING / MULTILINESTRING.
func (f *geoFileFetcher) TrackWKT(ctx context.Context, url string) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if cached, ok := f.cache[url]; ok && time.Since(cached.fetchedAt) < f.ttl {
		return cached.wkt, nil
	}

	if wait := f.minInterval - time.Since(f.lastCall); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
	f.lastCall = time.Now()

	body, err := f.download(ctx, url)
	if err != nil {
		return "", err
	}
	wkt, err := gpxToWKT(body)
	if err != nil {
		return "", err
	}

	if f.cache == nil {
		f.cache = map[string]cachedGeoFile{}
	}
	f.cache[url] = cachedGeoFile{wkt: wkt, fetchedAt: time.Now()}
	return wkt, nil
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

// gpxToWKT converts the GPX track segments (and routes) to WKT: one segment
// becomes a LINESTRING, several a MULTILINESTRING. Segments with fewer than
// two points are skipped, as they are not a valid line.
func gpxToWKT(data []byte) (string, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var lines [][]string
	var current []string

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("invalid GPX: %w", err)
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

	switch len(lines) {
	case 0:
		return "", errors.New("no track in GPX")
	case 1:
		return "LINESTRING (" + strings.Join(lines[0], ", ") + ")", nil
	default:
		parts := make([]string, len(lines))
		for i, line := range lines {
			parts[i] = "(" + strings.Join(line, ", ") + ")"
		}
		return "MULTILINESTRING (" + strings.Join(parts, ", ") + ")", nil
	}
}

// gpxPoint returns the "lon lat" WKT coordinate of a trkpt/rtept element.
func gpxPoint(el xml.StartElement) (string, bool) {
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
		return "", false
	}
	return strconv.FormatFloat(lonF, 'f', -1, 64) + " " + strconv.FormatFloat(latF, 'f', -1, 64), true
}

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

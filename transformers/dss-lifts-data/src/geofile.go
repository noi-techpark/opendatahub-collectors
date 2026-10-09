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
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// DSS geoPositionFiles (KML tracks) are used as a GPS fallback when a record has
// no location. The file endpoint sits behind Cloudflare bot protection: it only
// answers browser-like requests (User-Agent + Sec-Fetch-*) and blocks bursts, so
// downloads are throttled and cached. Any failure just means "no fallback".

type geoPoint struct {
	Lat float64
	Lon float64
}

type cachedGeoFile struct {
	points    []geoPoint
	fetchedAt time.Time
}

type geoFileFetcher struct {
	download    func(ctx context.Context, url string) ([]byte, error)
	minInterval time.Duration   // minimum pause between two downloads
	retryDelays []time.Duration // pauses before retrying a failed download
	maxFailures int             // consecutive failed downloads before pausing (0 = never)
	cooldown    time.Duration   // how long downloads pause after maxFailures
	ttl         time.Duration   // how long a downloaded track is reused

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

// Points returns the track points of the KML file at url, in file order.
func (f *geoFileFetcher) Points(ctx context.Context, url string) ([]geoPoint, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if cached, ok := f.cache[url]; ok && time.Since(cached.fetchedAt) < f.ttl {
		return cached.points, nil
	}

	body, err := f.fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	points, err := parseKMLPoints(body)
	if err != nil {
		return nil, err
	}

	if f.cache == nil {
		f.cache = map[string]cachedGeoFile{}
	}
	f.cache[url] = cachedGeoFile{points: points, fetchedAt: time.Now()}
	return points, nil
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

// parseKMLPoints collects all <coordinates> points ("lon,lat[,alt]") in file order.
func parseKMLPoints(data []byte) ([]geoPoint, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	var points []geoPoint
	inCoordinates := false

	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("invalid KML: %w", err)
		}

		switch t := tok.(type) {
		case xml.StartElement:
			inCoordinates = t.Name.Local == "coordinates"
		case xml.EndElement:
			inCoordinates = false
		case xml.CharData:
			if !inCoordinates {
				continue
			}
			for _, tuple := range strings.Fields(string(t)) {
				parts := strings.Split(tuple, ",")
				if len(parts) < 2 {
					continue
				}
				lon, lonErr := strconv.ParseFloat(parts[0], 64)
				lat, latErr := strconv.ParseFloat(parts[1], 64)
				if lonErr != nil || latErr != nil || !isFinite(lat) || !isFinite(lon) {
					continue
				}
				points = append(points, geoPoint{Lat: lat, Lon: lon})
			}
		}
	}

	if len(points) == 0 {
		return nil, errors.New("no coordinates in KML")
	}
	return points, nil
}

func formatCoordinate(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

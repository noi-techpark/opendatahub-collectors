// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"opendatahub.com/tr-gtfs-to-trip/dto"
)

type GtfsData struct {
	Agencies  []dto.Agency
	Stops     map[string]dto.Stop
	Routes    map[string]dto.Route
	Calendars map[string]dto.Calendar
	Trips     []dto.Trip
	StopTimes map[string][]dto.StopTime
	Shapes    map[string][]dto.ShapePoint
}

// ParseGtfsDate parses a GTFS date string "YYYYMMDD" into a time.Time (UTC, midnight).
func ParseGtfsDate(s string) (time.Time, error) {
	if len(s) != 8 {
		return time.Time{}, fmt.Errorf("invalid GTFS date %q: expected 8 characters", s)
	}
	return time.Parse("20060102", s)
}

// ParseGtfsTime parses a GTFS time string "H:MM:SS" or "HH:MM:SS" into a time.Duration.
// GTFS times can exceed 24:00:00 for trips spanning midnight.
func ParseGtfsTime(s string) (time.Duration, error) {
	s = strings.TrimSpace(s)
	parts := strings.Split(s, ":")
	if len(parts) != 3 {
		return 0, fmt.Errorf("invalid GTFS time %q: expected HH:MM:SS", s)
	}
	h, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, fmt.Errorf("invalid hours in %q: %w", s, err)
	}
	m, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, fmt.Errorf("invalid minutes in %q: %w", s, err)
	}
	sec, err := strconv.Atoi(parts[2])
	if err != nil {
		return 0, fmt.Errorf("invalid seconds in %q: %w", s, err)
	}
	return time.Duration(h)*time.Hour + time.Duration(m)*time.Minute + time.Duration(sec)*time.Second, nil
}

// DownloadAndParseGtfs downloads a GTFS zip from the given URL and parses it.
func DownloadAndParseGtfs(url string) (*GtfsData, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download GTFS zip: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GTFS download returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read GTFS zip body: %w", err)
	}

	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		return nil, fmt.Errorf("failed to open GTFS zip: %w", err)
	}

	return ParseGtfsFromZip(zr)
}

// ParseGtfsFromZip parses all GTFS CSV files from a zip reader.
func ParseGtfsFromZip(zr *zip.Reader) (*GtfsData, error) {
	data := &GtfsData{
		Stops:     make(map[string]dto.Stop),
		Routes:    make(map[string]dto.Route),
		Calendars: make(map[string]dto.Calendar),
		StopTimes: make(map[string][]dto.StopTime),
		Shapes:    make(map[string][]dto.ShapePoint),
	}

	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("failed to open %s: %w", f.Name, err)
		}

		err = parseGtfsFile(data, f.Name, rc)
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("failed to parse %s: %w", f.Name, err)
		}
	}

	// Sort stop_times by sequence
	for tripID, sts := range data.StopTimes {
		sort.Slice(sts, func(i, j int) bool {
			si, _ := strconv.Atoi(sts[i].StopSequence)
			sj, _ := strconv.Atoi(sts[j].StopSequence)
			return si < sj
		})
		data.StopTimes[tripID] = sts
	}

	return data, nil
}

func parseGtfsFile(data *GtfsData, name string, r io.Reader) error {
	reader := csv.NewReader(r)
	reader.LazyQuotes = true
	reader.TrimLeadingSpace = true

	headers, err := reader.Read()
	if err != nil {
		return fmt.Errorf("failed to read headers: %w", err)
	}

	// Strip BOM from first header if present
	if len(headers) > 0 {
		headers[0] = strings.TrimPrefix(headers[0], "\xef\xbb\xbf")
	}

	colIdx := make(map[string]int, len(headers))
	for i, h := range headers {
		colIdx[strings.TrimSpace(h)] = i
	}

	get := func(row []string, col string) string {
		if idx, ok := colIdx[col]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	for {
		row, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to read row: %w", err)
		}

		switch name {
		case "agency.txt":
			data.Agencies = append(data.Agencies, dto.Agency{
				AgencyID:       get(row, "agency_id"),
				AgencyName:     get(row, "agency_name"),
				AgencyURL:      get(row, "agency_url"),
				AgencyTimezone: get(row, "agency_timezone"),
				AgencyLang:     get(row, "agency_lang"),
			})

		case "stops.txt":
			stop := dto.Stop{
				StopID:       get(row, "stop_id"),
				StopName:     get(row, "stop_name"),
				StopLat:      get(row, "stop_lat"),
				StopLon:      get(row, "stop_lon"),
				StopURL:      get(row, "stop_url"),
				StopTimezone: get(row, "stop_timezone"),
			}
			data.Stops[stop.StopID] = stop

		case "routes.txt":
			route := dto.Route{
				RouteID:           get(row, "route_id"),
				RouteLongName:     get(row, "route_long_name"),
				RouteShortName:    get(row, "route_short_name"),
				AgencyID:          get(row, "agency_id"),
				RouteType:         get(row, "route_type"),
				RouteSortOrder:    get(row, "route_sort_order"),
				ContinuousPickup:  get(row, "continuous_pickup"),
				ContinuousDropOff: get(row, "continuous_drop_off"),
			}
			data.Routes[route.RouteID] = route

		case "calendar.txt":
			cal := dto.Calendar{
				ServiceID: get(row, "service_id"),
				Monday:    get(row, "monday"),
				Tuesday:   get(row, "tuesday"),
				Wednesday: get(row, "wednesday"),
				Thursday:  get(row, "thursday"),
				Friday:    get(row, "friday"),
				Saturday:  get(row, "saturday"),
				Sunday:    get(row, "sunday"),
				StartDate: get(row, "start_date"),
				EndDate:   get(row, "end_date"),
			}
			data.Calendars[cal.ServiceID] = cal

		case "trips.txt":
			data.Trips = append(data.Trips, dto.Trip{
				RouteID:       get(row, "route_id"),
				ServiceID:     get(row, "service_id"),
				TripHeadsign:  get(row, "trip_headsign"),
				TripShortName: get(row, "trip_short_name"),
				DirectionID:   get(row, "direction_id"),
				ShapeID:       get(row, "shape_id"),
				TripID:        get(row, "trip_id"),
			})

		case "stop_times.txt":
			st := dto.StopTime{
				TripID:            get(row, "trip_id"),
				ArrivalTime:       get(row, "arrival_time"),
				DepartureTime:     get(row, "departure_time"),
				StopID:            get(row, "stop_id"),
				StopSequence:      get(row, "stop_sequence"),
				PickupType:        get(row, "pickup_type"),
				DropOffType:       get(row, "drop_off_type"),
				ContinuousPickup:  get(row, "continuous_pickup"),
				ContinuousDropOff: get(row, "continuous_drop_off"),
				ShapeDistTraveled: get(row, "shape_dist_traveled"),
			}
			data.StopTimes[st.TripID] = append(data.StopTimes[st.TripID], st)

		case "shapes.txt":
			sp := dto.ShapePoint{
				ShapeID:           get(row, "shape_id"),
				ShapePtLat:        get(row, "shape_pt_lat"),
				ShapePtLon:        get(row, "shape_pt_lon"),
				ShapePtSequence:   get(row, "shape_pt_sequence"),
				ShapeDistTraveled: get(row, "shape_dist_traveled"),
			}
			data.Shapes[sp.ShapeID] = append(data.Shapes[sp.ShapeID], sp)
		}
	}

	return nil
}

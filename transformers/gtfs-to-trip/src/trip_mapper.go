// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"opendatahub.com/tr-gtfs-to-trip/dto"
	odhContentModel "opendatahub.com/tr-gtfs-to-trip/odh-content-model"
)

func Float64Ptr(f float64) *float64 {
	return &f
}

// MapperConfig holds deployment-specific configuration for the GTFS-to-Trip mapper.
type MapperConfig struct {
	Source string
	TagIDs []string
}

// MapGtfsToTrips maps parsed GTFS data to ODH Trip objects.
func MapGtfsToTrips(gtfs *GtfsData, cfg MapperConfig, tags clib.TagDefs, syncTime time.Time) ([]odhContentModel.Trip, error) {
	var trips []odhContentModel.Trip

	for _, gtfsTrip := range gtfs.Trips {
		id := clib.GenerateID(fmt.Sprintf("urn:trip:%s", cfg.Source), gtfsTrip.TripID)

		gtfsRoute, routeOk := gtfs.Routes[gtfsTrip.RouteID]
		gtfsCal, calOk := gtfs.Calendars[gtfsTrip.ServiceID]
		stopTimes := gtfs.StopTimes[gtfsTrip.TripID]

		// Resolve agency and language
		var agency *dto.Agency
		if routeOk {
			if a, ok := findAgency(gtfs, gtfsRoute.AgencyID); ok {
				agency = &a
			}
		}
		lang := agencyLang(agency)

		trip := odhContentModel.Trip{
			Generic: odhContentModel.Generic{
				ID:     clib.StringPtr(id),
				Active: true,
				Source: clib.StringPtr(cfg.Source),
				LicenseInfo: &clib.LicenseInfo{
					ClosedData: false,
					License:    clib.StringPtr("CC0"),
				},
				HasLanguage: []string{lang},
				Mapping: map[string]map[string]string{
					cfg.Source: {
						"TripID":   gtfsTrip.TripID,
						"SyncTime": syncTime.Format(time.RFC3339),
					},
				},
				TagIds: cfg.TagIDs,
			},
		}

		if agency != nil {
			trip.Agency = buildAgency(agency, lang)
		}

		if routeOk {
			trip.Shortname = clib.StringPtr(gtfsRoute.RouteShortName)

			tripRoute := &odhContentModel.TripRoute{
				Shortname: gtfsRoute.RouteShortName,
				TagIds:    cfg.TagIDs,
			}

			if gtfsRoute.RouteLongName != "" {
				tripRoute.Detail = map[string]*clib.DetailGeneric{
					lang: {Title: clib.StringPtr(gtfsRoute.RouteLongName)},
				}
			}

			if calOk {
				cal, err := buildCalendar(gtfsCal, stopTimes, lang)
				if err != nil {
					return nil, fmt.Errorf("failed to build calendar for trip %s: %w", gtfsTrip.TripID, err)
				}
				tripRoute.Calendar = cal
			}

			trip.Route = tripRoute
		}

		// Build StopTimes
		if len(stopTimes) > 0 && calOk {
			startDate, err := ParseGtfsDate(gtfsCal.StartDate)
			if err != nil {
				return nil, fmt.Errorf("failed to parse start_date for trip %s: %w", gtfsTrip.TripID, err)
			}

			var odhStopTimes []odhContentModel.TripStopTime
			for _, st := range stopTimes {
				odhSt, err := buildStopTime(gtfs, st, startDate, lang)
				if err != nil {
					return nil, fmt.Errorf("failed to build stop_time for trip %s, stop %s: %w", gtfsTrip.TripID, st.StopID, err)
				}
				odhStopTimes = append(odhStopTimes, odhSt)
			}
			trip.StopTimes = odhStopTimes
		}

		computeTripGeo(&trip)

		trips = append(trips, trip)
	}

	return trips, nil
}

// agencyLang returns the agency's language, falling back to "en".
func agencyLang(agency *dto.Agency) string {
	if agency != nil && agency.AgencyLang != "" {
		return agency.AgencyLang
	}
	return "en"
}

func buildAgency(agency *dto.Agency, lang string) *odhContentModel.TripAgency {
	ta := &odhContentModel.TripAgency{
		Shortname: agency.AgencyName,
	}

	ci := &odhContentModel.ContactInfos{}
	if agency.AgencyName != "" {
		ci.CompanyName = clib.StringPtr(agency.AgencyName)
	}
	if agency.AgencyURL != "" {
		ci.Url = clib.StringPtr(agency.AgencyURL)
	}
	ci.Language = clib.StringPtr(lang)

	ta.ContactInfos = map[string]*odhContentModel.ContactInfos{lang: ci}
	return ta
}

func buildCalendar(gtfsCal dto.Calendar, stopTimes []dto.StopTime, lang string) (*odhContentModel.Calendar, error) {
	startDate, err := ParseGtfsDate(gtfsCal.StartDate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse calendar start_date: %w", err)
	}
	endDate, err := ParseGtfsDate(gtfsCal.EndDate)
	if err != nil {
		return nil, fmt.Errorf("failed to parse calendar end_date: %w", err)
	}

	// Determine departure and arrival times from first/last stop_time
	var startTime, endTime string
	if len(stopTimes) > 0 {
		startTime = stopTimes[0].DepartureTime
		endTime = stopTimes[len(stopTimes)-1].ArrivalTime
	}
	if startTime == "" {
		startTime = "00:00:00"
	}
	if endTime == "" {
		endTime = "23:59:59"
	}

	os := odhContentModel.OperationSchedule{
		Start: startDate,
		Stop:  endDate,
		Type:  clib.StringPtr("1"),
		OperationScheduleTime: []odhContentModel.OperationScheduleTime{
			{
				Start:     startTime,
				End:       endTime,
				Monday:    gtfsCal.Monday == "1",
				Tuesday:   gtfsCal.Tuesday == "1",
				Wednesday: gtfsCal.Wednesday == "1",
				Thursday:  gtfsCal.Thursday == "1",
				Friday:    gtfsCal.Friday == "1",
				Saturday:  gtfsCal.Saturday == "1",
				Sunday:    gtfsCal.Sunday == "1",
				State:     2,
			},
		},
	}

	return &odhContentModel.Calendar{OperationSchedule: os}, nil
}

func findAgency(gtfs *GtfsData, agencyID string) (dto.Agency, bool) {
	for _, a := range gtfs.Agencies {
		if a.AgencyID == agencyID {
			return a, true
		}
	}
	// If only one agency exists, return it as default
	if len(gtfs.Agencies) == 1 {
		return gtfs.Agencies[0], true
	}
	return dto.Agency{}, false
}

func buildStopTime(gtfs *GtfsData, st dto.StopTime, startDate time.Time, lang string) (odhContentModel.TripStopTime, error) {
	arrDur, err := ParseGtfsTime(st.ArrivalTime)
	if err != nil {
		return odhContentModel.TripStopTime{}, fmt.Errorf("failed to parse arrival_time %q: %w", st.ArrivalTime, err)
	}
	depDur, err := ParseGtfsTime(st.DepartureTime)
	if err != nil {
		return odhContentModel.TripStopTime{}, fmt.Errorf("failed to parse departure_time %q: %w", st.DepartureTime, err)
	}

	arrivalTime := startDate.Add(arrDur)
	departureTime := startDate.Add(depDur)

	odhSt := odhContentModel.TripStopTime{
		Shortname:     st.StopID,
		ArrivalTime:   arrivalTime,
		DepartureTime: departureTime,
	}

	// Add stop details and geo if stop is known
	if stop, ok := gtfs.Stops[st.StopID]; ok {
		if stop.StopName != "" {
			odhSt.Detail = map[string]*clib.DetailGeneric{
				lang: {Title: clib.StringPtr(stop.StopName)},
			}
		}

		lat, latErr := strconv.ParseFloat(stop.StopLat, 64)
		lon, lonErr := strconv.ParseFloat(stop.StopLon, 64)
		if latErr == nil && lonErr == nil {
			odhSt.Geo = map[string]clib.GpsInfo{
				"position": {
					Latitude:  Float64Ptr(lat),
					Longitude: Float64Ptr(lon),
					Geometry:  clib.StringPtr(fmt.Sprintf("POINT(%f %f)", lon, lat)),
					Default:   true,
				},
			}
		}
	}

	return odhSt, nil
}

// computeTripGeo builds a trip-level Geo field containing a GEOMETRYCOLLECTION
// of all stop geometries plus a connecting LINESTRING, for visualization.
func computeTripGeo(trip *odhContentModel.Trip) {
	if len(trip.StopTimes) == 0 {
		return
	}

	var wktGeometries []string
	var coords []string
	var firstLat, firstLon *float64

	for _, st := range trip.StopTimes {
		pos, ok := st.Geo["position"]
		if !ok {
			continue
		}

		if pos.Geometry != nil && *pos.Geometry != "" {
			wktGeometries = append(wktGeometries, *pos.Geometry)
		}

		if pos.Latitude != nil && pos.Longitude != nil {
			coords = append(coords, fmt.Sprintf("%f %f", *pos.Longitude, *pos.Latitude))
			if firstLat == nil {
				firstLat = pos.Latitude
				firstLon = pos.Longitude
			}
		}
	}

	if len(coords) >= 2 {
		wktGeometries = append(wktGeometries, fmt.Sprintf("LINESTRING(%s)", strings.Join(coords, ",")))
	}

	if len(wktGeometries) == 0 {
		return
	}

	var wkt string
	if len(wktGeometries) == 1 {
		wkt = wktGeometries[0]
	} else {
		wkt = fmt.Sprintf("GEOMETRYCOLLECTION(%s)", strings.Join(wktGeometries, ","))
	}

	trip.Geo = map[string]clib.GpsInfo{
		"position": {
			Gpstype:   clib.StringPtr("position"),
			Latitude:  firstLat,
			Longitude: firstLon,
			Geometry:  &wkt,
			Default:   true,
		},
	}
}

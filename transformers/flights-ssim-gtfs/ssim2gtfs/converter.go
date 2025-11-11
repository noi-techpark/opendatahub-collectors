// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package converter

import (
	"fmt"
	"ssim_parser/enrichment"
	"ssim_parser/gtfs"
	"ssim_parser/ssim"
	"strings"
)

// Converter converts SSIM data to GTFS format
type Converter struct {
	ssimRecords    []interface{}
	airports       map[string]*gtfs.Stop
	enrichmentData *enrichment.EnrichmentData
}

// NewConverter creates a new SSIM to GTFS converter
func NewConverter(ssimRecords []interface{}) *Converter {
	return &Converter{
		ssimRecords: ssimRecords,
		airports:    make(map[string]*gtfs.Stop),
	}
}

// SetEnrichmentData sets the enrichment data for the converter
func (c *Converter) SetEnrichmentData(data *enrichment.EnrichmentData) {
	c.enrichmentData = data
}

// Convert transforms SSIM records into GTFS format
func (c *Converter) Convert() (*gtfs.GTFS, error) {
	result := &gtfs.GTFS{
		Agency:       []gtfs.Agency{},
		Stops:        []gtfs.Stop{},
		Routes:       []gtfs.Route{},
		Trips:        []gtfs.Trip{},
		StopTimes:    []gtfs.StopTime{},
		Calendar:     []gtfs.Calendar{},
		CalendarDates: []gtfs.CalendarDate{},
	}

	// Track unique airlines and routes
	airlines := make(map[string]bool)
	routes := make(map[string]bool)
	serviceIDs := make(map[string]*gtfs.Calendar)

	// Process all SSIM records
	for _, record := range c.ssimRecords {
		switch r := record.(type) {
		case *ssim.Carrier:
			if !airlines[r.AirlineDesignator] {
				result.Agency = append(result.Agency, c.convertCarrierToAgency(r))
				airlines[r.AirlineDesignator] = true
			}

		case *ssim.Flight:
			// Create route if not exists
			routeID := fmt.Sprintf("%s_%s", r.AirlineDesignator, r.FlightNumber)
			if !routes[routeID] {
				result.Routes = append(result.Routes, c.convertFlightToRoute(r))
				routes[routeID] = true
			}

			// Create service calendar
			serviceID := c.createServiceID(r)
			if _, exists := serviceIDs[serviceID]; !exists {
				calendar := c.convertFlightToCalendar(r, serviceID)
				result.Calendar = append(result.Calendar, calendar)
				serviceIDs[serviceID] = &calendar
			}

			// Create trip
			trip := c.convertFlightToTrip(r, routeID, serviceID)
			result.Trips = append(result.Trips, trip)

			// Create stops
			c.addAirport(r.DepartureStation)
			c.addAirport(r.ArrivalStation)

			// Create stop times
			stopTimes := c.convertFlightToStopTimes(r, trip.TripID)
			result.StopTimes = append(result.StopTimes, stopTimes...)
		}
	}

	// Add all collected airports to stops
	for _, stop := range c.airports {
		result.Stops = append(result.Stops, *stop)
	}

	return result, nil
}

func (c *Converter) convertCarrierToAgency(carrier *ssim.Carrier) gtfs.Agency {
	agency := gtfs.Agency{
		AgencyID:       carrier.AirlineDesignator,
		AgencyName:     carrier.AirlineDesignator + " Airlines",
		AgencyURL:      "http://www.example.com",
		AgencyTimezone: "UTC",
		AgencyLang:     "en",
	}

	// Apply enrichment data if available
	if c.enrichmentData != nil {
		if info, exists := c.enrichmentData.GetAgency(carrier.AirlineDesignator); exists {
			agency.AgencyName = info.Name
			agency.AgencyURL = info.URL
			agency.AgencyTimezone = info.Timezone
			if info.Lang != "" {
				agency.AgencyLang = info.Lang
			}
		}
	}

	return agency
}

func (c *Converter) convertFlightToRoute(flight *ssim.Flight) gtfs.Route {
	return gtfs.Route{
		RouteID:        fmt.Sprintf("%s_%s", flight.AirlineDesignator, flight.FlightNumber),
		AgencyID:       flight.AirlineDesignator,
		RouteShortName: flight.FlightNumber,
		RouteType:      1100, // Air service in GTFS
	}
}

func (c *Converter) convertFlightToTrip(flight *ssim.Flight, routeID, serviceID string) gtfs.Trip {
	tripID := fmt.Sprintf("%s_%s_%s_%s",
		flight.AirlineDesignator,
		flight.FlightNumber,
		flight.DepartureStation,
		flight.ArrivalStation)

	return gtfs.Trip{
		RouteID:      routeID,
		ServiceID:    serviceID,
		TripID:       tripID,
		TripHeadsign: flight.ArrivalStation,
	}
}

func (c *Converter) convertFlightToStopTimes(flight *ssim.Flight, tripID string) []gtfs.StopTime {
	return []gtfs.StopTime{
		{
			TripID:        tripID,
			ArrivalTime:   "", // No arrival at origin
			DepartureTime: gtfs.FormatGTFSTime(flight.STD),
			StopID:        flight.DepartureStation,
			StopSequence:  1,
		},
		{
			TripID:        tripID,
			ArrivalTime:   gtfs.FormatGTFSTime(flight.STA),
			DepartureTime: "", // No departure at destination
			StopID:        flight.ArrivalStation,
			StopSequence:  2,
		},
	}
}

func (c *Converter) convertFlightToCalendar(flight *ssim.Flight, serviceID string) gtfs.Calendar {
	return gtfs.Calendar{
		ServiceID: serviceID,
		Monday:    boolToInt(flight.DaysOfOperation[0]),
		Tuesday:   boolToInt(flight.DaysOfOperation[1]),
		Wednesday: boolToInt(flight.DaysOfOperation[2]),
		Thursday:  boolToInt(flight.DaysOfOperation[3]),
		Friday:    boolToInt(flight.DaysOfOperation[4]),
		Saturday:  boolToInt(flight.DaysOfOperation[5]),
		Sunday:    boolToInt(flight.DaysOfOperation[6]),
		StartDate: gtfs.FormatGTFSDate(flight.PeriodOfOperation.Start),
		EndDate:   gtfs.FormatGTFSDate(flight.PeriodOfOperation.End),
	}
}

func (c *Converter) createServiceID(flight *ssim.Flight) string {
	// Create unique service ID based on days of operation and period
	daysStr := ""
	for i, operates := range flight.DaysOfOperation {
		if operates {
			daysStr += fmt.Sprintf("%d", i+1)
		}
	}
	return fmt.Sprintf("%s_%s_%s_%s",
		flight.AirlineDesignator,
		daysStr,
		gtfs.FormatGTFSDate(flight.PeriodOfOperation.Start),
		gtfs.FormatGTFSDate(flight.PeriodOfOperation.End))
}

func (c *Converter) addAirport(iataCode string) {
	if _, exists := c.airports[iataCode]; !exists {
		stop := &gtfs.Stop{
			StopID:       iataCode,
			StopName:     iataCode + " Airport",
			StopLat:      0.0,
			StopLon:      0.0,
			LocationType: 1,
		}

		// Apply enrichment data if available
		if c.enrichmentData != nil {
			if info, exists := c.enrichmentData.GetAirport(iataCode); exists {
				stop.StopName = info.Name
				stop.StopLat = info.Latitude
				stop.StopLon = info.Longitude
				// Note: GTFS Stop doesn't have timezone field in basic spec
				// but it's used in stop_times.txt interpretation
			}
		}

		c.airports[iataCode] = stop
	}
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// ExtractCodes extracts all airline and airport codes from SSIM records
func ExtractCodes(ssimRecords []interface{}) (airlineCodes []string, airportCodes []string) {
	airlines := make(map[string]bool)
	airports := make(map[string]bool)

	for _, record := range ssimRecords {
		switch r := record.(type) {
		case *ssim.Carrier:
			airlines[r.AirlineDesignator] = true
		case *ssim.Flight:
			airlines[r.AirlineDesignator] = true
			airports[r.DepartureStation] = true
			airports[r.ArrivalStation] = true
		}
	}

	for code := range airlines {
		airlineCodes = append(airlineCodes, code)
	}
	for code := range airports {
		airportCodes = append(airportCodes, code)
	}

	return airlineCodes, airportCodes
}

// GetMissingFields returns information about SSIM fields that cannot be mapped to GTFS
func GetMissingFields() []string {
	return []string{
		"Airport coordinates (lat/lon) - SSIM only has IATA codes, coordinates must be looked up separately",
		"Agency URL - Not present in SSIM",
		"Agency timezone - SSIM has UTC variations per flight, not per agency",
		"Stop timezone - Not explicitly in SSIM",
		"Aircraft type - GTFS doesn't have a field for this",
		"Service type (passenger/cargo/etc) - No direct GTFS equivalent",
		"Itinerary variation identifier - GTFS uses different trip modeling",
		"Leg sequence number - GTFS models this differently with stop_sequence",
		"UTC time variations - GTFS uses local times at stops",
		"Creator reference - Administrative data not in GTFS",
		"Flight-specific codes (meal service, smoking, etc) - Not in core GTFS",
		"Passenger reservations booking designator - Not in GTFS",
		"Aircraft configuration/seating - Not in core GTFS",
		"Traffic restriction codes - Not in GTFS",
		"Operating/marketing carrier distinctions - GTFS has limited codeshare support",
	}
}

// GetGTFSFieldsMissingInSSIM returns GTFS fields that can't be populated from SSIM
func GetGTFSFieldsMissingInSSIM() []string {
	return []string{
		"stop_lat and stop_lon - Must be added from airport database",
		"agency_url - Must be provided manually or from external source",
		"agency_timezone - Must be determined from airline headquarters or route",
		"stop_timezone - Should be derived from airport location",
		"trip_short_name - Can be derived but not explicit in SSIM",
		"route_long_name - Not in SSIM",
		"route_desc - Not in SSIM",
		"route_color and route_text_color - Not in SSIM",
		"shape_id and shapes.txt - Flight paths not in SSIM",
		"block_id - Aircraft rotation not explicit in SSIM format",
		"wheelchair_accessible - Not in SSIM",
		"bikes_allowed - Not applicable for air travel",
	}
}

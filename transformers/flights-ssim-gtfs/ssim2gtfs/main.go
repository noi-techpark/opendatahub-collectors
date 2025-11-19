// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/patrickbr/gtfsparser"
	"github.com/patrickbr/gtfsparser/gtfs"
	"github.com/patrickbr/gtfswriter"
	"opendatahub.com/ssim2gtfs/ssim"
)

func main() {
	inputFile := flag.String("input", "", "Input SSIM file path")
	outputDir := flag.String("output", "gtfs_output", "Output directory for GTFS files")
	agencyName := flag.String("agency", "", "Agency name (optional, uses airline code if not provided)")
	agencyURL := flag.String("url", "http://example.com", "Agency URL")
	agencyTimezone := flag.String("timezone", "UTC", "Agency timezone")

	flag.Parse()

	if *inputFile == "" {
		log.Fatal("Input file is required. Use -input flag")
	}

	// Parse SSIM file
	file, err := os.Open(*inputFile)
	if err != nil {
		log.Fatalf("Error opening file: %v", err)
	}
	defer file.Close()

	parser := ssim.NewParser()
	ssimData, err := parser.Parse(file)
	if err != nil {
		log.Fatalf("Error parsing SSIM: %v", err)
	}

	// Convert to GTFS
	converter := NewSSIMToGTFSConverter(*agencyName, *agencyURL, *agencyTimezone)
	err = converter.Convert(ssimData, *outputDir)
	if err != nil {
		log.Fatalf("Error converting to GTFS: %v", err)
	}

	fmt.Printf("Successfully converted SSIM to GTFS in directory: %s\n", *outputDir)
}

type SSIMToGTFSConverter struct {
	agencyName     string
	agencyURL      string
	agencyTimezone string
	writer         *gtfswriter.Writer
	stopCache      map[string]*gtfs.Stop
	routeCache     map[string]*gtfs.Route
	serviceCache   map[string]*gtfs.Service
}

func NewSSIMToGTFSConverter(agencyName, agencyURL, timezone string) *SSIMToGTFSConverter {
	return &SSIMToGTFSConverter{
		agencyName:     agencyName,
		agencyURL:      agencyURL,
		agencyTimezone: timezone,
		stopCache:      make(map[string]*gtfs.Stop),
		routeCache:     make(map[string]*gtfs.Route),
		serviceCache:   make(map[string]*gtfs.Service),
	}
}

func (c *SSIMToGTFSConverter) Convert(ssimData *ssim.SSIM, outputDir string) error {
	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Initialize GTFS writer
	c.writer = &gtfswriter.Writer{}

	// Create agency
	agencyName := c.agencyName
	if agencyName == "" {
		agencyName = ssimData.Header.AirlineDesignator
	}

	agency := &gtfs.Agency{
		Id:       ssimData.Header.AirlineDesignator,
		Name:     agencyName,
		Url:      c.agencyURL,
		Timezone: c.agencyTimezone,
	}
	c.writer.AddAgency(agency)

	// Process flights
	for _, flight := range ssimData.Flights {
		if err := c.processFlight(flight, ssimData.Header.AirlineDesignator); err != nil {
			log.Printf("Warning: error processing flight %s%s: %v",
				flight.Leg.AirlineDesignator,
				flight.Leg.FlightNumber,
				err)
			continue
		}
	}

	// Write GTFS files
	if err := c.writer.Write(outputDir); err != nil {
		return fmt.Errorf("failed to write GTFS: %w", err)
	}

	return nil
}

func (c *SSIMToGTFSConverter) processFlight(flight ssim.Flight, airlineCode string) error {
	// Create or get stops
	depStop := c.getOrCreateStop(flight.Leg.DepartureStation)
	arrStop := c.getOrCreateStop(flight.Leg.ArrivalStation)

	// Create or get route
	routeID := fmt.Sprintf("%s%s", flight.Leg.AirlineDesignator, flight.Leg.FlightNumber)
	route := c.getOrCreateRoute(routeID, flight.Leg.AirlineDesignator)

	// Create service (calendar)
	serviceID := c.createService(flight)

	// Parse times
	depTime, err := parseSSIMTime(flight.Leg.PassengerSTD)
	if err != nil {
		return fmt.Errorf("invalid departure time: %w", err)
	}

	arrTime, err := parseSSIMTime(flight.Leg.PassengerSTA)
	if err != nil {
		return fmt.Errorf("invalid arrival time: %w", err)
	}

	// Create trip
	tripID := fmt.Sprintf("%s_%s_%s", routeID, flight.Leg.PeriodStart, flight.Leg.LegSequence)
	trip := &gtfsparser.Trip{
		Id:         tripID,
		Route:      route,
		Service:    c.serviceCache[serviceID],
		Headsign:   flight.Leg.ArrivalStation,
		Short_name: flight.Leg.FlightNumber,
	}
	c.writer.AddTrip(trip)

	// Create stop times
	depStopTime := &gtfsparser.StopTime{
		Trip:           trip,
		Stop:           depStop,
		Arrival_time:   depTime,
		Departure_time: depTime,
		Sequence:       1,
		Pickup_type:    0,
		Drop_off_type:  1,
		Timepoint:      true,
	}
	c.writer.AddStopTime(depStopTime)

	arrStopTime := &gtfsparser.StopTime{
		Trip:           trip,
		Stop:           arrStop,
		Arrival_time:   arrTime,
		Departure_time: arrTime,
		Sequence:       2,
		Pickup_type:    1,
		Drop_off_type:  0,
		Timepoint:      true,
	}
	c.writer.AddStopTime(arrStopTime)

	return nil
}

func (c *SSIMToGTFSConverter) getOrCreateStop(stationCode string) *gtfsparser.Stop {
	if stop, exists := c.stopCache[stationCode]; exists {
		return stop
	}

	stop := &gtfsparser.Stop{
		Id:            stationCode,
		Name:          stationCode + " Airport",
		Lat:           0.0, // You would need to look up actual coordinates
		Lon:           0.0,
		Location_type: 0,
	}
	c.writer.AddStop(stop)
	c.stopCache[stationCode] = stop
	return stop
}

func (c *SSIMToGTFSConverter) getOrCreateRoute(routeID, airlineCode string) *gtfsparser.Route {
	if route, exists := c.routeCache[routeID]; exists {
		return route
	}

	route := &gtfsparser.Route{
		Id:         routeID,
		Agency:     c.writer.Agencies[0], // First (and only) agency
		Short_name: routeID,
		Long_name:  fmt.Sprintf("Flight %s", routeID),
		Type:       1100, // Air service
		Color:      "0178BC",
		Text_color: "FFFFFF",
	}
	c.writer.AddRoute(route)
	c.routeCache[routeID] = route
	return route
}

func (c *SSIMToGTFSConverter) createService(flight ssim.Flight) string {
	serviceID := fmt.Sprintf("SVC_%s_%s_%s_%s",
		flight.Leg.AirlineDesignator,
		flight.Leg.FlightNumber,
		flight.Leg.PeriodStart,
		flight.Leg.DaysOfOperation)

	if _, exists := c.serviceCache[serviceID]; exists {
		return serviceID
	}

	// Parse start and end dates
	startDate, err := parseSSIMDate(flight.Leg.PeriodStart)
	if err != nil {
		log.Printf("Warning: invalid start date %s, using today", flight.Leg.PeriodStart)
		startDate = time.Now()
	}

	endDate, err := parseSSIMDate(flight.Leg.PeriodEnd)
	if err != nil {
		log.Printf("Warning: invalid end date %s, using start date + 1 year", flight.Leg.PeriodEnd)
		endDate = startDate.AddDate(1, 0, 0)
	}

	// Parse days of operation (1234567 where 1=Monday, 7=Sunday)
	daysStr := flight.Leg.DaysOfOperation
	daysOfWeek := [7]bool{}
	for i := 0; i < 7 && i < len(daysStr); i++ {
		daysOfWeek[i] = daysStr[i] != ' ' && daysStr[i] != '0'
	}

	service := &gtfsparser.Service{
		Id: serviceID,
		Daymap: [7]bool{
			daysOfWeek[0], // Monday
			daysOfWeek[1], // Tuesday
			daysOfWeek[2], // Wednesday
			daysOfWeek[3], // Thursday
			daysOfWeek[4], // Friday
			daysOfWeek[5], // Saturday
			daysOfWeek[6], // Sunday
		},
		Start_date: gtfsparser.Date{Day: int8(startDate.Day()), Month: int8(startDate.Month()), Year: int16(startDate.Year())},
		End_date:   gtfsparser.Date{Day: int8(endDate.Day()), Month: int8(endDate.Month()), Year: int16(endDate.Year())},
	}

	c.writer.AddService(service)
	c.serviceCache[serviceID] = service
	return serviceID
}

func parseSSIMTime(timeStr string) (gtfsparser.Time, error) {
	timeStr = strings.TrimSpace(timeStr)
	if len(timeStr) < 4 {
		return gtfsparser.Time{}, fmt.Errorf("invalid time format: %s", timeStr)
	}

	hours, err := strconv.Atoi(timeStr[0:2])
	if err != nil {
		return gtfsparser.Time{}, err
	}

	minutes, err := strconv.Atoi(timeStr[2:4])
	if err != nil {
		return gtfsparser.Time{}, err
	}

	return gtfsparser.Time{
		Hour:   int8(hours),
		Minute: int8(minutes),
		Second: 0,
	}, nil
}

func parseSSIMDate(dateStr string) (time.Time, error) {
	// DDMMMYY format (e.g., 01JAN24)
	dateStr = strings.TrimSpace(dateStr)
	if len(dateStr) < 7 {
		return time.Time{}, fmt.Errorf("invalid date format: %s", dateStr)
	}

	return time.Parse("02Jan06", dateStr[:7])
}

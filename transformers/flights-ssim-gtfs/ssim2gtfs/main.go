// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"flag"
	"fmt"
	"log"
	"net/mail"
	"net/url"
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
	feed           *gtfsparser.Feed
}

func NewSSIMToGTFSConverter(agencyName, agencyURL, timezone string) *SSIMToGTFSConverter {
	return &SSIMToGTFSConverter{
		agencyName:     agencyName,
		agencyURL:      agencyURL,
		agencyTimezone: timezone,
	}
}

func (c *SSIMToGTFSConverter) Convert(ssimData *ssim.SSIM, output string) error {
	if err := createOutputPath(output); err != nil {
		return err
	}

	c.writer = &gtfswriter.Writer{}
	c.feed = gtfsparser.NewFeed()

	// Create agency
	agencyName := c.agencyName
	if agencyName == "" {
		agencyName = ssimData.Carriers[0].AirlineDesignator // TODO: handle multiple agencies
	}
	url, err := url.Parse(c.agencyURL)
	if err != nil {
		return err
	}

	tz, err := gtfs.NewTimezone(c.agencyTimezone)
	if err != nil {
		return err
	}

	lang, err := gtfs.NewLanguageISO6391("en")
	if err != nil {
		return err
	}
	agency := &gtfs.Agency{
		Id:       agencyName, // TODO: handle multiple
		Name:     agencyName,
		Url:      url,
		Timezone: tz,
		Lang:     lang,
	}

	c.feed.Agencies[agency.Id] = agency

	// Create feed info
	if err := createFeedInfo(c); err != nil {
		return err
	}

	// Process flights
	for _, flight := range ssimData.Flights {
		if err := c.processFlight(flight, c.agencyName); err != nil {
			log.Printf("Warning: error processing flight %s%s: %v",
				flight.Leg.AirlineDesignator,
				flight.Leg.FlightNumber,
				err)
			continue
		}
	}

	// Write GTFS files
	if err := c.writer.Write(c.feed, output); err != nil {
		return fmt.Errorf("failed to write GTFS: %w", err)
	}

	return nil
}

func createFeedInfo(c *SSIMToGTFSConverter) error {
	publisherUrl, err := url.Parse("https://opendatahub.com")
	if err != nil {
		return err
	}
	contactEmail, err := mail.ParseAddress("info@opendatahub.com")
	if err != nil {
		return err
	}
	// Feed info
	info := gtfs.FeedInfo{
		Publisher_name: "Open Data Hub",
		Publisher_url:  publisherUrl,
		Lang:           "en", // consider "mul" if we have translations
		Contact_email:  contactEmail,
	}
	c.feed.FeedInfos = append(c.feed.FeedInfos, &info)
	return nil
}

func createOutputPath(output string) error {
	if strings.HasSuffix(strings.ToLower(output), ".zip") {
		if _, err := os.OpenFile("gtfs.zip", os.O_RDONLY|os.O_CREATE, 0666); err != nil {
			return fmt.Errorf("failed to create output file: %w", err)
		}
	} else {
		if err := os.MkdirAll(output, 0755); err != nil {
			return fmt.Errorf("failed to create output directory: %w", err)
		}
	}
	return nil
}

func (c *SSIMToGTFSConverter) processFlight(flight ssim.Flight, airlineCode string) error {
	// Create or get stops
	depStop := c.getOrCreateStop(flight.Leg.DepartureStation)
	arrStop := c.getOrCreateStop(flight.Leg.ArrivalStation)

	// Create or get route
	routeID := fmt.Sprintf("%s%s", airlineCode, flight.Leg.FlightNumber)
	route := c.getOrCreateRoute(routeID, airlineCode)

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
	tripID := fmt.Sprintf("%s_%s_%s", routeID, flight.Leg.PeriodStart, flight.Leg.LegSequenceNumber)
	trip := &gtfs.Trip{
		Id:         tripID,
		Route:      route,
		Service:    c.feed.Services[serviceID],
		Headsign:   &flight.Leg.ArrivalStation,
		Short_name: &flight.Leg.FlightNumber,
	}

	// Create stop times
	depStopTime := gtfs.StopTime{}
	depStopTime.SetStop(depStop)
	depStopTime.SetArrival_time(depTime)
	depStopTime.SetDeparture_time(depTime)
	depStopTime.SetSequence(1)
	depStopTime.SetPickup_type(0)
	depStopTime.SetDrop_off_type(1)
	depStopTime.SetTimepoint(true)
	depStopTime.SetHeadsign(new(string))

	trip.StopTimes = append(trip.StopTimes, depStopTime)

	arrStopTime := gtfs.StopTime{}
	arrStopTime.SetStop(arrStop)
	arrStopTime.SetArrival_time(arrTime)
	arrStopTime.SetDeparture_time(arrTime)
	arrStopTime.SetSequence(2)
	arrStopTime.SetPickup_type(1)
	arrStopTime.SetDrop_off_type(0)
	arrStopTime.SetTimepoint(true)
	arrStopTime.SetHeadsign(new(string))

	trip.StopTimes = append(trip.StopTimes, arrStopTime)

	c.feed.Trips[tripID] = trip

	return nil
}

func (c *SSIMToGTFSConverter) getOrCreateStop(stationCode string) *gtfs.Stop {
	if stop, exists := c.feed.Stops[stationCode]; exists {
		return stop
	}

	stop := &gtfs.Stop{
		Id:            stationCode,
		Name:          stationCode + " Airport",
		Lat:           0.0, // You would need to look up actual coordinates
		Lon:           0.0,
		Location_type: 0,
	}
	c.feed.Stops[stop.Id] = stop
	return stop
}

func (c *SSIMToGTFSConverter) getOrCreateRoute(routeID, airlineCode string) *gtfs.Route {
	if route, exists := c.feed.Routes[routeID]; exists {
		return route
	}

	route := &gtfs.Route{
		Id:         routeID,
		Agency:     c.feed.Agencies[airlineCode],
		Short_name: routeID,
		Long_name:  fmt.Sprintf("Flight %s", routeID),
		Type:       1100, // Air service
		Color:      "0178BC",
		Text_color: "FFFFFF",
	}
	c.feed.Routes[route.Id] = route
	return route
}

func (c *SSIMToGTFSConverter) createService(flight ssim.Flight) string {
	serviceID := fmt.Sprintf("SVC_%s_%s_%s_%s",
		flight.Leg.AirlineDesignator,
		flight.Leg.FlightNumber,
		flight.Leg.PeriodStart,
		flight.Leg.DaysOfOperation)

	if _, exists := c.feed.Services[serviceID]; exists {
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

	service := gtfs.EmptyService()
	service.SetId(serviceID)
	service.SetDaymap(0, daysOfWeek[0]) //monday
	service.SetDaymap(1, daysOfWeek[1])
	service.SetDaymap(2, daysOfWeek[2])
	service.SetDaymap(3, daysOfWeek[3])
	service.SetDaymap(4, daysOfWeek[4])
	service.SetDaymap(5, daysOfWeek[5])
	service.SetDaymap(6, daysOfWeek[6])

	service.SetStart_date(gtfs.NewDate(uint8(startDate.Day()), uint8(startDate.Month()), uint16(startDate.Year())))
	service.SetEnd_date(gtfs.NewDate(uint8(endDate.Day()), uint8(endDate.Month()), uint16(endDate.Year())))

	c.feed.Services[service.Id()] = service
	return serviceID
}

func parseSSIMTime(timeStr string) (gtfs.Time, error) {
	timeStr = strings.TrimSpace(timeStr)
	if len(timeStr) < 4 {
		return gtfs.Time{}, fmt.Errorf("invalid time format: %s", timeStr)
	}

	hours, err := strconv.Atoi(timeStr[0:2])
	if err != nil {
		return gtfs.Time{}, err
	}

	minutes, err := strconv.Atoi(timeStr[2:4])
	if err != nil {
		return gtfs.Time{}, err
	}

	return gtfs.Time{
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

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package ssim2gtfs

import (
	"fmt"
	"log"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/patrickbr/gtfsparser"
	"github.com/patrickbr/gtfsparser/gtfs"
	"github.com/patrickbr/gtfswriter"
	"github.com/umahmood/haversine"
	"github.com/zsefvlol/timezonemapper"
	ssim "opendatahub.com/ssimparser"
)

type SSIMToGTFSConverter struct {
	agencyName     string
	agencyURL      string
	agencyTimezone string
	writer         *gtfswriter.Writer
	feed           *gtfsparser.Feed
	airports       map[string]airport
}

func NewSSIMToGTFSConverter(agencyName string, agencyURL string, timezone string) *SSIMToGTFSConverter {
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

	airports, err := loadAirports("airports.csv")
	if err != nil {
		return err
	}
	c.airports = airports

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
		Id:       agencyName,
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
			return fmt.Errorf("Warning: error processing flight %s%s: %w",
				flight.Leg.AirlineDesignator,
				flight.Leg.FlightNumber,
				err)
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
	agencyTz, err := time.LoadLocation(c.agencyTimezone)
	if err != nil {
		return fmt.Errorf("invalid timezone %s: %w", c.agencyTimezone, err)
	}
	// The timezone thing is a bit iffy because GTFS only has a single timezone per agency (or for each stop, but we don't have those)
	// Since the agency timezone can be e.g. Europe/Rome it automatically applies daylight savings, but the ssim times are all in UTC + offset.
	// To estimate the actual date of the flight, we take the start date of the schedule.
	// But if the schedule includes a daylight savings switchover, we're probably giving out wrong times
	// This can be avoided by using something fixed like UTC or CET as agency timezone.
	depTime, err := ssimTime2Local(flight.Leg.PeriodStart+flight.Leg.PassengerSTD+flight.Leg.DepartureUTCOffset, agencyTz)
	if err != nil {
		return fmt.Errorf("invalid departure time: %w", err)
	}

	arrTime, err := ssimTime2Local(flight.Leg.PeriodStart+flight.Leg.PassengerSTA+flight.Leg.ArrivalUTCOffset, agencyTz)
	if err != nil {
		return fmt.Errorf("invalid arrival time: %w", err)
	}

	// Create trip
	tripID := fmt.Sprintf("%s_%s_%s", routeID, flight.Leg.PeriodStart, flight.Leg.LegSequenceNumber)
	headsign := fmt.Sprintf("%s -> %s", flight.Leg.DepartureStation, flight.Leg.ArrivalStation)
	trip := &gtfs.Trip{
		Id:         tripID,
		Route:      route,
		Service:    c.feed.Services[serviceID],
		Headsign:   &headsign,
		Short_name: &flight.Leg.FlightNumber,
	}

	// calculate distance between departure and arrival airport
	_, distance := haversine.Distance(
		haversine.Coord{Lat: float64(depStop.Lat), Lon: float64(depStop.Lon)},
		haversine.Coord{Lat: float64(arrStop.Lat), Lon: float64(arrStop.Lon)},
	)

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
	arrStopTime.SetShape_dist_traveled(float32(distance))

	trip.StopTimes = append(trip.StopTimes, arrStopTime)

	// create shape for trip
	shape := gtfs.Shape{
		Id: tripID,
		Points: gtfs.ShapePoints{
			gtfs.ShapePoint{Lat: depStop.Lat, Lon: depStop.Lon, Sequence: 0, Dist_traveled: 0},
			gtfs.ShapePoint{Lat: arrStop.Lat, Lon: arrStop.Lon, Sequence: 1, Dist_traveled: float32(distance)},
		},
	}
	trip.Shape = &shape
	c.feed.Shapes[shape.Id] = &shape

	c.feed.Trips[tripID] = trip

	return nil
}

func (c *SSIMToGTFSConverter) getOrCreateStop(iataCode string) *gtfs.Stop {
	if stop, exists := c.feed.Stops[iataCode]; exists {
		return stop
	}
	airport := c.airports[iataCode]

	// map gps point to timezone
	tzString := timezonemapper.LatLngToTimezoneString(airport.LatitudeDeg, airport.LongitudeDeg)
	tz, err := gtfs.NewTimezone(tzString)
	if err != nil {
		tz, _ = gtfs.NewTimezone("") // empty timezone, defaults to agency
	}
	url, _ := url.Parse(airport.HomeLink)

	stop := &gtfs.Stop{
		Id:            iataCode,
		Name:          airport.Name,
		Lat:           float32(airport.LatitudeDeg),
		Lon:           float32(airport.LongitudeDeg),
		Location_type: 0,
		Timezone:      tz,
		Url:           url,
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
		// Color:      "0178BC",
		// Text_color: "FFFFFF",
	}
	c.feed.Routes[route.Id] = route
	return route
}

func (c *SSIMToGTFSConverter) createService(flight ssim.Flight) string {
	serviceID := fmt.Sprintf("SVC_%s_%s_%s_%s",
		flight.Leg.AirlineDesignator,
		flight.Leg.FlightNumber,
		flight.Leg.PeriodStart,
		strings.ReplaceAll(flight.Leg.DaysOfOperation, " ", ""))

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
	for i := 0; i < 7; i++ {
		daysOfWeek[i] = daysStr[i] != ' ' && daysStr[i] != '0'
		// log.Printf("day %s %d %s to bool %v", daysStr, i, string(daysStr[i]), daysOfWeek[i])
	}

	service := gtfs.EmptyService()
	service.SetId(serviceID)
	service.SetDaymap(1, daysOfWeek[0]) //monday
	service.SetDaymap(2, daysOfWeek[1])
	service.SetDaymap(3, daysOfWeek[2])
	service.SetDaymap(4, daysOfWeek[3])
	service.SetDaymap(5, daysOfWeek[4])
	service.SetDaymap(6, daysOfWeek[5])
	service.SetDaymap(0, daysOfWeek[6]) //sunday

	// log.Printf("daymap in:%v, map:%v,  out:%#b", flight.Leg.DaysOfOperation, daysOfWeek, service.RawDaymap())

	service.SetStart_date(gtfs.NewDate(uint8(startDate.Day()), uint8(startDate.Month()), uint16(startDate.Year())))
	service.SetEnd_date(gtfs.NewDate(uint8(endDate.Day()), uint8(endDate.Month()), uint16(endDate.Year())))

	c.feed.Services[service.Id()] = service
	return serviceID
}

func ssimTime2Local(dateTimeStr string, localTz *time.Location) (gtfs.Time, error) {
	t, err := time.Parse("02Jan061504-0700", dateTimeStr)
	if err != nil {
		return gtfs.Time{}, err
	}

	converted := t.In(localTz)
	return gtfs.Time{
		Hour:   int8(converted.Hour()),
		Minute: int8(converted.Minute()),
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

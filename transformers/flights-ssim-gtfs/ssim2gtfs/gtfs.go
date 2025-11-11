// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package gtfs

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// GTFS represents a complete GTFS feed
type GTFS struct {
	Agency       []Agency
	Stops        []Stop
	Routes       []Route
	Trips        []Trip
	StopTimes    []StopTime
	Calendar     []Calendar
	CalendarDates []CalendarDate
}

// Agency represents agency.txt
type Agency struct {
	AgencyID       string
	AgencyName     string
	AgencyURL      string
	AgencyTimezone string
	AgencyLang     string
}

// Stop represents stops.txt
type Stop struct {
	StopID        string
	StopName      string
	StopLat       float64
	StopLon       float64
	LocationType  int
}

// Route represents routes.txt
type Route struct {
	RouteID        string
	AgencyID       string
	RouteShortName string
	RouteType      int // 1100 for air service
}

// Trip represents trips.txt
type Trip struct {
	RouteID     string
	ServiceID   string
	TripID      string
	TripHeadsign string
}

// StopTime represents stop_times.txt
type StopTime struct {
	TripID            string
	ArrivalTime       string
	DepartureTime     string
	StopID            string
	StopSequence      int
}

// Calendar represents calendar.txt
type Calendar struct {
	ServiceID string
	Monday    int
	Tuesday   int
	Wednesday int
	Thursday  int
	Friday    int
	Saturday  int
	Sunday    int
	StartDate string
	EndDate   string
}

// CalendarDate represents calendar_dates.txt
type CalendarDate struct {
	ServiceID     string
	Date          string
	ExceptionType int
}

// Writer handles writing GTFS files
type Writer struct {
	outputDir string
}

// NewWriter creates a new GTFS writer
func NewWriter(outputDir string) *Writer {
	return &Writer{outputDir: outputDir}
}

// Write writes all GTFS files to the output directory
func (w *Writer) Write(gtfs *GTFS) error {
	if err := os.MkdirAll(w.outputDir, 0755); err != nil {
		return err
	}

	if err := w.writeAgency(gtfs.Agency); err != nil {
		return err
	}
	if err := w.writeStops(gtfs.Stops); err != nil {
		return err
	}
	if err := w.writeRoutes(gtfs.Routes); err != nil {
		return err
	}
	if err := w.writeTrips(gtfs.Trips); err != nil {
		return err
	}
	if err := w.writeStopTimes(gtfs.StopTimes); err != nil {
		return err
	}
	if err := w.writeCalendar(gtfs.Calendar); err != nil {
		return err
	}
	if err := w.writeCalendarDates(gtfs.CalendarDates); err != nil {
		return err
	}

	return nil
}

func (w *Writer) writeAgency(agencies []Agency) error {
	file, err := os.Create(filepath.Join(w.outputDir, "agency.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"agency_id", "agency_name", "agency_url", "agency_timezone", "agency_lang"})
	for _, a := range agencies {
		writer.Write([]string{a.AgencyID, a.AgencyName, a.AgencyURL, a.AgencyTimezone, a.AgencyLang})
	}
	return nil
}

func (w *Writer) writeStops(stops []Stop) error {
	file, err := os.Create(filepath.Join(w.outputDir, "stops.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"stop_id", "stop_name", "stop_lat", "stop_lon", "location_type"})
	for _, s := range stops {
		writer.Write([]string{
			s.StopID,
			s.StopName,
			fmt.Sprintf("%.6f", s.StopLat),
			fmt.Sprintf("%.6f", s.StopLon),
			fmt.Sprintf("%d", s.LocationType),
		})
	}
	return nil
}

func (w *Writer) writeRoutes(routes []Route) error {
	file, err := os.Create(filepath.Join(w.outputDir, "routes.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"route_id", "agency_id", "route_short_name", "route_type"})
	for _, r := range routes {
		writer.Write([]string{r.RouteID, r.AgencyID, r.RouteShortName, fmt.Sprintf("%d", r.RouteType)})
	}
	return nil
}

func (w *Writer) writeTrips(trips []Trip) error {
	file, err := os.Create(filepath.Join(w.outputDir, "trips.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"route_id", "service_id", "trip_id", "trip_headsign"})
	for _, t := range trips {
		writer.Write([]string{t.RouteID, t.ServiceID, t.TripID, t.TripHeadsign})
	}
	return nil
}

func (w *Writer) writeStopTimes(stopTimes []StopTime) error {
	file, err := os.Create(filepath.Join(w.outputDir, "stop_times.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"trip_id", "arrival_time", "departure_time", "stop_id", "stop_sequence"})
	for _, st := range stopTimes {
		writer.Write([]string{
			st.TripID,
			st.ArrivalTime,
			st.DepartureTime,
			st.StopID,
			fmt.Sprintf("%d", st.StopSequence),
		})
	}
	return nil
}

func (w *Writer) writeCalendar(calendars []Calendar) error {
	if len(calendars) == 0 {
		return nil
	}

	file, err := os.Create(filepath.Join(w.outputDir, "calendar.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"service_id", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday", "sunday", "start_date", "end_date"})
	for _, c := range calendars {
		writer.Write([]string{
			c.ServiceID,
			fmt.Sprintf("%d", c.Monday),
			fmt.Sprintf("%d", c.Tuesday),
			fmt.Sprintf("%d", c.Wednesday),
			fmt.Sprintf("%d", c.Thursday),
			fmt.Sprintf("%d", c.Friday),
			fmt.Sprintf("%d", c.Saturday),
			fmt.Sprintf("%d", c.Sunday),
			c.StartDate,
			c.EndDate,
		})
	}
	return nil
}

func (w *Writer) writeCalendarDates(dates []CalendarDate) error {
	if len(dates) == 0 {
		return nil
	}

	file, err := os.Create(filepath.Join(w.outputDir, "calendar_dates.txt"))
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	writer.Write([]string{"service_id", "date", "exception_type"})
	for _, d := range dates {
		writer.Write([]string{d.ServiceID, d.Date, fmt.Sprintf("%d", d.ExceptionType)})
	}
	return nil
}

// FormatGTFSDate converts time.Time to GTFS date format (YYYYMMDD)
func FormatGTFSDate(t time.Time) string {
	return t.Format("20060102")
}

// FormatGTFSTime converts HHMM string to GTFS time format (HH:MM:SS)
func FormatGTFSTime(hhmm string) string {
	if len(hhmm) != 4 {
		return "00:00:00"
	}
	return fmt.Sprintf("%s:%s:00", hhmm[:2], hhmm[2:])
}

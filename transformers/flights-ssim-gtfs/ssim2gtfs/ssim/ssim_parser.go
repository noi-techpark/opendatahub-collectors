// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package ssim

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"
)

// SSIM represents the complete parsed SSIM file
type SSIM struct {
	Header  HeaderRecord
	Flights []Flight
	Trailer TrailerRecord
}

// HeaderRecord represents Type 1 record
type HeaderRecord struct {
	RecordType        string
	TimeMode          string
	AirlineDesignator string
	Title             string
	DataSetSerial     string
}

// Flight represents a complete flight with Type 2, 3, and 4 records
type Flight struct {
	Leg     FlightLegRecord
	Segment *SegmentDataRecord
	SegLegs []SegmentLegRecord
}

// FlightLegRecord represents Type 2 record
type FlightLegRecord struct {
	RecordType            string
	OperationalSuffix     string
	AirlineDesignator     string
	FlightNumber          string
	ItineraryVariation    string
	LegSequence           string
	ServiceType           string
	PeriodStart           string
	PeriodEnd             string
	DaysOfOperation       string
	DepartureStation      string
	PassengerSTD          string
	AircraftSTD           string
	DepartureUTCVariation string
	ArrivalStation        string
	AircraftSTA           string
	PassengerSTA          string
	ArrivalUTCVariation   string
}

// SegmentDataRecord represents Type 3 record
type SegmentDataRecord struct {
	RecordType         string
	AirlineDesignator  string
	FlightNumber       string
	ItineraryVariation string
	LegSequence        string
	AircraftType       string
	Configuration      string
	DateVariation      string
}

// SegmentLegRecord represents Type 4 record
type SegmentLegRecord struct {
	RecordType         string
	AirlineDesignator  string
	FlightNumber       string
	ItineraryVariation string
	LegSequence        string
	Station            string
}

// TrailerRecord represents Type 5 record
type TrailerRecord struct {
	RecordType   string
	SerialNumber string
	RecordCount  string
}

// Parser handles SSIM file parsing
type Parser struct{}

// NewParser creates a new SSIM parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse reads and parses an SSIM file
func (p *Parser) Parse(reader io.Reader) (*SSIM, error) {
	ssim := &SSIM{
		Flights: []Flight{},
	}

	scanner := bufio.NewScanner(reader)
	var currentFlight *Flight
	var flightKey string

	flightMap := make(map[string]*Flight)

	for scanner.Scan() {
		line := scanner.Text()
		if len(line) == 0 {
			continue
		}

		// Ensure line is at least 200 chars by padding
		if len(line) < 200 {
			line = fmt.Sprintf("%-200s", line)
		}

		recordType := string(line[0])

		switch recordType {
		case "1":
			ssim.Header = p.parseHeader(line)
		case "2":
			leg := p.parseFlightLeg(line)
			flightKey = p.getFlightKey(leg)

			if existingFlight, exists := flightMap[flightKey]; exists {
				currentFlight = existingFlight
			} else {
				currentFlight = &Flight{Leg: leg}
				flightMap[flightKey] = currentFlight
				ssim.Flights = append(ssim.Flights, *currentFlight)
			}

		case "3":
			if currentFlight != nil {
				seg := p.parseSegmentData(line)
				currentFlight.Segment = &seg
			}

		case "4":
			if currentFlight != nil {
				segLeg := p.parseSegmentLeg(line)
				currentFlight.SegLegs = append(currentFlight.SegLegs, segLeg)
			}

		case "5":
			ssim.Trailer = p.parseTrailer(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	return ssim, nil
}

func (p *Parser) getFlightKey(leg FlightLegRecord) string {
	return fmt.Sprintf("%s-%s-%s-%s",
		leg.AirlineDesignator,
		leg.FlightNumber,
		leg.ItineraryVariation,
		leg.LegSequence)
}

func (p *Parser) parseHeader(line string) HeaderRecord {
	return HeaderRecord{
		RecordType:        p.extractField(line, 0, 2),
		TimeMode:          p.extractField(line, 2, 5),
		AirlineDesignator: p.extractField(line, 5, 8),
		Title:             p.extractField(line, 9, 24),
		DataSetSerial:     p.extractField(line, 24, 34),
	}
}

func (p *Parser) parseFlightLeg(line string) FlightLegRecord {
	return FlightLegRecord{
		RecordType:            p.extractField(line, 0, 1),
		OperationalSuffix:     p.extractField(line, 1, 2),
		AirlineDesignator:     p.extractField(line, 3, 5),
		FlightNumber:          p.extractField(line, 6, 10),
		ItineraryVariation:    p.extractField(line, 11, 13),
		LegSequence:           p.extractField(line, 14, 15),
		ServiceType:           p.extractField(line, 16, 18),
		PeriodStart:           p.extractField(line, 20, 28),
		PeriodEnd:             p.extractField(line, 28, 36),
		DaysOfOperation:       p.extractField(line, 36, 43),
		DepartureStation:      p.extractField(line, 44, 48),
		PassengerSTD:          p.extractField(line, 49, 53),
		AircraftSTD:           p.extractField(line, 54, 58),
		DepartureUTCVariation: p.extractField(line, 59, 63),
		ArrivalStation:        p.extractField(line, 64, 68),
		AircraftSTA:           p.extractField(line, 69, 73),
		PassengerSTA:          p.extractField(line, 74, 78),
		ArrivalUTCVariation:   p.extractField(line, 79, 83),
	}
}

func (p *Parser) parseSegmentData(line string) SegmentDataRecord {
	return SegmentDataRecord{
		RecordType:         p.extractField(line, 0, 1),
		AirlineDesignator:  p.extractField(line, 3, 5),
		FlightNumber:       p.extractField(line, 6, 10),
		ItineraryVariation: p.extractField(line, 11, 13),
		LegSequence:        p.extractField(line, 14, 16),
		AircraftType:       p.extractField(line, 17, 20),
		Configuration:      p.extractField(line, 28, 48),
		DateVariation:      p.extractField(line, 49, 70),
	}
}

func (p *Parser) parseSegmentLeg(line string) SegmentLegRecord {
	return SegmentLegRecord{
		RecordType:         p.extractField(line, 0, 1),
		AirlineDesignator:  p.extractField(line, 3, 5),
		FlightNumber:       p.extractField(line, 6, 10),
		ItineraryVariation: p.extractField(line, 11, 13),
		LegSequence:        p.extractField(line, 14, 16),
		Station:            p.extractField(line, 17, 21),
	}
}

func (p *Parser) parseTrailer(line string) TrailerRecord {
	return TrailerRecord{
		RecordType:   p.extractField(line, 0, 1),
		SerialNumber: p.extractField(line, 1, 6),
		RecordCount:  p.extractField(line, 7, 12),
	}
}

func (p *Parser) extractField(line string, start, end int) string {
	if end > len(line) {
		end = len(line)
	}
	if start >= len(line) {
		return ""
	}
	return strings.TrimSpace(line[start:end])
}

// Helper methods for Flight

// GetDepartureTime parses the departure time
func (f *Flight) GetDepartureTime() (time.Time, error) {
	return parseSSIMDateTime(f.Leg.PeriodStart, f.Leg.PassengerSTD)
}

// GetArrivalTime parses the arrival time
func (f *Flight) GetArrivalTime() (time.Time, error) {
	return parseSSIMDateTime(f.Leg.PeriodStart, f.Leg.PassengerSTA)
}

// GetOperatingDays returns which days of the week the flight operates (1=Monday, 7=Sunday)
func (f *Flight) GetOperatingDays() []int {
	days := []int{}
	for i, char := range f.Leg.DaysOfOperation {
		if i < 7 && char != ' ' && char != '0' {
			days = append(days, i+1)
		}
	}
	return days
}

func parseSSIMDateTime(dateStr, timeStr string) (time.Time, error) {
	// DDMMMYY format for date, HHMM for time
	if len(dateStr) < 7 || len(timeStr) < 4 {
		return time.Time{}, fmt.Errorf("invalid date/time format")
	}

	// This is a simplified parser - full implementation would need proper date parsing
	return time.Parse("02Jan06 1504", dateStr[:7]+" "+timeStr[:4])
}

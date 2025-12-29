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
	Header   HeaderRecord
	Carriers []CarrierRecord
	Flights  []Flight
	Trailer  TrailerRecord
}

// HeaderRecord represents Type 1 record
type HeaderRecord struct {
	RecordType          string
	TitleOfContents     string
	NumberOfSeasons     string
	DataSetSerialNumber string
	RecordSerialNumber  string
}

// CarrierRecord represents Type 2 record
type CarrierRecord struct {
	RecordType                       string
	TimeMode                         string
	AirlineDesignator                string
	Season                           string
	ValidityStart                    string
	ValidityEnd                      string
	CreationDate                     string
	TitleOfData                      string
	ReleaseDate                      string
	ScheduleStatus                   string
	CreatorReference                 string
	DuplicateAirlineDesignatorMarker string
	GeneralInformation               string
	InFlightServiceInformation       string
	ElectronicTicketingInformation   string
	CreationTime                     string
	RecordSerialNumber               string
}

// Flight represents a complete flight with Type 3 and 4 records
type Flight struct {
	Leg      FlightLegRecord
	Segments []SegmentDataRecord
}

// FlightLegRecord represents Type 3 record
type FlightLegRecord struct {
	RecordType                             string
	OperationalSuffix                      string
	AirlineDesignator                      string
	FlightNumber                           string
	ItineraryVariationIdentifier           string
	LegSequenceNumber                      string
	ServiceType                            string
	PeriodStart                            string
	PeriodEnd                              string
	DaysOfOperation                        string
	FrequencyRate                          string
	DepartureStation                       string
	PassengerSTD                           string // scheduled time of departure
	AircraftSTD                            string
	DepartureUTCOffset                     string
	DepartureTerminal                      string
	ArrivalStation                         string
	AircraftSTA                            string // scheduled time of arrival
	PassengerSTA                           string
	ArrivalUTCOffset                       string
	ArrivalTerminal                        string
	AircraftType                           string
	PassengerReservationsBookingDesignator string
	PassengerReservationsBookingModifier   string
	MealServiceNote                        string
	JointOperationAirlineDesignators       string
	MinimumConnectionTimeStatus            string
	SecureFlightIndicator                  string
	ItineraryVariationIdentifierOverflow   string
	AircraftOwner                          string
	CockpitCrewEmployer                    string
	CabinCrewEmployer                      string
	OnwardAirlineDesignator                string
	OnwardFlightNumber                     string
	OnwardAircraftRotationLayover          string
	OnwardOperationalSuffix                string
	FlightTransitLayover                   string
	OperatingAirlineDisclosure             string
	TrafficRestrictionCode                 string
	TrafficRestrictionCodeOverflow         string
	AircraftConfiguration                  string
	DateVariation                          string
	RecordSerialNumber                     string
}

// SegmentDataRecord represents Type 4 record
type SegmentDataRecord struct {
	RecordType                           string
	OperationalSuffix                    string
	AirlineDesignator                    string
	FlightNumber                         string
	ItineraryVariationIdentifier         string
	LegSequenceNumber                    string
	ServiceType                          string
	ItineraryVariationIdentifierOverflow string
	BoardPointIndicator                  string
	OffPointIndicator                    string
	DataElementIdentifier                string
	SegmentBoardPoint                    string
	SegmentOffPoint                      string
	Data                                 string
	RecordSerialNumber                   string
}

// TrailerRecord represents Type 5 record
type TrailerRecord struct {
	RecordType                 string
	AirlineDesignator          string
	ReleaseDate                string
	SerialNumberCheckReference string
	ContinuationEndCode        string
	RecordSerialNumber         string
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
		Carriers: []CarrierRecord{},
		Flights:  []Flight{},
	}

	scanner := bufio.NewScanner(reader)
	var currentFlight *Flight

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
			carrier := p.parseCarrier(line)
			ssim.Carriers = append(ssim.Carriers, carrier)
		case "3":
			leg := p.parseFlightLeg(line)
			currentFlight = &Flight{
				Leg:      leg,
				Segments: []SegmentDataRecord{},
			}
			ssim.Flights = append(ssim.Flights, *currentFlight)
		case "4":
			if currentFlight != nil {
				seg := p.parseSegmentData(line)
				currentFlight.Segments = append(currentFlight.Segments, seg)
			}
		case "5":
			ssim.Trailer = p.parseTrailer(line)
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	if len(ssim.Carriers)+len(ssim.Flights) == 0 {
		return ssim, fmt.Errorf("could not parse any flights or carriers from ssim file. Maybe invalid format or empty?")
	}

	return ssim, nil
}

func (p *Parser) parseHeader(line string) HeaderRecord {
	return HeaderRecord{
		RecordType:          p.extractField(line, 0, 1),
		TitleOfContents:     p.extractField(line, 1, 35),
		NumberOfSeasons:     p.extractField(line, 40, 41),
		DataSetSerialNumber: p.extractField(line, 191, 194),
		RecordSerialNumber:  p.extractField(line, 194, 200),
	}
}

func (p *Parser) parseCarrier(line string) CarrierRecord {
	return CarrierRecord{
		RecordType:                       p.extractField(line, 0, 1),
		TimeMode:                         p.extractField(line, 1, 2),
		AirlineDesignator:                p.extractField(line, 2, 5),
		Season:                           p.extractField(line, 10, 13),
		ValidityStart:                    p.extractField(line, 14, 21),
		ValidityEnd:                      p.extractField(line, 21, 28),
		CreationDate:                     p.extractField(line, 28, 35),
		ReleaseDate:                      p.extractField(line, 64, 71),
		ScheduleStatus:                   p.extractField(line, 71, 72),
		CreatorReference:                 p.extractField(line, 72, 107),
		DuplicateAirlineDesignatorMarker: p.extractField(line, 107, 108),
		GeneralInformation:               p.extractField(line, 108, 169),
		InFlightServiceInformation:       p.extractField(line, 169, 188),
		ElectronicTicketingInformation:   p.extractField(line, 188, 190),
		CreationTime:                     p.extractField(line, 190, 191),
		RecordSerialNumber:               p.extractField(line, 194, 200),
	}
}

func (p *Parser) parseFlightLeg(line string) FlightLegRecord {
	return FlightLegRecord{
		RecordType:                             p.extractField(line, 0, 1),
		OperationalSuffix:                      p.extractField(line, 1, 2),
		AirlineDesignator:                      p.extractField(line, 2, 5),
		FlightNumber:                           p.extractField(line, 5, 9),
		ItineraryVariationIdentifier:           p.extractField(line, 9, 11),
		LegSequenceNumber:                      p.extractField(line, 11, 13),
		ServiceType:                            p.extractField(line, 13, 14),
		PeriodStart:                            p.extractField(line, 14, 21),
		PeriodEnd:                              p.extractField(line, 21, 28),
		DaysOfOperation:                        p.extractField(line, 28, 35),
		FrequencyRate:                          p.extractField(line, 35, 36),
		DepartureStation:                       p.extractField(line, 36, 39),
		PassengerSTD:                           p.extractField(line, 39, 43),
		AircraftSTD:                            p.extractField(line, 43, 47),
		DepartureUTCOffset:                     p.extractField(line, 47, 52),
		DepartureTerminal:                      p.extractField(line, 52, 54),
		ArrivalStation:                         p.extractField(line, 54, 57),
		AircraftSTA:                            p.extractField(line, 57, 61),
		PassengerSTA:                           p.extractField(line, 61, 65),
		ArrivalUTCOffset:                       p.extractField(line, 65, 70),
		ArrivalTerminal:                        p.extractField(line, 70, 72),
		AircraftType:                           p.extractField(line, 72, 75),
		PassengerReservationsBookingDesignator: p.extractField(line, 75, 95),
		PassengerReservationsBookingModifier:   p.extractField(line, 95, 100),
		MealServiceNote:                        p.extractField(line, 100, 110),
		JointOperationAirlineDesignators:       p.extractField(line, 110, 119),
		MinimumConnectionTimeStatus:            p.extractField(line, 119, 121),
		SecureFlightIndicator:                  p.extractField(line, 121, 122),
		ItineraryVariationIdentifierOverflow:   p.extractField(line, 127, 128),
		AircraftOwner:                          p.extractField(line, 128, 131),
		CockpitCrewEmployer:                    p.extractField(line, 131, 134),
		CabinCrewEmployer:                      p.extractField(line, 134, 137),
		OnwardAirlineDesignator:                p.extractField(line, 137, 140),
		OnwardFlightNumber:                     p.extractField(line, 140, 144),
		OnwardAircraftRotationLayover:          p.extractField(line, 144, 145),
		OnwardOperationalSuffix:                p.extractField(line, 145, 146),
		FlightTransitLayover:                   p.extractField(line, 147, 148),
		OperatingAirlineDisclosure:             p.extractField(line, 148, 149),
		TrafficRestrictionCode:                 p.extractField(line, 149, 160),
		TrafficRestrictionCodeOverflow:         p.extractField(line, 160, 161),
		AircraftConfiguration:                  p.extractField(line, 172, 192),
		DateVariation:                          p.extractField(line, 192, 194),
		RecordSerialNumber:                     p.extractField(line, 194, 200),
	}
}

func (p *Parser) parseSegmentData(line string) SegmentDataRecord {
	return SegmentDataRecord{
		RecordType:                           p.extractField(line, 0, 1),
		OperationalSuffix:                    p.extractField(line, 1, 2),
		AirlineDesignator:                    p.extractField(line, 2, 5),
		FlightNumber:                         p.extractField(line, 5, 9),
		ItineraryVariationIdentifier:         p.extractField(line, 9, 11),
		LegSequenceNumber:                    p.extractField(line, 11, 13),
		ServiceType:                          p.extractField(line, 13, 14),
		ItineraryVariationIdentifierOverflow: p.extractField(line, 27, 28),
		BoardPointIndicator:                  p.extractField(line, 28, 29),
		OffPointIndicator:                    p.extractField(line, 29, 30),
		DataElementIdentifier:                p.extractField(line, 30, 33),
		SegmentBoardPoint:                    p.extractField(line, 33, 36),
		SegmentOffPoint:                      p.extractField(line, 36, 39),
		Data:                                 p.extractField(line, 39, 194),
		RecordSerialNumber:                   p.extractField(line, 194, 200),
	}
}

func (p *Parser) parseTrailer(line string) TrailerRecord {
	return TrailerRecord{
		RecordType:                 p.extractField(line, 0, 1),
		AirlineDesignator:          p.extractField(line, 2, 5),
		ReleaseDate:                p.extractField(line, 5, 12),
		SerialNumberCheckReference: p.extractField(line, 187, 193),
		ContinuationEndCode:        p.extractField(line, 193, 194),
		RecordSerialNumber:         p.extractField(line, 194, 200),
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

	return time.Parse("02Jan06 1504", dateStr[:7]+" "+timeStr[:4])
}

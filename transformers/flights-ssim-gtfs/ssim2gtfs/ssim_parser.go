// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ssim

import (
	"bufio"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// RecordType identifies the SSIM record type
type RecordType int

const (
	RecordTypeHeader RecordType = 1
	RecordTypeCarrier RecordType = 2
	RecordTypeFlight RecordType = 3
	RecordTypeSegment RecordType = 4
	RecordTypeTrailer RecordType = 5
)

// Header represents a Type 1 record (Header)
type Header struct {
	Title                string
	DataSetSerialNumber  int
	CreationDate         time.Time
}

// Carrier represents a Type 2 record (Carrier)
type Carrier struct {
	TimeMode             string
	AirlineDesignator    string
	CreatorReference     string
	PeriodOfSchedule     DateRange
	CreationDate         time.Time
}

// Flight represents a Type 3 record (Flight Leg)
type Flight struct {
	AirlineDesignator    string
	FlightNumber         string
	ItineraryVariation   string
	LegSequence          int
	ServiceType          string
	PeriodOfOperation    DateRange
	DaysOfOperation      [7]bool
	DepartureStation     string
	STD                  string // Scheduled Time of Departure
	UTCVariation         string
	ArrivalStation       string
	STA                  string // Scheduled Time of Arrival
	AircraftType         string
	ServiceCode          string
}

// DateRange represents a date range
type DateRange struct {
	Start time.Time
	End   time.Time
}

// Trailer represents a Type 5 record (Trailer)
type Trailer struct {
	AirlineDesignator string
	ReleaseDate       time.Time
	SerialNumber      int
}

// Parser handles SSIM file parsing
type Parser struct {
	reader *bufio.Reader
}

// NewParser creates a new SSIM parser
func NewParser(r io.Reader) *Parser {
	return &Parser{
		reader: bufio.NewReader(r),
	}
}

// Parse reads and parses all records from the SSIM file
func (p *Parser) Parse() ([]interface{}, error) {
	var records []interface{}
	
	for {
		line, err := p.reader.ReadString('\n')
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		
		line = strings.TrimRight(line, "\r\n")
		if len(line) == 0 {
			continue
		}
		
		recordType := p.getRecordType(line)
		
		switch recordType {
		case RecordTypeHeader:
			record, err := p.parseHeader(line)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		case RecordTypeCarrier:
			record, err := p.parseCarrier(line)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		case RecordTypeFlight:
			record, err := p.parseFlight(line)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		case RecordTypeTrailer:
			record, err := p.parseTrailer(line)
			if err != nil {
				return nil, err
			}
			records = append(records, record)
		}
	}
	
	return records, nil
}

func (p *Parser) getRecordType(line string) RecordType {
	if len(line) == 0 {
		return 0
	}
	
	switch line[0] {
	case '1':
		return RecordTypeHeader
	case '2':
		return RecordTypeCarrier
	case '3':
		return RecordTypeFlight
	case '4':
		return RecordTypeSegment
	case '5':
		return RecordTypeTrailer
	default:
		return 0
	}
}

func (p *Parser) parseHeader(line string) (*Header, error) {
	if len(line) < 30 {
		return nil, fmt.Errorf("header record too short")
	}
	
	header := &Header{
		Title: strings.TrimSpace(line[1:26]),
	}
	
	if len(line) >= 29 {
		serialNum, err := strconv.Atoi(strings.TrimSpace(line[26:29]))
		if err == nil {
			header.DataSetSerialNumber = serialNum
		}
	}
	
	if len(line) >= 36 {
		date, err := parseDate(line[29:36])
		if err == nil {
			header.CreationDate = date
		}
	}
	
	return header, nil
}

func (p *Parser) parseCarrier(line string) (*Carrier, error) {
	if len(line) < 36 {
		return nil, fmt.Errorf("carrier record too short")
	}
	
	carrier := &Carrier{
		TimeMode:          strings.TrimSpace(line[1:2]),
		AirlineDesignator: strings.TrimSpace(line[2:5]),
		CreatorReference:  strings.TrimSpace(line[5:12]),
	}
	
	if len(line) >= 28 {
		startDate, _ := parseDate(line[14:21])
		endDate, _ := parseDate(line[21:28])
		carrier.PeriodOfSchedule = DateRange{Start: startDate, End: endDate}
	}
	
	if len(line) >= 35 {
		date, err := parseDate(line[28:35])
		if err == nil {
			carrier.CreationDate = date
		}
	}
	
	return carrier, nil
}

func (p *Parser) parseFlight(line string) (*Flight, error) {
	if len(line) < 65 {
		return nil, fmt.Errorf("flight record too short")
	}
	
	flight := &Flight{
		AirlineDesignator:  strings.TrimSpace(line[2:5]),
		FlightNumber:       strings.TrimSpace(line[5:9]),
		ItineraryVariation: strings.TrimSpace(line[9:11]),
		LegSequence:        parseInt(line[11:13]),
		ServiceType:        strings.TrimSpace(line[13:14]),
	}
	
	// Period of operation
	startDate, _ := parseDate(line[14:21])
	endDate, _ := parseDate(line[21:28])
	flight.PeriodOfOperation = DateRange{Start: startDate, End: endDate}
	
	// Days of operation (positions 28-34)
	daysStr := line[28:35]
	for i, char := range daysStr {
		if i < 7 {
			flight.DaysOfOperation[i] = (char != ' ' && char != '0')
		}
	}
	
	// Station and time information
	flight.DepartureStation = strings.TrimSpace(line[36:39])
	flight.STD = strings.TrimSpace(line[39:43])
	
	if len(line) >= 48 {
		flight.UTCVariation = strings.TrimSpace(line[43:48])
	}
	
	flight.ArrivalStation = strings.TrimSpace(line[48:51])
	flight.STA = strings.TrimSpace(line[51:55])
	
	if len(line) >= 58 {
		flight.AircraftType = strings.TrimSpace(line[55:58])
	}
	
	if len(line) >= 194 {
		flight.ServiceCode = strings.TrimSpace(line[193:194])
	}
	
	return flight, nil
}

func (p *Parser) parseTrailer(line string) (*Trailer, error) {
	if len(line) < 21 {
		return nil, fmt.Errorf("trailer record too short")
	}
	
	trailer := &Trailer{
		AirlineDesignator: strings.TrimSpace(line[2:5]),
	}
	
	if len(line) >= 12 {
		date, err := parseDate(line[5:12])
		if err == nil {
			trailer.ReleaseDate = date
		}
	}
	
	if len(line) >= 21 {
		serialNum, err := strconv.Atoi(strings.TrimSpace(line[18:21]))
		if err == nil {
			trailer.SerialNumber = serialNum
		}
	}
	
	return trailer, nil
}

// parseDate parses SSIM date format (DDMMMYY)
func parseDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if len(s) != 7 {
		return time.Time{}, fmt.Errorf("invalid date format")
	}
	
	layout := "02Jan06"
	return time.Parse(layout, s)
}

// parseInt safely parses an integer from a string
func parseInt(s string) int {
	s = strings.TrimSpace(s)
	val, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return val
}

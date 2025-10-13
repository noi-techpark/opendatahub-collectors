// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/xml"
	"os"
	"strconv"
	"strings"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
)

// KML structure definitions
type KML struct {
	XMLName  xml.Name `xml:"kml"`
	Document Document `xml:"Document"`
}

type Document struct {
	Folder Folder `xml:"Folder"`
}

type Folder struct {
	Placemarks []Placemark `xml:"Placemark"`
}

type Placemark struct {
	Code    string   `xml:"code"`
	Name    string   `xml:"name"`
	Sensors *Sensors `xml:"Sensors"`
}

type Sensors struct {
	Sensor []Sensor `xml:"Sensor"`
}

type Sensor struct {
	ID          string `xml:"id,attr"`
	Type        string `xml:"type,attr"`
	Coordinates string `xml:"coordinates"`
}

// Station represents a traffic sensor station
type Station struct {
	ID     string
	Name   string
	Lat    float64
	Lon    float64
	Origin string
}

type StationLookup map[string]*Station

// ReadStations parses the KML file and returns a map of stations indexed by sensor ID
func ReadStations(filename string) StationLookup {
	f, err := os.Open(filename)
	ms.FailOnError(context.Background(), err, "failed opening KML file")
	defer f.Close()

	var kml KML
	decoder := xml.NewDecoder(f)
	err = decoder.Decode(&kml)
	ms.FailOnError(context.Background(), err, "failed parsing KML file")

	stations := make(StationLookup)

	// Iterate through all placemarks
	for _, placemark := range kml.Document.Folder.Placemarks {
		// Only process placemarks with sensors
		if placemark.Sensors == nil {
			continue
		}

		// Process each sensor in the placemark
		for _, sensor := range placemark.Sensors.Sensor {
			// Parse coordinates (format: lat,lon,elevation)
			lat, lon, err := parseCoordinates(sensor.Coordinates)
			if err != nil {
				continue
			}

			// Create station name as "Placemark_name_SensorID"
			stationName := placemark.Name + "_" + sensor.ID

			station := &Station{
				ID:     stationName,
				Name:   stationName,
				Lat:    lat,
				Lon:    lon,
				Origin: "Municipality of Merano",
			}

			stations[sensor.ID] = station
		}
	}

	return stations
}

// parseCoordinates extracts latitude and longitude from the KML coordinates string
// Format: "lat,lon,elevation"
func parseCoordinates(coords string) (float64, float64, error) {
	parts := strings.Split(strings.TrimSpace(coords), ",")
	if len(parts) < 2 {
		return 0, 0, nil
	}

	lat, err := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	if err != nil {
		return 0, 0, err
	}

	lon, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err != nil {
		return 0, 0, err
	}

	return lat, lon, nil
}

// GetStationByID returns a station by its ID
func (s StationLookup) GetStationByID(id string) *Station {
	return s[id]
}

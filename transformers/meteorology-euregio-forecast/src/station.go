// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
)

// Station represents one record from the json.
type Station struct {
	ProviderId   string
	ID           string
	Lat          float64
	Lon          float64
	Elevation    float64
	NameDe       string
	NameEn       string
	NameIt       string
	Type         string
	Webcam       *string
	Webcams      map[string]any
	Neighbors    map[string]any
	Weight       float64
	IdAvenueType string `json:"id_venue_type"`
	IdRegion     string `json:"id_region"`
}

// Stations represents a slice of Station.
type Stations []Station

// GetStationByID returns a pointer to a Station with the matching ID.
// Returns nil if no matching record is found.
func (s Stations) GetStationByID(id string) *Station {
	for i := range s {
		if s[i].ProviderId == id {
			return &s[i]
		}
	}
	return nil
}

// ToMetadata converts the Station record into a map[string]any,
// including only non-empty fields and the coordinates.
func (f *Station) toMetadata() map[string]any {
	result := make(map[string]any)
	if f.NameDe != "" {
		result["name_de"] = f.NameDe
	}
	if f.NameEn != "" {
		result["name_en"] = f.NameEn
	}
	if f.NameIt != "" {
		result["name_it"] = f.NameIt
	}
	result["lat"] = f.Lat
	result["lon"] = f.Lon
	result["elevation"] = f.Elevation
	result["weight"] = f.Weight
	result["id_venue_type"] = f.IdAvenueType
	result["id_region"] = f.IdRegion
	result["neighbors"] = f.Neighbors
	if f.Webcam != nil {
		result["webcam"] = *f.Webcam
	}
	return result
}

func (f *Station) ToBdp(bdp bdplib.Bdp) bdplib.Station {
	s := bdplib.CreateStation(f.ID, fmt.Sprintf("%s | %s", f.NameEn, f.NameIt),
		DataStationType, f.Lat, f.Lon, bdp.GetOrigin())
	s.MetaData = f.toMetadata()
	return s
}

// StationJSON is a helper struct for unmarshalling the JSON files.
type StationJSON struct {
	ID           string  `json:"id"`
	Lat          float64 `json:"lat"`
	Lon          float64 `json:"lon"`
	Elevation    float64
	Name         string `json:"name"`
	Webcam       *string
	Webcams      map[string]any
	Neighbors    map[string]any
	Weight       float64
	IdAvenueType string `json:"id_venue_type"`
	IdRegion     string `json:"id_region"`
}

// LoadAllStations loads and combines station data from all language-specific JSON files.
func LoadAllStations() (Stations, error) {
	slog.Info("Loading station data from JSON files...")

	// Use a map to merge data from different language files.
	stationMap := make(map[string]*Station)

	// A helper function to load and unmarshal a single JSON file.
	loadJSON := func(filename string) ([]StationJSON, error) {
		content, err := os.ReadFile(filename)
		if err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filename, err)
		}
		var stations []StationJSON
		if err := json.Unmarshal(content, &stations); err != nil {
			return nil, fmt.Errorf("failed to unmarshal JSON from %s: %w", filename, err)
		}
		return stations, nil
	}

	// Load data for English stations.
	enStations, err := loadJSON("../resources/stations_en.json")
	if err != nil {
		return nil, err
	}
	for _, s := range enStations {
		stationMap[s.ID] = &Station{
			ProviderId:   s.ID,
			ID:           fmt.Sprintf("EUREGIO:%s", s.ID),
			Lat:          s.Lat,
			Lon:          s.Lon,
			Elevation:    s.Elevation,
			Weight:       s.Weight,
			NameEn:       s.Name,
			Webcam:       s.Webcam,
			Webcams:      s.Webcams,
			Neighbors:    s.Neighbors,
			IdAvenueType: s.IdAvenueType,
			IdRegion:     s.IdRegion,
		}
	}

	// Load data for German stations.
	deStations, err := loadJSON("../resources/stations_de.json")
	if err != nil {
		return nil, err
	}
	for _, s := range deStations {
		if station, ok := stationMap[s.ID]; ok {
			station.NameDe = s.Name
		}
	}

	// Load data for Italian stations.
	itStations, err := loadJSON("../resources/stations_it.json")
	if err != nil {
		return nil, err
	}
	for _, s := range itStations {
		if station, ok := stationMap[s.ID]; ok {
			station.NameIt = s.Name
		}
	}

	// Convert the map to a slice of stations.
	var stations []Station
	for _, s := range stationMap {
		stations = append(stations, *s)
	}

	slog.Info(fmt.Sprintf("Successfully loaded and combined %d stations.", len(stations)))
	return stations, nil
}

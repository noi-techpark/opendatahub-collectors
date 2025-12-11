// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/csv"
	"os"
	"strconv"
)

type Parking struct {
	ID                    string
	Latitude              float64
	Longitude             float64
	Name                  string
	NameEn                string
	NameIt                string
	NameDe                string
	StandardName          string
	NetexType             string
	NetexVehicletypes     string
	NetexLayout           string
	NetexHazardProhibited bool
	NetexCharging         bool
	NetexSurveillance     bool
	NetexReservation      string
}

func LoadMeta(filename string) (map[string]Parking, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	r := csv.NewReader(f)
	records, err := r.ReadAll()
	if err != nil {
		return nil, err
	}

	parkings := make(map[string]Parking)
	for i, record := range records {
		if i == 0 { // Skip header
			continue
		}

		lat, _ := strconv.ParseFloat(record[1], 64)
		lon, _ := strconv.ParseFloat(record[2], 64)
		hazard, _ := strconv.ParseBool(record[11])
		charging, _ := strconv.ParseBool(record[12])
		surveillance, _ := strconv.ParseBool(record[13])

		parkings[record[0]] = Parking{
			ID:                    record[0],
			Latitude:              lat,
			Longitude:             lon,
			Name:                  record[3],
			NameEn:                record[4],
			NameIt:                record[5],
			NameDe:                record[6],
			StandardName:          record[7],
			NetexType:             record[8],
			NetexVehicletypes:     record[9],
			NetexLayout:           record[10],
			NetexHazardProhibited: hazard,
			NetexCharging:         charging,
			NetexSurveillance:     surveillance,
			NetexReservation:      record[14],
		}
	}

	return parkings, nil
}

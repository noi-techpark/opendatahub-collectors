// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package ssim2gtfs

import (
	"encoding/csv"
	"os"
	"strconv"
)

type airport struct {
	ID               int
	Ident            string
	Type             string
	Name             string
	LatitudeDeg      float64
	LongitudeDeg     float64
	ElevationFt      int
	Continent        string
	ISOCountry       string
	ISORegion        string
	Municipality     string
	ScheduledService string
	ICAOCode         string
	IATACode         string
	GPSCode          string
	LocalCode        string
	HomeLink         string
	WikipediaLink    string
	Keywords         string
}

func loadAirports(filename string) (map[string]airport, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	result := make(map[string]airport)
	for _, record := range records[1:] {
		if len(record) < 19 || record[13] == "" {
			continue
		}

		id, _ := strconv.Atoi(record[0])
		lat, _ := strconv.ParseFloat(record[4], 64)
		lon, _ := strconv.ParseFloat(record[5], 64)
		elev, _ := strconv.Atoi(record[6])

		airport := airport{
			ID:               id,
			Ident:            record[1],
			Type:             record[2],
			Name:             record[3],
			LatitudeDeg:      lat,
			LongitudeDeg:     lon,
			ElevationFt:      elev,
			Continent:        record[7],
			ISOCountry:       record[8],
			ISORegion:        record[9],
			Municipality:     record[10],
			ScheduledService: record[11],
			ICAOCode:         record[12],
			IATACode:         record[13],
			GPSCode:          record[14],
			LocalCode:        record[15],
			HomeLink:         record[16],
			WikipediaLink:    record[17],
			Keywords:         record[18],
		}

		result[airport.IATACode] = airport
	}

	return result, nil
}

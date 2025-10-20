// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"strconv"
)

type SensorData struct {
	ID     string  `csv:"id"`
	Sensor string  `csv:"sensor"`
	Name   string  `csv:"name"`
	Lat    float64 `csv:"lat"`
	Lon    float64 `csv:"lon"`
}

func readStationCsv(filename string) (map[string]SensorData, error) {
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
	sensorMap := make(map[string]SensorData)
	for i, record := range records[1:] {
		if len(record) != 5 {
			return nil, fmt.Errorf("row %d has %d columns, expected 5", i+2, len(record))
		}

		lat, err := strconv.ParseFloat(record[3], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid lat value '%s'", i+2, record[3])
		}

		lon, err := strconv.ParseFloat(record[4], 64)
		if err != nil {
			return nil, fmt.Errorf("row %d: invalid lon value '%s'", i+2, record[4])
		}

		sensor := record[1]
		if _, exists := sensorMap[sensor]; exists {
			return nil, fmt.Errorf("duplicate sensor found: %s", sensor)
		}

		sensorMap[sensor] = SensorData{
			ID:     record[0],
			Sensor: sensor,
			Name:   record[2],
			Lat:    lat,
			Lon:    lon,
		}
	}

	return sensorMap, nil
}

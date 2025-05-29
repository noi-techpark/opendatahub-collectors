// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"log"
	"log/slog"
	"os"
	"strings"
)

type SensorTypeUtil struct {
	sensorTypeByStation map[string]string
}

func NewSensorTypeUtil() *SensorTypeUtil {
	util := &SensorTypeUtil{
		sensorTypeByStation: make(map[string]string),
	}
	util.initializeMap()
	return util
}

func (s *SensorTypeUtil) initializeMap() {
	filepath := "../resources/sensor-type-mapping.csv"
	slog.Info("SensorTypeUtil loading file", "filepath", filepath)

	file, err := os.Open(filepath)
	if err != nil {
		slog.Error("SensorTypeUtil file not found", "filepath", filepath)
		panic("SensorTypeUtil file not found")
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		parts := strings.Split(line, ",")
		if len(parts) < 2 {
			continue
		}
		code := strings.TrimSpace(parts[0])
		sensorType := strings.TrimSpace(parts[1])
		s.sensorTypeByStation[code] = sensorType
	}

	if err := scanner.Err(); err != nil {
		log.Printf("Error reading sensor-type-mapping.csv: %v", err)
	}
}

// AddSensorTypeMetadata updates stations' metadata with sensor type if available.
func (s *SensorTypeUtil) AddSensorTypeMetadata(stations []Station) {
	for i := range stations {
		station := &stations[i]
		sensorType, ok := s.sensorTypeByStation[station.Id]
		if ok && sensorType != "" {
			if station.MetaData == nil {
				station.MetaData = make(map[string]any)
			}
			station.MetaData["sensor_type"] = sensorType
		} else {
			log.Printf("Station with code %s not found in sensor-type-mapping.csv", station.Id)
		}
	}
}

// IsCamera checks if a station is a camera.
func IsCamera(station Station) bool {
	return station.MetaData["sensor_type"] == "camera"
}

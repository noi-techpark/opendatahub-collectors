// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bufio"
	"encoding/csv"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	EURO0 = "EURO0"
	EURO1 = "EURO1"
	EURO2 = "EURO2"
	EURO3 = "EURO3"
	EURO4 = "EURO4"
	EURO5 = "EURO5"
	EURO6 = "EURO6"
	EUROE = "ELECTRIC"
)

type EUROType struct {
	Targa         string
	Probabilities map[string]float64
}

type EUROTypeUtil struct {
	vehicleDataMap2023 map[string]EUROType
	vehicleDataMap2024 map[string]EUROType
	cutoff2024         time.Time
}

func NewEUROTypeUtil() *EUROTypeUtil {
	util := &EUROTypeUtil{
		vehicleDataMap2023: make(map[string]EUROType),
		vehicleDataMap2024: make(map[string]EUROType),
		cutoff2024:         time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	util.initializeMap()
	return util
}

func (e *EUROTypeUtil) GetVehicleDataMap(ts time.Time) map[string]EUROType {
	if ts.Before(e.cutoff2024) {
		return e.vehicleDataMap2023
	}
	return e.vehicleDataMap2024
}

func (e *EUROTypeUtil) initializeMap() {
	e.readCSV("../resources/associaz_prob_euro_targa_AV_2023.csv", e.vehicleDataMap2023)
	e.readCSV("../resources/associaz_prob_euro_targa_AV_2024.csv", e.vehicleDataMap2024)
}

func (e *EUROTypeUtil) readCSV(filepath string, resultMap map[string]EUROType) {
	slog.Info("EUROTypeUtil loading file", "filepath", filepath)

	file, err := os.Open(filepath)
	if err != nil {
		slog.Error("EUROTypeUtil file not found", "filepath", filepath)
		panic("EUROTypeUtil file not found")
	}
	defer file.Close()

	reader := csv.NewReader(bufio.NewReader(file))
	reader.Comma = ','

	// Read and skip header
	_, err = reader.Read()
	if err != nil {
		slog.Error("EUROTypeUtil error reading header", "filepath", filepath, "err", err)
		panic("EUROTypeUtil error reading header")
	}

	categories := []string{
		"", EURO0, EURO1, EURO2, EURO3, EURO4, EURO5, EURO6, EUROE,
	}

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil || len(record) != len(categories) {
			slog.Warn("Skipping malformed line", "filepath", filepath, "line", record)
			continue
		}

		targa := strings.TrimSpace(record[0])
		probs := make(map[string]float64)

		for i := 1; i < len(record); i++ {
			val := strings.TrimSpace(record[i])
			if val == "" {
				continue
			}
			f, err := strconv.ParseFloat(val, 64)
			if err != nil {
				slog.Warn("Invalid float", "filepath", filepath, "category", categories[i], "val", val)
				continue
			}
			probs[categories[i]] = f
		}

		resultMap[targa] = EUROType{Targa: targa, Probabilities: probs}
	}
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/bdplib"
)

const NULL_VALUE = -999

func elaborate(ctx context.Context, dataMap *bdplib.DataMap, existingMeasurements *measurementMap,
	station Station, vehicles []Vehicle, timestamp int64, period uint64) error {
	// existingMeasurements is used to check wether a specific DataType should be elaborate by checking the time of last
	// measurement in the ninja.
	t := time.Unix(timestamp/1000, (timestamp%1000)*1_000_000).UTC()

	// Euro distribution
	if IsCamera(station) && existingMeasurements.shouldElaborate(DataTypeEuroPct, t) {
		euroPct := createVehicleEuro(vehicles, t)
		dataMap.AddRecord(station.Id, DataTypeEuroPct, bdplib.CreateRecord(timestamp, euroPct, period))
	}

	// Nationality distribution
	if IsCamera(station) && existingMeasurements.shouldElaborate(DataTypeNationalityCount, t) {
		natCounts := createVehicleNationality(vehicles)
		dataMap.AddRecord(station.Id, DataTypeNationalityCount, bdplib.CreateRecord(timestamp, natCounts, period))
	}

	// Vehicle counts
	classCounts := createVehicleCounts(vehicles)

	var nrLight, nrHeavy, nrBuses float64
	if val, ok := classCounts[DataTypeLightVehicles]; ok && existingMeasurements.shouldElaborate(DataTypeLightVehicles, t) {
		nrLight = float64(val)
		dataMap.AddRecord(station.Id, DataTypeLightVehicles, bdplib.CreateRecord(timestamp, val, period))
	}
	if val, ok := classCounts[DataTypeHeavyVehicles]; ok && existingMeasurements.shouldElaborate(DataTypeHeavyVehicles, t) {
		nrHeavy = float64(val)
		dataMap.AddRecord(station.Id, DataTypeHeavyVehicles, bdplib.CreateRecord(timestamp, val, period))
	}
	if val, ok := classCounts[DataTypeBuses]; ok && existingMeasurements.shouldElaborate(DataTypeBuses, t) {
		nrBuses = float64(val)
		dataMap.AddRecord(station.Id, DataTypeBuses, bdplib.CreateRecord(timestamp, val, period))
	}

	// Equivalent vehicles
	equivVehicles := nrLight + 2.5*(nrHeavy+nrBuses)
	if existingMeasurements.shouldElaborate(DataTypeEquivalentVehicles, t) {
		dataMap.AddRecord(station.Id, DataTypeEquivalentVehicles, bdplib.CreateRecord(timestamp, equivVehicles, period))
	}

	// Average speeds
	classAvgSpeeds := createClassAvgSpeeds(vehicles)
	for dataType, val := range classAvgSpeeds {
		if existingMeasurements.shouldElaborate(dataType, t) {
			dataMap.AddRecord(station.Id, dataType, bdplib.CreateRecord(timestamp, val, period))
		}
	}

	// Variance of speeds
	classVarSpeeds := createClassVarSpeeds(vehicles, classAvgSpeeds)
	for dataType, val := range classVarSpeeds {
		if existingMeasurements.shouldElaborate(dataType, t) {
			dataMap.AddRecord(station.Id, dataType, bdplib.CreateRecord(timestamp, val, period))
		}
	}

	// Average metrics
	classAvgs := createClassAvgs(vehicles, equivVehicles, int64(period))
	for dataType, val := range classAvgs {
		if existingMeasurements.shouldElaborate(dataType, t) {
			dataMap.AddRecord(station.Id, dataType, bdplib.CreateRecord(timestamp, val, period))
		}
	}

	// Direction
	stationDirection := station.Direction()
	if stationDirection != STATION_DIRECTION_UNKNOWN {
		var normalCount int
		for _, v := range vehicles {
			if v.Direction == int(stationDirection) {
				normalCount++
			}
		}
		total := len(vehicles)

		var direction int = 1
		var score float64 = 1
		if total > 0 {
			score = float64(normalCount) / float64(total)
			if normalCount < total/2 {
				direction = 0
			}
		}

		if existingMeasurements.shouldElaborate(DataTypeDirection, t) {
			dataMap.AddRecord(station.Id, DataTypeDirection, bdplib.CreateRecord(timestamp, direction, period))
		}
		if existingMeasurements.shouldElaborate(DataTypeDirectionScore, t) {
			dataMap.AddRecord(station.Id, DataTypeDirectionScore, bdplib.CreateRecord(timestamp, score, period))
		}
	}

	return nil
}

func createVehicleCounts(vehicles []Vehicle) map[string]int {
	counts := map[string]int{
		DataTypeLightVehicles: 0,
		DataTypeHeavyVehicles: 0,
		DataTypeBuses:         0,
	}
	for _, v := range vehicles {
		if v.IsLight() {
			counts[DataTypeLightVehicles]++
		} else if v.IsHeavy() {
			counts[DataTypeHeavyVehicles]++
		} else if v.IsBus() {
			counts[DataTypeBuses]++
		}
	}
	return counts
}

func createClassAvgSpeeds(vehicles []Vehicle) map[string]float64 {
	var sumLight, sumHeavy, sumBus float64
	var countLight, countHeavy, countBus int

	for _, v := range vehicles {
		s := v.Speed
		if v.IsLight() {
			sumLight += s
			countLight++
		} else if v.IsHeavy() {
			sumHeavy += s
			countHeavy++
		} else if v.IsBus() {
			sumBus += s
			countBus++
		}
	}

	return map[string]float64{
		DataTypeAvgSpeedLight: average(sumLight, countLight),
		DataTypeAvgSpeedHeavy: average(sumHeavy, countHeavy),
		DataTypeAvgSpeedBuses: average(sumBus, countBus),
	}
}

func createClassVarSpeeds(vehicles []Vehicle, avgSpeeds map[string]float64) map[string]float64 {
	var varLight, varHeavy, varBus float64
	var countLight, countHeavy, countBus int

	for _, v := range vehicles {
		s := v.Speed
		if v.IsLight() {
			varLight += squareDiff(s, avgSpeeds[DataTypeAvgSpeedLight])
			countLight++
		} else if v.IsHeavy() {
			varHeavy += squareDiff(s, avgSpeeds[DataTypeAvgSpeedHeavy])
			countHeavy++
		} else if v.IsBus() {
			varBus += squareDiff(s, avgSpeeds[DataTypeAvgSpeedBuses])
			countBus++
		}
	}

	return map[string]float64{
		DataTypeVarSpeedLight: average(varLight, countLight),
		DataTypeVarSpeedHeavy: average(varHeavy, countHeavy),
		DataTypeVarSpeedBuses: average(varBus, countBus),
	}
}

func createClassAvgs(vehicles []Vehicle, equivalentVehicles float64, windowLength int64) map[string]float64 {
	var sumGap, sumHeadway, sumSpeed float64
	for _, v := range vehicles {
		sumGap += v.Distance
		sumHeadway += v.Headway
		sumSpeed += v.Speed
	}

	var avgHeadway float64 = NULL_VALUE
	var avgGap float64 = NULL_VALUE
	var avgSpeed float64 = NULL_VALUE
	// windowLength is in seconds
	var avgFlow float64 = NULL_VALUE
	var avgDensity float64 = NULL_VALUE

	count := float64(len(vehicles))
	if count != 0 {
		avgHeadway = sumHeadway / count
		avgGap = sumGap / count
		avgSpeed = sumSpeed / count
		// windowLength is in seconds
		avgFlow = equivalentVehicles * 3.6 / float64(windowLength)
		if avgSpeed == 0 {
			avgDensity = 0
		} else {
			avgDensity = avgFlow / avgSpeed
		}
	}

	return map[string]float64{
		DataTypeAvgGap:     avgGap,
		DataTypeAvgHeadway: avgHeadway,
		DataTypeAvgDensity: avgDensity,
		DataTypeAvgFlow:    avgFlow,
	}
}

func createVehicleNationality(vehicles []Vehicle) map[string]int {
	nations := map[string]int{
		"I": 0, "F": 0, "GB": 0, "D": 0, "CH": 0, "A": 0, "NL": 0, "E": 0, "B": 0, "DK": 0,
		"L": 0, "S": 0, "PL": 0, "GR": 0, "H": 0, "CZ": 0, "SK": 0, "BG": 0, "EST": 0, "FIN": 0,
		"HR": 0, "IRL": 0, "LT": 0, "LV": 0, "P": 0, "RO": 0, "RSM": 0, "SLO": 0, "XXX": 0,
	}

	for _, v := range vehicles {
		if nil == v.PlateNat || "" == *v.PlateNat {
			continue
		}
		nat := *v.PlateNat
		if _, ok := nations[nat]; !ok {
			nations[nat] = 1
		} else {
			nations[nat]++
		}
	}
	return nations
}

func createVehicleEuro(vehicles []Vehicle, t time.Time) map[string]float64 {
	euroProb := map[string]float64{
		EURO0: 0.0,
		EURO1: 0.0,
		EURO2: 0.0,
		EURO3: 0.0,
		EURO4: 0.0,
		EURO5: 0.0,
		EURO6: 0.0,
		EUROE: 0.0,
	}
	validVehicleCount := 0

	euroTypeMap := euroTypeUtils.GetVehicleDataMap(t)

	for _, vehicle := range vehicles {
		if nil == vehicle.PlateInitials || *vehicle.PlateInitials == "" {
			continue
		}

		euroData, exists := euroTypeMap[*vehicle.PlateInitials]
		if !exists {
			continue
		}

		probabilities := euroData.Probabilities
		for euroClass, prob := range probabilities {
			euroProb[euroClass] += prob
		}
		validVehicleCount++
	}

	if validVehicleCount > 0 {
		for euroClass := range euroProb {
			euroProb[euroClass] /= float64(validVehicleCount)
		}
	}

	return euroProb
}

func squareDiff(value, mean float64) float64 {
	diff := value - mean
	return diff * diff
}

func average(sum float64, count int) float64 {
	if count == 0 {
		return NULL_VALUE
	}
	return sum / float64(count)
}

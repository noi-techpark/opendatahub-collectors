// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationTypeLocation = "BikeBoxLocation"
	StationTypeStation  = "BikeBoxStation"
	StationTypeBay      = "BikeBoxBay"
)

const (
	DataTypeUsageState        = "usageState"
	DataTypeFree              = "free"
	DataTypeFreeRegularBikes  = "freeSpotsRegularBikes"
	DataTypeFreeElectricBikes = "freeSpotsElectricBikes"
)

var env tr.Env

func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		err = godotenv.Load(".env")
		if err != nil {
			log.Fatal("Error loading .env file:", err)
		}
	}

	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data (bike boxes) transformer...")

	b := bdplib.FromEnv()
	defer tel.FlushOnPanic()

	slog.Info("Syncing data types on startup")
	syncDataTypes(b)

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[string](context.Background(), env)

	err = listener.Start(context.Background(),
		tr.RawString2JsonMiddleware[BikeBoxRawData](TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[BikeBoxRawData] {
	return func(ctx context.Context, payload *rdb.Raw[BikeBoxRawData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[BikeBoxRawData]) error {
	slog.Info("Processing bike boxes data transformation", "timestamp", payload.Timestamp)

	var locationStations []bdplib.Station
	var bikeStations []bdplib.Station
	var bayStations []bdplib.Station

	locationDataMap := bdp.CreateDataMap()
	stationDataMap := bdp.CreateDataMap()
	bayDataMap := bdp.CreateDataMap()

	ts := payload.Timestamp.UnixMilli()

	for _, locationData := range payload.Rawdata.Locations {
		slog.Debug("Processing location", "locationID", locationData.LocationID, "name", locationData.Name)

		locationStation := createLocationStation(bdp, locationData)
		locationStations = append(locationStations, locationStation)

		var locationFreeTotal int
		var locationLatSum, locationLonSum float64
		var stationCount int

		for _, stationData := range locationData.Stations {
			slog.Debug("Processing station", "stationID", stationData.StationID, "name", stationData.Name)

			fullStationData := BikeStation{
				StationID:                              stationData.StationID,
				LocationID:                             locationData.LocationID,
				Name:                                   stationData.Name,
				Type:                                   stationData.Type,
				Latitude:                               0,             
				Longitude:                              0,             
				State:                                  1,             
				CountFreePlacesAvailable:               0,            
				CountFreePlacesAvailable_MuscularBikes: 0,            
				CountFreePlacesAvailable_AssistedBikes: 0,             
				TotalPlaces:                            0,             
				Places:                                 []BikePlace{}, 
				TranslatedNames:                        make(map[string]string),
				Addresses:                              make(map[string]string),
			}

			station := createBikeStation(bdp, fullStationData, locationStation.Id)
			bikeStations = append(bikeStations, station)

			addStationMeasurements(stationDataMap, station.Id, fullStationData, ts)

			locationFreeTotal += fullStationData.CountFreePlacesAvailable
			locationLatSum += fullStationData.Latitude
			locationLonSum += fullStationData.Longitude
			stationCount++

			for _, place := range fullStationData.Places {
				bay := createBayStation(bdp, place, fullStationData, station.Id)
				bayStations = append(bayStations, bay)

				addBayMeasurements(bayDataMap, bay.Id, place, ts)
			}
		}

		if stationCount > 0 {
			locationStation.Latitude = locationLatSum / float64(stationCount)
			locationStation.Longitude = locationLonSum / float64(stationCount)
		}

		locationDataMap.AddRecord(locationStation.Id, DataTypeFree, bdplib.CreateRecord(ts, locationFreeTotal, 600))
	}

	slog.Info("Syncing stations and pushing data",
		"locations", len(locationStations),
		"stations", len(bikeStations),
		"bays", len(bayStations))

	bdp.SyncStations(StationTypeLocation, locationStations, true, false)
	bdp.SyncStations(StationTypeStation, bikeStations, true, false)
	bdp.SyncStations(StationTypeBay, bayStations, true, false)

	bdp.PushData(StationTypeLocation, locationDataMap)
	bdp.PushData(StationTypeStation, stationDataMap)
	bdp.PushData(StationTypeBay, bayDataMap)

	slog.Info("Bike boxes data transformation completed successfully")

	return nil
}

func createLocationStation(bdp bdplib.Bdp, locationData BikeLocation) bdplib.Station {
	stationID := strconv.Itoa(locationData.LocationID)

	station := bdplib.CreateStation(
		stationID,
		locationData.Name,
		StationTypeLocation,
		0, 0,
		bdp.GetOrigin(),
	)

	metadata := make(map[string]interface{})
	metadata["locationID"] = locationData.LocationID
	metadata["names"] = locationData.TranslatedLocationNames
	metadata["totalStations"] = len(locationData.Stations)

	station.MetaData = metadata

	return station
}

func createBikeStation(bdp bdplib.Bdp, stationData BikeStation, parentStationID string) bdplib.Station {
	stationID := strconv.Itoa(stationData.StationID)

	station := bdplib.CreateStation(
		stationID,
		stationData.Name,
		StationTypeStation,
		stationData.Latitude,
		stationData.Longitude,
		bdp.GetOrigin(),
	)

	station.ParentStation = parentStationID

	metadata := make(map[string]interface{})
	metadata["stationID"] = stationData.StationID
	metadata["locationID"] = stationData.LocationID
	metadata["type"] = mapBikeStationType(stationData.Type)
	metadata["totalPlaces"] = stationData.TotalPlaces
	metadata["names"] = stationData.TranslatedNames
	metadata["addresses"] = stationData.Addresses
	metadata["state"] = mapBikeStationState(stationData.State)

	var placesMetadata []map[string]interface{}
	for _, place := range stationData.Places {
		placeMetadata := map[string]interface{}{
			"position": place.Position,
			"type":     mapBikeStationBayType(place.Type),
			"level":    place.Level,
		}
		placesMetadata = append(placesMetadata, placeMetadata)
	}
	metadata["places"] = placesMetadata

	metadata["netex_parking"] = map[string]interface{}{
		"type":              "other",
		"layout":            "covered",
		"charging":          false,
		"reservation":       "reservationRequired",
		"surveillance":      true,
		"vehicletypes":      "cycle",
		"hazard_prohibited": true,
	}

	station.MetaData = metadata
	return station
}

func createBayStation(bdp bdplib.Bdp, place BikePlace, stationData BikeStation, parentStationID string) bdplib.Station {
	bayID := fmt.Sprintf("%s_%d", parentStationID, place.Position)
	bayName := fmt.Sprintf("%s / Bay %d", stationData.Name, place.Position)

	bay := bdplib.CreateStation(
		bayID,
		bayName,
		StationTypeBay,
		stationData.Latitude,
		stationData.Longitude,
		bdp.GetOrigin(),
	)

	bay.ParentStation = parentStationID

	metadata := make(map[string]interface{})
	metadata["position"] = place.Position
	metadata["type"] = mapBikeStationBayType(place.Type)
	metadata["level"] = place.Level
	metadata["stationID"] = stationData.StationID

	bay.MetaData = metadata
	return bay
}

func addStationMeasurements(dataMap bdplib.DataMap, stationID string, stationData BikeStation, timestamp int64) {
	dataMap.AddRecord(stationID, DataTypeUsageState, bdplib.CreateRecord(timestamp, mapBikeStationState(stationData.State), 600))

	dataMap.AddRecord(stationID, DataTypeFree, bdplib.CreateRecord(timestamp, stationData.CountFreePlacesAvailable, 600))

	dataMap.AddRecord(stationID, DataTypeFreeRegularBikes, bdplib.CreateRecord(timestamp, stationData.CountFreePlacesAvailable_MuscularBikes, 600))

	dataMap.AddRecord(stationID, DataTypeFreeElectricBikes, bdplib.CreateRecord(timestamp, stationData.CountFreePlacesAvailable_AssistedBikes, 600))
}

func addBayMeasurements(dataMap bdplib.DataMap, bayID string, place BikePlace, timestamp int64) {
	dataMap.AddRecord(bayID, DataTypeUsageState, bdplib.CreateRecord(timestamp, mapBayUsageState(place.State), 600))
}

func mapBikeStationType(stationType int) string {
	switch stationType {
	case 4:
		return "veloHub"
	case 5:
		return "bikeBoxGroup"
	default:
		return "unknown"
	}
}

func mapBikeStationState(state int) string {
	switch state {
	case 1:
		return "in service"
	case 2:
		return "out of service"
	default:
		return "unknown"
	}
}

func mapBikeStationBayType(bayType int) string {
	switch bayType {
	case 1:
		return "withoutRefill"
	case 2:
		return "withRefill"
	default:
		return "unknown"
	}
}

func mapBayUsageState(state int) string {
	switch state {
	case 1:
		return "in service"
	case 2:
		return "occupied - in service"
	case 3:
		return "out of service"
	default:
		return "unknown"
	}
}

func syncDataTypes(bdp bdplib.Bdp) {
	var dataTypes []bdplib.DataType

	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeUsageState, "state", "Usage state of the bike box", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFree, "count", "Free parking spots", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFreeRegularBikes, "count", "Free parking spots for regular bikes", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFreeElectricBikes, "count", "Free parking spots for electric bikes", "Instantaneous"))

	// syncing datatypes for all stations, correct? or should do one by one and how?
	bdp.SyncDataTypes(dataTypes)
}

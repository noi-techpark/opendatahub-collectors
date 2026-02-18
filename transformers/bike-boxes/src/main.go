// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationTypeLocation = "BikeParkingLocation"
	StationTypeStation  = "BikeParking"
	StationTypeBay      = "BikeParkingBay"
)

const (
	DataTypeUsageState        = "usageState"
	DataTypeFree              = "free"
	DataTypeFreeRegularBikes  = "freeSpotsRegularBike"
	DataTypeFreeElectricBikes = "freeSpotsElectricBike"
)

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data (bike boxes) transformer...")

	b := bdplib.FromEnv(bdplib.BdpEnv{
		BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
		BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
		BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
		BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
		BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),
		BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
		BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
	})
	defer tel.FlushOnPanic()

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync types")

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

	findByLocationId := func(locations []BikeLocation, id int) *BikeLocation {
		for _, l := range locations {
			if l.LocationID == id {
				return &l
			}
		}
		return nil
	}

	findStationById := func(stations []BikeLocationStation, id int) *BikeLocationStation {
		for _, l := range stations {
			if l.StationID == id {
				return &l
			}
		}
		return nil
	}

	for _, locationData := range payload.Rawdata.It {
		slog.Debug("Processing location", "locationID", locationData.LocationID, "name", locationData.Name)

		locationStation := createLocationStation(bdp, locationData)
		locationEn := findByLocationId(payload.Rawdata.En, locationData.LocationID)
		locationDe := findByLocationId(payload.Rawdata.De, locationData.LocationID)
		locationLld := findByLocationId(payload.Rawdata.Lld, locationData.LocationID)

		// add translations
		locationStation.MetaData["names"] = map[string]any{
			"it":  locationData.Name,
			"de":  locationDe.Name,
			"en":  locationEn.Name,
			"lld": locationLld.Name,
		}

		var locationFreeTotal, totalLocationPlaces int
		var locationLatSum, locationLonSum float64
		var stationCount int

		for _, stationData := range locationData.Stations {
			slog.Debug("Processing station", "stationID", stationData.StationID, "name", stationData.Name)

			station := createBikeStation(bdp, stationData, locationStation)
			stationEn := findStationById(locationEn.Stations, stationData.StationID)
			stationDe := findStationById(locationDe.Stations, stationData.StationID)
			stationLld := findStationById(locationLld.Stations, stationData.StationID)

			// add names and addresses
			station.MetaData["names"] = map[string]any{
				"it":  stationData.Name,
				"de":  stationDe.Name,
				"en":  stationEn.Name,
				"lld": stationLld.Name,
			}
			station.MetaData["addresses"] = map[string]any{
				"it":  stationData.Address,
				"de":  stationDe.Address,
				"en":  stationEn.Address,
				"lld": stationLld.Address,
			}

			bikeStations = append(bikeStations, station)

			addStationMeasurements(stationDataMap, station.Id, stationData, ts)

			totalLocationPlaces += stationData.TotalPlaces
			locationFreeTotal += stationData.CountFreePlacesAvailable
			locationLatSum += stationData.Latitude
			locationLonSum += stationData.Longitude
			stationCount++

			for _, place := range stationData.Places {
				bay := createBayStation(bdp, place, station, locationStation)
				bayStations = append(bayStations, bay)

				addBayMeasurements(bayDataMap, bay.Id, place, ts)
			}
		}

		if stationCount > 0 {
			locationStation.Latitude = locationLatSum / float64(stationCount)
			locationStation.Longitude = locationLonSum / float64(stationCount)
		}

		locationStation.MetaData["totalPlaces"] = totalLocationPlaces
		locationStations = append(locationStations, locationStation)

		locationDataMap.AddRecord(locationStation.Id, DataTypeFree, bdplib.CreateRecord(ts, locationFreeTotal, 600))
	}

	slog.Info("Syncing stations and pushing data",
		"locations", len(locationStations),
		"stations", len(bikeStations),
		"bays", len(bayStations))

	err := bdp.SyncStations(StationTypeLocation, locationStations, true, false)
	ms.FailOnError(ctx, err, "failed to push StationTypeLocation")
	err = bdp.SyncStations(StationTypeStation, bikeStations, true, false)
	ms.FailOnError(ctx, err, "failed to push StationTypeStation")
	err = bdp.SyncStations(StationTypeBay, bayStations, true, false)
	ms.FailOnError(ctx, err, "failed to push StationTypeBay")

	err = bdp.PushData(StationTypeLocation, locationDataMap)
	ms.FailOnError(ctx, err, "failed to push StationTypeLocation records")
	err = bdp.PushData(StationTypeStation, stationDataMap)
	ms.FailOnError(ctx, err, "failed to push StationTypeStation records")
	err = bdp.PushData(StationTypeBay, bayDataMap)
	ms.FailOnError(ctx, err, "failed to push StationTypeBay records")

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
	metadata["totalStations"] = len(locationData.Stations)

	station.MetaData = metadata

	return station
}

func createBikeStation(bdp bdplib.Bdp, stationData BikeLocationStation, parentStation bdplib.Station) bdplib.Station {
	stationID := strconv.Itoa(stationData.StationID)

	station := bdplib.CreateStation(
		stationID,
		fmt.Sprintf("%s / %s", parentStation.Name, stationData.Name),
		StationTypeStation,
		stationData.Latitude,
		stationData.Longitude,
		bdp.GetOrigin(),
	)

	station.ParentStation = parentStation.Id

	metadata := make(map[string]interface{})
	metadata["stationID"] = stationData.StationID
	metadata["locationID"] = stationData.LocationID
	metadata["type"] = mapBikeStationType(stationData.Type)
	metadata["totalPlaces"] = stationData.TotalPlaces

	var placesMetadata []map[string]interface{}
	for _, place := range stationData.Places {
		placeMetadata := map[string]interface{}{
			"position": place.Position,
			"type":     mapBikeStationBayType(place.Type),
			"level":    place.Level,
		}
		placesMetadata = append(placesMetadata, placeMetadata)
	}
	metadata["stationPlaces"] = placesMetadata

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

func createBayStation(bdp bdplib.Bdp, place BikePlace, station, locationStation bdplib.Station) bdplib.Station {
	bayID := fmt.Sprintf("%s_%s/%d", station.Id, station.Id, place.Position)

	// station.Name already is "locationname  / stationname"
	bayName := fmt.Sprintf("%s / %d", station.Name, place.Position)

	bay := bdplib.CreateStation(
		bayID,
		bayName,
		StationTypeBay,
		station.Latitude,
		station.Longitude,
		bdp.GetOrigin(),
	)

	bay.ParentStation = station.Id
	bay.ParentStationType = StationTypeStation

	metadata := make(map[string]interface{})
	metadata["position"] = place.Position
	metadata["type"] = mapBikeStationBayType(place.Type)
	metadata["level"] = place.Level

	bay.MetaData = metadata
	return bay
}

func addStationMeasurements(dataMap bdplib.DataMap, stationID string, stationData BikeLocationStation, timestamp int64) {
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

func syncDataTypes(bdp bdplib.Bdp) error {
	var dataTypes []bdplib.DataType

	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeUsageState, "state", "Usage state", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFree, "count", "Free parking spots", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFreeRegularBikes, "count", "Free parking spots (regular bikes)", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFreeElectricBikes, "count", "Free parking spots (electric bikes)", "Instantaneous"))

	// syncing datatypes for all stations, correct? or should do one by one and how?
	return bdp.SyncDataTypes(dataTypes)
}

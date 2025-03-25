// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
)

// hard coded bz coordinates for main Station Dto location 46.49067, 11.33982
const (
	stationTypeParent = "ParkingFacility"
	stationType       = "ParkingStation"

	shortStay   = "short_stay"
	subscribers = "subscribers"

	dataTypeFreeShort     = "free_" + shortStay
	dataTypeFreeSubs      = "free_" + subscribers
	dataTypeFreeTotal     = "free"
	dataTypeOccupiedShort = "occupied_" + shortStay
	dataTypeOccupiedSubs  = "occupied_" + subscribers
	dataTypeOccupiedTotal = "occupied"
)

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[FacilityData] {
	return func(ctx context.Context, payload *dto.Raw[FacilityData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *dto.Raw[FacilityData]) error {
	var parentStations []bdplib.Station
	// save stations by stationCode
	stations := make(map[string]bdplib.Station)

	dataMapParent := bdp.CreateDataMap()
	dataMap := bdp.CreateDataMap()

	ts := payload.Timestamp.UnixMilli()

	for _, facility := range payload.Rawdata {
		id := facility.GetID()
		parent_station_data := station_proto.GetStationByID(strconv.Itoa(id))
		if nil == parent_station_data {
			slog.Error("no parent station data", "facility_id", strconv.Itoa(id))
			panic("no parent station data")
		}

		parentStation := bdplib.CreateStation(parent_station_data.ID, parent_station_data.Name,
			stationTypeParent, parent_station_data.Lat, parent_station_data.Lon, bdp.GetOrigin())
		parentStation.MetaData = parent_station_data.ToMetadata()

		parentStations = append(parentStations, parentStation)

		// total facility measurements
		freeTotalSum := 0
		occupiedTotalSum := 0
		capacityTotal := 0
		// total facility subscribers measurements
		freeSubscribersSum := 0
		occupiedSubscribersSum := 0
		capacitySubscribers := 0
		// total facility short stay measurements
		freeShortStaySum := 0
		occupiedShortStaySum := 0
		capacityShortStay := 0

		// freeplaces is array of a single categories data
		// if multiple parkNo exist, multiple entries for every parkNo and its categories exist
		// so iterating over freeplaces and checking if the station with the parkNo has already been created is needed
		for _, freePlace := range facility.FacilityDetails {
			// create ParkingStation
			facility_id := strconv.Itoa(facility.GetID()) + "_" + strconv.Itoa(freePlace.ParkNo)
			station_data := station_proto.GetStationByID(facility_id)
			if nil == station_data {
				slog.Error("no station data", "facility_id", facility_id)
				panic("no station data")
			}
			station, ok := stations[facility_id]

			if !ok {

				station = bdplib.CreateStation(
					station_data.ID, station_data.Name, stationType, station_data.Lat, station_data.Lon, bdp.GetOrigin())
				station.ParentStation = parentStation.Id

				station.MetaData = station_data.ToMetadata()

				stations[station_data.ID] = station
				slog.Debug("Create station " + station_data.ID)
			}

			switch freePlace.CountingCategoryNo {
			// Short Stay
			case 1:
				station.MetaData["free_limit_"+shortStay] = freePlace.FreeLimit
				station.MetaData["occupancy_limit_"+shortStay] = freePlace.OccupancyLimit
				station.MetaData["capacity_"+shortStay] = freePlace.Capacity
				dataMap.AddRecord(station_data.ID, dataTypeFreeShort, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(station_data.ID, dataTypeOccupiedShort, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// facility data
				freeShortStaySum += freePlace.FreePlaces
				occupiedShortStaySum += freePlace.CurrentLevel
				capacityShortStay += freePlace.Capacity
			// Subscribed
			case 2:
				station.MetaData["free_limit_"+subscribers] = freePlace.FreeLimit
				station.MetaData["occupancy_limit_"+subscribers] = freePlace.OccupancyLimit
				station.MetaData["capacity_"+subscribers] = freePlace.Capacity
				dataMap.AddRecord(station_data.ID, dataTypeFreeSubs, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(station_data.ID, dataTypeOccupiedSubs, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// facility data
				freeSubscribersSum += freePlace.FreePlaces
				occupiedSubscribersSum += freePlace.CurrentLevel
				capacitySubscribers += freePlace.Capacity
			// Total
			default:
				station.MetaData["free_limit"] = freePlace.FreeLimit
				station.MetaData["occupancy_limit"] = freePlace.OccupancyLimit
				station.MetaData["capacity"] = freePlace.Capacity
				dataMap.AddRecord(station_data.ID, dataTypeFreeTotal, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(station_data.ID, dataTypeOccupiedTotal, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// total facility data
				freeTotalSum += freePlace.FreePlaces
				occupiedTotalSum += freePlace.CurrentLevel
				capacityTotal += freePlace.Capacity
			}
		}

		// assign total facility data, if data is not 0
		if freeTotalSum > 0 {
			dataMapParent.AddRecord(parent_station_data.ID, dataTypeFreeTotal, bdplib.CreateRecord(ts, freeTotalSum, 600))
		}
		if occupiedTotalSum > 0 {
			dataMapParent.AddRecord(parent_station_data.ID, dataTypeOccupiedTotal, bdplib.CreateRecord(ts, occupiedTotalSum, 600))
		}
		if capacityTotal > 0 {
			parentStation.MetaData["capacity"] = capacityTotal
		}

		// subscribers
		if freeSubscribersSum > 0 {
			dataMapParent.AddRecord(parent_station_data.ID, dataTypeFreeSubs, bdplib.CreateRecord(ts, freeSubscribersSum, 600))
		}
		if occupiedSubscribersSum > 0 {
			dataMapParent.AddRecord(parent_station_data.ID, dataTypeOccupiedSubs, bdplib.CreateRecord(ts, occupiedTotalSum, 600))
		}
		if capacitySubscribers > 0 {
			parentStation.MetaData["capacity_"+subscribers] = capacityTotal
		}

		// short stay
		if freeShortStaySum > 0 {
			dataMapParent.AddRecord(parent_station_data.ID, dataTypeFreeShort, bdplib.CreateRecord(ts, freeShortStaySum, 600))
		}
		if occupiedShortStaySum > 0 {
			dataMapParent.AddRecord(parent_station_data.ID, dataTypeOccupiedShort, bdplib.CreateRecord(ts, occupiedShortStaySum, 600))
		}
		if capacityShortStay > 0 {
			parentStation.MetaData["capacity_"+shortStay] = capacityTotal
		}
	}

	// -------
	bdp.SyncStations(stationTypeParent, parentStations, true, true)
	bdp.SyncStations(stationType, values(stations), true, true)
	bdp.PushData(stationTypeParent, dataMapParent)
	bdp.PushData(stationType, dataMap)
	return nil
}

// to extract values array from map, without external dependency
// https://stackoverflow.com/questions/13422578/in-go-how-to-get-a-slice-of-values-from-a-map
func values[M ~map[K]V, K comparable, V any](m M) []V {
	r := make([]V, 0, len(m))
	for _, v := range m {
		r = append(r, v)
	}
	return r
}

func SyncDataTypes(bdp bdplib.Bdp) {
	var dataTypes []bdplib.DataType
	// free
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeFreeShort, "", "Amount of free 'short stay' parking slots", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeFreeSubs, "", "Amount of free 'subscribed' parking slots", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeFreeTotal, "", "Amount of free parking slots", "Instantaneous"))
	// occupied
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeOccupiedShort, "", "Amount of occupied 'short stay' parking slots", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeOccupiedSubs, "", "Amount of occupied 'subscribed' parking slots", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeOccupiedTotal, "", "Amount of occupied parking slots", "Instantaneous"))

	err := bdp.SyncDataTypes(stationType, dataTypes)
	ms.FailOnError(err, "failed to sync types")
}

var env tr.Env
var station_proto Stations

func main() {
	envconfig.MustProcess("", &env)
	b := bdplib.FromEnv()
	station_proto = ReadStations("../resources/stations.csv")

	SyncDataTypes(b)

	slog.Info("listening")
	listener := tr.NewTrStack[FacilityData](&env)
	err := listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(err, "error while listening to queue")
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
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

func getLocationOrDefault(facilityId int, lat float64, lon float64) (float64, float64) {
	// if lat != 0 && lon != 0 {
	// 	return lat, lon
	// }
	// if facilityId == brunicoId {
	// 	return brunicoLat, brunicoLon
	// }
	// if facilityId == bressanoneId {
	// 	return bressanoneLat, bressanoneLon
	// }
	// slog.Info("No default location found for facilityID" + strconv.Itoa(facilityId))
	// return lat, lon
	return 0, 0
}

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
		parentStationCode := strconv.Itoa(id)
		lat, lon := getLocationOrDefault(id, facility.Latitude, facility.Longitude)
		parentStation := bdplib.CreateStation(parentStationCode, facility.Description, stationTypeParent, lat, lon, bdp.GetOrigin())
		parentStation.MetaData = map[string]interface{}{
			"IdCompany":    facility.IdCompany,
			"City":         facility.City,
			"Address":      facility.Address,
			"ZIPCode":      facility.ZIPCode,
			"Telephone1":   facility.Telephone1,
			"Telephone2":   facility.Telephone2,
			"municipality": facility.City,
		}

		// set City=Brunico for parking lot "Parcheggio Stazione Brunico Mobilitätszentrum"
		// old api gives wrongly Bolzano
		if parentStationCode == "608612" {
			parentStation.MetaData["City"] = "Brunico"
			parentStation.MetaData["municipality"] = "Brunico"
		}

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
			stationCode := parentStationCode + "_" + strconv.Itoa(freePlace.ParkNo)
			station, ok := stations[stationCode]
			if !ok {
				lat, lon := getLocationOrDefault(freePlace.FacilityId, freePlace.Latitude, freePlace.Longitude)
				station = bdplib.CreateStation(
					stationCode, fmt.Sprintf("%s %s", facility.Description, freePlace.FacilityDescription),
					stationType, lat, lon, bdp.GetOrigin())
				station.ParentStation = parentStation.Id

				station.MetaData = make(map[string]interface{})
				station.MetaData["FacilityDescription"] = freePlace.FacilityDescription
				station.MetaData["municipality"] = facility.City

				// set City=Brunico for parking lot "Parcheggio Stazione Brunico Mobilitätszentrum"
				// old api gives wrongly Bolzano
				if parentStationCode == "608612" {
					station.MetaData["municipality"] = "Brunico"
				}

				stations[stationCode] = station
				slog.Debug("Create station " + stationCode)
			}

			switch freePlace.CountingCategoryNo {
			// Short Stay
			case 1:
				station.MetaData["free_limit_"+shortStay] = freePlace.FreeLimit
				station.MetaData["occupancy_limit_"+shortStay] = freePlace.OccupancyLimit
				station.MetaData["capacity_"+shortStay] = freePlace.Capacity
				dataMap.AddRecord(stationCode, dataTypeFreeShort, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(stationCode, dataTypeOccupiedShort, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// facility data
				freeShortStaySum += freePlace.FreePlaces
				occupiedShortStaySum += freePlace.CurrentLevel
				capacityShortStay += freePlace.Capacity
			// Subscribed
			case 2:
				station.MetaData["free_limit_"+subscribers] = freePlace.FreeLimit
				station.MetaData["occupancy_limit_"+subscribers] = freePlace.OccupancyLimit
				station.MetaData["capacity_"+subscribers] = freePlace.Capacity
				dataMap.AddRecord(stationCode, dataTypeFreeSubs, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(stationCode, dataTypeOccupiedSubs, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// facility data
				freeSubscribersSum += freePlace.FreePlaces
				occupiedSubscribersSum += freePlace.CurrentLevel
				capacitySubscribers += freePlace.Capacity
			// Total
			default:
				station.MetaData["free_limit"] = freePlace.FreeLimit
				station.MetaData["occupancy_limit"] = freePlace.OccupancyLimit
				station.MetaData["capacity"] = freePlace.Capacity
				dataMap.AddRecord(stationCode, dataTypeFreeTotal, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(stationCode, dataTypeOccupiedTotal, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// total facility data
				freeTotalSum += freePlace.FreePlaces
				occupiedTotalSum += freePlace.CurrentLevel
				capacityTotal += freePlace.Capacity
			}
		}

		// assign total facility data, if data is not 0
		if freeTotalSum > 0 {
			dataMapParent.AddRecord(parentStationCode, dataTypeFreeTotal, bdplib.CreateRecord(ts, freeTotalSum, 600))
		}
		if occupiedTotalSum > 0 {
			dataMapParent.AddRecord(parentStationCode, dataTypeOccupiedTotal, bdplib.CreateRecord(ts, occupiedTotalSum, 600))
		}
		if capacityTotal > 0 {
			parentStation.MetaData["capacity"] = capacityTotal
		}

		// subscribers
		if freeSubscribersSum > 0 {
			dataMapParent.AddRecord(parentStationCode, dataTypeFreeSubs, bdplib.CreateRecord(ts, freeSubscribersSum, 600))
		}
		if occupiedSubscribersSum > 0 {
			dataMapParent.AddRecord(parentStationCode, dataTypeOccupiedSubs, bdplib.CreateRecord(ts, occupiedTotalSum, 600))
		}
		if capacitySubscribers > 0 {
			parentStation.MetaData["capacity_"+subscribers] = capacityTotal
		}

		// short stay
		if freeShortStaySum > 0 {
			dataMapParent.AddRecord(parentStationCode, dataTypeFreeShort, bdplib.CreateRecord(ts, freeShortStaySum, 600))
		}
		if occupiedShortStaySum > 0 {
			dataMapParent.AddRecord(parentStationCode, dataTypeOccupiedShort, bdplib.CreateRecord(ts, occupiedShortStaySum, 600))
		}
		if capacityShortStay > 0 {
			parentStation.MetaData["capacity_"+shortStay] = capacityTotal
		}
	}

	// -------
	bdp.SyncStations(stationTypeParent, parentStations, true, false)
	bdp.SyncStations(stationType, values(stations), true, false)
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

	bdp.SyncDataTypes(stationType, dataTypes)
}

var env tr.Env

func main() {
	envconfig.MustProcess("", &env)
	b := bdplib.FromEnv()

	SyncDataTypes(b)

	slog.Info("listening")
	listener := tr.NewTrStack[FacilityData](&env)
	err := listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(err, "error while listening to queue")
}

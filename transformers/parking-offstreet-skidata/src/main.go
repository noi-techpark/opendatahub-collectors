// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"os"
	"slices"
	"strconv"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
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
	return func(ctx context.Context, payload *rdb.Raw[FacilityData]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[FacilityData]) error {
	log := logger.Get(ctx)

	var parentStations []bdplib.Station
	// save stations by stationCode
	stations := make(map[string]bdplib.Station)

	dataMapParent := bdp.CreateDataMap()
	dataMap := bdp.CreateDataMap()

	ts := payload.Timestamp.UnixMilli()

	for _, facility := range payload.Rawdata {
		facilityID := strconv.Itoa(facility.GetID())
		if missing := unknownStations(facility); len(missing) > 0 {
			log.Error("skipping facility, stations without enrichment record",
				"facility_id", facilityID, "missing", missing)
			continue
		}

		parentCode := stationCode(facilityID)
		parentStation := bdplib.CreateStation(parentCode, parentCode,
			stationTypeParent, 0, 0, bdp.GetOrigin())
		parentStation.MetaData = map[string]any{}
		enrichStation(parentCode, &parentStation)

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
			code := stationCode(carparkProviderID(facilityID, freePlace.ParkNo))
			station, ok := stations[code]

			if !ok {
				station = bdplib.CreateStation(code, code, stationType, 0, 0, bdp.GetOrigin())
				station.ParentStation = parentStation.Id
				station.MetaData = map[string]any{}
				enrichStation(code, &station)

				stations[code] = station
				log.Debug("Create station " + code)
			}

			switch freePlace.CountingCategoryNo {
			// Short Stay
			case 1:
				station.MetaData["free_limit_"+shortStay] = freePlace.FreeLimit
				station.MetaData["occupancy_limit_"+shortStay] = freePlace.OccupancyLimit
				station.MetaData["capacity_"+shortStay] = freePlace.Capacity
				dataMap.AddRecord(code, dataTypeFreeShort, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(code, dataTypeOccupiedShort, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// facility data
				freeShortStaySum += freePlace.FreePlaces
				occupiedShortStaySum += freePlace.CurrentLevel
				capacityShortStay += freePlace.Capacity
			// Subscribed
			case 2:
				station.MetaData["free_limit_"+subscribers] = freePlace.FreeLimit
				station.MetaData["occupancy_limit_"+subscribers] = freePlace.OccupancyLimit
				station.MetaData["capacity_"+subscribers] = freePlace.Capacity
				dataMap.AddRecord(code, dataTypeFreeSubs, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(code, dataTypeOccupiedSubs, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// facility data
				freeSubscribersSum += freePlace.FreePlaces
				occupiedSubscribersSum += freePlace.CurrentLevel
				capacitySubscribers += freePlace.Capacity
			// Total
			case 3:
				station.MetaData["free_limit"] = freePlace.FreeLimit
				station.MetaData["occupancy_limit"] = freePlace.OccupancyLimit
				station.MetaData["capacity"] = freePlace.Capacity
				dataMap.AddRecord(code, dataTypeFreeTotal, bdplib.CreateRecord(ts, freePlace.FreePlaces, 600))
				dataMap.AddRecord(code, dataTypeOccupiedTotal, bdplib.CreateRecord(ts, freePlace.CurrentLevel, 600))
				// total facility data
				freeTotalSum += freePlace.FreePlaces
				occupiedTotalSum += freePlace.CurrentLevel
				capacityTotal += freePlace.Capacity
			}
		}

		// assign total facility data, if data is not 0
		if freeTotalSum > 0 {
			dataMapParent.AddRecord(parentCode, dataTypeFreeTotal, bdplib.CreateRecord(ts, freeTotalSum, 600))
		}
		if occupiedTotalSum > 0 {
			dataMapParent.AddRecord(parentCode, dataTypeOccupiedTotal, bdplib.CreateRecord(ts, occupiedTotalSum, 600))
		}
		if capacityTotal > 0 {
			parentStation.MetaData["capacity"] = capacityTotal
		}

		// subscribers
		if freeSubscribersSum > 0 {
			dataMapParent.AddRecord(parentCode, dataTypeFreeSubs, bdplib.CreateRecord(ts, freeSubscribersSum, 600))
		}
		if occupiedSubscribersSum > 0 {
			dataMapParent.AddRecord(parentCode, dataTypeOccupiedSubs, bdplib.CreateRecord(ts, occupiedTotalSum, 600))
		}
		if capacitySubscribers > 0 {
			parentStation.MetaData["capacity_"+subscribers] = capacityTotal
		}

		// short stay
		if freeShortStaySum > 0 {
			dataMapParent.AddRecord(parentCode, dataTypeFreeShort, bdplib.CreateRecord(ts, freeShortStaySum, 600))
		}
		if occupiedShortStaySum > 0 {
			dataMapParent.AddRecord(parentCode, dataTypeOccupiedShort, bdplib.CreateRecord(ts, occupiedShortStaySum, 600))
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

	err := bdp.SyncDataTypes(dataTypes)
	ms.FailOnError(context.Background(), err, "failed to sync types")
}

var env tr.Env

// legacyCodes are station codes published before codes followed the provider
// id. Changing them would orphan the stations' history.
var legacyCodes = map[string]string{
	"406983":   "105_facility",
	"406983_0": "105",
}

func stationCode(providerID string) string {
	if code, ok := legacyCodes[providerID]; ok {
		return code
	}
	return providerID
}

func carparkProviderID(facilityID string, parkNo int) string {
	return facilityID + "_" + strconv.Itoa(parkNo)
}

// unknownStations lists the stations of a facility the enrichment table does not
// describe. A facility is published whole or not at all: its totals are summed
// over its carparks.
func unknownStations(facility Facility) []string {
	facilityID := strconv.Itoa(facility.GetID())
	var missing []string
	if code := stationCode(facilityID); !hasRecord(code) {
		missing = append(missing, code)
	}
	for _, d := range facility.FacilityDetails {
		if code := stationCode(carparkProviderID(facilityID, d.ParkNo)); !hasRecord(code) && !slices.Contains(missing, code) {
			missing = append(missing, code)
		}
	}
	return missing
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	ms.InitWithEnv(context.Background(), "", &enrichEnv)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(bdplib.BdpEnv{
		BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
		BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
		BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
		BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
		BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),
		BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
		BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
	})

	var err error
	err = startEnrichment(context.Background(), env)
	ms.FailOnError(context.Background(), err, "failed to start the enrichment table")
	defer stopEnrichment()

	SyncDataTypes(b)

	listener := tr.NewTr[FacilityData](context.Background(), env)
	err = listener.Start(context.Background(), TransformWithBdp(b))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

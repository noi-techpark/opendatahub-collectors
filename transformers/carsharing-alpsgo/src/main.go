// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	StationTypeCarSharing = "CarsharingStation"
	StationTypeVechile    = "CarsharingCar"

	dataTypeAvailableVehicles = "number-available"

	dataTypeVehicleCleanness      = "cleaness"
	dataTypeVehicleFuel           = "fuel-level-pct"
	dataTypeVehicleCurrentStation = "current-station"

	dataTypeVehicleAvailability           = "availability"
	dataTypeVehicleFutureAvailability30   = "future-availability-30"
	dataTypeVehicleFutureAvailability60   = "future-availability-60"
	dataTypeVehicleFutureAvailability120  = "future-availability-120"
	dataTypeVehicleFutureAvailability360  = "future-availability-360"
	dataTypeVehicleFutureAvailability720  = "future-availability-720"
	dataTypeVehicleFutureAvailability1440 = "future-availability-1440"
)

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[Root] {
	return func(ctx context.Context, payload *rdb.Raw[Root]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Root]) error {
	// save stations by stationCode
	carsharing_stations := make(map[int]bdplib.Station)
	vehiles_stations := make(map[int]bdplib.Station)
	availability_by_car := make(map[int]*Availability)

	car_sharing_DataMap := bdp.CreateDataMap()
	vechile_dataMap := bdp.CreateDataMap()

	ts := payload.Timestamp.UnixMilli()
	now := payload.Timestamp

	for _, a := range payload.Rawdata.Availabilities {
		availability_by_car[a.VehicleID] = &a
	}

	for _, s := range payload.Rawdata.Stations {
		station_code := fmt.Sprintf("%d", s.ID)
		carsharing_stations[s.ID] = s.ToBDPStation(bdp)

		car_sharing_DataMap.AddRecord(station_code, dataTypeAvailableVehicles, bdplib.CreateRecord(ts, s.CapacityMax-s.CapacityCurrentlyFree, 300))
	}

	for _, v := range payload.Rawdata.Vehicles {
		vehicle_code := fmt.Sprintf("%d", v.ID)
		vehiles_stations[v.ID] = v.ToBDPStation(bdp)

		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFuel, bdplib.CreateRecord(ts, v.Fuel.Cents, 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleCleanness, bdplib.CreateRecord(ts, v.Cleanness, 300))
		if nil != v.Location {
			vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleCurrentStation, bdplib.CreateRecord(ts, fmt.Sprintf("%d", v.Location.ID), 300))
		} else {
			vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleCurrentStation, bdplib.CreateRecord(ts, v.Location.ID, 300))
		}

		checkAvailabilityAt := func(avail *Availability, t time.Time) int {
			if nil == avail {
				return 0
			}
			for _, slot := range avail.Slots {
				if !slot.Available {
					continue
				}

				from, err := time.Parse(time.RFC3339, slot.From)
				if err != nil {
					// Skip slots with invalid "from" time.
					continue
				}

				// Determine the end time. If Until is nil, treat it as an open-ended availability.
				var until time.Time
				if slot.Until != nil {
					until, err = time.Parse(time.RFC3339, *slot.Until)
					if err != nil {
						// Skip slots with an invalid "until" value.
						continue
					}
				} else {
					// Open-ended availability: set until to a far future date.
					until = time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
				}

				// Check if t falls within [from, until)
				if (t.Equal(from) || t.After(from)) && t.Before(until) {
					return 1
				}
			}
			return 0
		}

		avail, _ := availability_by_car[v.ID]

		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleAvailability, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now), 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFutureAvailability30, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now.Add(30*time.Minute)), 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFutureAvailability60, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now.Add(60*time.Minute)), 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFutureAvailability120, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now.Add(120*time.Minute)), 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFutureAvailability360, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now.Add(360*time.Minute)), 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFutureAvailability720, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now.Add(720*time.Minute)), 300))
		vechile_dataMap.AddRecord(vehicle_code, dataTypeVehicleFutureAvailability1440, bdplib.CreateRecord(ts, checkAvailabilityAt(avail, now.Add(1440*time.Minute)), 300))
	}

	// -------
	bdp.SyncStations(StationTypeCarSharing, values(carsharing_stations), true, true)
	bdp.SyncStations(StationTypeVechile, values(vehiles_stations), true, true)
	bdp.PushData(StationTypeCarSharing, car_sharing_DataMap)
	bdp.PushData(StationTypeVechile, vechile_dataMap)
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
	// station
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeAvailableVehicles, "", "number of available vehicles / charging points", "Instantaneous"))

	err := bdp.SyncDataTypes(StationTypeCarSharing, dataTypes)
	ms.FailOnError(context.Background(), err, fmt.Sprintf("failed to sync types for station %s", StationTypeCarSharing))

	dataTypes = []bdplib.DataType{}
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleCurrentStation, "", "The current station the car is parked in", "Instantaneous"))

	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleCleanness, "", "How clean is a Car", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFuel, "%", "Fuel Level in pct of a Car", "Instantaneous"))

	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleAvailability, "", "Indicates if a vehicle is available for rental ", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFutureAvailability30, "", "Availability in 30 minutes ", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFutureAvailability60, "", "Availability in 60 minutes ", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFutureAvailability120, "", "Availability in 120 minutes ", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFutureAvailability360, "", "Availability in 360 minutes ", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFutureAvailability720, "", "Availability in 720 minutes ", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(dataTypeVehicleFutureAvailability1440, "", "Availability in 1440 minutes ", "Instantaneous"))

	err = bdp.SyncDataTypes(StationTypeVechile, dataTypes)
	ms.FailOnError(context.Background(), err, fmt.Sprintf("failed to sync types for station %s", StationTypeVechile))
}

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()

	SyncDataTypes(b)

	listener := tr.NewTr[Root](context.Background(), env)
	err := listener.Start(context.Background(), TransformWithBdp(b))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env tr.Env

type RawType struct {
	Geometry struct {
		X float64 `json:"x"`
		Y float64 `json:"y"`
	} `json:"geometry"`

	Attributes struct {
		ID                           string   `json:"id"`
		ProviderID                   string   `json:"provider_id"`
		ProviderName                 string   `json:"provider_name"`
		ProviderTimezone             string   `json:"provider_timezone"`
		ProviderAppsIOS              string   `json:"provider_apps_ios_store_uri"`
		ProviderAppsAndroid          string   `json:"provider_apps_android_store_uri"`
		Available                    bool     `json:"available"`
		PickupType                   string   `json:"pickup_type"`
		StationName                  string   `json:"station_name"`
		StationStatusInstalled       bool     `json:"station_status_installed"`
		StationStatusRenting         bool     `json:"station_status_renting"`
		StationStatusReturning       bool     `json:"station_status_returning"`
		StationStatusNumVehicleAvail int      `json:"station_status_num_vehicle_available"`
		StationRegionID              string   `json:"station_region_id"`
		VehicleStatusDisabled        bool     `json:"vehicle_status_disabled"`
		VehicleStatusReserved        bool     `json:"vehicle_status_reserved"`
		VehicleType                  []string `json:"vehicle_type"`
	} `json:"attributes"`
}

const PERIOD = 600

var (
	dtAvailability     = bdplib.CreateDataType("availability", "", "Disponibile (1/0)", "instant")
	dtNumVehicles      = bdplib.CreateDataType("num_vehicle_available", "", "Veicoli disponibili", "instant")
	dtStationRenting   = bdplib.CreateDataType("station_status_renting", "", "Stazione abilitata al ritiro (1/0)", "instant")
	dtStationReturning = bdplib.CreateDataType("station_status_returning", "", "Stazione abilitata alla riconsegna (1/0)", "instant")
	dtVehicleDisabled  = bdplib.CreateDataType("vehicle_status_disabled", "", "Veicolo disabilitato (1/0)", "instant")
	dtVehicleReserved  = bdplib.CreateDataType("vehicle_status_reserved", "", "Veicolo prenotato (1/0)", "instant")
)

func first(a []string) string {
	if len(a) > 0 {
		return a[0]
	}
	return ""
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}

func stationType(pickup string, vehicle []string) string {
	switch pickup {
	case "station_based":
		switch first(vehicle) {
		case "E-Car", "Car":
			return "CarsharingStation"
		case "E-Bike", "Bike":
			return "BikesharingStation"
		case "E-CargoBike":
			return "BikesharingStation"
		case "E-Moped":
			return "MopedSharingStation"
		case "E-scooter":
			return "ScooterSharingStation"
		}
	case "free_floating":
		switch first(vehicle) {
		case "E-Car", "Car":
			return "CarsharingCar"
		case "E-Bike", "Bike", "E-CargoBike":
			return "Bicycle"
		case "E-Moped":
			return "Moped"
		case "E-scooter":
			return "Scooter"
		}
	}
	return "Unknown"
}

func register(b bdplib.Bdp) {
	slog.Info("Registering data types for all station types")
	for _, st := range []string{
		"CarsharingStation", "BikesharingStation", "MopedSharingStation", "ScooterSharingStation",
		"CarsharingCar", "Bicycle", "Moped", "Scooter",
	} {
		slog.Debug("SyncDataTypes for stationType", "stationType", st)
		b.SyncDataTypes(st, []bdplib.DataType{
			dtAvailability, dtNumVehicles, dtStationRenting,
			dtStationReturning, dtVehicleDisabled, dtVehicleReserved,
		})
	}
	slog.Info("Data types registration complete")
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()
	register(b)

	listener := tr.NewTr[RawType](context.Background(), env)
	slog.Info("Listener created, starting Start()...")
	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[RawType]) error {
		slog.Info("Callback invoked")

		raw := r.Rawdata
		attr := raw.Attributes
		slog.Debug("Raw extracted", "id", attr.ID)

		lon, lat := raw.Geometry.X, raw.Geometry.Y
		slog.Debug("Geometry values", "lat", lat, "lon", lon)

		st := stationType(attr.PickupType, attr.VehicleType)
		slog.Info("Determined stationType", "stationType", st)
		if st == "Unknown" {
			slog.Warn("Unknown stationType, skipping message", "pickupType", attr.PickupType)
			return nil
		}

		slog.Info("Syncing station metadata", "stationID", attr.ID)
		station := bdplib.CreateStation(
			attr.ID,
			firstNonEmpty(attr.StationName, attr.ID),
			st,
			lat,
			lon,
			attr.ProviderName,
		)
		station.MetaData = map[string]interface{}{
			"provider_id":               attr.ProviderID,
			"provider_timezone":         attr.ProviderTimezone,
			"provider_apps_ios_uri":     attr.ProviderAppsIOS,
			"provider_apps_android_uri": attr.ProviderAppsAndroid,
			"pickup_type":               attr.PickupType,
			"vehicle_type":              attr.VehicleType,
			"station_region_id":         attr.StationRegionID,
		}

		if err := b.SyncStations(st, []bdplib.Station{station}, true, false); err != nil {
			slog.Error("Error SyncStations", "err", err)
			return err
		}

		slog.Info("Creating records for station", "stationID", station.Id)
		recs := b.CreateDataMap()
		now := r.Timestamp.UnixMilli()

		iv := func(v bool) int {
			if v {
				return 1
			}
			return 0
		}

		recs.AddRecord(station.Id, dtAvailability.Name, bdplib.CreateRecord(now, iv(attr.Available), PERIOD))

		if attr.PickupType == "station_based" {
			recs.AddRecord(station.Id, dtNumVehicles.Name, bdplib.CreateRecord(now, attr.StationStatusNumVehicleAvail, PERIOD))
			recs.AddRecord(station.Id, dtStationRenting.Name, bdplib.CreateRecord(now, iv(attr.StationStatusRenting), PERIOD))
			recs.AddRecord(station.Id, dtStationReturning.Name, bdplib.CreateRecord(now, iv(attr.StationStatusReturning), PERIOD))
		} else {
			recs.AddRecord(station.Id, dtVehicleDisabled.Name, bdplib.CreateRecord(now, iv(attr.VehicleStatusDisabled), PERIOD))
			recs.AddRecord(station.Id, dtVehicleReserved.Name, bdplib.CreateRecord(now, iv(attr.VehicleStatusReserved), PERIOD))
		}
		slog.Info("Records created", "records", recs)

		slog.Info("Pushing data to backend", "stationType", st)
		if err := b.PushData(st, recs); err != nil {
			slog.Error("Error PushData", "err", err)
			return err
		}
		slog.Info("PushData successful", "stationID", station.Id)

		return nil
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
	slog.Info("Transformer exiting")
}

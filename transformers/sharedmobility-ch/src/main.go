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

const (
	StationTypeStation = "Station"
	StationTypeVehicle = "Vehicle"
	StationTypeGenericSharing = "SharingMobilityService"

	DataTypeNumberAvailable = "number-available"
	DataTypeDocksAvailable  = "num-docks-available"
	DataTypeIsInstalled     = "is-installed"
	DataTypeIsRenting       = "is-renting"
	DataTypeIsReturning     = "is-returning"
	DataTypeAvailability    = "availability"
	DataTypeInMaintenance   = "in-maintenance"

	Period = 300 // 5 minutes
)

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[Root] {
	return func(ctx context.Context, payload *rdb.Raw[Root]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Root]) error {
	ts := payload.Timestamp.UnixMilli()

	// 1. Create maps for quick lookup
	providersMap := make(map[string]Provider)
	for _, p := range payload.Rawdata.Providers {
		providersMap[p.ProviderID] = p
	}

	regionsMap := make(map[string]SystemRegion)
	for _, r := range payload.Rawdata.SystemRegions {
		regionsMap[r.RegionID] = r
	}

	stationStatusMap := make(map[string]StationStatus)
	for _, s := range payload.Rawdata.StationStatus {
		stationStatusMap[s.StationID] = s
	}

	// 2. Group stations and vehicles by region to calculate centroids
	regionStations := make(map[string][]StationInformation)
	for _, s := range payload.Rawdata.StationInformation {
		if s.RegionID != "" {
			regionStations[s.RegionID] = append(regionStations[s.RegionID], s)
		}
	}

	// 3. Transform Virtual Service Stations (one per region)
	virtualStations := make(map[string]bdplib.Station)
	virtualStationTypes := make(map[string]string)

	for regionID, region := range regionsMap {
		// Calculate centroid
		var lat, lon float64
		count := 0
		for _, s := range regionStations[regionID] {
			lat += s.Lat
			lon += s.Lon
			count++
		}
		if count > 0 {
			lat /= float64(count)
			lon /= float64(count)
		}

		// Determine station type from providers
		// According to spec: "The stationtype field should be defined according the field vehicle_type in the providers web-service"
		// If all providers have the same vehicle type, use that type; otherwise use generic
		stationType := StationTypeGenericSharing
		if len(payload.Rawdata.Providers) > 0 {
			firstType := payload.Rawdata.Providers[0].GetStationType()
			allSameType := true
			for _, p := range payload.Rawdata.Providers {
				if p.GetStationType() != firstType {
					allSameType = false
					break
				}
			}
			if allSameType {
				stationType = firstType
			}
		}

		bdpStation := bdplib.CreateStation(regionID, region.Name, stationType, lat, lon, bdp.GetOrigin())
		bdpStation.MetaData = make(map[string]any)
		// Per spec Table 2: providers, system_pricing_plans, geofencing_zones, system_hours → METADATA
		// Note: Spec mentions filtering by provider_id, but without provider_id in SystemRegion,
		// we store all providers/plans/hours/zones for the region as metadata
		bdpStation.MetaData["providers"] = payload.Rawdata.Providers
		bdpStation.MetaData["system_pricing_plans"] = payload.Rawdata.Plans
		bdpStation.MetaData["geofencing_zones"] = payload.Rawdata.GeofencingZones
		bdpStation.MetaData["system_hours"] = payload.Rawdata.SystemHours

		virtualStations[regionID] = bdpStation
		virtualStationTypes[regionID] = stationType
	}

	// 4. Transform Physical Stations
	physicalStations := make(map[string]bdplib.Station)
	physicalDataMap := bdp.CreateDataMap()

	for _, s := range payload.Rawdata.StationInformation {
		bdpStation := bdplib.CreateStation(s.StationID, s.Name, StationTypeStation, s.Lat, s.Lon, bdp.GetOrigin())
		if s.RegionID != "" {
			bdpStation.ParentStation = s.RegionID
		}
		physicalStations[s.StationID] = bdpStation

		// Real-time measurements
		if status, ok := stationStatusMap[s.StationID]; ok {
			physicalDataMap.AddRecord(s.StationID, DataTypeNumberAvailable, bdplib.CreateRecord(ts, status.NumBikesAvailable, Period))
			physicalDataMap.AddRecord(s.StationID, DataTypeDocksAvailable, bdplib.CreateRecord(ts, status.NumDocksAvailable, Period))
			physicalDataMap.AddRecord(s.StationID, DataTypeIsInstalled, bdplib.CreateRecord(ts, status.IsInstalled, Period))
			physicalDataMap.AddRecord(s.StationID, DataTypeIsRenting, bdplib.CreateRecord(ts, status.IsRenting, Period))
			physicalDataMap.AddRecord(s.StationID, DataTypeIsReturning, bdplib.CreateRecord(ts, status.IsReturning, Period))
		}
	}

	// 5. Transform Vehicles
	vehicleStations := make(map[string]bdplib.Station)
	vehicleDataMap := bdp.CreateDataMap()

	for _, v := range payload.Rawdata.FreeBikeStatus {
		// Pointprojection empty, location in measurementJSON (per spec line 141)
		bdpStation := bdplib.CreateStation(v.BikeID, v.BikeID, StationTypeVehicle, 0, 0, bdp.GetOrigin())
		// Vehicles don't have parent stations per specification
		vehicleStations[v.BikeID] = bdpStation

		// Location in measurementJSON (per spec: "It is proposed to leave the point projection empty and to manage it as measurementJSON")
		location := map[string]float64{
			"lat": v.Lat,
			"lon": v.Lon,
		}
		measurementJSON := map[string]any{
			"location": location,
		}

		// Real-time measurements (per spec lines 153-154)
		availability := !v.IsReserved && !v.IsDisabled
		inMaintenance := v.IsDisabled

		vehicleDataMap.AddRecord(v.BikeID, DataTypeAvailability, bdplib.CreateRecord(ts, availability, Period))
		vehicleDataMap.AddRecord(v.BikeID, DataTypeInMaintenance, bdplib.CreateRecord(ts, inMaintenance, Period))
		// Store location in measurementJSON via status-details data type
		vehicleDataMap.AddRecord(v.BikeID, "status-details", bdplib.CreateRecord(ts, measurementJSON, Period))
	}

	// 6. Sync and Push
	// Sync virtual stations first (parents)
	// We need to group virtual stations by their type because SyncStations takes one type at a time
	typeGroupedVirtual := make(map[string][]bdplib.Station)
	for regionID, s := range virtualStations {
		sType := virtualStationTypes[regionID]
		typeGroupedVirtual[sType] = append(typeGroupedVirtual[sType], s)
	}
	for sType, stations := range typeGroupedVirtual {
		bdp.SyncStations(sType, stations, true, true)
	}

	bdp.SyncStations(StationTypeStation, values(physicalStations), true, true)
	bdp.SyncStations(StationTypeVehicle, values(vehicleStations), true, true)

	bdp.PushData(StationTypeStation, physicalDataMap)
	bdp.PushData(StationTypeVehicle, vehicleDataMap)
	// Virtual stations might not have periodic data besides metadata in sync, 
	// but we could push data if there were aggregated measurements.

	return nil
}

func values[M ~map[K]V, K comparable, V any](m M) []V {
	r := make([]V, 0, len(m))
	for _, v := range m {
		r = append(r, v)
	}
	return r
}

func SyncDataTypes(bdp bdplib.Bdp) {
	stationDataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeNumberAvailable, "", "number of available vehicles", "Instantaneous"),
		bdplib.CreateDataType(DataTypeDocksAvailable, "", "number of available docks", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsInstalled, "", "is the station installed", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsRenting, "", "is the station renting", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsReturning, "", "is the station returning", "Instantaneous"),
	}
	bdp.SyncDataTypes(stationDataTypes)

	vehicleDataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeAvailability, "", "is the vehicle available", "Instantaneous"),
		bdplib.CreateDataType(DataTypeInMaintenance, "", "is the vehicle in maintenance", "Instantaneous"),
		bdplib.CreateDataType("status-details", "", "detailed status and location", "Instantaneous"),
	}
	bdp.SyncDataTypes(vehicleDataTypes)
}

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting sharedmobility-ch data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()

	SyncDataTypes(b)

	listener := tr.NewTr[Root](context.Background(), env)
	err := listener.Start(context.Background(), TransformWithBdp(b))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}


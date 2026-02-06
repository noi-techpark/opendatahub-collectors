// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
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

func bool2Int(b bool) int {
	if b {
		return 1
	} else {
		return 0
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
		// If all providers have the same vehicle type, use that type; otherwise use the most common type
		stationType := StationTypeGenericSharing
		typeCount := make(map[string]int)
		if len(payload.Rawdata.Providers) > 0 {
			firstType := payload.Rawdata.Providers[0].GetStationType()
			allSameType := true


			if allSameType {
				stationType = firstType
			} else {
				// Use the most common provider type instead of generic
				maxCount := 0
				mostCommonType := StationTypeGenericSharing
				for providerType, count := range typeCount {
					if count > maxCount {
						maxCount = count
						mostCommonType = providerType
					}
				}
				stationType = mostCommonType
			}
		}

		bdpStation := bdplib.CreateStation(fmt.Sprintf("%s:re:%s", bdp.GetOrigin(), regionID), region.Name, stationType, lat, lon, bdp.GetOrigin())
		bdpStation.MetaData = make(map[string]any)
		// Per spec Table 2: providers, system_pricing_plans, geofencing_zones, system_hours → METADATA
		// Note: Spec mentions filtering by provider_id, but without provider_id in SystemRegion,
		// we store all providers/plans/hours/zones for the region as metadata
		// Only add non-empty metadata to avoid null pointer issues
		if len(payload.Rawdata.Providers) > 0 {
			bdpStation.MetaData["providers"] = payload.Rawdata.Providers
		}
		if len(payload.Rawdata.Plans) > 0 {
			bdpStation.MetaData["system_pricing_plans"] = payload.Rawdata.Plans
		}
		if payload.Rawdata.GeofencingZones.Features != nil && len(payload.Rawdata.GeofencingZones.Features) > 0 {
			bdpStation.MetaData["geofencing_zones"] = payload.Rawdata.GeofencingZones
		}
		if len(payload.Rawdata.SystemHours) > 0 {
			bdpStation.MetaData["system_hours"] = payload.Rawdata.SystemHours
		}

		virtualStations[regionID] = bdpStation
		virtualStationTypes[regionID] = stationType
	}

	// 4. Transform Physical Stations
	// Group physical stations by their station type (based on parent virtual station type)
	physicalStationsByType := make(map[string]map[string]bdplib.Station)
	physicalDataMapsByType := make(map[string]*bdplib.DataMap)

	for _, s := range payload.Rawdata.StationInformation {
		// Determine station type from parent virtual station
		stationType := GetStationTypeForPhysicalStation(StationTypeGenericSharing)
		if s.RegionID != "" && s.RegionID != ":" && len(s.RegionID) > 1 {
			// Clean up malformed region IDs (e.g., "velospot:" -> try to find matching region)
			regionID := s.RegionID
			if strings.HasSuffix(regionID, ":") {
				// Try to find a region that starts with this prefix
				for rID := range virtualStationTypes {
					if strings.HasPrefix(rID, strings.TrimSuffix(regionID, ":")) {
						regionID = rID
						break
					}
				}
			}

			if parentType, ok := virtualStationTypes[regionID]; ok {
				stationType = GetStationTypeForPhysicalStation(parentType)
			} else {
				// If region not found, use the most common provider type as fallback
				mostCommonType := payload.Rawdata.getMostCommonProviderType()
				stationType = GetStationTypeForPhysicalStation(mostCommonType)
			}
		} else {
			// No region_id or malformed, use most common provider type
			mostCommonType := payload.Rawdata.getMostCommonProviderType()
			stationType = GetStationTypeForPhysicalStation(mostCommonType)
		}

		// Initialize maps for this station type if needed
		if physicalStationsByType[stationType] == nil {
			physicalStationsByType[stationType] = make(map[string]bdplib.Station)
			dataMap := bdp.CreateDataMap()
			physicalDataMapsByType[stationType] = &dataMap
		}

		bdpStation := bdplib.CreateStation(fmt.Sprintf("%s:st:%s", bdp.GetOrigin(), s.StationID), s.Name, stationType, s.Lat, s.Lon, bdp.GetOrigin())
		if s.RegionID != "" {
			bdpStation.ParentStation = virtualStations[s.RegionID].Id
		}
		physicalStationsByType[stationType][s.StationID] = bdpStation

		// Real-time measurements
		if status, ok := stationStatusMap[s.StationID]; ok {
			physicalDataMapsByType[stationType].AddRecord(bdpStation.Id, DataTypeNumberAvailable, bdplib.CreateRecord(ts, status.NumBikesAvailable, Period))
			physicalDataMapsByType[stationType].AddRecord(bdpStation.Id, DataTypeDocksAvailable, bdplib.CreateRecord(ts, status.NumDocksAvailable, Period))
			physicalDataMapsByType[stationType].AddRecord(bdpStation.Id, DataTypeIsInstalled, bdplib.CreateRecord(ts, bool2Int(status.IsInstalled), Period))
			physicalDataMapsByType[stationType].AddRecord(bdpStation.Id, DataTypeIsRenting, bdplib.CreateRecord(ts, bool2Int(status.IsRenting), Period))
			physicalDataMapsByType[stationType].AddRecord(bdpStation.Id, DataTypeIsReturning, bdplib.CreateRecord(ts, bool2Int(status.IsReturning), Period))
		}
	}

	// 5. Transform Vehicles
	// Group vehicles by their station type (based on vehicle_type_id)
	vehicleStationsByType := make(map[string]map[string]bdplib.Station)
	vehicleDataMapsByType := make(map[string]*bdplib.DataMap)

	for _, v := range payload.Rawdata.FreeBikeStatus {
		// Determine vehicle type from vehicle_type_id
		vehicleServiceType := payload.Rawdata.GetVehicleTypeFromVehicleTypeID(v.VehicleTypeID, providersMap)
		vehicleStationType := GetStationTypeForVehicle(vehicleServiceType)

		// Initialize maps for this vehicle type if needed
		if vehicleStationsByType[vehicleStationType] == nil {
			vehicleStationsByType[vehicleStationType] = make(map[string]bdplib.Station)
			dataMap := bdp.CreateDataMap()
			vehicleDataMapsByType[vehicleStationType] = &dataMap
		}

		// Pointprojection empty, location in measurementJSON (per spec line 141)
		bdpStation := bdplib.CreateStation(fmt.Sprintf("%s:vh:%s", bdp.GetOrigin(), v.BikeID), v.BikeID, vehicleStationType, 0, 0, bdp.GetOrigin())
		// Vehicles don't have parent stations per specification
		vehicleStationsByType[vehicleStationType][v.BikeID] = bdpStation

		// Location in measurementJSON (per spec: "It is proposed to leave the point projection empty and to manage it as measurementJSON")
		location := map[string]float64{
			"lat": v.Lat,
			"lon": v.Lon,
		}
		measurementJSON := map[string]any{
			"location": location,
		}

		// Real-time measurements (per spec lines 153-154)
		availability := bool2Int(!v.IsReserved && !v.IsDisabled)
		inMaintenance := bool2Int(v.IsDisabled)

		vehicleDataMapsByType[vehicleStationType].AddRecord(bdpStation.Id, DataTypeAvailability, bdplib.CreateRecord(ts, availability, Period))
		vehicleDataMapsByType[vehicleStationType].AddRecord(bdpStation.Id, DataTypeInMaintenance, bdplib.CreateRecord(ts, inMaintenance, Period))
		// Store location in measurementJSON via status-details data type
		vehicleDataMapsByType[vehicleStationType].AddRecord(bdpStation.Id, "status-details", bdplib.CreateRecord(ts, measurementJSON, Period))
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
		slog.Info("Syncing virtual stations", "type", sType, "count", len(stations))
		if err := bdp.SyncStations(sType, stations, true, true); err != nil {
			return err
		}
	}

	// Sync physical stations grouped by type
	for stationType, stations := range physicalStationsByType {
		slog.Info("Syncing physical stations", "type", stationType, "count", len(stations))
		if err := bdp.SyncStations(stationType, values(stations), true, true); err != nil {
			return err
		}
	}

	// Sync vehicles grouped by type
	for vehicleType, vehicles := range vehicleStationsByType {
		slog.Info("Syncing vehicles", "type", vehicleType, "count", len(vehicles))
		if err := bdp.SyncStations(vehicleType, values(vehicles), true, true); err != nil {
			return err
		}
	}

	// Push data for physical stations grouped by type
	for stationType, dataMap := range physicalDataMapsByType {
		if err := bdp.PushData(stationType, *dataMap); err != nil {
			return err
		}
	}

	// Push data for vehicles grouped by type
	for vehicleType, dataMap := range vehicleDataMapsByType {
		if err := bdp.PushData(vehicleType, *dataMap); err != nil {
			return err
		}
	}

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

func SyncDataTypes(bdp bdplib.Bdp) error {
	// Station data types (same for all sharing mobility station types)
	stationDataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeNumberAvailable, "", "number of available vehicles", "Instantaneous"),
		bdplib.CreateDataType(DataTypeDocksAvailable, "", "number of available docks", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsInstalled, "", "is the station installed", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsRenting, "", "is the station renting", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsReturning, "", "is the station returning", "Instantaneous"),
	}

	// Vehicle data types (same for all sharing mobility vehicle types)
	vehicleDataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeAvailability, "", "is the vehicle available", "Instantaneous"),
		bdplib.CreateDataType(DataTypeInMaintenance, "", "is the vehicle in maintenance", "Instantaneous"),
		bdplib.CreateDataType("status-details", "", "detailed status and location", "Instantaneous"),
	}

	// Sync data types (they will be associated with the appropriate station types when used)
	if err := bdp.SyncDataTypes(stationDataTypes); err != nil {
		return err
	}
	if err := bdp.SyncDataTypes(vehicleDataTypes); err != nil {
		return err
	}
	return nil
}

var env tr.Env

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting sharedmobility-ch data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()

	ms.FailOnError(context.Background(), SyncDataTypes(b), "failed syncing data types")

	listener := tr.NewTr[Root](context.Background(), env)
	err := listener.Start(context.Background(), TransformWithBdp(b))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

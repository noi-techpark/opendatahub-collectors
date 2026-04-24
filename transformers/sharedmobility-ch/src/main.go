// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"

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
	DataTypeFreeBikeStatus  = "free-bike-status"

	Period = 300 // 5 minutes

	swissLat = 46.8182
	swissLon = 8.2275
)

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[Root] {
	return func(ctx context.Context, payload *rdb.Raw[Root]) error {
		return Transform(ctx, bdp, payload)
	}
}

// expandGeoBounds recursively walks a GeoJSON coordinates value (decoded as any)
// and expands the bounding box to include every [lon, lat] leaf pair.
func expandGeoBounds(v any, minLat, maxLat, minLon, maxLon *float64) {
	arr, ok := v.([]any)
	if !ok {
		return
	}
	if len(arr) == 2 {
		lon, lonOK := arr[0].(float64)
		lat, latOK := arr[1].(float64)
		if lonOK && latOK {
			if lat < *minLat {
				*minLat = lat
			}
			if lat > *maxLat {
				*maxLat = lat
			}
			if lon < *minLon {
				*minLon = lon
			}
			if lon > *maxLon {
				*maxLon = lon
			}
			return
		}
	}
	for _, item := range arr {
		expandGeoBounds(item, minLat, maxLat, minLon, maxLon)
	}
}

func bool2Int(b bool) int {
	if b {
		return 1
	}
	return 0
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[Root]) error {
	ts := payload.Timestamp.UnixMilli()

	// 1. Build lookup maps
	regionsMap := make(map[string]SystemRegion)
	for _, r := range payload.Rawdata.SystemRegions {
		regionsMap[r.RegionID] = r
	}

	stationStatusMap := make(map[string]StationStatus)
	for _, s := range payload.Rawdata.StationStatus {
		stationStatusMap[s.StationID] = s
	}

	// 2. Group stations and free bikes by provider
	stationsByProvider := make(map[string][]StationInformation)
	for _, s := range payload.Rawdata.StationInformation {
		stationsByProvider[s.ProviderID] = append(stationsByProvider[s.ProviderID], s)
	}

	freeBikesByProvider := make(map[string][]FreeBikeStatus)
	for _, v := range payload.Rawdata.FreeBikeStatus {
		freeBikesByProvider[v.ProviderID] = append(freeBikesByProvider[v.ProviderID], v)
	}

	// 3. Create one virtual station per provider, compute aggregated measurements
	providerStationsByType := make(map[string][]bdplib.Station)
	providerDataMapsByType := make(map[string]*bdplib.DataMap)
	providerStationIDByProviderID := make(map[string]string)

	for i := range payload.Rawdata.Providers {
		p := &payload.Rawdata.Providers[i]
		stationType := p.GetStationType()

		if providerDataMapsByType[stationType] == nil {
			dm := bdp.CreateDataMap()
			providerDataMapsByType[stationType] = &dm
		}

		providerStationID := fmt.Sprintf("%s:pr:%s", bdp.GetOrigin(), p.ProviderID)
		providerStationIDByProviderID[p.ProviderID] = providerStationID

		bdpStation := bdplib.CreateStation(providerStationID, p.Name, stationType, 0, 0, bdp.GetOrigin())
		bdpStation.MetaData = make(map[string]any)

		var plans []PricingPlan
		for _, plan := range payload.Rawdata.Plans {
			if plan.ProviderID == p.ProviderID {
				plans = append(plans, plan)
			}
		}
		if len(plans) > 0 {
			bdpStation.MetaData["system_pricing_plans"] = plans
		}

		var hours []RentalHour
		for _, h := range payload.Rawdata.SystemHours {
			if h.ProviderID == p.ProviderID {
				hours = append(hours, h)
			}
		}
		if len(hours) > 0 {
			bdpStation.MetaData["system_hours"] = hours
		}

		var geoFeatures []any
		minLat, maxLat := 90.0, -90.0
		minLon, maxLon := 180.0, -180.0
		for _, f := range payload.Rawdata.GeofencingZones.Features {
			if f.Properties.ProviderID == p.ProviderID {
				geoFeatures = append(geoFeatures, f)
				if geom, ok := f.Geometry.(map[string]any); ok {
					expandGeoBounds(geom["coordinates"], &minLat, &maxLat, &minLon, &maxLon)
				}
			}
		}
		if len(geoFeatures) > 0 {
			bdpStation.MetaData["geofencing_zones"] = map[string]any{
				"type":     payload.Rawdata.GeofencingZones.Type,
				"features": geoFeatures,
			}
		}

		provLat, provLon := swissLat, swissLon
		if minLat <= maxLat {
			provLat = (minLat + maxLat) / 2
			provLon = (minLon + maxLon) / 2
		}
		bdpStation.Latitude = provLat
		bdpStation.Longitude = provLon

		providerStationsByType[stationType] = append(providerStationsByType[stationType], bdpStation)

		// Aggregated number-available: sum from physical stations + available free bikes
		numAvailable := 0
		numDocksAvailable := 0

		for _, s := range stationsByProvider[p.ProviderID] {
			if status, ok := stationStatusMap[s.StationID]; ok {
				numAvailable += status.NumBikesAvailable
				numDocksAvailable += status.NumDocksAvailable
			}
		}

		for _, v := range freeBikesByProvider[p.ProviderID] {
			if !v.IsReserved && !v.IsDisabled {
				numAvailable++
			}
		}

		dm := providerDataMapsByType[stationType]
		dm.AddRecord(providerStationID, DataTypeNumberAvailable, bdplib.CreateRecord(ts, numAvailable, Period))
		dm.AddRecord(providerStationID, DataTypeDocksAvailable, bdplib.CreateRecord(ts, numDocksAvailable, Period))

		if bikes := freeBikesByProvider[p.ProviderID]; len(bikes) > 0 {
			records := make([]FreeBikeStatusRecord, len(bikes))
			for i, b := range bikes {
				records[i] = FreeBikeStatusRecord{
					BikeID:             b.BikeID,
					Lat:                b.Lat,
					Lon:                b.Lon,
					IsReserved:         b.IsReserved,
					IsDisabled:         b.IsDisabled,
					VehicleTypeID:      b.VehicleTypeID,
					PricingPlanID:      b.PricingPlanID,
					CurrentRangeMeters: b.CurrentRangeMeters,
				}
			}
			dm.AddRecord(providerStationID, DataTypeFreeBikeStatus, bdplib.CreateRecord(ts, map[string]any{
				"free_bike_status": records,
			}, Period))
		}
	}

	// 4. Create physical stations as children of their provider station
	physicalStationsByType := make(map[string][]bdplib.Station)
	physicalDataMapsByType := make(map[string]*bdplib.DataMap)

	providerTypeByID := make(map[string]string)
	for _, p := range payload.Rawdata.Providers {
		providerTypeByID[p.ProviderID] = p.GetStationType()
	}

	for _, s := range payload.Rawdata.StationInformation {
		if s.ProviderID == "" {
			slog.Warn("Skipping station without provider_id", "station_id", s.StationID)
			continue
		}

		serviceType := providerTypeByID[s.ProviderID]
		if serviceType == "" {
			serviceType = StationTypeGenericSharing
		}
		stationType := GetStationTypeForPhysicalStation(serviceType)

		if physicalDataMapsByType[stationType] == nil {
			dm := bdp.CreateDataMap()
			physicalDataMapsByType[stationType] = &dm
		}

		name := s.Name
		if name == "" {
			name = s.StationID
		}
		bdpStation := bdplib.CreateStation(fmt.Sprintf("%s:st:%s", bdp.GetOrigin(), s.StationID), name, stationType, s.Lat, s.Lon, bdp.GetOrigin())
		bdpStation.MetaData = make(map[string]any)

		if region, ok := regionsMap[s.RegionID]; ok {
			bdpStation.MetaData["region_id"] = region.RegionID
			bdpStation.MetaData["region_name"] = region.Name
		}

		bdpStation.ParentStation = providerStationIDByProviderID[s.ProviderID]

		physicalStationsByType[stationType] = append(physicalStationsByType[stationType], bdpStation)

		if status, ok := stationStatusMap[s.StationID]; ok {
			dm := physicalDataMapsByType[stationType]
			dm.AddRecord(bdpStation.Id, DataTypeNumberAvailable, bdplib.CreateRecord(ts, status.NumBikesAvailable, Period))
			dm.AddRecord(bdpStation.Id, DataTypeDocksAvailable, bdplib.CreateRecord(ts, status.NumDocksAvailable, Period))
			dm.AddRecord(bdpStation.Id, DataTypeIsInstalled, bdplib.CreateRecord(ts, bool2Int(status.IsInstalled), Period))
			dm.AddRecord(bdpStation.Id, DataTypeIsRenting, bdplib.CreateRecord(ts, bool2Int(status.IsRenting), Period))
			dm.AddRecord(bdpStation.Id, DataTypeIsReturning, bdplib.CreateRecord(ts, bool2Int(status.IsReturning), Period))
		}
	}

	// 5. Sync provider stations first (parents), then physical stations (children)
	for sType, stations := range providerStationsByType {
		slog.Info("Syncing provider stations", "type", sType, "count", len(stations))
		if err := bdp.SyncStations(sType, stations, true, true); err != nil {
			return err
		}
	}

	for sType, stations := range physicalStationsByType {
		slog.Info("Syncing physical stations", "type", sType, "count", len(stations))
		if err := bdp.SyncStations(sType, stations, true, true); err != nil {
			return err
		}
	}

	for sType, dataMap := range providerDataMapsByType {
		if err := bdp.PushData(sType, *dataMap); err != nil {
			return err
		}
	}

	for sType, dataMap := range physicalDataMapsByType {
		if err := bdp.PushData(sType, *dataMap); err != nil {
			return err
		}
	}

	return nil
}

func SyncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(DataTypeNumberAvailable, "", "number of available vehicles", "Instantaneous"),
		bdplib.CreateDataType(DataTypeDocksAvailable, "", "number of available docks", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsInstalled, "", "is the station installed", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsRenting, "", "is the station renting", "Instantaneous"),
		bdplib.CreateDataType(DataTypeIsReturning, "", "is the station returning", "Instantaneous"),
		bdplib.CreateDataType(DataTypeFreeBikeStatus, "", "free floating bike statuses as JSON", "Instantaneous"),
	}
	return bdp.SyncDataTypes(dataTypes)
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

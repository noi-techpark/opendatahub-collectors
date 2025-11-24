// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"
	"strings"

	// Added for time.Time in CountingArea
	"github.com/noi-techpark/go-bdp-client/bdplib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

// --- CONSTANTS ---
const (
	StationTypeParkingFacility = "ParkingFacility"
	StationTypeParkingStation  = "ParkingStation"
)

// Aggregated Data Types
const (
	DataTypeFree     = "free"
	DataTypeOccupied = "occupied"

	PERIOD = 300
)

// Per-Type Data Types (Explicitly Declared)
const (
	// Standard
	DataTypeFreeStandard     = "free_standard"
	DataTypeOccupiedStandard = "occupied_standard"
	// Disabled
	DataTypeFreeDisabled     = "free_accessible"
	DataTypeOccupiedDisabled = "occupied_accessible"
	// ShortTerm
	DataTypeFreeShortTerm     = "free_shortterm"
	DataTypeOccupiedShortTerm = "occupied_shortterm"
	// Electric
	DataTypeFreeElectric     = "free_electric"
	DataTypeOccupiedElectric = "occupied_electric"
	// Motorbike
	DataTypeFreeMotorbike     = "free_motorbike"
	DataTypeOccupiedMotorbike = "occupied_motorbike"
	// Truck
	DataTypeFreeTruck     = "free_truck"
	DataTypeOccupiedTruck = "occupied_truck"
)

var env tr.Env

// Helper struct for parsed name components
type AreaNameComponents struct {
	Standort                string
	AreaNr                  string
	AnzahlGesamtparkplaetze int
	Typ                     string
}

// --- MAIN FUNCTION AND TRANSFORMER SETUP ---

var StationProto Stations = nil

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data (parking) transformer...")

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

	StationProto = ReadStations("./resources/stations.csv")

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync types")

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[string](context.Background(), env)

	err = listener.Start(context.Background(),
		tr.RawString2JsonMiddleware[CountingAreaList](TransformWithBdp(b)))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[CountingAreaList] {
	return func(ctx context.Context, payload *rdb.Raw[CountingAreaList]) error {
		return Transform(ctx, bdp, payload)
	}
}

// --- TRANSFORM IMPLEMENTATION ---

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[CountingAreaList]) error {
	slog.Info("Processing parking counting area data transformation", "timestamp", payload.Timestamp)

	// Group stations by their site_id to later create ParkingFacilities
	stationsByFacility := make(map[string][]bdplib.Station)
	stationDataMap := bdp.CreateDataMap()
	facilityDataMap := bdp.CreateDataMap() // Data map for Facility measurements

	ts := payload.Timestamp.UnixMilli()

	// Grouping all data by SiteID
	areasBySite := make(map[string][]CountingArea)
	// Iterate over the pointer to the slice of areas
	for _, area := range payload.Rawdata {
		// Important: Filter out "line_crossing" areas as per requirements
		if area.Type == "line_crossing" {
			slog.Debug("Skipping line_crossing area", "id", area.ID, "name", area.Name)
			continue
		}

		// Get the latest measurement (assuming 'totals' only has one or the last is the latest)
		if len(area.Counts.Totals) == 0 {
			slog.Warn("Skipping area with no count data", "id", area.ID, "name", area.Name)
			continue
		}

		areasBySite[area.SiteID] = append(areasBySite[area.SiteID], area)
	}

	var allParkingStations []*bdplib.Station
	var allParkingFacilities []*bdplib.Station

	for siteID, areas := range areasBySite {
		facilityName := ""
		totalCapacity := 0

		// Maps to hold capacity sums for ParkingFacility metadata
		facilityCapacityByType := make(map[string]int)

		// Accumulators for Facility Measurements (NEW)
		facilityOccupancyTotal := 0
		facilityOccupancyByType := make(map[string]int)

		// --- 2. Create Parking Stations (Cameras) and Push Measurements ---
		for _, area := range areas {
			// 1. Parse name and capacity from the area name
			parsed := parseAreaName(area.Name)

			// Set facility name from the first part of the area name
			if facilityName == "" {
				facilityName = parsed.Standort
			}

			capacity := parsed.AnzahlGesamtparkplaetze

			// 2. Create the ParkingStation (Camera)
			singleTypeCapacityMap := map[string]int{parsed.Typ: capacity}
			station := createParkingStation(bdp, area, parsed, capacity, singleTypeCapacityMap)
			if nil == station {
				continue
			}

			allParkingStations = append(allParkingStations, station)
			stationsByFacility[siteID] = append(stationsByFacility[siteID], *station)

			// 3. Calculate and Add Measurements for the ParkingStation
			occupancy := area.Counts.Totals[len(area.Counts.Totals)-1].CountVehicle

			// Free is calculated as Capacity - Occupancy
			free := capacity - occupancy
			if free < 0 {
				free = 0 // Cap free spots at zero
			}

			// Get the constant data type names for the current area's type
			freeTypeDataType := getFreeTypeConst(parsed.Typ)
			occupiedTypeDataType := getOccupiedTypeConst(parsed.Typ)

			// --- STATION Aggregated Measurements ---
			stationDataMap.AddRecord(station.Id, DataTypeOccupied, bdplib.CreateRecord(ts, occupancy, PERIOD))
			stationDataMap.AddRecord(station.Id, DataTypeFree, bdplib.CreateRecord(ts, free, PERIOD))

			// --- STATION Per-Type Measurements ---
			stationDataMap.AddRecord(station.Id, occupiedTypeDataType, bdplib.CreateRecord(ts, occupancy, PERIOD))
			stationDataMap.AddRecord(station.Id, freeTypeDataType, bdplib.CreateRecord(ts, free, PERIOD))

			// --- Accumulate for Facility ---
			totalCapacity += capacity
			facilityCapacityByType[parsed.Typ] += capacity   // For Facility Metadata
			facilityOccupancyTotal += occupancy              // For Facility Aggregated Measurement
			facilityOccupancyByType[parsed.Typ] += occupancy // For Facility Per-Type Measurement
		}

		// --- 1. Create Parking Facility (Site) ---
		facility := createParkingFacility(bdp, siteID, facilityName, totalCapacity, facilityCapacityByType)
		if nil == facility {
			continue
		}
		allParkingFacilities = append(allParkingFacilities, facility)

		// --- 4. Push Facility Measurements ---

		// a) Facility Aggregated Measurements
		facilityFreeTotal := totalCapacity - facilityOccupancyTotal
		if facilityFreeTotal < 0 {
			facilityFreeTotal = 0
		}

		facilityDataMap.AddRecord(facility.Id, DataTypeOccupied, bdplib.CreateRecord(ts, facilityOccupancyTotal, PERIOD))
		facilityDataMap.AddRecord(facility.Id, DataTypeFree, bdplib.CreateRecord(ts, facilityFreeTotal, PERIOD))

		// b) Facility Per-Type Measurements
		for typeName, typeCapacity := range facilityCapacityByType {
			// Get the total occupancy for this type
			typeOccupancy := facilityOccupancyByType[typeName]

			// Calculate free spots for this type
			typeFree := typeCapacity - typeOccupancy
			if typeFree < 0 {
				typeFree = 0
			}

			// Get the constant data type names for the current type
			freeTypeDataType := getFreeTypeConst(typeName)
			occupiedTypeDataType := getOccupiedTypeConst(typeName)

			// Push per-type records
			facilityDataMap.AddRecord(facility.Id, occupiedTypeDataType, bdplib.CreateRecord(ts, typeOccupancy, PERIOD))
			facilityDataMap.AddRecord(facility.Id, freeTypeDataType, bdplib.CreateRecord(ts, typeFree, PERIOD))
		}
	}

	slog.Info("Syncing stations and pushing data",
		"facilities", len(allParkingFacilities),
		"stations", len(allParkingStations))

	// --- 5. Sync and Push Data ---

	// Sync ParkingFacilities first (parents)
	err := bdp.SyncStations(StationTypeParkingFacility, ptrToRef(allParkingFacilities), true, false)
	ms.FailOnError(ctx, err, "failed to push StationTypeParkingFacility")

	// Sync ParkingStations (children)
	err = bdp.SyncStations(StationTypeParkingStation, ptrToRef(allParkingStations), true, false)
	ms.FailOnError(ctx, err, "failed to push StationTypeParkingStation")

	// Push measurements for ParkingStations
	err = bdp.PushData(StationTypeParkingStation, stationDataMap)
	ms.FailOnError(ctx, err, "failed to push StationTypeParkingStation records")

	// Push measurements for ParkingFacilities (NEW)
	err = bdp.PushData(StationTypeParkingFacility, facilityDataMap)
	ms.FailOnError(ctx, err, "failed to push StationTypeParkingFacility records")

	slog.Info("Parking counting area data transformation completed successfully")

	return nil
}

func ptrToRef(ptrs []*bdplib.Station) []bdplib.Station {
	s := make([]bdplib.Station, len(ptrs))
	for i, p := range ptrs {
		s[i] = *p
	}
	return s
}

// --- HELPER FUNCTIONS ---

func parseAreaName(name string) AreaNameComponents {
	components := strings.Split(name, "_")

	if len(components) < 4 {
		slog.Warn("Area name does not match expected format", "name", name)
		return AreaNameComponents{Standort: name, Typ: mapAreaType("STD"), AnzahlGesamtparkplaetze: 0}
	}

	// [Standort]_[AreaNr.]_[Anzahl Gesamtparkplätze in Area]_[TYP]
	standort := components[0]
	areaNr := components[1]
	anzahlStr := components[2]
	typ := components[3]

	capacity, err := strconv.Atoi(anzahlStr)
	if err != nil {
		slog.Error("Could not parse capacity from area name", "name", name, "value", anzahlStr, "error", err)
		capacity = 0
	}

	return AreaNameComponents{
		Standort:                standort,
		AreaNr:                  areaNr,
		AnzahlGesamtparkplaetze: capacity,
		Typ:                     mapAreaType(typ), // Normalize type name
	}
}

// createParkingStation now accepts a map for capacityByType
func createParkingStation(bdp bdplib.Bdp, area CountingArea, parsed AreaNameComponents, capacity int, capacityByType map[string]int) *bdplib.Station {
	proto := StationProto.GetStationByID(area.ID)
	if nil == proto {
		slog.Warn("area not found in station.csv configuration", "id", area.ID)
		return nil
	}

	station := bdplib.CreateStation(
		area.Name,
		proto.StandardName,
		StationTypeParkingStation,
		proto.Lat, proto.Lon,
		bdp.GetOrigin(),
	)

	// Set Parent Station ID to the site_id (ParkingFacility)
	station.ParentStation = area.SiteID

	metadata := proto.ToMetadata()

	// METADATA fields
	metadata["class_categories"] = area.AppearanceParams.ClassCategories
	metadata["vehicle_types"] = area.AppearanceParams.VehicleTypes
	metadata["station_type_suffix"] = parsed.Typ
	metadata["provider_id"] = area.ID

	// Required capacity fields: capacity is total
	metadata["capacity"] = capacity

	// Per-Type Capacity for this Station (Requested change)
	for typ, cap := range capacityByType {
		// e.g., capacity_standard (since each camera covers one type)
		metadata["capacity_"+strings.ToLower(typ)] = cap
	}

	station.MetaData = metadata
	return &station
}

func createParkingFacility(bdp bdplib.Bdp, siteID, facilityName string, totalCapacity int, capacityByType map[string]int) *bdplib.Station {
	proto := StationProto.GetStationByID(siteID)
	if nil == proto {
		slog.Warn("site not found in station.csv configuration", "id", siteID)
		return nil
	}

	station := bdplib.CreateStation(
		facilityName,
		proto.StandardName,
		StationTypeParkingFacility,
		proto.Lat, proto.Lon,
		bdp.GetOrigin(),
	)

	metadata := proto.ToMetadata()

	// Required capacity fields: capacity is total
	metadata["capacity"] = totalCapacity
	metadata["provider_id"] = siteID

	// Map of capacity by type for the metadata
	for typ, cap := range capacityByType {
		// e.g., capacity_disabled
		metadata["capacity_"+strings.ToLower(typ)] = cap
	}

	station.MetaData = metadata
	return &station
}

// Normalizes the type suffix from the name string
func mapAreaType(typ string) string {
	switch strings.ToUpper(typ) {
	case "STD":
		return "Standard"
	case "DIS":
		return "Disabled"
	case "SHT":
		return "ShortTerm"
	case "ELE":
		return "Electric"
	case "MOT":
		return "Motorbike"
	case "LKW":
		return "Truck"
	default:
		return "Standard" // Assuming STD is the default/fallback
	}
}

// Function to map normalized type to explicit 'free' constant
func getFreeTypeConst(normalizedType string) string {
	switch normalizedType {
	case "Standard":
		return DataTypeFreeStandard
	case "Disabled":
		return DataTypeFreeDisabled
	case "ShortTerm":
		return DataTypeFreeShortTerm
	case "Electric":
		return DataTypeFreeElectric
	case "Motorbike":
		return DataTypeFreeMotorbike
	case "Truck":
		return DataTypeFreeTruck
	default:
		slog.Warn("Unknown parking type for Free measurement", "type", normalizedType)
		return DataTypeFreeStandard // Fallback
	}
}

// Function to map normalized type to explicit 'occupied' constant
func getOccupiedTypeConst(normalizedType string) string {
	switch normalizedType {
	case "Standard":
		return DataTypeOccupiedStandard
	case "Disabled":
		return DataTypeOccupiedDisabled
	case "ShortTerm":
		return DataTypeOccupiedShortTerm
	case "Electric":
		return DataTypeOccupiedElectric
	case "Motorbike":
		return DataTypeOccupiedMotorbike
	case "Truck":
		return DataTypeOccupiedTruck
	default:
		slog.Warn("Unknown parking type for Occupied measurement", "type", normalizedType)
		return DataTypeOccupiedStandard // Fallback
	}
}

// syncDataTypes now uses the explicit constants for all data types.
func syncDataTypes(bdp bdplib.Bdp) error {
	var dataTypes []bdplib.DataType

	// --- Aggregated Types ---
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeFree, "count", "Free parking spots (aggregated)", "Instantaneous"))
	dataTypes = append(dataTypes, bdplib.CreateDataType(
		DataTypeOccupied, "count", "Occupied parking spots (aggregated)", "Instantaneous"))

	// --- Per-Type Definitions (using explicit constants) ---
	typeDefs := []struct {
		Type          string
		FreeConst     string
		OccupiedConst string
	}{
		{"Standard", DataTypeFreeStandard, DataTypeOccupiedStandard},
		{"Accessible", DataTypeFreeDisabled, DataTypeOccupiedDisabled},
		{"ShortTerm", DataTypeFreeShortTerm, DataTypeOccupiedShortTerm},
		{"Electric", DataTypeFreeElectric, DataTypeOccupiedElectric},
		{"Motorbike", DataTypeFreeMotorbike, DataTypeOccupiedMotorbike},
		{"Truck", DataTypeFreeTruck, DataTypeOccupiedTruck},
	}

	for _, def := range typeDefs {
		dataTypes = append(dataTypes, bdplib.CreateDataType(
			def.FreeConst, "count", "Free "+def.Type+" parking spots", "Instantaneous"))

		dataTypes = append(dataTypes, bdplib.CreateDataType(
			def.OccupiedConst, "count", "Occupied "+def.Type+" parking spots", "Instantaneous"))
	}

	return bdp.SyncDataTypes(dataTypes)
}

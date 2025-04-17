// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"
	"strconv"

	"github.com/gocarina/gocsv"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
)

// Station represents one record from the CSV.
type Station struct {
	ID                    string  `csv:"id"`
	StationID             string  `csv:"facility_id"`
	Name                  string  `csv:"name"`
	Municipality          string  `csv:"municipality"`
	NameEn                string  `csv:"name_en"`
	NameIt                string  `csv:"name_it"`
	NameDe                string  `csv:"name_de"`
	StandardName          string  `csv:"standard_name"`
	NetexType             string  `csv:"netex_type"`
	NetexVehicleTypes     string  `csv:"netex_vehicletypes"`
	NetexLayout           string  `csv:"netex_layout"`
	NetexHazardProhibited string  `csv:"netex_hazard_prohibited"`
	NetexCharging         string  `csv:"netex_charging"`
	NetexSurveillance     string  `csv:"netex_surveillance"`
	NetexReservation      string  `csv:"netex_reservation"`
	Lat                   float64 `csv:"lat"`
	Lon                   float64 `csv:"lon"`
}

type Stations []Station

// readStations opens and unmarshals the CSV file into a slice of Station pointers.
func ReadStations(filename string) Stations {
	f, err := os.Open(filename)
	ms.FailOnError(context.Background(), err, "failed opening csv file")
	defer f.Close()

	var facilities Stations
	err = gocsv.UnmarshalFile(f, &facilities)
	ms.FailOnError(context.Background(), err, "failed unmarshalling csv")

	return facilities
}

// getStationByID returns a pointer to a Station with the matching StationID.
// Returns nil if no matching record is found.
func (s Stations) GetStationByID(facilityID string) *Station {
	for _, f := range s {
		if f.StationID == facilityID {
			return &f
		}
	}
	return nil
}

// ToMetadata converts the Station record into a map[string]any,
// including only non-empty fields. It also nests the netex-related fields
// under the "netex_parking" key.
func (f *Station) ToMetadata() map[string]any {
	result := make(map[string]any)

	// Add top-level fields if non-empty.
	if f.NameDe != "" {
		result["name_de"] = f.NameDe
	}
	if f.NameEn != "" {
		result["name_en"] = f.NameEn
	}
	if f.NameIt != "" {
		result["name_it"] = f.NameIt
	}
	if f.StandardName != "" {
		result["standard_name"] = f.StandardName
	}
	if f.Municipality != "" {
		result["municipality"] = f.Municipality
	}

	// Build nested netex_parking map.
	netex := make(map[string]any)
	if f.NetexType != "" {
		netex["type"] = f.NetexType
	}
	if f.NetexLayout != "" {
		netex["layout"] = f.NetexLayout
	}
	if f.NetexCharging != "" {
		if b, err := strconv.ParseBool(f.NetexCharging); err == nil {
			netex["charging"] = b
		}
	}
	if f.NetexReservation != "" {
		netex["reservation"] = f.NetexReservation
	}
	if f.NetexSurveillance != "" {
		if b, err := strconv.ParseBool(f.NetexSurveillance); err == nil {
			netex["surveillance"] = b
		}
	}
	if f.NetexVehicleTypes != "" {
		netex["vehicletypes"] = f.NetexVehicleTypes
	}
	if f.NetexHazardProhibited != "" {
		if b, err := strconv.ParseBool(f.NetexHazardProhibited); err == nil {
			netex["hazard_prohibited"] = b
		}
	}
	// Only add the nested map if it has at least one field.
	if len(netex) > 0 {
		result["netex_parking"] = netex
	}

	return result
}

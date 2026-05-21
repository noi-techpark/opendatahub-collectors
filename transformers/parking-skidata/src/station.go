// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
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

// Station represents one row from the stations CSV.
type Station struct {
	ID                    string  `csv:"id"`
	StationType           string  `csv:"station_type"`
	ParentID              string  `csv:"parent_id"`
	CarparkID             int     `csv:"carpark_id"`
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

func ReadStations(filename string) Stations {
	f, err := os.Open(filename)
	ms.FailOnError(context.Background(), err, "failed opening csv file")
	defer f.Close()

	var facilities Stations
	err = gocsv.UnmarshalFile(f, &facilities)
	ms.FailOnError(context.Background(), err, "failed unmarshalling csv")

	return facilities
}

// ReadStationsOptional reads a stations CSV file like ReadStations, but
// returns an empty slice if the file does not exist (instead of failing).
// It is used to merge in optional sources like a `*.dev.csv` overlay that
// is excluded from production builds via .dockerignore.
func ReadStationsOptional(filename string) Stations {
	f, err := os.Open(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		ms.FailOnError(context.Background(), err, "failed opening optional csv file")
	}
	defer f.Close()

	var facilities Stations
	err = gocsv.UnmarshalFile(f, &facilities)
	ms.FailOnError(context.Background(), err, "failed unmarshalling optional csv")
	return facilities
}

// ToMetadata converts the Station into a map suitable for BDP station metadata.
// Only non-empty fields are included. NeTEx fields are nested under "netex_parking".
func (f *Station) ToMetadata() map[string]any {
	result := make(map[string]any)

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
	if len(netex) > 0 {
		result["netex_parking"] = netex
	}

	return result
}

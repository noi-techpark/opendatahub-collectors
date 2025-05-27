// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/jmoiron/sqlx"
)

type Vehicle struct {
	StationCode    string  `db:"stationcode"`
	Timestamp      int64   `db:"timestamp"`
	Distance       float64 `db:"distance"`
	Headway        float64 `db:"headway"`
	Length         float64 `db:"length"`
	Axles          int     `db:"axles"`
	AgainstTraffic bool    `db:"against_traffic"`
	// classe = 1 -> "AUTOVETTURA"
	// classe = 2 -> "FURGONE"
	// classe = 3 -> "AUTOCARRO"
	// classe = 4 -> "AUTOARTICOLATO"
	// classe = 5 -> "AUTOTRENO"
	// classe = 6 -> "PULLMAN"
	// classe = 7 -> "MOTO O MOTOCICLO"
	ClassNr       int     `db:"class"`
	Speed         float64 `db:"speed"`
	Direction     int     `db:"direction"`
	PlateInitials *string `db:"license_plate_initials"`
	PlateNat      *string `db:"country"`
}

// isHeavy returns true for heavy vehicles
func (v Vehicle) IsHeavy() bool {
	return v.ClassNr == 4 || v.ClassNr == 5 || (v.ClassNr == 3 && v.Length >= 890)
}

// isLight returns true for light vehicles
func (v Vehicle) IsLight() bool {
	return v.ClassNr == 1 || v.ClassNr == 2 || v.ClassNr == 7
}

// isBus returns true for buses
func (v Vehicle) IsBus() bool {
	return v.ClassNr == 6 || (v.ClassNr == 3 && v.Length < 890)
}

func ReadVehiclesWindow(ctx context.Context, db *sqlx.DB, fromTs, toTs int64, stationCode string) ([]Vehicle, error) {
	// without station mapping
	// query := `
	// 	SELECT * FROM a22.a22_traffic
	//   		WHERE "timestamp" >= $1 AND "timestamp" < $2
	//     	and stationcode = $3;
	// `

	// with station mapping
	query := `
		SELECT
			$3::text AS stationcode,
			"timestamp",
			distance,
			headway,
			length,
			axles,
			against_traffic,
			"class",
			speed,
			direction,
			license_plate_initials,
			country
		FROM a22.a22_traffic
		WHERE "timestamp" >= $1
		AND "timestamp" < $2
		AND stationcode IN (
			SELECT m.old::text
			FROM a22.a22_station_mapping m
			WHERE m.new::text = $3::text
			UNION
			SELECT $3::text
		);
	`

	var vehicles []Vehicle
	err := db.SelectContext(ctx, &vehicles, query, fromTs/1000, toTs/1000, stationCode)
	if err != nil {
		return nil, err
	}

	return vehicles, nil
}

func splitVehiclesByWindow(vehicles []Vehicle, windowStart int64, windowLength int64) map[int][]Vehicle {
	windowMap := make(map[int][]Vehicle)

	for _, v := range vehicles {
		ts := v.Timestamp * 1000 // assuming v.Timestamp is in seconds
		offset := int((ts - windowStart) / windowLength)
		windowMap[offset] = append(windowMap[offset], v)
	}

	return windowMap
}

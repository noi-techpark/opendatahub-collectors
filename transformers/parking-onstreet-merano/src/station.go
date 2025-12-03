// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"os"

	"github.com/gocarina/gocsv"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
)

// Station represents one record from the CSV.
type Station struct {
	GUID         string `csv:"guid"`
	Name         string `csv:"name"`
	Group        string `csv:"group"`
	Municipality string `csv:"municipality"`
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

func (s Stations) GetStationByGUID(guid string) *Station {
	for _, f := range s {
		if f.GUID == guid {
			return &f
		}
	}
	return nil
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env struct {
	tr.Env
	PERIOD      uint64
	REPORT_ID   string
	REPORT_NAME string
}

const STATIONTYPE = "WebStatistics"

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")
	slog.Info("Matomo data collector starting up...")
	b := bdplib.FromEnv()

	defer tel.FlushOnPanic()

	// DANGER! this ordering matters, because we use it to map indices to top level array
	// Top level array are the aggregated requests of BulkRequest function.
	// for NOI transparency, we are requesting per period = (year, month, week, day), in that order
	dts := []bdplib.DataType{
		bdplib.CreateDataType("yearlyVisits", "amount", "Yearly visits on a website", ""),
		bdplib.CreateDataType("monthlyVisits", "amount", "Monthly visits on a website", ""),
		bdplib.CreateDataType("weeklyVisits", "amount", "Weekly visits on a website", ""),
		bdplib.CreateDataType("dailyVisits", "amount", "Daily visits on a website", ""),
	}

	ms.FailOnError(context.Background(), b.SyncDataTypes(STATIONTYPE, dts), "could not sync data types")

	listener := tr.NewTr[string](context.Background(), env.Env)

	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		slog.Info("New message received")
		dto, err := unmarshalRawJson(r.Rawdata)
		if err != nil {
			return fmt.Errorf("could not unmarshal the raw payload json: %w", err)
		}

		stations := []bdplib.Station{}
		dm := b.CreateDataMap()

		for p, period := range dto {
			// if there were no visits in a period, the array is empty
			if len(period) == 0 {
				continue
			}
			segment := period[0] // we should only have one in our use case
			dt := dts[p]         // datatypes are in same order as bulk requests

			// Create top level records stations
			report := bdplib.CreateStation(env.REPORT_ID, env.REPORT_NAME, STATIONTYPE, 0, 0, b.GetOrigin())
			report.MetaData = map[string]any{
				"url": env.REPORT_ID,
			}
			stations = append(stations, report)

			dm.AddRecord(report.Id, dt.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), segment.NbVisits, env.PERIOD))

			for _, rec := range segment.Subtable {
				var site bdplib.Station
				// "Others" is a bit ugly, so we pretty it up a bit
				if rec.Label == "Others" {
					site = bdplib.CreateStation(report.Id+":others", report.Name+" - Others", STATIONTYPE, 0, 0, b.GetOrigin())
				} else {
					site = bdplib.CreateStation(shortenUnique(rec.Label), shortenUnique(rec.Label), STATIONTYPE, 0, 0, b.GetOrigin())
				}
				site.MetaData = map[string]any{
					"report": report.Id,
					"url":    rec.Label,
				}
				stations = append(stations, site)

				dm.AddRecord(site.Id, dt.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), rec.NbVisits, env.PERIOD))
			}
		}

		ms.FailOnError(ctx, b.SyncStations(STATIONTYPE, stations, true, false), "error syncing stations")
		ms.FailOnError(ctx, b.PushData(STATIONTYPE, dm), "error pushing measurements")

		return nil
	})
	ms.FailOnError(context.Background(), err, "transformer handler failed")
}

// since DB fields on station are limited to 255 chars, we cut the string,
// and append a hash of the complete one to the end to guarantee uniqueness
func shortenUnique(s string) string {
	if len(s) > 255 {
		hash := md5.Sum([]byte(s))
		return s[:245] + hex.EncodeToString(hash[:])[:10]
	}
	return s
}

func unmarshalRawJson(s string) (MatomoCustomReport, error) {
	dto := MatomoCustomReport{}
	err := json.Unmarshal([]byte(s), &dto)
	return dto, err
}

// https://developer.matomo.org/api-reference/reporting-api
// Next level are segments. Not sure why we sometimes get two segments, even though we didn't request it (usually only for year period).
type MatomoCustomReport [][]struct {
	Label          string `json:"label"`
	NbVisits       int    `json:"nb_visits"`
	Level          int    `json:"level"`
	Idsubdatatable int    `json:"idsubdatatable"`
	Segment        string `json:"segment"`
	Subtable       []struct {
		Label    string `json:"label"`
		NbVisits int    `json:"nb_visits"`
		Level    int    `json:"level"`
	} `json:"subtable"`
}

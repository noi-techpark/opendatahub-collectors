// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
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

// Create your own structure for unmarshalling the Raw Data
type RawDataPlaceholder struct {
	Field string
}

const STATIONTYPE = "ExampleStation"
const PERIOD = 600

var datatype = bdplib.CreateDataType("temperature", "°C", "Current temperature", "instant")

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv()

	b.SyncDataTypes(STATIONTYPE, []bdplib.DataType{datatype})

	listener := tr.NewTr[RawDataPlaceholder](context.Background(), env)
	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[RawDataPlaceholder]) error {
		err := b.SyncStations(STATIONTYPE, []bdplib.Station{}, true, false)
		if err != nil {
			return err
		}
		recs := b.CreateDataMap()
		recs.AddRecord("stationcode", datatype.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), -999, PERIOD))
		err = b.PushData(STATIONTYPE, recs)
		if err != nil {
			return err
		}
		return nil
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

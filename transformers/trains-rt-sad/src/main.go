// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

var env struct {
	tr.Env
	bdplib.BdpEnv
}

const STATIONTYPE = "ExampleStation"
const PERIOD = 600

type Dto struct {
	Status string `json:"status"`
	Data   []struct {
		RollingStock struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"rolling_stock"`
		Position struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
			Time      string  `json:"time"`
		} `json:"position"`
		Status struct {
			Code int    `json:"code"`
			Time string `json:"time"`
		} `json:"status"`
		Trip struct {
			Line  string `json:"line"`
			Trip  string `json:"trip"`
			Train any    `json:"train"`
			Delay int    `json:"delay"`
			Time  string `json:"time"`
		} `json:"trip"`
		Composition struct {
			Chain struct {
				PositionInChain int      `json:"positionInChain"`
				Chain           []string `json:"chain"`
			} `json:"chain"`
			Time string `json:"time"`
		} `json:"composition"`
	} `json:"data"`
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	listener := tr.NewTr[string](context.Background(), env.Env)
	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		raw := Dto{}
		if err := json.Unmarshal([]byte(r.Rawdata), &raw); err != nil {
			return err
		}

		// get relevant netex
		// compose siri-vm
		// upload
		return nil
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

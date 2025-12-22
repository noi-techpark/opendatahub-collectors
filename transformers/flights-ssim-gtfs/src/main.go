// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
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

var env struct {
	tr.Env
	bdplib.BdpEnv
}

// Create your own datatype for unmarshalling the Raw Data
type RawType struct {
	File     []byte
	Filename string
	Dir      string
	Mtime    string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	listener := tr.NewTr[RawType](context.Background(), env.Env)
	err := listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[RawType]) error {

		return nil
	})

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

// PushPayload carries the raw JSON body together with the facilityId from the URL.
type PushPayload struct {
	Body json.RawMessage
}

var env struct {
	dc.Env

	INBOUND_AUTH_USER string `required:"true"`
	INBOUND_AUTH_PASS string `required:"true"`

	SKIDATA_CREDENTIALS_JSON string `required:"true"`
}

var collector *dc.Dc[PushPayload]

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting rest-push-skidata collector...")

	defer tel.FlushOnPanic()

	creds, err := ParseCredentials(env.SKIDATA_CREDENTIALS_JSON)
	ms.FailOnError(context.Background(), err, "failed to parse credentials")
	slog.Info("Loaded facility credentials", "count", len(creds))

	collector = dc.NewDc[PushPayload](context.Background(), env.Env)

	go func() {
		defer tel.FlushOnPanic()
		collector.Start(context.Background(), func(ctx context.Context, p PushPayload) (*rdb.RawAny, error) {
			return &rdb.RawAny{
				Provider:  env.PROVIDER,
				Timestamp: time.Now(),
				Rawdata:   p.Body,
			}, nil
		})
	}()

	SubscribeAll(creds)

	serve(collector.GetInputChannel())
}

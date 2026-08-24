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

	SKIDATA_BASE_URL         string `required:"true"`
	SKIDATA_CREDENTIALS_JSON string `required:"true"`

	// Counting categories are collected as a second flow under their own
	// provider, so they land in their own collection and can be read back
	// keyed by facility.
	SKIDATA_CATEGORIES_PROVIDER string        `default:"skidata/counting-categories"`
	SKIDATA_CATEGORY_REFRESH    time.Duration `default:"1h"`
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
			// Stated rather than left to the SDK default. The content type
			// decides how the writer stores the payload — a text type is kept
			// verbatim as a string, anything else becomes BSON binary — and the
			// transformer decodes it as a string. Leaving that to a default
			// somewhere else couples the two ends through a value neither of
			// them declares.
			return &rdb.RawAny{
				Provider:    env.PROVIDER,
				Timestamp:   time.Now(),
				Rawdata:     p.Body,
				ContentType: "application/json",
			}, nil
		})
	}()

	SubscribeAll(context.Background(), creds)

	serve(collector.GetInputChannel())
}

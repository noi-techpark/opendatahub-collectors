// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string

	SOURCE    string
	TRIP_TAGS string
}

type transformedMessage struct {
	Url       string     `json:"url"`
	Timestamp *time.Time `json:"timestamp"`
}

var timeNow = time.Now
var tags clib.TagDefs
var contentClient clib.ContentAPI

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting GTFS to Trip transformer...")

	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error

	contentClient, err = clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create client")

	tags, err = clib.ReadTagDefs("../resources/tags.json")
	ms.FailOnError(context.Background(), err, "failed to read tags")

	err = clib.SyncTags(context.Background(), contentClient, tags, clib.SyncTagsConfig{Source: "trip"})
	ms.FailOnError(context.Background(), err, "failed to sync tags")

	cfg := MapperConfig{
		Source: env.SOURCE,
		TagIDs: splitTags(env.TRIP_TAGS),
	}

	listener := tr.NewCTr[transformedMessage](context.Background(), env.Env)

	err = listener.Start(context.Background(), TransformWithClient(contentClient, cfg))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// splitTags splits a comma-separated tag string into a slice, trimming whitespace.
func splitTags(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var tags []string
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			tags = append(tags, t)
		}
	}
	return tags
}

// TransformWithClient returns a handler that uses the given ContentAPI client and config.
func TransformWithClient(client clib.ContentAPI, cfg MapperConfig) tr.CHandler[transformedMessage] {
	return func(ctx context.Context, payload *transformedMessage) error {
		return Transform(ctx, client, cfg, payload)
	}
}

// Transform downloads GTFS data, maps it to Trip objects, and upserts them.
func Transform(ctx context.Context, client clib.ContentAPI, cfg MapperConfig, r *transformedMessage) error {
	logger.Get(ctx).Info("Received GTFS notification", "url", r.Url)

	url := strings.TrimSpace(r.Url)
	if url == "" {
		logger.Get(ctx).Warn("Empty URL in message, skipping")
		return nil
	}

	gtfsData, err := DownloadAndParseGtfs(url)
	if err != nil {
		return fmt.Errorf("failed to download/parse GTFS: %w", err)
	}

	logger.Get(ctx).Info("Parsed GTFS data",
		"agencies", len(gtfsData.Agencies),
		"stops", len(gtfsData.Stops),
		"routes", len(gtfsData.Routes),
		"trips", len(gtfsData.Trips),
	)

	syncTime := timeNow().UTC().Truncate(time.Millisecond)

	trips, err := MapGtfsToTrips(gtfsData, cfg, tags, syncTime)
	if err != nil {
		return fmt.Errorf("failed to map GTFS to trips: %w", err)
	}

	logger.Get(ctx).Info("Mapped trips", "count", len(trips))

	if len(trips) == 0 {
		return nil
	}

	err = client.PutMultiple(ctx, "Trip", trips)
	if err != nil {
		return fmt.Errorf("failed to upsert trips: %w", err)
	}

	logger.Get(ctx).Info("Successfully upserted trips", "count", len(trips))
	return nil
}

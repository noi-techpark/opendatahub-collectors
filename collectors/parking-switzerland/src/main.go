// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"github.com/robfig/cron/v3"
)

var env struct {
	dc.Env
	CRON string

	BIKE_PARKING_URL string
	CAR_PARKING_URL  string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Swiss parking data collector...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		jobstart := time.Now()

		ctx, col := collector.StartCollection(context.Background())
		defer col.End(ctx)

		logger.Get(ctx).Debug("collecting Swiss parking data")

		bikeData, err := fetchEndpoint(ctx, env.BIKE_PARKING_URL)
		if err != nil {
			ms.FailOnError(ctx, err, "failed to fetch bike parking data")
			return
		}

		carData, err := fetchEndpoint(ctx, env.CAR_PARKING_URL)
		if err != nil {
			ms.FailOnError(ctx, err, "failed to fetch car parking data")
			return
		}

		combined := map[string]interface{}{
			"bikeParking": bikeData,
			"carParking":  carData,
		}

		jsonBytes, err := json.Marshal(combined)
		ms.FailOnError(ctx, err, "failed to marshal combined data")

		err = col.Publish(ctx, &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   string(jsonBytes),
		})
		ms.FailOnError(ctx, err, "failed to publish data")

		logger.Get(ctx).Info("collection completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	c.Run()
}

func fetchEndpoint(ctx context.Context, url string) (interface{}, error) {
	client := retryablehttp.NewClient()
	client.Logger = nil

	req, err := retryablehttp.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", url, err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetching %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, url)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response from %s: %w", url, err)
	}

	var data interface{}
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("parsing JSON from %s: %w", url, err)
	}

	logger.Get(ctx).Info("fetched endpoint", "url", url, "bytes", len(body))

	return data, nil
}

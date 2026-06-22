// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
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
	CRON    string
	XML_URL string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector xml...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	client := retryablehttp.NewClient()
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		shouldRetry, checkErr := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		if resp != nil && (resp.StatusCode >= 400 && resp.StatusCode < 500) {
			if resp.StatusCode != 429 {
				return false, fmt.Errorf("unrecoverable client error: %d", resp.StatusCode)
			}
		}
		return shouldRetry, checkErr
	}
	httpClient := client.StandardClient()

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		jobstart := time.Now()

		ctx, collection := collector.StartCollection(context.Background())
		defer collection.End(ctx)

		logger.Get(ctx).Debug("collecting XML data")

		req, err := http.NewRequestWithContext(ctx, "GET", env.XML_URL, nil)
		ms.FailOnError(ctx, err, "failed to create request", "err", err)

		resp, err := httpClient.Do(req)
		ms.FailOnError(ctx, err, "failed to execute request", "err", err)
		defer resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			ms.FailOnError(ctx, fmt.Errorf("bad status code: %d", resp.StatusCode), "request failed")
		}

		bodyBytes, err := io.ReadAll(resp.Body)
		ms.FailOnError(ctx, err, "failed to read body", "err", err)

		err = collection.Publish(ctx, &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   string(bodyBytes),
		})
		ms.FailOnError(ctx, err, "failed to publish", "err", err)

		logger.Get(ctx).Info("collection completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	c.Run()
}

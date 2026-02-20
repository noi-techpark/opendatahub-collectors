// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
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
	CRON_STATIC  string `default:"0 0 * * * *"`
	CRON_REALTIME string `default:"0 */10 * * * *"`

	URL_STATIC  string `default:"https://data.geo.admin.ch/ch.bfe.ladestellen-elektromobilitaet/data/oicp/ch.bfe.ladestellen-elektromobilitaet.json"`
	URL_REALTIME string `default:"https://data.geo.admin.ch/ch.bfe.ladestellen-elektromobilitaet/status/oicp/ch.bfe.ladestellen-elektromobilitaet.json"`
}

// Envelope wraps raw API data with a type indicator for the transformer
type Envelope struct {
	Type string `json:"type"` // "static" or "realtime"
	Data string `json:"data"` // raw JSON string from the API
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting e-mobility Switzerland data collector...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	client := retryablehttp.NewClient()
	client.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		shouldRetry, checkErr := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		if resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return false, fmt.Errorf("unrecoverable client error: %d", resp.StatusCode)
		}
		return shouldRetry, checkErr
	}

	slog.Info("Setup complete. Starting cron scheduler")

	c := cron.New(cron.WithSeconds())

	c.AddFunc(env.CRON_STATIC, func() {
		fetchAndPublish(collector, client.StandardClient(), env.URL_STATIC, "static")
	})

	c.AddFunc(env.CRON_REALTIME, func() {
		fetchAndPublish(collector, client.StandardClient(), env.URL_REALTIME, "realtime")
	})

	c.Run()
}

func fetchAndPublish(collector *dc.Dc[dc.EmptyData], httpClient *http.Client, url string, dataType string) {
	jobStart := time.Now()

	ctx, col := collector.StartCollection(context.Background())
	defer col.End(ctx)

	logger.Get(ctx).Info("fetching data", "type", dataType, "url", url)

	resp, err := httpClient.Get(url)
	if err != nil {
		ms.FailOnError(ctx, err, "failed to fetch data", "type", dataType, "url", url)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ms.FailOnError(ctx, fmt.Errorf("unexpected status code: %d", resp.StatusCode),
			"failed to fetch data", "type", dataType, "url", url)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ms.FailOnError(ctx, err, "failed to read response body", "type", dataType)
		return
	}

	envelope := Envelope{
		Type: dataType,
		Data: string(body),
	}

	envelopeJSON, err := json.Marshal(envelope)
	if err != nil {
		ms.FailOnError(ctx, err, "failed to marshal envelope", "type", dataType)
		return
	}

	err = col.Publish(ctx, &rdb.RawAny{
		Provider:  env.PROVIDER,
		Timestamp: time.Now(),
		Rawdata:   string(envelopeJSON),
	})
	ms.FailOnError(ctx, err, "failed to publish", "type", dataType)

	logger.Get(ctx).Info("collection completed", "type", dataType, "runtime_ms", time.Since(jobStart).Milliseconds())
}

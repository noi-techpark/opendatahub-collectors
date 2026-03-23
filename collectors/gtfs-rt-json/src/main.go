// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/go-silky"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

var env struct {
	CRON         string `required:"true"`
	CONFIG_PATH  string `required:"true"`
	CALLBACK_URL string `required:"true"`
}

var (
	craw   *silky.ApiCrawler
	client *http.Client
)

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting gtfs-rt-json collector...")

	defer tel.FlushOnPanic()

	var err error
	var validationErrors []silky.ValidationError
	craw, validationErrors, err = silky.NewApiCrawler(env.CONFIG_PATH)
	ms.FailOnError(context.Background(), err, "failed to load silky config", "validation", validationErrors)

	rc := retryablehttp.NewClient()
	rc.CheckRetry = func(ctx context.Context, resp *http.Response, err error) (bool, error) {
		shouldRetry, checkErr := retryablehttp.DefaultRetryPolicy(ctx, resp, err)
		if resp != nil && resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != 429 {
			return false, fmt.Errorf("unrecoverable client error: %d", resp.StatusCode)
		}
		return shouldRetry, checkErr
	}
	client = rc.StandardClient()
	craw.SetClient(client)

	c := cron.New(cron.WithSeconds())
	_, err = c.AddFunc(env.CRON, poll)
	ms.FailOnError(context.Background(), err, "invalid cron expression")

	slog.Info("Setup complete. Starting cron scheduler", "cron", env.CRON)
	c.Run()
}

func postCallback(ctx context.Context, data []byte) error {
	ctx, span := tel.TraceStart(
		ctx,
		fmt.Sprintf("nginx-files: %s", env.CALLBACK_URL),
		trace.WithSpanKind(trace.SpanKindClient),
	)
	defer span.End()

	span.SetAttributes(
		attribute.String("db.name", "nginx-files"),
		attribute.String("peer.host", "nginx-files"),
		attribute.String("http.method", "PUT"),
		attribute.String("http.url", env.CALLBACK_URL),
		attribute.Int("http.request_content_length", len(data)),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, env.CALLBACK_URL, bytes.NewReader(data))
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "request creation failed")
		return fmt.Errorf("failed to create PUT request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, "PUT failed")
		return fmt.Errorf("PUT failed: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))

	if resp.StatusCode >= 300 {
		err := fmt.Errorf("callback returned %d", resp.StatusCode)
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	span.SetStatus(codes.Ok, "")
	return nil
}

func poll() {
	start := time.Now()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if craw.GetDataStream() != nil {
		// Streaming: forward each item as it arrives
		go func() {
			defer tel.FlushOnPanic()
			for {
				select {
				case <-ctx.Done():
					return
				case item, ok := <-craw.GetDataStream():
					if !ok {
						return
					}
					data, err := json.Marshal(item)
					if err != nil {
						slog.Error("Failed to encode streamed item", "err", err)
						continue
					}
					if err := postCallback(ctx, data); err != nil {
						slog.Error("Callback failed for streamed item", "err", err)
					}
				}
			}
		}()
	}

	err := craw.Run(ctx, map[string]any{})
	if err != nil {
		slog.Error("Crawl failed", "err", err)
		return
	}

	// Non-streaming: send complete result
	if craw.GetDataStream() == nil {
		data, err := json.Marshal(craw.GetData())
		if err != nil {
			slog.Error("Failed to encode crawled data", "err", err)
			return
		}
		if err := postCallback(ctx, data); err != nil {
			slog.Error("Callback failed", "err", err)
			return
		}
		slog.Info("Poll cycle completed", "bytes", len(data), "runtime_ms", time.Since(start).Milliseconds())
	} else {
		slog.Info("Streaming poll cycle completed", "runtime_ms", time.Since(start).Milliseconds())
	}
}

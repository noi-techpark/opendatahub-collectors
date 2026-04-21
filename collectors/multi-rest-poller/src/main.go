// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"github.com/robfig/cron/v3"
	"go.opentelemetry.io/otel/trace"
	"go.opentelemetry.io/otel/trace/noop"
)

var env struct {
	dc.Env
	CRON string

	HTTP_CONFIG_PATH string

	PAGING_PARAM_TYPE  string // query, header, path...
	PAGING_SIZE        int
	PAGING_LIMIT_NAME  string
	PAGING_OFFSET_NAME string

	AUTH_STRATEGY string

	BASIC_AUTH_USERNAME string
	BASIC_AUTH_PASSWORD string

	AUTH_BEARER_TOKEN string

	RAW_WRITER_BASE_URL string `default:"http://raw-writer-2.core.svc.cluster.local"`
}

func sendRaw(baseURL, provider string, timestamp time.Time, data string, contentType string) error {
	parts := strings.SplitN(provider, "/", 2)
	if len(parts) != 2 {
		return fmt.Errorf("PROVIDER must be in the form 'provider1/provider2', got: %s", provider)
	}
	p1 := url.PathEscape(parts[0])
	p2 := url.PathEscape(parts[1])
	path := fmt.Sprintf("%s/%s/%s/%s", baseURL, p1, p2, url.PathEscape(timestamp.UTC().Format(time.RFC3339)))
	req, err := http.NewRequest(http.MethodPost, path, bytes.NewBufferString(data))
	if err != nil {
		return fmt.Errorf("could not create raw writer request: %w", err)
	}
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("raw writer request failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("raw writer returned non-2xx status %d", resp.StatusCode)
	}
	return nil
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	config, err := LoadConfig(env.HTTP_CONFIG_PATH)
	encoder := GetEncoder(*config)

	ms.FailOnError(context.Background(), err, "failed to load call config")

	contentType := ""
	if config.SelectorType() == "json" {
		contentType = "application/json"
	}

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		jobstart := time.Now()

		ctx, col := collector.StartCollection(context.Background())
		defer col.End(ctx)

		logger.Get(ctx).Debug("collecting")

		// Create cancelable context for the job
		streamCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		stream_channel := make(chan any, 1)

		go func(ctx context.Context) {
			defer close(stream_channel)
			for {
				select {
				case <-ctx.Done():
					return
				case stream_d, ok := <-stream_channel:
					if !ok {
						return
					}
					// streamed results
					enc_data, err := encoder(stream_d)
					ms.FailOnError(ctx, err, "failed to encode data", "err", err, "data", stream_d)

					pubCtx := ctx
					var pubSpan trace.Span = noop.Span{}
					// link span without full trace
					rootContext := trace.SpanContextFromContext(ctx)
					if rootContext.IsValid() {
						pubCtx, pubSpan = tel.TraceStart(context.Background(), fmt.Sprintf("%s.data-stream", tel.GetServiceName()),
							trace.WithLinks(trace.Link{
								SpanContext: rootContext,
							}),
							trace.WithSpanKind(trace.SpanKindInternal),
						)
					}

					err = sendRaw(env.RAW_WRITER_BASE_URL, env.PROVIDER, time.Now(), enc_data, contentType)
					ms.FailOnError(pubCtx, err, "failed to send raw data", "err", err)
					pubSpan.End()
				}
			}
		}(streamCtx)

		data, err := Poll(config, stream_channel)
		ms.FailOnError(ctx, err, "failed to poll", "err", err)

		// only publish if something returned
		if data != nil {
			enc_data, err := encoder(data)
			ms.FailOnError(ctx, err, "failed to encode data", "err", err, "data", data)

			err = sendRaw(env.RAW_WRITER_BASE_URL, env.PROVIDER, time.Now(), enc_data, contentType)
			ms.FailOnError(ctx, err, "failed to send raw data", "err", err)
		}

		logger.Get(ctx).Info("collection completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	c.Run()
}

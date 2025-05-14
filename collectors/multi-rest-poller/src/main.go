// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
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
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	config, err := LoadConfig(env.HTTP_CONFIG_PATH)
	encoder := GetEncoder(*config)

	ms.FailOnError(context.Background(), err, "failed to load call config")

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		jobstart := time.Now()

		ctx, c := collector.StartCollection(context.Background())
		defer c.End(ctx)

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

					err = c.Publish(pubCtx, &rdb.RawAny{
						Provider:  env.PROVIDER,
						Timestamp: time.Now(),
						Rawdata:   enc_data,
					})
					ms.FailOnError(pubCtx, err, "failed to publish", "err", err)
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

			err = c.Publish(ctx, &rdb.RawAny{
				Provider:  env.PROVIDER,
				Timestamp: time.Now(),
				Rawdata:   enc_data,
			})
		}

		ms.FailOnError(ctx, err, "failed to publish", "err", err)

		logger.Get(ctx).Info("collection completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	c.Run()
}

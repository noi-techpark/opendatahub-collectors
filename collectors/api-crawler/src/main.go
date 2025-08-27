// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/go-apigorowler"
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
	CRON        string
	CONFIG_PATH string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	slog.Info("Setup complete. Starting cron scheduler")

	craw, errors, err := apigorowler.NewApiCrawler(env.CONFIG_PATH)
	ms.FailOnError(context.Background(), err, "failed to load call config", "validation", errors)
	client := retryablehttp.NewClient()
	craw.SetClient(client.StandardClient())

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		jobstart := time.Now()

		ctx, c := collector.StartCollection(context.Background())
		defer c.End(ctx)

		logger.Get(ctx).Debug("collecting")

		// Create cancelable context for the job
		crawlCtx, cancel := context.WithCancel(ctx)
		defer cancel()

		if craw.GetDataStream() != nil {
			// handle streaming
			go func(ctx context.Context) {
				for {
					select {
					case <-ctx.Done():
						return
					case stream_d, ok := <-craw.GetDataStream():
						if !ok {
							return
						}
						// streamed results
						enc_data, err := json.Marshal(stream_d)
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
			}(crawlCtx)
		}

		err = craw.Run(crawlCtx)
		ms.FailOnError(ctx, err, "failed to crawl", "err", err)

		// only publish if something returned
		if craw.GetDataStream() == nil {
			enc_data, err := json.Marshal(craw.GetData())
			ms.FailOnError(ctx, err, "failed to encode data", "err", err, "data", craw.GetData())

			err = c.Publish(ctx, &rdb.RawAny{
				Provider:  env.PROVIDER,
				Timestamp: time.Now(),
				Rawdata:   string(enc_data),
			})
			ms.FailOnError(ctx, err, "failed to publish", "err", err)
		}

		logger.Get(ctx).Info("collection completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	c.Run()
}

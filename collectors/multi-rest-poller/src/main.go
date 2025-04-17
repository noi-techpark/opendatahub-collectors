// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/robfig/cron/v3"
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
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data collector...")

	defer tel.FlushOnPanic()

	config, err := LoadConfig(env.HTTP_CONFIG_PATH)
	ms.FailOnError(context.Background(), err, "failed to load call config")

	collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		collector.GetInputChannel() <- dc.NewInput[dc.EmptyData](context.Background(), nil)
	})

	slog.Info("Setup complete. Starting cron scheduler")
	go func() {
		c.Run()
	}()

	err = collector.Start(context.Background(), func(ctx context.Context, a dc.EmptyData) (*rdb.RawAny, error) {
		data, err := Poll(config)
		if err != nil {
			return nil, err
		}

		return &rdb.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   data,
		}, nil
	})
	ms.FailOnError(context.Background(), err, err.Error())
}

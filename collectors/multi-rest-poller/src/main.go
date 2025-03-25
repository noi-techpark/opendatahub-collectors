// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dc"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
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
	slog.Info("Starting data collector...")
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	mq, err := dc.PubFromEnv(env.Env)
	ms.FailOnError(err, "failed creating mq publisher")

	config, err := LoadConfig(env.HTTP_CONFIG_PATH)
	ms.FailOnError(err, "failed to load call config")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()

		data, err := Poll(config)
		ms.FailOnError(err, "failed to poll data")

		mq <- dto.RawAny{
			Provider:  env.PROVIDER,
			Timestamp: time.Now(),
			Rawdata:   data,
		}
		slog.Info("Polling job completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	slog.Info("Setup complete. Starting cron scheduler")
	c.Run()
}

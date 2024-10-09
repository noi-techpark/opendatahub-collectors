// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"time"

	"github.com/robfig/cron/v3"
	"opendatahub.com/rest-paging-poller/dc"
)

var env struct {
	dc.Env
	CRON               string
	POLL_URL           string
	PAGING_PARAM_TYPE  string
	PAGING_SIZE        int
	PAGING_LIMIT_NAME  string
	PAGING_OFFSET_NAME string
}

func main() {
	dc.LoadEnv(&env)
	dc.InitLog(env.LogLevel)
	slog.Debug("Dumping environment:", "env", env)
	mq := dc.PubFromEnv(env.Env)
	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("job called")
		mq <- dc.MqMsg{
			Provider:  env.Env.Provider,
			Timestamp: time.Now(),
			Rawdata:   "test123",
		}
	})
	c.Run()
}

// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"os"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/kelseyhightower/envconfig"
	"github.com/rabbitmq/amqp091-go"
	"github.com/robfig/cron/v3"
	sloggin "github.com/samber/slog-gin"
)

var cfg struct {
	RABBITMQ_URI      string
	RABBITMQ_EXCHANGE string

	PULL_TOKEN              string
	PULL_LOCATIONS_ENDPOINT string
	PULL_LOCATIONS_CRON     string

	OCPI_TOKENS []string

	PROVIDER string
	LOGLEVEL string `default:"INFO"`
}

const ver string = "2.2"

func initLogger() {
	level := &slog.LevelVar{}
	level.UnmarshalText([]byte(cfg.LOGLEVEL))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}

func main() {
	envconfig.MustProcess("", &cfg)
	initLogger()

	mq := connectMq()
	defer mq.Close()

	// polling jobs run via cron schedule
	go startCron(mq)

	// data pushes are handled by a REST endpoint
	go startEndpoint(mq)

	select {}
}

func connectMq() RabbitC {
	rabbit, err := RabbitConnect(cfg.RABBITMQ_URI)
	if err != nil {
		slog.Error("cannot open rabbitmq connection. aborting")
		panic(err)
	}
	rabbit.OnClose(func(err amqp091.Error) {
		slog.Error("rabbit connection closed unexpectedly", "err", err)
		panic(err)
	})
	return rabbit
}

func startCron(rabbit RabbitC) {
	c := cron.New()

	// Poll locations endpoint to get all charging stations and their plugs
	if _, err := c.AddFunc(cfg.PULL_LOCATIONS_CRON, func() {
		if err := getAllLocations(rabbit, cfg.PROVIDER+"-pull-locations"); err != nil {
			slog.Error("pull locations job failed")
		}
	}); err != nil {
		panic(err)
	}
	c.Start()
}

func startEndpoint(rabbit RabbitC) {
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	r.Use(sloggin.NewWithFilters(
		slog.Default(),
		// prevent log spam, as we don't have any implemented GET endpoints at time of writing
		sloggin.IgnoreMethod("GET")))

	r.GET("/health", health)

	rEmsp := r.Group("/ocpi/emsp")
	rEmsp.Use(tokenAuth(validTokens(cfg.OCPI_TOKENS)))
	{
		rVer := rEmsp.Group("/" + ver)
		{
			rLoc := rVer.Group("/locations")

			// Recieve status updates of plugs wia push
			rLoc.PATCH("/:country_code/:party_id/:location_id/:evse_uid", handlePush(rabbit, cfg.PROVIDER+"-push-evse"))
		}
	}

	slog.Info("START GIN")
	panic(r.Run())
}

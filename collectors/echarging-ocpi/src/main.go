// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/kelseyhightower/envconfig"
	"github.com/rabbitmq/amqp091-go"
	sloggin "github.com/samber/slog-gin"
)

var cfg struct {
	RABBITMQ_URI      string
	RABBITMQ_EXCHANGE string

	OCPI_TOKENS []string

	PROVIDER string
	LOGLEVEL string `default:"INFO"`
}

const ver string = "2.2"

type OCPIResp struct {
	Data          any          `json:"data,omitempty"`
	StatusCode    int          `json:"status_code"`
	Timestamp     OCPIDateTime `json:"timestamp"`
	StatusMessage *string      `json:"status_message,omitempty"`
}

type OCPIDateTime struct {
	time.Time
}

func (t OCPIDateTime) MarshalJSON() ([]byte, error) {
	// OCPI is particular about date formats, only a subset of RFC 3339 is supported, and must be in UTC
	f := fmt.Sprintf("\"%s\"", t.Time.UTC().Format("2006-01-02T15:04:05Z"))
	return []byte(f), nil
}

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
	slog.Info("dumping config", "cfg", cfg)

	rabbit, err := RabbitConnect(cfg.RABBITMQ_URI)
	if err != nil {
		slog.Error("cannot open rabbitmq connection. aborting")
		panic(err)
	}
	defer rabbit.Close()

	rabbit.OnClose(func(err amqp091.Error) {
		slog.Error("rabbit connection closed unexpectedly", "err", err)
		panic(err)
	})

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	r.Use(sloggin.NewWithFilters(
		slog.Default(),
		sloggin.IgnorePath("/health", "/favicon.ico"))) // prevent log spam

	r.GET("/health", health)

	rEmsp := r.Group("/ocpi/emsp")
	rEmsp.Use(tokenAuth(validTokens(cfg.OCPI_TOKENS)))
	{
		rVer := rEmsp.Group("/" + ver)
		{
			rLoc := rVer.Group("/locations")
			rLoc.PATCH("/:country_code/:party_id/:location_id/:evse_uid", handlePush(rabbit))
		}
	}

	slog.Info("START GIN")
	r.Run()
}

func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func handlePush(rabbit RabbitC) gin.HandlerFunc {
	return func(c *gin.Context) {
		var body map[string]any
		if err := c.BindJSON(&body); err != nil {
			body, _ := io.ReadAll(c.Request.Body)
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("cannot unmarshal json: %s", body))
			return
		}

		params := map[string]string{}
		for _, p := range c.Params {
			params[p.Key] = p.Value
		}

		slog.Debug("Received message", "params", params, "body", body, "path", c.FullPath())

		err := rabbit.Publish(mqMsg{
			Provider:  cfg.PROVIDER,
			Timestamp: time.Now(),
			Rawdata: map[string]any{
				"params": params,
				"body":   body,
			},
		}, cfg.RABBITMQ_EXCHANGE)

		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("cannot publish to rabbitmq: %w", err))
			return
		}

		resp := OCPIResp{
			StatusCode: 1000,
			Timestamp:  OCPIDateTime{time.Now()},
		}

		c.JSONP(http.StatusOK, resp)
	}
}

func validTokens(tokens []string) map[string]struct{} {
	ret := map[string]struct{}{}
	for _, t := range tokens {
		ret[t] = struct{}{}
	}
	return ret
}

func tokenAuth(ts map[string]struct{}) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.Request.Header.Get("Authorization")

		var token string
		if _, err := fmt.Sscanf(header, "Token %s", &token); err != nil {
			c.AbortWithError(http.StatusBadRequest, fmt.Errorf("invalid authorization header format: %w", err))
			return
		}
		if _, found := ts[token]; !found {
			c.AbortWithStatus(http.StatusUnauthorized)
			return
		}

		c.Next()
	}
}

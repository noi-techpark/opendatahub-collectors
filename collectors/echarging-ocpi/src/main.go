// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

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

type EVSE struct {
	Status       string
	Last_updated time.Time
}

func initLogger() {
	logLevel := os.Getenv("LOG_LEVEL")
	level := &slog.LevelVar{}
	level.UnmarshalText([]byte(logLevel))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}

func main() {
	initLogger()

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(cors.Default())

	if os.Getenv("GIN_LOG") == "PRETTY" {
		r.Use(gin.Logger())
	} else {
		r.Use(sloggin.NewWithFilters(
			slog.Default(),
			sloggin.IgnorePath("/health", "/favicon.ico"))) // prevent log spam
	}

	r.GET("/health", health)

	rEmsp := r.Group("/ocpi/emsp")
	rEmsp.Use(tokenAuth(validTokens()))
	{
		rVer := rEmsp.Group("/" + ver)
		{
			rLoc := rVer.Group("/locations")
			rLoc.PATCH("/:country_code/:party_id/:location_id/:evse_uid", patchEvse)
		}
	}

	slog.Info("START GIN")
	r.Run()
}

func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func patchEvse(c *gin.Context) {
	evse := EVSE{}
	if err := c.BindJSON(&evse); err != nil {
		body, _ := io.ReadAll(c.Request.Body)
		c.AbortWithError(http.StatusBadRequest, fmt.Errorf("cannot unmarshal evse object: %s", body))
		return
	}

	slog.Info("Recieved EVSE PATCH:", "msg", evse)

	resp := OCPIResp{
		StatusCode: 1000,
		Timestamp:  OCPIDateTime{time.Now()},
	}

	c.JSONP(http.StatusOK, resp)
}

func validTokens() map[string]struct{} {
	tokens := os.Getenv("TOKENS")
	ret := map[string]struct{}{}
	for _, t := range strings.Split(tokens, ",") {
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

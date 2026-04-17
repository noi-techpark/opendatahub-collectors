// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
)

func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func baseURL(c *gin.Context) string {
	scheme := "https"
	if c.Request.TLS == nil {
		scheme = "http"
	}
	return scheme + "://" + c.Request.Host
}

func versions(ver string) gin.HandlerFunc {
	type versionEntry struct {
		Version string `json:"version"`
		URL     string `json:"url"`
	}
	return func(c *gin.Context) {
		c.JSONP(http.StatusOK, OCPIResp[[]versionEntry]{
			StatusCode: 1000,
			Timestamp:  OCPIDateTime{time.Now()},
			Data: []versionEntry{
				{Version: ver, URL: baseURL(c) + "/ocpi/emsp/" + ver},
			},
		})
	}
}

func versionDetails(ver string) gin.HandlerFunc {
	type endpoint struct {
		Identifier string `json:"identifier"`
		Role       string `json:"role"`
		URL        string `json:"url"`
	}
	type details struct {
		Version   string     `json:"version"`
		Endpoints []endpoint `json:"endpoints"`
	}
	return func(c *gin.Context) {
		base := baseURL(c) + "/ocpi/emsp/" + ver
		c.JSONP(http.StatusOK, OCPIResp[details]{
			StatusCode: 1000,
			Timestamp:  OCPIDateTime{time.Now()},
			Data: details{
				Version: ver,
				Endpoints: []endpoint{
					{Identifier: "credentials", Role: "SENDER", URL: base + "/credentials"},
					{Identifier: "locations", Role: "RECEIVER", URL: base + "/locations"},
				},
			},
		})
	}
}

func notImplemented(c *gin.Context) {
	msg := "not implemented"
	slog.Warn("not implemented endpoint called", "method", c.Request.Method, "path", c.FullPath())
	c.JSONP(http.StatusOK, OCPIResp[any]{
		StatusCode:    3000,
		Timestamp:     OCPIDateTime{time.Now()},
		StatusMessage: &msg,
	})
}

func handlePush(rabbit mq.R, provider string) gin.HandlerFunc {
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

		err := rabbit.Publish(dto.RawAny{
			Provider:  provider,
			Timestamp: time.Now(),
			Rawdata: map[string]any{
				"params": params,
				"body":   body,
				// TODO: once more than one endpoint are implemented, send some details about which endpoint this belongs to. or put it into the routing key
			},
		}, cfg.MQ_EXCHANGE)

		if err != nil {
			c.AbortWithError(http.StatusInternalServerError, fmt.Errorf("cannot publish to rabbitmq: %w", err))
			return
		}

		resp := OCPIResp[any]{
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

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"opendatahub.com/rest-poller/dc"
)

var env struct {
	dc.Env
	CRON string

	RAW_BINARY bool

	HTTP_URL    string
	HTTP_METHOD string `default:"GET"`

	PAGING_PARAM_TYPE  string // query, header, path...
	PAGING_SIZE        int
	PAGING_LIMIT_NAME  string
	PAGING_OFFSET_NAME string
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

func main() {
	slog.Info("Starting data collector...")
	dc.LoadEnv(&env)
	dc.InitLog(env.LogLevel)

	headers := customHeaders()
	u, err := url.Parse(env.HTTP_URL)
	dc.FailOnError(err, "failed parsing poll URL")

	mq := dc.PubFromEnv(env.Env)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()

		req, err := http.NewRequest(env.HTTP_METHOD, u.String(), http.NoBody)
		dc.FailOnError(err, "could not create http request")

		req.Header = headers

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			slog.Error("error during http request:", "err", err)
			return
		}
		if resp.StatusCode != http.StatusOK {
			slog.Error("http request returned non-OK status", "statusCode", resp.StatusCode)
			return
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			slog.Error("error reading response body:", "err", err)
			return
		}

		var raw any
		if env.RAW_BINARY {
			raw = body
		} else {
			raw = string(body)
		}

		mq <- dc.MqMsg{
			Provider:  env.Env.Provider,
			Timestamp: time.Now(),
			Rawdata:   raw,
		}
		slog.Info("Polling job completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
	slog.Info("Setup complete. Starting cron scheduler")
	c.Run()
}

func customHeaders() http.Header {
	headers := http.Header{}

	// custom headers can be specified in format: HTTP_HEADER_XYZ='Accept: application/json'
	// so we look at env variables with that prefix and parse out the header name and value
	for _, env := range os.Environ() {
		for i := 1; i < len(env); i++ {
			if env[i] == '=' {
				envk := env[:i]
				if strings.HasPrefix(envk, ENV_HEADER_PREFIX) {
					envv := env[i+1:]
					headerName, headerValue, found := strings.Cut(envv, ":")
					if !found {
						slog.Error("invalid header config", "env", envk, "val", envv)
						panic("invalid header config")
					}
					headers[headerName] = []string{strings.TrimSpace(headerValue)}
				}
				break
			}
		}
	}
	return headers
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dc"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/robfig/cron/v3"
)

var env struct {
	dc.Env
	CRON        string
	RAW_BINARY  bool
	HTTP_URL    string
	HTTP_METHOD string `default:"GET"`
}

type ParkingMetadata struct {
	ID        string `json:"id"`
	NameDE    string `json:"name_DE"`
	NameIT    string `json:"name_IT"`
	Latitude  string `json:"latitude"`
	Longitude string `json:"longitude"`
	Capacity  int    `json:"capacity"`
}

type ParkingData struct {
	ID        string `json:"id"`
	Timestamp string `json:"timestamp"`
	Occupancy int    `json:"occupancy"`
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

func httpRequest(url *url.URL, httpHeaders http.Header, httpMethod string) (string, error) {
	headers := httpHeaders
	u := url
	client := retryablehttp.NewClient()
	req, err := retryablehttp.NewRequest(httpMethod, u.String(), http.NoBody)
	if err != nil {
		slog.Error("error creating http request:", "err", err)
		return "", err
	}
	req.Header = headers

	resp, err := client.Do(req)
	if err != nil {
		slog.Error("error during http request:", "err", err)
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("error reading response body:", "err", err)
		return "", err
	}
	return string(body), nil
}

func main() {
	slog.Info("Starting data collector...")
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	headers := customHeaders()
	u, err := url.Parse(env.HTTP_URL)
	ms.FailOnError(err, "failed parsing poll URL")

	httpMethod := env.HTTP_METHOD

	mq, err := dc.PubFromEnv(env.Env)
	ms.FailOnError(err, "failed creating mq publisher")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()

		body, err := httpRequest(u, headers, httpMethod)
		if err != nil {
			slog.Error("error during http request:", "err", err)
		}

		var parkingMetaDataSlice []ParkingMetadata

		if err := json.Unmarshal([]byte(body), &parkingMetaDataSlice); err != nil {
			log.Fatalf("failed: %v", err)
		}

		var parkingDataSingle ParkingData
		for _, parking := range parkingMetaDataSlice {
			urlData := fmt.Sprintf("https://parking.valgardena.it/get_station_data?id=%s", url.QueryEscape(parking.ID))
			urlDataParsed, err := url.Parse(urlData)
			if err != nil {
				slog.Error("error parsing url:", "err", err)
			}
			body, err = httpRequest(urlDataParsed, headers, httpMethod)
			if err != nil {
				slog.Error("error during http request:", "err", err)
			}
			if err := json.Unmarshal([]byte(body), &parkingDataSingle); err != nil {
				slog.Error("failed to unmarshal parking data", "err", err)
			}

			mq <- dto.RawAny{
				Provider:  env.PROVIDER,
				Timestamp: time.Now(),
				Rawdata:   body,
			}
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

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"io"
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
	CRON string
	RAW_BINARY bool
	HTTP_URL    string
	HTTP_METHOD string `default:"GET"`
	LORAWAN_PASSWORD string
	PAGING_PARAM_TYPE  string // query, header, path...
	PAGING_SIZE        int
	PAGING_LIMIT_NAME  string
	PAGING_OFFSET_NAME string
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

type Config struct {
	Sensors []string `json:"sensors"`	
}

func httpRequest(url *url.URL, httpHeaders http.Header, httpMethod string) (string,error) {
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

func buildLorawanUrls(devicenames []string, password string, url string) (urls []string) {
	var urlsLorawanDevices []string
	for _, device := range devicenames {
		deviceurl := fmt.Sprintf(url, password, device)
		urlsLorawanDevices = append(urlsLorawanDevices, deviceurl)
	}
	return urlsLorawanDevices
}

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)
	httpMethod := env.HTTP_METHOD
	headers := customHeaders()
	sensorNames,err := os.ReadFile("../sensors.json")
	if err != nil {
		slog.Error("error reading sensors file", "err", err)
	}
	var sensors Config
	err = json.Unmarshal(sensorNames, &sensors)
	if err != nil {
		slog.Error("error unmarshalling sensors file", "err", err)
	}
	deviceNames := sensors.Sensors
	urls := buildLorawanUrls(deviceNames, env.LORAWAN_PASSWORD, env.HTTP_URL)
	var urlsSlice []*url.URL
	for _, singleUrl := range urls {
		u, err := url.Parse(singleUrl)
		if err != nil {	
			slog.Error("error parsing url", "url", singleUrl, "err", err)
			continue
		}
		urlsSlice = append(urlsSlice, u)
	}

	mq, err := dc.PubFromEnv(env.Env)
	ms.FailOnError(err, "failed creating mq publisher")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()
		for _, singleHttp := range urlsSlice {
			body,err := httpRequest(singleHttp, headers, httpMethod)
			if err != nil {
				slog.Error("error during http request")
				continue
			}
			slog.Info("received raw data", "data", body)
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

// custom headers can be specified in format: HTTP_HEADER_XYZ='Accept: application/json'
// so we look at env variables with that prefix and parse out the header name and value
func customHeaders() http.Header {
	headers := http.Header{}
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

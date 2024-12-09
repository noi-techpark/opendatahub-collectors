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

type SensorData struct {
	ApplicationName string       `json:"application_name"`
	Timestamp       string       `json:"time"`
	DevEui          string       `json:"dev_eui"`
	DeviceName      string       `json:"device_name"`
	FPort           int          `json:"f_port"`
	Value           ValueReading `json:"value"`
}

type ValueReading struct {
	Battery     int `json:"battery"`
	Temperature int `json:"temperature"`
	Humidity    int `json:"hunidity"`
	Co2         int `json:"co2"`
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

const URL = "http://saocompute.eurac.edu/sensordb/query?db=db_opendatahub&u=opendatahub&p=H84o0VpLqqnZ0Drm&q=select%%20*%%20from%%20device_frmpayload_data_message%%20WHERE%%20%%22device_name%%22%%3D%%27%s%%27%%20limit%%2010"

var deviceNames = []string{"NOI-Brunico-Temperature", "FreeSoftwareLab-Temperature", "NOI-A1-Floor1-CO2"}

func httpRequest(url *url.URL, httpHeaders http.Header, httpMethod string) []byte {

	headers := httpHeaders
	u := url
	req, err := http.NewRequest(httpMethod, u.String(), http.NoBody)
	ms.FailOnError(err, "could not create http request")

	req.Header = headers

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		slog.Error("error during http request:", "err", err)
		return nil
	}
	if resp.StatusCode != http.StatusOK {
		slog.Error("http request returned non-OK status", "statusCode", resp.StatusCode)
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		slog.Error("error reading response body:", "err", err)
		return nil
	}
	return body

}

// function to build a slice of url starting from a slice of device names
func buildLorawanUrls(devicenames []string, password string, url string) (urls []string) {

	var urlsLorawanDevices []string

	for _, device := range devicenames {

		deviceurl := fmt.Sprintf(url, password, device)
		urlsLorawanDevices = append(urlsLorawanDevices, deviceurl)
	}

	return urlsLorawanDevices

}

func main() {
	slog.Info("Starting data collector...")
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	headers := customHeaders()
	urls := buildLorawanUrls(deviceNames, env.LORAWAN_PASSWORD, env.HTTP_URL)
	var urlsSlice []*url.URL
	for _, singleUrl := range urls {
		u, err := url.Parse(singleUrl)
		ms.FailOnError(err, "failed parsing poll URL")
		urlsSlice = append(urlsSlice, u)
	}

	httpMethod := env.HTTP_METHOD

	mq, err := dc.PubFromEnv(env.Env)
	ms.FailOnError(err, "failed creating mq publisher")

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()
		for _, singleHttp := range urlsSlice {
			body := httpRequest(singleHttp, headers, httpMethod)
			var newSensorData SensorData
			if err := json.Unmarshal(body, &newSensorData); err != nil {
				log.Fatalf("failed: %v", err)
			}
			var raw any
			if env.RAW_BINARY {
				raw = body
			} else {
				raw = string(body)
			}

			fmt.Println(raw)

			mq <- dto.RawAny{
				Provider:  env.PROVIDER,
				Timestamp: time.Now(),
				Rawdata:   raw,
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

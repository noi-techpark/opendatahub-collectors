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

// const auth string = "Basic bm9pOiVwIXh+RlNleV1Bc2p1"

// func callHttp(url string, auth string) []byte {
// 	req, err := http.NewRequest("GET", url, nil)
// 	if err != nil {
// 		log.Fatalf("An Error Occured %v", err)
// 	}

// 	req.Header.Add("Authorization", auth)

// 	resp, err := http.DefaultClient.Do(req)
// 	if err != nil {
// 		log.Fatalf("An Error Occured %v", err)
// 	}

// 	defer resp.Body.Close()

// 	body, err := io.ReadAll(resp.Body)
// 	if err != nil {
// 		log.Fatalf("An Error Occured %v", err)
// 	}

//		return body
//	}
func httpRequest(url *url.URL, httpHeaders http.Header, httpMethod string) []byte {

	headers := httpHeaders
	u := url
	req, err := http.NewRequest(httpMethod, u.String(), http.NoBody)
	dc.FailOnError(err, "could not create http request")

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

func main() {
	slog.Info("Starting data collector...")
	dc.LoadEnv(&env)
	dc.InitLog(env.LogLevel)

	headers := customHeaders()
	u, err := url.Parse(env.HTTP_URL)
	dc.FailOnError(err, "failed parsing poll URL")

	httpMethod := env.HTTP_METHOD

	mq := dc.PubFromEnv(env.Env)

	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()

		body := httpRequest(u, headers, httpMethod)

		var parkingMetaDataSlice []ParkingMetadata

		//var parkingDataSlice []ParkingData

		var parkingDataSingle ParkingData

		if err := json.Unmarshal(body, &parkingMetaDataSlice); err != nil {
			log.Fatalf("failed: %v", err)
		}

		for _, parking := range parkingMetaDataSlice {
			url2 := fmt.Sprintf("https://parking.valgardena.it/get_station_data?id=%s", parking.ID)
			u2, err := url.Parse(url2)
			dc.FailOnError(err, "failed parsing poll URL")
			body = httpRequest(u2, headers, httpMethod)
			if err := json.Unmarshal(body, &parkingDataSingle); err != nil {
				log.Fatalf("failed: %v", err)
			}
			//parkingDataSlice = append(parkingDataSlice, parkingDataSingle)

			var raw any
			if env.RAW_BINARY {
				raw = body
			} else {
				raw = string(body)
			}

			fmt.Println(body)

			mq <- dc.MqMsg{
				Provider:  env.Env.Provider,
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
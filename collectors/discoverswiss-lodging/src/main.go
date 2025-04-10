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
	"github.com/noi-techpark/go-opendatahub-discoverswiss/models"
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

	SUBSCRIPTION_KEY string

	PAGING_PARAM_TYPE  string // query, header, path...
	PAGING_SIZE        int
	PAGING_LIMIT_NAME  string
	PAGING_OFFSET_NAME string
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

func lodgingRequest(url *url.URL, httpHeaders http.Header, httpMethod string) (string, error) {
    headers := httpHeaders
    u := url
	client := retryablehttp.NewClient()
    req, err := retryablehttp.NewRequest(httpMethod, u.String(), http.NoBody)
    if err != nil {
        return "", fmt.Errorf("could not create http request: %w", err)
    }

    req.Header = headers  
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error during http request: %w", err)
    }
    defer resp.Body.Close()

    body, err := io.ReadAll(resp.Body)
    if err != nil {
        return "", fmt.Errorf("error reading DiscoverSwissResponse body: %w", err)
    }

    return string(body), nil
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



func main() {
	slog.Info("Starting data collector...")

	// err := godotenv.Load("../.env")	
	// ms.FailOnError(err, "could not load .env file")
	
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)
	mq, err := dc.PubFromEnv(env.Env)
	ms.FailOnError(err, "failed creating mq publisher")

	httpMethod := env.HTTP_METHOD
	headers := customHeaders()
	discoverswissUrl, err := url.Parse(env.HTTP_URL)
	if err != nil {
		slog.Error("failed parsing url", "url", env.HTTP_URL, "err", err)
	}	
	c := cron.New(cron.WithSeconds())
		c.AddFunc(env.CRON, func() {
			slog.Info("Starting poll job")
			jobstart := time.Now()
			continuationToken := ""

		for {
			currentURL := *discoverswissUrl

		if continuationToken != "" {
			q := currentURL.Query()
			q.Set("continuationToken", continuationToken)
			currentURL.RawQuery = q.Encode()
		}

			body, err := lodgingRequest(&currentURL, headers, httpMethod)
			if err != nil{
			slog.Error("Could not perform the query", "err", err)
			}
			var response models.DiscoverSwissResponse
			err = json.Unmarshal([]byte(body), &response)
			if err != nil {
				slog.Error("failed unmarshalling DiscoverSwissResponse object", "err", err)
				return
			}

			for _, lodging := range response.Data {
				jsonLodging, err := json.Marshal(lodging)
				if err != nil {
					slog.Error("failed marshalling lodging object", "err", err)
					continue
				}
				fmt.Println("ADDITIONAL,TYPE",lodging.AdditionalType)
				
				mq <- dto.RawAny{
					Provider:  env.PROVIDER,
					Timestamp: time.Now(),
					Rawdata:   string(jsonLodging),
				}
				
			}

			if !response.HasNextPage || response.NextPageToken == "" {
				break
			}
		
			continuationToken = response.NextPageToken
		}
		slog.Info("Polling job completed", "runtime_ms", time.Since(jobstart).Milliseconds())
	})
c.Run()
}

	











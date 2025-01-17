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

	SUBSCRIPTION_KEY string

	PAGING_PARAM_TYPE  string // query, header, path...
	PAGING_SIZE        int
	PAGING_LIMIT_NAME  string
	PAGING_OFFSET_NAME string
}

type DiscoverSwissResponse struct {
	Count         int               `json:"count"`
	HasNextPage   bool              `json:"hasNextPage"`
	NextPageToken string            `json:"nextPageToken"`
	Data          []LodgingBusiness `json:"data"`
}
type LodgingBusiness struct {
	Name string `json:"name"`

	Address struct {
		AddressCountry  string `json:"addressCountry"`
		AddressLocality string `json:"addressLocality"`
		PostalCode      string `json:"postalCode"`
		StreetAddress   string `json:"streetAddress"`
		Email           string `json:"email"`
		Telephone       string `json:"telephone"`
	} `json:"address"`

	Geo struct {
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"geo"`

	NumberOfRooms []struct {
		PropertyID string `json:"propertyId"`
		Value      string `json:"value"`
	} `json:"numberOfRooms"`

	StarRating StarRating `json:"starRating"`

	NumberOfBeds int `json:"numberOfBeds"`

	Identifier string `json:"identifier"`

	CheckinTime      string `json:"checkinTime"`
	CheckinTimeTo    string `json:"checkinTimeTo"`
	CheckoutTimeFrom string `json:"checkoutTimeFrom"`
	CheckoutTime     string `json:"checkoutTime"`

	License string `json:"license"`
}

	

type StarRating struct {
	RatingValue    float64 `json:"ratingValue"`
	AdditionalType string  `json:"additionalType"`
	Name           string  `json:"name"`
}



const ENV_HEADER_PREFIX = "HTTP_HEADER_"

func retryOnError(httpreq func() (string, error), resp http.Response) (string, error) {
	retryAfter := resp.Header.Get("Retry-After")
	if retryAfter != "" {
		retryAfterDuration, err := time.ParseDuration(retryAfter + "s")
		if err != nil {
			return "", err
		}
		fmt.Println("Retrying after", retryAfterDuration)
		time.Sleep(retryAfterDuration)
		return httpreq()
	}else{
		return "", fmt.Errorf("http request returned non-Ok status: %d", resp.StatusCode)}
}

func lodgingRequest(url *url.URL, httpHeaders http.Header, httpMethod string) (string, error) {
    headers := httpHeaders
    u := url
    req, err := http.NewRequest(httpMethod, u.String(), http.NoBody)
    if err != nil {
        return "", fmt.Errorf("could not create http request: %w", err)
    }

    req.Header = headers

    client := &http.Client{}
    resp, err := client.Do(req)
    if err != nil {
        return "", fmt.Errorf("error during http request: %w", err)
    }

    defer resp.Body.Close()

    if resp.StatusCode != http.StatusOK {
        if resp.StatusCode == http.StatusTooManyRequests {
            retryFunc := func() (string, error) {
                return lodgingRequest(url, httpHeaders, httpMethod)
            }
			// retryAfter := resp.Header.Get("Retry-After")
			// if retryAfter != "" {
			// 	retryAfterDuration, err := time.ParseDuration(retryAfter + "s")
			// 	if err != nil {
			// 		return "", err
			// 	}
			// 	fmt.Println("Retrying after", retryAfterDuration)
			// 	time.Sleep(retryAfterDuration)
            return retryOnError(retryFunc, *resp)
    //     }else{
    //     return "", fmt.Errorf("http request returned non-Ok status: %d", resp.StatusCode)}
    // }
}else{
	return "", fmt.Errorf("http request returned non-Ok status: %d", resp.StatusCode)
}
	}

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
	fmt.Println("URL: ", discoverswissUrl)
	c := cron.New(cron.WithSeconds())
	c.AddFunc(env.CRON, func() {
		slog.Info("Starting poll job")
		jobstart := time.Now()
	continuationToken := ""

	for {
		numberRequest := 0
		currentURL := *discoverswissUrl

	if continuationToken != "" {
		q := currentURL.Query()

		q.Set("continuationToken", continuationToken)
		currentURL.RawQuery = q.Encode()
	}

		body, err := lodgingRequest(&currentURL, headers, httpMethod)
		if err != nil{
		fmt.Println("Could not perform the query due to err: ", err)
		}
		numberRequest++
		fmt.Println("Lodging: ", numberRequest)

		var response DiscoverSwissResponse
		err = json.Unmarshal([]byte(body), &response)
		if err != nil {
			slog.Error("failed unmarshalling DiscoverSwissResponse object", "err", err)
			return
		}
		fmt.Println("UP TO HERE")
		for _, lodging := range response.Data {

			jsonLodging, err := json.Marshal(lodging)
			if err != nil {
				slog.Error("failed marshalling lodging object", "err", err)
				continue
			}
			
			fmt.Println(string(jsonLodging))


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

	











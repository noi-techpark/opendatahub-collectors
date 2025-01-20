// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	//"fmt"

	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	//"strconv"

	"github.com/joho/godotenv"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	//"go.starlark.net/lib/time"
)

type dsid struct{
	Id string	`json:"id"`
}

type Accommodation struct {
	Source     string `default:"discoverswiss"`
	Active     bool   `default:"true"`
	Shortname  string

	Mapping struct {
		DiscoverSwiss dsid `json:"discoverswiss"`
	} `json:"Mapping"`

	AccoDetail struct {
		Language AccoDetailLanguage `json:"de"`
	} `json:"AccoDetail"`

	GpsInfo []struct {
		Gpstype   string  `json:"Gpstype"`
		Latitude  float64 `json:"Latitude"`
		Longitude float64 `json:"Longitude"`
		Altitude  float64 `json:"Altitude"`
		AltitudeUnitofMeasure string `json:"AltitudeUnitofMeasure"`
	} `json:"GpsInfo"`

	AccoType struct {
		Id string `json:"Id"`
	} `json:"AccoType"`

	AccoOverview struct {
		TotalRooms   int    `json:"TotalRooms"`
		SingleRooms  int    `json:"SingleRooms"`
		DoubleRooms  int    `json:"DoubleRooms"`
		CheckInFrom  string `json:"CheckInFrom"`
		CheckInTo    string `json:"CheckInTo"`
		CheckOutFrom string `json:"CheckOutFrom"`
		CheckOutTo   string `json:"CheckOutTo"`
		MaxPersons   int    `json:"MaxPersons"`
	} `json:"AccoOverview"`

	LicenseInfo struct {
		Author string `json:"Author"`
		License string `json:"License"`
		ClosedData bool `json:"ClosedData"`
		LicenseHolder string `json:"LicenseHolder"`
	} `json:"LicenseInfo"`
}

type AccoDetailLanguage struct {
	Name        string `json:"Name"`
	Street      string `json:"Street"`
	Zip         string `json:"Zip"`
	City        string `json:"City"`
	CountryCode string `json:"CountryCode"`
	Email       string `json:"Email"`
	Phone       string `json:"Phone"`
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

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

var env struct {
	tr.Env

	HTTP_URL    string
	HTTP_METHOD string `default:"GET"`

	TOKEN_URL string
	TOKEN_USERNAME string
	TOKEN_PASSWORD string
	TOKEN_CLIENT_ID string
	TOKEN_CLIENT_SECRET string

	ODH_API_CORE_URL string

	SUBSCRIPTION_KEY string
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

const RAW_FILTER_URL_TEMPLATE = "https://api.tourism.testingmachine.eu/v1/Accommodation?rawfilter=eq(Mapping.discoverswiss.id,%%22%s%%22)&fields=Id"

type RawFilterId struct {
	Items []struct {
		Id string `json:"Id"`
	} `json:"Items"`
}

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
		return "", fmt.Errorf("http request returned non-Ok status: %d for url %s", resp.StatusCode)}
}

func rawFilterHttpRequest(id string) (string, error) {
	url,err := url.Parse(fmt.Sprintf(RAW_FILTER_URL_TEMPLATE, id))
	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	req,err := http.NewRequest("GET", url.String(), nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error during http request: %w", err)
	}

	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		if resp.StatusCode == http.StatusTooManyRequests {			
			retryfunc := func() (string, error) {
				return rawFilterHttpRequest(id)
			}
			return retryOnError(retryfunc, *resp)

		}else{
		return "", fmt.Errorf("http request returned non-Ok status: %d for url %s", resp.StatusCode, url.String())
		}
	}
	
	var rawFilterId RawFilterId

	err = json.NewDecoder(resp.Body).Decode(&rawFilterId)
	if err != nil {
		return "", fmt.Errorf("could not decode response: %w", err)
	}

	fmt.Println("RAWFILTERID: ",rawFilterId)

	if len(rawFilterId.Items) > 0 {
		return rawFilterId.Items[0].Id, nil
	}else{
		return "",nil
	}

}

func getAccessToken(tokenURL, username, password,clientID, clientSecret string) (*TokenResponse, error) {
	data := url.Values{}
	data.Set("grant_type", "password")
	data.Set("username", username)
	data.Set("password", password)
	data.Set("client_id", clientID)
	data.Set("client_secret", clientSecret)

	req, err := http.NewRequest("POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, fmt.Errorf("could not create http request: %w", err)
	}

	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("error during http request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http request returned non-Ok status: %d for url %s", resp.StatusCode, tokenURL)
	}

	defer resp.Body.Close()

	var tokenResponse TokenResponse

	err = json.NewDecoder(resp.Body).Decode(&tokenResponse)
	if err != nil {
		return nil, fmt.Errorf("error decoding token response: %w", err)
	}

	return &tokenResponse, nil

}

func makeAuthorizedRequest(url *url.URL, token string, payload interface{}, httpMethod string, id string) (string,error) {

    jsonData, err := json.Marshal(payload)
    if err != nil {
		return "", fmt.Errorf("could not marshal payload: %w", err)
	}	

    u := url
    if httpMethod == "POST" {
        req, err := http.NewRequest("POST", u.String(), bytes.NewBuffer(jsonData))
        if err != nil {
			return "", fmt.Errorf("could not create http request: %w", err)
		}

        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
        req.Header.Set("Content-Type", "application/json")

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil {
			return "", fmt.Errorf("error during http request: %w", err)
		}

        if resp.StatusCode != http.StatusOK {
            return "", fmt.Errorf("http request returned non-Ok status: %d for url %s", resp.StatusCode, u.String())
        }

        defer resp.Body.Close()

        return strconv.Itoa(resp.StatusCode), nil

    } else if httpMethod == "PUT" {
 
            u := fmt.Sprintf("%s/%s", url.String(), id)
			fmt.Println("RAWPATH: ",u)
			newurl, err := url.Parse(u)
			if err != nil {
				return "", fmt.Errorf("could not parse url: %w", err)
			}

        req, err := http.NewRequest("PUT", newurl.String(), bytes.NewBuffer(jsonData))
        if err != nil {
			return "", fmt.Errorf("could not create http request: %w", err)
		}

        req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
        req.Header.Set("Content-Type", "application/json")

        client := &http.Client{}
        resp, err := client.Do(req)
        if err != nil {
			return "", fmt.Errorf("error during http request: %w", err)
		}

        if resp.StatusCode != http.StatusOK {
            
			if resp.StatusCode == http.StatusTooManyRequests {
				retryFunc := func() (string, error) {
					return makeAuthorizedRequest(url, token, payload, httpMethod, id)
				}
				
				return retryOnError(retryFunc, *resp)
		}else{
		return string(resp.StatusCode),fmt.Errorf("http request returned non-Ok status: %d for url %s", resp.StatusCode, newurl.String())
		}
	}
		
        defer resp.Body.Close()

        return fmt.Sprint("RESPCODE: ",string(resp.StatusCode)), nil
    }
	

    return "",fmt.Errorf("unsupported HTTP method: %s", httpMethod)
}


func main() {
	err := godotenv.Load("../.env")
	if err != nil {
		slog.Error("Error loading .env file")
	}
	envconfig.MustProcess("", &env)
	ms.InitLog(env.Env.LOG_LEVEL)

	rabbit, err := mq.Connect(env.Env.MQ_URI, env.Env.MQ_CLIENT)
	ms.FailOnError(err, "failed connecting to rabbitmq")
	defer rabbit.Close()

	fmt.Println("MQ_URI: ",env.Env.MQ_URI)
	fmt.Println("MQ_CLIENT: ",env.Env.MQ_CLIENT)
	fmt.Println("MQ_EXCHANGE: ",env.Env.MQ_EXCHANGE)
	fmt.Println("MQ_QUEUE: ",env.Env.MQ_QUEUE)
	dataMQ, err := rabbit.Consume(env.Env.MQ_EXCHANGE, env.Env.MQ_QUEUE, env.Env.MQ_KEY)
	ms.FailOnError(err, "failed creating data queue")

	fmt.Println("Waiting for messages. To exit press CTRL+C")
	go tr.HandleQueue(dataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		fmt.Println("DATA FLOWING")
		payload, err := unmarshalGeneric[LodgingBusiness](r.Rawdata)
		if err != nil {
			slog.Error("cannot unmarshall raw data", "err", err)
			return err
		}

		fmt.Println("PAYLOAD: ",payload)
		return nil

	})
	
	select {}
}

func unmarshalGeneric[T any](values string) (*T, error) {
	var result T
	if err := json.Unmarshal([]byte(values), &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
	}
	return &result, nil
}

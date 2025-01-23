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

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"

	"github.com/hashicorp/go-retryablehttp"
	"github.com/noi-techpark/go-opendatahub-discoverswiss/mappers"
	"github.com/noi-techpark/go-opendatahub-discoverswiss/models"
)

type TokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

var env struct {
	tr.Env

	HTTP_URL    string
	HTTP_METHOD string `default:"GET"`

	ODH_CORE_TOKEN_URL string
	ODH_CORE_TOKEN_USERNAME string
	ODH_CORE_TOKEN_PASSWORD string
	ODH_CORE_TOKEN_CLIENT_ID string
	ODH_CORE_TOKEN_CLIENT_SECRET string

	ODH_API_CORE_URL string 

	SUBSCRIPTION_KEY string

	RAW_FILTER_URL_TEMPLATE string
}

const ENV_HEADER_PREFIX = "HTTP_HEADER_"

const RAW_FILTER_URL_TEMPLATE = "https://api.tourism.testingmachine.eu/v1/Accommodation?rawfilter=eq(Mapping.discoverswiss.id,%%22%s%%22)&fields=Id"

type RawFilterId struct {
	Items []struct {
		Id string `json:"Id"`
	} `json:"Items"`
}

func rawFilterHttpRequest(id string) (string, error) {
	url,err := url.Parse(fmt.Sprintf(env.RAW_FILTER_URL_TEMPLATE, id))
	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	client := retryablehttp.NewClient()
	req,err := retryablehttp.NewRequest("GET", url.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not create http request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error during http request: %w", err)
	}

	defer resp.Body.Close()
	
	var rawFilterId RawFilterId

	err = json.NewDecoder(resp.Body).Decode(&rawFilterId)
	if err != nil {
		return "", fmt.Errorf("could not decode response: %w", err)
	}

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

func putContentApi(url *url.URL, token string, payload interface{}, id string) (string,error) {
    jsonData, err := json.Marshal(payload)
    if err != nil {
		return "", fmt.Errorf("could not marshal payload: %w", err)
	}	

	u := fmt.Sprintf("%s/%s", url.String(), id)
	slog.Info("PUT URL", "url", u)
	newurl, err := url.Parse(u)
	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	req, err := retryablehttp.NewRequest("PUT", newurl.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("could not create http request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := retryablehttp.NewClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error during http request: %w", err)
	}

	return strconv.Itoa(resp.StatusCode), nil   
}
	

func postContentApi(url *url.URL, token string, payload interface{}) (string,error) {

    jsonData, err := json.Marshal(payload)
    if err != nil {
		return "", fmt.Errorf("could not marshal payload: %w", err)
	}	
    u := url

	req, err := retryablehttp.NewRequest("POST", u.String(), bytes.NewBuffer(jsonData))
	if err != nil {
		return "", fmt.Errorf("could not create http request: %w", err)
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	client := retryablehttp.NewClient()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("error during http request: %w", err)
	}

	return strconv.Itoa(resp.StatusCode), nil
}

type idplusaccomodation struct{
	Id string
	Accommodation models.Accommodation
}

func main() {
	// err := godotenv.Load("../.env")
	// if err != nil {
	// 	slog.Error("Error loading .env file")
	// }
	envconfig.MustProcess("", &env)
	ms.InitLog(env.Env.LOG_LEVEL)

	rabbit, err := mq.Connect(env.Env.MQ_URI, env.Env.MQ_CLIENT)
	ms.FailOnError(err, "failed connecting to rabbitmq")
	defer rabbit.Close()

	dataMQ, err := rabbit.Consume(env.Env.MQ_EXCHANGE, env.Env.MQ_QUEUE, env.Env.MQ_KEY)
	ms.FailOnError(err, "failed creating data queue")


	fmt.Println("Waiting for messages. To exit press CTRL+C")
	lbChannel := make(chan models.LodgingBusiness,400)
	go tr.HandleQueue(dataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		fmt.Println("DATA FLOWING")
		payload, err := unmarshalGeneric[models.LodgingBusiness](r.Rawdata)
		if err != nil {
			slog.Error("cannot unmarshall raw data", "err", err)
			return err
		}

		//fmt.Println("PAYLOAD: ",payload)
		lbChannel <- *payload
		return nil

	})


	accoChannel := make(chan models.Accommodation,400)
	go func(){
		fmt.Println("STARTED THE MAPPING OF THE CHANNEL!")
		for lb := range lbChannel {
			acco := mappers.MapLodgingBusinessToAccommodation(lb)
			accoChannel <- acco
			fmt.Println("ACCOMODATION: ",acco)
		}
	}()
	
	

	var putChannel = make(chan idplusaccomodation,1000)
	var postChannel = make(chan models.Accommodation,1000)

	go func(){		
		fmt.Println("STARTED THE PUT AND POST CHANNELS!")
		for acco := range accoChannel {
			fmt.Println("ACCOMODATING")
			
			rawfilter,err := rawFilterHttpRequest(acco.Mapping.DiscoverSwiss.Id)
			if err != nil {
				slog.Error("cannot get rawfilter", "err", err)
				return
			}
			if len(rawfilter)>0 && rawfilter != "" {
				fmt.Println("INSERTING IN PUT CHANNEL")
				idplusaccomodation := idplusaccomodation{Id: rawfilter, Accommodation: acco}
				putChannel <- idplusaccomodation
			}else{
				postChannel <- acco
			}
		}}()
	
		go func(){			
			fmt.Println("PUSHING DATA TO OPENDATAHUB!")
			token,err := getAccessToken(env.ODH_CORE_TOKEN_URL, env.ODH_CORE_TOKEN_USERNAME, env.ODH_CORE_TOKEN_PASSWORD, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
			if err != nil {
				slog.Error("cannot get token", "err", err)
				return
			}

			for acco := range putChannel {
				fmt.Println("PUTTING")
				u, err := url.Parse(env.ODH_API_CORE_URL)
				fmt.Println("URL: ",u)
				if err != nil {
					slog.Error("cannot parse url", "err", err)
					return
				}
				respStatus,err := putContentApi(u, token.AccessToken, acco.Accommodation, acco.Id)
				if err != nil {
					slog.Error("cannot make authorized request", "err", err)
					return
				}
				fmt.Println("RESPONSE STATUS: ",respStatus)
			}}()

	
		go func(){				
				fmt.Println("PUSHING DATA TO OPENDATAHUB!")
				u,err := url.Parse(env.ODH_API_CORE_URL)
				if err != nil {
					slog.Error("cannot parse url", "err", err)
					return
				}
				token,err := getAccessToken(env.ODH_CORE_TOKEN_URL, env.ODH_CORE_TOKEN_USERNAME, env.ODH_CORE_TOKEN_PASSWORD, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
				ms.FailOnError(err, "cannot get token")
				for acco := range postChannel {
					fmt.Println("POSTING")
					respStatus,err := postContentApi(u, token.AccessToken, acco)
					if err != nil {
						slog.Error("cannot make authorized request", "err", err)
						return
					}
					 fmt.Println("RESPONSE STATUS: ",respStatus)
				}
			}()

		 select{}
}

func unmarshalGeneric[T any](values string) (*T, error) {
	var result T
	if err := json.Unmarshal([]byte(values), &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
	}
	return &result, nil
}

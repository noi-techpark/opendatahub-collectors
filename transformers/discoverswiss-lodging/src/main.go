// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"

	"github.com/noi-techpark/go-opendatahub-discoverswiss/mappers"
	"github.com/noi-techpark/go-opendatahub-discoverswiss/models"
	"github.com/noi-techpark/go-opendatahub-discoverswiss/utilities"
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

func unmarshalGeneric[T any](values string) (*T, error) {
	var result T
	if err := json.Unmarshal([]byte(values), &result); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
	}
	return &result, nil
}

type idplusaccomodation struct{
	Id string
	Accommodation models.Accommodation
}

func main() {
	// FOR LOCAL TESTING UNCOMMENT THIS LINES
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

		lbChannel <- *payload
		return nil

	})

	accoChannel := make(chan models.Accommodation,400)
	go func(){
		for lb := range lbChannel {
			acco := mappers.MapLodgingBusinessToAccommodation(lb)
			accoChannel <- acco
		}
	}()
	
	var putChannel = make(chan idplusaccomodation,1000)
	var postChannel = make(chan models.Accommodation,1000)
	go func(){		
		for acco := range accoChannel {			
			rawfilter,err := utilities.GetAccomodationIdByRawFilter(acco.Mapping.DiscoverSwiss.Id,env.RAW_FILTER_URL_TEMPLATE)
			if err != nil {
				slog.Error("cannot get rawfilter", "err", err)
				return
			}
			if len(rawfilter)>0 && rawfilter != "" {
				idplusaccomodation := idplusaccomodation{Id: rawfilter, Accommodation: acco}
				putChannel <- idplusaccomodation
			}else{
				postChannel <- acco
			}
		}}()
	
		go func(){			
			token,err := utilities.GetAccessToken(env.ODH_CORE_TOKEN_URL, env.ODH_CORE_TOKEN_USERNAME, env.ODH_CORE_TOKEN_PASSWORD, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
			if err != nil {
				slog.Error("cannot get token", "err", err)
				return
			}

			for acco := range putChannel {
				u, err := url.Parse(env.ODH_API_CORE_URL)
				fmt.Println("URL: ",u)
				if err != nil {
					slog.Error("cannot parse url", "err", err)
					return
				}
				respStatus,err := utilities.PutContentApi(u, token.AccessToken, acco.Accommodation, acco.Id)
				if err != nil {
					slog.Error("cannot make authorized request", "err", err)
					return
				}
				fmt.Println("RESPONSE STATUS: ",respStatus)
			}}()
	
		go func(){				
				u,err := url.Parse(env.ODH_API_CORE_URL)
				if err != nil {
					slog.Error("cannot parse url", "err", err)
					return
				}
				token,err := utilities.GetAccessToken(env.ODH_CORE_TOKEN_URL, env.ODH_CORE_TOKEN_USERNAME, env.ODH_CORE_TOKEN_PASSWORD, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
				ms.FailOnError(err, "cannot get token")
				for acco := range postChannel {
					fmt.Println("POSTING")
					respStatus,err := utilities.PostContentApi(u, token.AccessToken, acco)
					if err != nil {
						slog.Error("cannot make authorized request", "err", err)
						return
					}
					 fmt.Println("RESPONSE STATUS: ",respStatus)
				}
			}()

		 select{}
}



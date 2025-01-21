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
		return "", fmt.Errorf("http request returned non-Ok status: %d for url", resp.StatusCode)}
}

func rawFilterHttpRequest(id string) (string, error) {
	url,err := url.Parse(fmt.Sprintf(env.RAW_FILTER_URL_TEMPLATE, id))
	if err != nil {
		return "", fmt.Errorf("could not parse url: %w", err)
	}

	req,err := http.NewRequest("GET", url.String(), nil)
	if err != nil {
		return "", fmt.Errorf("could not create http request: %w", err)
	}
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

	//fmt.Println("RAWFILTERID: ",rawFilterId)

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
		return strconv.Itoa(resp.StatusCode),fmt.Errorf("http request returned non-Ok status: %d for url %s", resp.StatusCode, newurl.String())
		}
	}
		
        defer resp.Body.Close()

        return fmt.Sprint("RESPCODE: ",string(resp.StatusCode)), nil
    }
	

    return "",fmt.Errorf("unsupported HTTP method: %s", httpMethod)
}

func mapAdditionalTypeToAccoTypeId(additionalType string) string {
	if strings.EqualFold(additionalType, "Hotel") {
		return "HotelPension"
	}
	return additionalType
}

func mapLodgingBusinessToAccommodation(lb LodgingBusiness) Accommodation {
	acco := Accommodation{
		Source:    "discoverswiss",
		Active:    true,
		Shortname: lb.Name,
	}

	acco.Mapping.DiscoverSwiss.Id = lb.Identifier
	acco.LicenseInfo.Author = ""
	acco.LicenseInfo.License = "TEST" //lb.License	
	acco.LicenseInfo.ClosedData = false
	acco.LicenseInfo.LicenseHolder = "www.discover.swiss"

	acco.GpsInfo = []struct {
		Gpstype              string  `json:"Gpstype"`
		Latitude             float64 `json:"Latitude"`
		Longitude            float64 `json:"Longitude"`
		Altitude             float64 `json:"Altitude"`
		AltitudeUnitofMeasure string `json:"AltitudeUnitofMeasure"`
	}{
		{
			Gpstype:              "position",
			Latitude:             lb.Geo.Latitude,
			Longitude:            lb.Geo.Longitude,
			Altitude:             0,
			AltitudeUnitofMeasure: "m",
		},
	}

	acco.AccoDetail.Language = AccoDetailLanguage{
		Name:        lb.Name,
		Street:      lb.Address.StreetAddress,
		Zip:         lb.Address.PostalCode,
		City:        lb.Address.AddressLocality,
		CountryCode: lb.Address.AddressCountry,
		Email:       lb.Address.Email,
		Phone:       lb.Address.Telephone,
	}

	var totalRooms, singleRooms, doubleRooms int
	for _, room := range lb.NumberOfRooms {
		value := 0
		fmt.Sscanf(room.Value, "%d", &value)

		switch room.PropertyID {
		case "total":
			totalRooms = value
		case "single":
			singleRooms = value
		case "double":
			doubleRooms = value
		}
	}

	acco.AccoOverview.TotalRooms = totalRooms
	acco.AccoOverview.SingleRooms = singleRooms
	acco.AccoOverview.DoubleRooms = doubleRooms
	acco.AccoOverview.CheckInFrom = lb.CheckinTime
	acco.AccoOverview.CheckInTo = lb.CheckinTimeTo
	acco.AccoOverview.CheckOutFrom = lb.CheckoutTimeFrom
	acco.AccoOverview.CheckOutTo = lb.CheckoutTime
	acco.AccoOverview.MaxPersons = lb.NumberOfBeds
	
	acco.AccoType = struct {
		Id string `json:"Id"`
	}{
		Id: mapAdditionalTypeToAccoTypeId(lb.StarRating.AdditionalType),
	}

	return acco
}

type idplusaccomodation struct{
	Id string
	Accommodation Accommodation
}

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.Env.LOG_LEVEL)

	rabbit, err := mq.Connect(env.Env.MQ_URI, env.Env.MQ_CLIENT)
	ms.FailOnError(err, "failed connecting to rabbitmq")
	defer rabbit.Close()

	dataMQ, err := rabbit.Consume(env.Env.MQ_EXCHANGE, env.Env.MQ_QUEUE, env.Env.MQ_KEY)
	ms.FailOnError(err, "failed creating data queue")


	fmt.Println("Waiting for messages. To exit press CTRL+C")
	lbChannel := make(chan LodgingBusiness,400)
	go tr.HandleQueue(dataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		fmt.Println("DATA FLOWING")
		payload, err := unmarshalGeneric[LodgingBusiness](r.Rawdata)
		if err != nil {
			slog.Error("cannot unmarshall raw data", "err", err)
			return err
		}

		//fmt.Println("PAYLOAD: ",payload)
		lbChannel <- *payload
		return nil

	})


	accoChannel := make(chan Accommodation,400)
	go func(){
		fmt.Println("STARTED THE MAPPING OF THE CHANNEL!")
		for lb := range lbChannel {
			acco := mapLodgingBusinessToAccommodation(lb)
			accoChannel <- acco
			fmt.Println("ACCOMODATION: ",acco)
		}
	}()
	
	

	var putChannel = make(chan idplusaccomodation,1000)
	var postChannel = make(chan Accommodation,1000)

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
			token,err := getAccessToken(env.TOKEN_URL, env.ODH_CORE_TOKEN_USERNAME, env.ODH_CORE_TOKEN_PASSWORD, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
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
				respStatus,err := makeAuthorizedRequest(u, token.AccessToken, acco.Accommodation, "PUT", acco.Id)
				if err != nil {
					slog.Error("cannot make authorized request", "err", err)
					return
				}
				fmt.Println("RESPONSE STATUS: ",respStatus)
				// if respStatus != "200" {
				// 	slog.Error("response status not 200", "err", err)
				// 	return
				// }
			}}()

	
		go func(){
				
				fmt.Println("PUSHING DATA TO OPENDATAHUB!")
				u,err := url.Parse(env.ODH_API_CORE_URL)
				if err != nil {
					slog.Error("cannot parse url", "err", err)
					return
				}
				token,err := getAccessToken(env.TOKEN_URL, env.ODH_CORE_TOKEN_USERNAME, env.ODH_CORE_TOKEN_PASSWORD, env.ODH_CORE_TOKEN_CLIENT_ID, env.ODH_CORE_TOKEN_CLIENT_SECRET)
				if err != nil {
					slog.Error("cannot get token", "err", err)
					return
				}
				for acco := range postChannel {
					fmt.Println("POSTING")
					respStatus,err := makeAuthorizedRequest(u, token.AccessToken, acco, "POST", "")
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

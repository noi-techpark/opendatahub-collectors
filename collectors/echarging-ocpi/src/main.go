// SPDX-FileCopyrightText: NOI Techpark <digital@noi.bz.it>
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	sloggin "github.com/samber/slog-gin"
)

const ver string = "2.2.1"

var baseUrl string = os.Getenv("BASE_URL")
var provider string = os.Getenv("VERSIONS_ENDPOINT")
var tokenA string = os.Getenv("TOKEN_A")
var tokenB string = os.Getenv("TOKEN_B")

var versionUrl string = baseUrl + "/versions"

type VersionsData struct {
	Version string `json:"version"`
	Url     string `json:"url"`
}

type Versions struct {
	Data       []VersionsData `json:"data"`
	StatusCode int            `json:"status_code"`
	Timestamp  string         `json:"timestamp"`
}

type VersionEndpoint struct {
	Identifier string `json:"identifier"`
	Role       string `json:"role"`
	Url        string `json:"url"`
}

type VersionData struct {
	Version   string            `json:"version"`
	Endpoints []VersionEndpoint `json:"endpoints"`
}

type Version struct {
	Data       VersionData `json:"data"`
	StatusCode int         `json:"status_code"`
	Timestamp  string      `json:"timestamp"`
}

type VersionRes struct {
	Data       []VersionData `json:"data"`
	StatusCode int           `json:"status_code"`
	Timestamp  string        `json:"timestamp"`
}

type Credentials struct {
	Url   string   `json:"url"`
	Token string   `json:"token"`
	Roles []string `json:"roles"`
}

type Locations struct {
	Address            string `json:"address"`
	ChargingWhenClosed bool   `json:"charging_when_closed,omitempty"`
	City               string `json:"city"`
	Coordinates        string `json:"coordinates"`
	Country            string `json:"country"`
	CountryCode        string `json:"country_code,omitempty"`
	Directions         string `json:"directions,omitempty"`
	EnergyMix          string `json:"energy_mix,omitempty"`
	Evses              string `json:"evses,omitempty"`
	Facilities         string `json:"facilities,omitempty"`
	Id                 string `json:"id,omitempty"`
	Images             string `json:"images,omitempty"`
	LastUpdated        string `json:"last_updated"`
	Name               string `json:"name,omitempty"`
	OpeningTimes       string `json:"opening_times,omitempty"`
	Operator           string `json:"operator,omitempty"`
	Owner              string `json:"owner,omitempty"`
	ParkingType        string `json:"parking_type,omitempty"`
	PartyId            string `json:"party_id,omitempty"`
	PostalCode         string `json:"postal_code,omitempty"`
	Publish            bool   `json:"publish,omitempty"`
	PublishAllowedTo   string `json:"publish_allowed_to,omitempty"`
	RelatedLocations   string `json:"related_locations,omitempty"`
	State              string `json:"state,omitempty"`
	Suboperator        string `json:"suboperator,omitempty"`
	TimeZone           string `json:"time_zone"`
}

func initLogger() {
	logLevel := os.Getenv("LOG_LEVEL")
	level := &slog.LevelVar{}
	level.UnmarshalText([]byte(logLevel))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}

func main() {
	initLogger()

	r := gin.New()

	if os.Getenv("GIN_LOG") == "PRETTY" {
		r.Use(gin.Logger())
	} else {
		// Enable slog logging for gin framework
		// https://github.com/samber/slog-gin
		r.Use(sloggin.New(slog.Default()))
	}

	slog.Info("START GIN")

	r.Use(gin.Recovery())

	r.GET("/versions", versions)
	r.GET("/"+ver, version)
	r.GET("/"+ver+"/credentials", credentials)
	r.GET("/"+ver+"/locations", locations)
	r.GET("/health", health)
	r.GET("/driwe", driwe)
	r.Run()

	slog.Info("GET BLA")

}

// ////////////////////////////
// GIN functions
// ////////////////////////////
func health(c *gin.Context) {
	c.Status(http.StatusOK)
}

func versions(c *gin.Context) {
	t := time.Now()

	var res Versions
	res.StatusCode = 1000
	res.Timestamp = t.Format(time.RFC3339)

	var data VersionsData
	data.Url = baseUrl + "/" + ver
	data.Version = ver

	res.Data = append(res.Data, data)

	c.JSON(http.StatusOK, res)
}

func version(c *gin.Context) {
	t := time.Now()

	var res Version
	res.StatusCode = 1000
	res.Timestamp = t.Format(time.RFC3339)

	var cred VersionEndpoint
	cred.Role = "HUB"
	cred.Url = baseUrl + "/" + ver + "/credentials"
	cred.Identifier = "credentials"

	var loc VersionEndpoint
	loc.Role = "HUB"
	loc.Url = baseUrl + "/" + ver + "/locations"
	loc.Identifier = "locations"

	var data VersionData
	data.Version = ver
	data.Endpoints = append(data.Endpoints, cred)
	data.Endpoints = append(data.Endpoints, loc)

	res.Data = data

	c.JSON(http.StatusOK, res)
}

func credentials(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"token": tokenB})
}

func locations(c *gin.Context) {
	c.Value(http.StatusOK)
}

func driwe(c *gin.Context) {

	// register to providers
	credUrl := "https://ocpi.driwe.club/2.2.1/credentials"
	tokenC := register(credUrl, tokenA)

	// get locations
	// TODO use url gotten from endpoint instead of hard coded
	locUrl := "https://ocpi.driwe.club/2.2.1/locations"
	locations := getLocations(locUrl, tokenC)
	locStr, err := json.Marshal(locations)
	if err != nil {
		panic(err)
	}
	slog.Info(string(locStr))

	c.Status(http.StatusOK)
}

// ////////////////////////////
// Generic functions
// ////////////////////////////
func register(url string, tokenA string) string {
	tokenB := "1d082cbe-cdc4-4e33-b11b-9001837d65aa"
	tokenC := postTokenB(url, tokenA, tokenB)
	slog.Info("TOKEN C: " + tokenC)

	return tokenC
}

func getLocations(url string, tokenC string) Locations {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		slog.Error("Get locations: http get error")
	}
	req.Header.Set("Authorization", "Token "+tokenC)
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}

	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	var locResp Locations
	json.Unmarshal(body, &locResp)
	return locResp
}

func postTokenB(url string, tokenA string, tokenB string) string {
	var creds Credentials
	creds.Token = tokenB
	creds.Url = versionUrl
	creds.Roles = append(creds.Roles, "HUB")

	credsJSON, err := json.Marshal(creds)
	if err != nil {
		slog.Error("Post Token B: marshal error")

	}

	req, err := http.NewRequest("POST", url, bytes.NewBuffer(credsJSON))
	if err != nil {
		slog.Error("Post Token B: http post error")
	}
	req.Header.Set("Authorization", "Token "+tokenA)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var credsResp Credentials
	json.Unmarshal(body, &credsResp)

	slog.Info("Post TokenB status code:" + resp.Status)
	slog.Info("Post TokenB body:" + string(body))
	slog.Info("Post TokenB  TokenC:" + credsResp.Token)

	return credsResp.Token
}

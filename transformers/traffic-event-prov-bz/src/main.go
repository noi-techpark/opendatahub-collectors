// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
)

var env tr.Env

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.LOG_LEVEL)

	b := bdplib.FromEnv()

	tr.ListenFromEnv(env, func(r *dto.Raw[string]) error {
		dtos := trafficEvent{}
		if err := json.Unmarshal([]byte(r.Rawdata), &dtos); err != nil {
			return fmt.Errorf("could not unmarshal the raw payload json: %w", err)
		}
		events := []bdplib.Event{}
		for d, _ := range dtos {
			fmt.Print(d)
		}

		b.SyncEvents(events)
		return nil
	})

}

func getUuidFields(e trafficEvent) map[string]any {
	return nil
}

type trafficEvent []struct {
	JSONFeaturetype             string  `json:"json_featuretype"`
	PublishDateTime             string  `json:"publishDateTime"`
	BeginDate                   string  `json:"beginDate"`
	EndDate                     string  `json:"endDate"`
	DescriptionDe               string  `json:"descriptionDe"`
	DescriptionIt               string  `json:"descriptionIt"`
	TycodeValue                 string  `json:"tycodeValue"`
	TycodeDe                    string  `json:"tycodeDe"`
	TycodeIt                    string  `json:"tycodeIt"`
	SubTycodeValue              string  `json:"subTycodeValue"`
	SubTycodeDe                 string  `json:"subTycodeDe"`
	SubTycodeIt                 string  `json:"subTycodeIt"`
	PlaceDe                     string  `json:"placeDe"`
	PlaceIt                     string  `json:"placeIt"`
	ActualMail                  int     `json:"actualMail"`
	MessageID                   int     `json:"messageId"`
	MessageStatus               int     `json:"messageStatus"`
	MessageZoneID               int     `json:"messageZoneId"`
	MessageZoneDescDe           string  `json:"messageZoneDescDe"`
	MessageZoneDescIt           string  `json:"messageZoneDescIt"`
	MessageGradID               int     `json:"messageGradId"`
	MessageGradDescDe           string  `json:"messageGradDescDe"`
	MessageGradDescIt           string  `json:"messageGradDescIt"`
	MessageStreetID             int     `json:"messageStreetId"`
	MessageStreetWapDescDe      string  `json:"messageStreetWapDescDe"`
	MessageStreetWapDescIt      string  `json:"messageStreetWapDescIt"`
	MessageStreetInternetDescDe string  `json:"messageStreetInternetDescDe"`
	MessageStreetInternetDescIt string  `json:"messageStreetInternetDescIt"`
	MessageStreetNr             string  `json:"messageStreetNr"`
	MessageStreetHierarchie     int     `json:"messageStreetHierarchie"`
	MessageTypeID               int     `json:"messageTypeId"`
	MessageTypeDescDe           string  `json:"messageTypeDescDe"`
	MessageTypeDescIt           string  `json:"messageTypeDescIt"`
	X                           float64 `json:"X"`
	Y                           float64 `json:"Y"`
}

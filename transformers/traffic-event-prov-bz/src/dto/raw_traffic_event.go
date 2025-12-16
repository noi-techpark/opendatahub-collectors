// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package dto

type TrafficEvent struct {
	JSONFeaturetype             string `json:"json_featuretype"`
	PublishDateTime             string `json:"publishDateTime"`
	BeginDate                   string `json:"beginDate"`
	EndDate                     string `json:"endDate"`
	DescriptionDe               string `json:"descriptionDe"`
	DescriptionIt               string `json:"descriptionIt"`
	TycodeValue                 string `json:"tycodeValue"`
	TycodeDe                    string `json:"tycodeDe"`
	TycodeIt                    string `json:"tycodeIt"`
	SubTycodeValue              string `json:"subTycodeValue"`
	SubTycodeDe                 string `json:"subTycodeDe"`
	SubTycodeIt                 string `json:"subTycodeIt"`
	PlaceDe                     string `json:"placeDe"`
	PlaceIt                     string `json:"placeIt"`
	ActualMail                  int    `json:"actualMail"`
	MessageID                   int    `json:"messageId"`
	MessageStatus               any    `json:"messageStatus"`
	MessageZoneID               any    `json:"messageZoneId"`
	MessageZoneDescDe           string `json:"messageZoneDescDe"`
	MessageZoneDescIt           string `json:"messageZoneDescIt"`
	MessageGradID               any    `json:"messageGradId"`
	MessageGradDescDe           string `json:"messageGradDescDe"`
	MessageGradDescIt           string `json:"messageGradDescIt"`
	MessageStreetID             any    `json:"messageStreetId"`
	MessageStreetWapDescDe      string `json:"messageStreetWapDescDe"`
	MessageStreetWapDescIt      string `json:"messageStreetWapDescIt"`
	MessageStreetInternetDescDe string `json:"messageStreetInternetDescDe"`
	MessageStreetInternetDescIt string `json:"messageStreetInternetDescIt"`
	MessageStreetNr             string `json:"messageStreetNr"`

	// MessageStreetHierarchie seems to be inconsistent having sometimes type int and sometimes type string
	//MessageStreetHierarchie     any      `json:"messageStreetHierarchie"`
	MessageTypeID     int    `json:"messageTypeId"`
	MessageTypeDescDe string `json:"messageTypeDescDe"`
	MessageTypeDescIt string `json:"messageTypeDescIt"`
	X                 any    `json:"X"`
	Y                 any    `json:"Y"`
}

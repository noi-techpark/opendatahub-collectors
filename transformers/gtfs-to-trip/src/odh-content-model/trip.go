// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

import (
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

type Generic struct {
	ID          *string                      `json:"Id,omitempty"`
	Meta        *clib.Metadata               `json:"_Meta,omitempty"`
	LicenseInfo *clib.LicenseInfo            `json:"LicenseInfo,omitempty"`
	Shortname   *string                      `json:"Shortname,omitempty"`
	Active      bool                         `json:"Active"`
	FirstImport *time.Time                   `json:"FirstImport,omitempty"`
	LastChange  *time.Time                   `json:"LastChange,omitempty"`
	HasLanguage []string                     `json:"HasLanguage,omitempty"`
	Mapping     map[string]map[string]string `json:"Mapping,omitempty"`
	Source      *string                      `json:"Source,omitempty"`
	TagIds      []string                     `json:"TagIds,omitempty"`
}

type OperationSchedule struct {
	OperationscheduleName map[string]string       `json:"OperationscheduleName,omitempty"`
	Start                 time.Time               `json:"Start"`
	Stop                  time.Time               `json:"Stop"`
	Type                  *string                 `json:"Type,omitempty"`
	OperationScheduleTime []OperationScheduleTime `json:"OperationScheduleTime,omitempty"`
}

type OperationScheduleTime struct {
	Start     string `json:"Start"`
	End       string `json:"End"`
	Monday    bool   `json:"Monday"`
	Tuesday   bool   `json:"Tuesday"`
	Wednesday bool   `json:"Wednesday"`
	Thursday  bool   `json:"Thursday"`
	Friday    bool   `json:"Friday"`
	Saturday  bool   `json:"Saturday"`
	Sunday    bool   `json:"Sunday"`
	State     int    `json:"State"`
}

type Calendar struct {
	OperationSchedule OperationSchedule `json:"OperationSchedule"`
	AdditionalDates   []time.Time       `json:"AdditionalDates,omitempty"`
	ExcludedDates     []time.Time       `json:"ExcludedDates,omitempty"`
}

type TripRoute struct {
	Shortname string                        `json:"Shortname"`
	Detail    map[string]*clib.DetailGeneric `json:"Detail,omitempty"`
	TagIds    []string                       `json:"TagIds,omitempty"`
	Calendar  *Calendar                      `json:"Calendar,omitempty"`
}

type TripStopTime struct {
	Shortname     string                        `json:"Shortname"`
	Detail        map[string]*clib.DetailGeneric `json:"Detail,omitempty"`
	Geo           map[string]clib.GpsInfo        `json:"Geo,omitempty"`
	ArrivalTime   time.Time                      `json:"ArrivalTime"`
	DepartureTime time.Time                      `json:"DepartureTime"`
}

type ContactInfos struct {
	CompanyName *string `json:"CompanyName,omitempty"`
	Url         *string `json:"Url,omitempty"`
	Language    *string `json:"Language,omitempty"`
}

type TripAgency struct {
	Shortname    string                   `json:"Shortname"`
	ContactInfos map[string]*ContactInfos `json:"ContactInfos,omitempty"`
}

type Trip struct {
	Generic
	Route     *TripRoute              `json:"Route,omitempty"`
	Agency    *TripAgency             `json:"Agency,omitempty"`
	StopTimes []TripStopTime          `json:"StopTimes,omitempty"`
	Geo       map[string]clib.GpsInfo `json:"Geo,omitempty"`
}

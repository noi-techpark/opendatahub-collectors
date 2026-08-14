// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// RawData is the top-level envelope from the collector.
// The collector merges the DSS response under the key "dssLifts".
type RawData struct {
	DssLifts DssLiftFeed `json:"dssLifts"`
}

// DssLiftFeed is the full response from
// https://www.dolomitisuperski.com/jsonexport/export/liftbasis
type DssLiftFeed struct {
	Modification int64     `json:"modification"`
	LastUpdate   string    `json:"lastUpdate"`
	Items        []DssLift `json:"items"`
}

// DssLift is one lift record exactly as delivered by the DSS API.
type DssLift struct {
	Rid                 int64        `json:"rid"`
	Pid                 int64        `json:"pid"`
	RegionId            int64        `json:"regionId"`
	SubregionId         string       `json:"subregionId"`
	Duration            string       `json:"duration"` // seconds as string e.g. "782"
	StateWinter         int          `json:"state-winter"`
	StateSummer         int          `json:"state-summer"`
	DatacenterId        string       `json:"datacenterId"`
	Number              string       `json:"number"`
	WinterOperation     bool         `json:"winterOperation"`
	Sorter              *bool        `json:"sorter"`
	SummerOperation     bool         `json:"summerOperation"`
	SorterSummer        *bool        `json:"sorterSummer"`
	UpdateDate          int64        `json:"update-date"` // unix seconds
	Lifttype            DssLifttype  `json:"lifttype"`
	Name                DssMultilang `json:"name"`
	Description         DssMultilang `json:"description"`
	InfoText            DssMultilang `json:"info-text"`
	InfoTextSummer      DssMultilang `json:"info-text-summer"`
	Skiresort           DssSkiresort `json:"skiresort"`
	Data                DssLiftData  `json:"data"`
	Location            *DssLocation `json:"location"`         // nullable
	LocationMountain    *DssLocation `json:"locationMountain"` // nullable
	GeoPositionFile     string       `json:"geoPositionFile"`
	IsRelated           int          `json:"isRelated"`
	IsRelatedWithLiftId int          `json:"isRelatedWithLiftId"`
}

type DssLifttype struct {
	Rid  int64        `json:"rid"`
	Name DssMultilang `json:"name"`
}

// DssMultilang holds localised strings. Any field can be null in the API.
type DssMultilang struct {
	De *string `json:"de"`
	It *string `json:"it"`
	En *string `json:"en"`
}

type DssSkiresort struct {
	Rid  int64        `json:"rid"`
	Pid  int64        `json:"pid"`
	Name DssMultilang `json:"name"`
}

type DssLiftData struct {
	OpeningTimes       DssOpeningTimes     `json:"opening-times"`
	OpeningTimesSummer DssOpeningTimes     `json:"opening-times-summer"`
	SeasonWinter       DssSeason           `json:"season-winter"`
	SeasonSummer       DssSeason           `json:"season-summer"`
	Length             *float64            `json:"length"`
	Capacity           *int                `json:"capacity"`
	CapacityPerHour    *int                `json:"capacity-per-hour"`
	AltitudeStart      *int                `json:"altitude-start"`
	AltitudeEnd        *int                `json:"altitude-end"`
	HeightDifference   *int                `json:"height-difference"`
	SummercardPoints   DssSummercardPoints `json:"summercard-points"`
	BikeTransport      bool                `json:"bike-transport"`
}

type DssOpeningTimes struct {
	Start          string `json:"start"`
	End            string `json:"end"`
	StartAfternoon string `json:"startAfternoon"`
	EndAfternoon   string `json:"endAfternoon"`
}

// DssSeason holds unix-second timestamps; both can be null.
type DssSeason struct {
	Start *int64 `json:"start"`
	End   *int64 `json:"end"`
}

type DssSummercardPoints struct {
	Up        *int `json:"up"`
	Down      *int `json:"down"`
	Roundtrip *int `json:"roundtrip"`
}

// DssLocation holds lat/lon as strings in the raw JSON.
type DssLocation struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

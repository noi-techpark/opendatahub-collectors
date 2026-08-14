// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// SlopeRawData is the top-level envelope from the collector.
type RawData struct {
	DssSlopes DssSlopeFeed `json:"dssSlopes"`
}

type DssSlopeFeed struct {
	Modification int64      `json:"modification"`
	LastUpdate   string     `json:"lastUpdate"`
	Items        []DssSlope `json:"items"`
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

// DssSeason holds unix-second timestamps; both can be null.
type DssSeason struct {
	Start *int64 `json:"start"`
	End   *int64 `json:"end"`
}

type DssOpeningTimes struct {
	Start          string `json:"start"`
	End            string `json:"end"`
	StartAfternoon string `json:"startAfternoon"`
	EndAfternoon   string `json:"endAfternoon"`
}

type DssSlope struct {
	Rid             int64             `json:"rid"`
	Pid             int64             `json:"pid"`
	RegionId        int64             `json:"regionId"`
	Duration        string            `json:"duration"`
	State           int               `json:"state"`
	DatacenterId    string            `json:"datacenterId"`
	Number          string            `json:"number"`
	Sorter          *bool             `json:"sorter"`
	UpdateDate      int64             `json:"update-date"`
	SlopeType       string            `json:"slopeType"`
	Slopetype       string            `json:"slopetype"`
	Name            DssMultilang      `json:"name"`
	Description     DssMultilang      `json:"description"`
	InfoText        DssMultilang      `json:"info-text-winter"`
	Skiresort       DssSkiresort      `json:"skiresort"`
	Data            DssSlopeData      `json:"data"`
	Location        *DssSlopeLocation `json:"location"`
	GeoPositionFile string            `json:"geoPositionFile"`
	SeasonWinter    DssSeason         `json:"season-winter"`
	SeasonSummer    DssSeason         `json:"season-summer"`
	OpeningTimes    DssOpeningTimes   `json:"opening-times"`
}

type DssSlopeData struct {
	Length             *float64         `json:"length"`
	Altitude           DssSlopeAltitude `json:"altitude"`
	HeightDifference   *int             `json:"height-difference"`
	ArtificiallySnowed *bool            `json:"artificially-snowed"`
	FloodLighted       *bool            `json:"flood-lighted"`
	ValleyRun          *bool            `json:"valley-run"`
	DatacenterId       string           `json:"datacenterId"`
}

type DssSlopeAltitude struct {
	Start *int `json:"start"`
	End   *int `json:"end"`
}

type DssSlopeLocation struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// SlopeRawData is the top-level envelope from the collector.
type RawData struct {
	DssSlopes   DssSlopeFeed   `json:"dssSlopes"`
	DssSkiAreas DssSkiAreaFeed `json:"dssSkiAreas"` // talschaften, to resolve the SkiArea of a slope
}

// DssSkiAreaFeed holds the talschaften fields needed to map a skiresort to its
// SkiArea (talschaft rid = SkiArea Mapping.dss.rid).
type DssSkiAreaFeed struct {
	Items []DssSkiArea `json:"items"`
}

type DssSkiArea struct {
	Rid        string         `json:"rid"`
	Skiresorts []DssSkiresort `json:"skiresorts"`
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
	OnlyForExport   int               `json:"onlyForExport"`
	Data            DssSlopeData      `json:"data"`
	Location        *DssSlopeLocation `json:"location"`
	GeoPositionFile string            `json:"geoPositionFile"`
	SeasonStart     *int64            `json:"seasonStart"` // unix seconds, winter season; nullable
	SeasonEnd       *int64            `json:"seasonEnd"`
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

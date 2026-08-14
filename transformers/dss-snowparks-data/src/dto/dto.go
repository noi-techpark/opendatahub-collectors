// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// RawData is the top-level envelope from the collector.
// The collector merges the DSS response under the key "dssSnowparks".
type RawData struct {
	DssSnowparks DssSnowparkFeed `json:"dssSnowparks"`
}

// DssSnowparkFeed is the full response from
// https://www.dolomitisuperski.com/jsonexport/export/snowparks
type DssSnowparkFeed struct {
	Modification int64         `json:"modification"`
	LastUpdate   string        `json:"lastUpdate"`
	Items        []DssSnowpark `json:"items"`
}

// DssSnowpark is one snowpark record as delivered by the DSS API.
// Key facts confirmed from live data:
//   - rid is always 0 — only pid is a stable key
//   - NO geoPositionFile field (unlike lifts/slopes)
//   - NO seasonStart/seasonEnd (unlike slopes)
//   - NO update-date — use feed-level modification timestamp
//   - NO skiresort object — only regionId links to a region
//   - data.altitude is a flat nullable int (not nested start/end like slopes)
type DssSnowpark struct {
	State      int             `json:"state"` // 0=closed, any non-zero=open
	Rid        int64           `json:"rid"`   // always 0 — not a stable ID
	Pid        int64           `json:"pid"`   // stable key
	RegionId   int64           `json:"regionId"`
	Image      *string         `json:"image"` // nullable image URL
	Name       DssMultilang    `json:"name"`
	DetailText DssMultilang    `json:"detailText"` // equivalent of Description in slopes
	Url        DssMultilang    `json:"url"`
	Lift       DssMultilang    `json:"lift"` // associated lift name
	Data       DssSnowparkData `json:"data"`
	Location   *DssLocation    `json:"location"` // nullable; lat/lon as strings
}

// DssSnowparkData holds the snowpark-specific data fields.
// All numeric fields are nullable in the live feed.
type DssSnowparkData struct {
	Length             *float64           `json:"length"`
	Altitude           *int               `json:"altitude"` // flat nullable int (not nested start/end)
	IngroundFeatures   bool               `json:"inground-features"`
	Pipe               bool               `json:"pipe"`
	Bordercross        bool               `json:"bordercross"`
	ArtificiallySnowed bool               `json:"artificially-snowed"`
	Jibs               *int               `json:"jibs"`
	Jumps              DssJumps           `json:"jumps"`
	Lines              DssLines           `json:"lines"`
	Snowparks          DssSnowparkSubType `json:"snowparks"`
	FamilyFun          DssFamilyFun       `json:"familyFun"`
	Crossline          DssCrossline       `json:"crossline"`
}

type DssJumps struct {
	Blue  *int `json:"blue"`
	Red   *int `json:"red"`
	Black *int `json:"black"`
}

type DssLines struct {
	Blue  *int `json:"blue"`
	Red   *int `json:"red"`
	Black *int `json:"black"`
}

type DssSnowparkSubType struct {
	IsSnowpark         bool `json:"isSnowpark"`
	SnowparkOverview   *int `json:"snowparkOverview"`
	SnowparkPro        *int `json:"snowparkPro"`
	SnowparkMed        *int `json:"snowparkMed"`
	SnowparkEasy       *int `json:"snowparkEasy"`
	SnowparkJib        *int `json:"snowparkJib"`
	SnowparkWifi       bool `json:"snowparkWifi"`
	SnowparkHalfpipe   bool `json:"snowparkHalfpipe"`
	SnowparkNightslope bool `json:"snowparkNightslope"`
}

type DssFamilyFun struct {
	IsFamilyFun         bool `json:"isFamilyFun"`
	FamilyFunOverview   *int `json:"familyFunOverview"`
	FamilyFunCurves     *int `json:"familyFunCurves"`
	FamilyFunTunnel     *int `json:"familyFunTunnel"`
	FamilyFunTools      *int `json:"familyFunTools"`
	FamilyFunWifi       bool `json:"familyFunWifi"`
	FamilyFunHalfpipe   bool `json:"familyFunHalfpipe"`
	FamilyFunNightslope bool `json:"familyFunNightslope"`
}

type DssCrossline struct {
	IsCrossline         bool `json:"isCrossline"`
	CrosslineOverview   *int `json:"crosslineOverview"`
	CrosslineCurves     *int `json:"crosslineCurves"`
	CrosslineWaves      *int `json:"crosslineWaves"`
	CrosslineJumps      *int `json:"crosslineJumps"`
	CrosslineWifi       bool `json:"crosslineWifi"`
	CrosslineHalfpipe   bool `json:"crosslineHalfpipe"`
	CrosslineNightslope bool `json:"crosslineNightslope"`
}

// DssMultilang holds localised strings. Any language can be null.
type DssMultilang struct {
	De *string `json:"de"`
	It *string `json:"it"`
	En *string `json:"en"`
}

// DssLocation holds lat/lon as strings (same as lifts/slopes).
type DssLocation struct {
	Lat string `json:"lat"`
	Lon string `json:"lon"`
}

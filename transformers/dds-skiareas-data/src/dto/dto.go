// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// RawData is the top-level envelope from the collector.
// The collector merges the DSS response under the key "dssSkiAreas".
type RawData struct {
	DssSkiAreas DssSkiAreaFeed `json:"dssSkiAreas"`
}

// DssSkiAreaFeed is the full response from
// https://www.dolomitisuperski.com/jsonexport/export/talschaften
type DssSkiAreaFeed struct {
	Modification int64        `json:"modification"`
	LastUpdate   string       `json:"lastUpdate"`
	Items        []DssSkiArea `json:"items"`
}

// DssSkiArea is one skiarea (talschaft) record from the DSS API.
// Key facts from live data:
//   - rid is a STRING (e.g. "1", "4a", "4b") — NEVER cast to int
//   - pid is always "" (empty string) — not usable as ID
//   - season-winter.start/end can be null (e.g. rid "4b" Seiser Alm)
//   - email has two sub-fields: tourist-board and lifts
type DssSkiArea struct {
	Name         DssMultilang     `json:"name"`
	Rid          string           `json:"rid"` // string — includes "4a", "4b" etc.
	Pid          string           `json:"pid"` // always empty
	SeasonSummer DssSkiAreaSeason `json:"season-summer"`
	SeasonWinter DssSkiAreaSeason `json:"season-winter"`
	Phone        string           `json:"phone"`
	Email        DssSkiAreaEmail  `json:"email"`
	RegionMap    string           `json:"regionMap"`
	ActiveBike   int              `json:"activeBike"`
	ActiveHike   int              `json:"activeHike"`
	ActiveWinter int              `json:"activeWinter"`
	Skiresorts   []DssSkiresort   `json:"skiresorts"`
}

// DssSkiAreaSeason — both start and end are nullable unix timestamps.
type DssSkiAreaSeason struct {
	Start *int64 `json:"start"`
	End   *int64 `json:"end"`
}

// DssSkiAreaEmail holds two contact addresses.
type DssSkiAreaEmail struct {
	TouristBoard string `json:"tourist-board"`
	Lifts        string `json:"lifts"`
}

// DssSkiresort is a child resort within a talschaft.
type DssSkiresort struct {
	Rid  int64        `json:"rid"`
	Pid  int64        `json:"pid"`
	Name DssMultilang `json:"name"`
}

// DssMultilang holds localised strings. Any language can be null.
type DssMultilang struct {
	De *string `json:"de"`
	It *string `json:"it"`
	En *string `json:"en"`
}

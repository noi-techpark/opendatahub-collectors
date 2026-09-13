// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// RawData is the top-level envelope from the collector.
// The collector merges the DSS response under the key "dssWebcams".
type RawData struct {
	DssWebcams DssWebcamFeed `json:"dssWebcams"`
}

// DssWebcamFeed is the full response from
// https://www.dolomitisuperski.com/jsonexport/export/webcams
type DssWebcamFeed struct {
	Modification int64       `json:"modification"`
	LastUpdate   string      `json:"lastUpdate"`
	Items        []DssWebcam `json:"items"`
}

// DssWebcam is one webcam record as delivered by the DSS API.
// Key differences from lifts/slopes:
//   - rid is always 0 — never use as ID
//   - skiresort is a plain string (not an object)
//   - location lat/lon are float64 (not strings like lifts/slopes)
//   - altitude is int (not nested under data)
//   - feratelId may be non-empty — used for dedup against feratel records
type DssWebcam struct {
	Pid           int64              `json:"pid"`       // stable ID — always use this
	Rid           int64              `json:"rid"`       // always 0 — not useful
	Skiresort     string             `json:"skiresort"` // plain string, not an object
	Altitude      int                `json:"altitude"`
	OriginalImage string             `json:"original-image"`
	RegionId      int64              `json:"regionId"`
	WebcamType    string             `json:"webcamType"` // "Image" | "iFrame"
	FeratelId     string             `json:"feratelId"`  // non-empty → cross-reference to feratel
	ShowOnSummer  bool               `json:"showOnSummer"`
	Name          DssMultilang       `json:"name"`
	Iframe        DssMultilang       `json:"iframe"`   // embed URL, usually only "it" is set
	Location      *DssWebcamLocation `json:"location"` // nullable
}

// DssMultilang holds localised strings. Any language can be null.
type DssMultilang struct {
	De *string `json:"de"`
	It *string `json:"it"`
	En *string `json:"en"`
}

// DssWebcamLocation — unlike lifts/slopes, webcam lat/lon are already float64 in the JSON.
type DssWebcamLocation struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

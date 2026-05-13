// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

// UrbanGreenRow represents a single row from the urban green export CSV
type UrbanGreenRow struct {
	Provider              string `csv:"provider"`
	SpecVersion           string `csv:"spec_version"`
	ID                    string `csv:"id"`
	Code                  string `csv:"code"`
	AdditionalInformation string `csv:"additional_information"`
	State                 string `csv:"state"`
	PutOnSite             string `csv:"put_on_site"`
	RemovedFromSite       string `csv:"removed_from_site"`
	UpdatedAt             string `csv:"updated_at"`
	TheGeom               string `csv:"the_geom"`
}

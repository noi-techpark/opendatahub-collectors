// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"opendatahub.com/rest-push-skidata/skidata"
)

// FacilityCredential is the in-package alias for skidata.FacilityCredential
// so existing references in this package keep working unchanged.
type FacilityCredential = skidata.FacilityCredential

// ParseCredentials parses the JSON blob from SKIDATA_CREDENTIALS_JSON.
func ParseCredentials(jsonBlob string) ([]FacilityCredential, error) {
	return skidata.ParseCredentials([]byte(jsonBlob))
}

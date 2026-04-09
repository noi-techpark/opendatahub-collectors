// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
)

type FacilityCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Facility string `json:"facility"`
	URL      string `json:"url"`
}

// ApiURL returns the full URL for a Skidata Dynamic Data API call.
func (c FacilityCredential) ApiURL(path string) string {
	return fmt.Sprintf("%s/bei/advconn/dynamicdata/v1/%s", c.URL, path)
}

func ParseCredentials(jsonBlob string) ([]FacilityCredential, error) {
	var creds []FacilityCredential
	err := json.Unmarshal([]byte(jsonBlob), &creds)
	return creds, err
}

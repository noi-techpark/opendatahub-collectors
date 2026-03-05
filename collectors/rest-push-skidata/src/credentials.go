// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import "encoding/json"

type FacilityCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// CredentialsMap maps facilityId to its credentials.
type CredentialsMap map[string]FacilityCredential

func ParseCredentials(jsonBlob string) (CredentialsMap, error) {
	var creds CredentialsMap
	err := json.Unmarshal([]byte(jsonBlob), &creds)
	return creds, err
}

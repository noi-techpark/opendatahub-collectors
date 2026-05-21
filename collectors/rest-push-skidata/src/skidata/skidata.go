// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

// Package skidata holds the shared types and HTTP helpers for talking to the
// Skidata Dynamic Data API. It is consumed by both the rest-push-skidata
// collector (subscription / health-check logic) and the offline
// sync-stations script (one-shot harvesting of counting categories).
package skidata

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
)

// FacilityCredential is one entry from credentials.json — a single Skidata
// facility's basic-auth credentials together with the facility number.
type FacilityCredential struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Facility string `json:"facility"`
}

// CountingCategory is one row of the response of
// GET /bei/advconn/dynamicdata/v1/countingcategories/{facility}.
type CountingCategory struct {
	CarparkId          int    `json:"carparkId"`
	CountingCategoryId int    `json:"countingCategoryId"`
	Name               string `json:"name"`
	Capacity           int    `json:"capacity"`
	OccupancyLimit     int    `json:"occupancyLimit"`
	FreeLimit          int    `json:"freeLimit"`
}

// ParseCredentials parses the JSON blob from the SKIDATA_CREDENTIALS_JSON
// env var (or credentials.json file) into a slice of FacilityCredential.
func ParseCredentials(jsonBlob []byte) ([]FacilityCredential, error) {
	var creds []FacilityCredential
	err := json.Unmarshal(jsonBlob, &creds)
	return creds, err
}

// NewHTTPClient returns a retryable HTTP client suitable for talking to the
// Skidata API (silent logger, default retry/backoff policy).
func NewHTTPClient() *http.Client {
	rc := retryablehttp.NewClient()
	rc.Logger = nil
	return rc.StandardClient()
}

// ApiURL returns the full URL for a Skidata Dynamic Data API call.
func ApiURL(baseURL, path string) string {
	return fmt.Sprintf("%s/bei/advconn/dynamicdata/v1/%s", baseURL, path)
}

// GetCountingCategories fetches the counting categories for a facility
// using the given credential and base URL.
func GetCountingCategories(client *http.Client, baseURL string, cred FacilityCredential) ([]CountingCategory, error) {
	url := ApiURL(baseURL, fmt.Sprintf("countingcategories/%s", cred.Facility))

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cred.Username, cred.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("counting categories returned %d: %s", resp.StatusCode, string(body))
	}

	var categories []CountingCategory
	if err := json.NewDecoder(resp.Body).Decode(&categories); err != nil {
		return nil, fmt.Errorf("failed to decode counting categories: %w", err)
	}
	return categories, nil
}

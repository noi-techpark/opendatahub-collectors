// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/hashicorp/go-retryablehttp"
)

type CountingArea struct {
	CarparkId      int    `json:"carparkId"`
	CountingAreaId int    `json:"countingAreaId"`
	Name           string `json:"name"`
	Capacity       int    `json:"capacity"`
	OccupancyLimit int    `json:"occupancyLimit"`
	FreeLimit      int    `json:"freeLimit"`
}

var httpClient *http.Client

func init() {
	rc := retryablehttp.NewClient()
	rc.Logger = nil
	httpClient = rc.StandardClient()
}

func SubscribeAll(creds CredentialsMap) {
	for facilityId, cred := range creds {
		go subscribeFacility(facilityId, cred)
	}
}

func subscribeFacility(facilityId string, cred FacilityCredential) {
	areas, err := getCountingAreas(facilityId, cred)
	if err != nil {
		slog.Error("Failed to get counting areas", "facilityId", facilityId, "err", err)
		return
	}

	carparkIds := make([]int, 0, len(areas))
	for _, a := range areas {
		carparkIds = append(carparkIds, a.CarparkId)
	}

	slog.Info("Fetched counting areas", "facilityId", facilityId, "carparkIds", carparkIds)

	err = enableNotifications(facilityId, cred, carparkIds)
	if err != nil {
		slog.Error("Failed to enable notifications", "facilityId", facilityId, "err", err)
		return
	}

	slog.Info("Subscribed to push notifications", "facilityId", facilityId)
}

func getCountingAreas(facilityId string, cred FacilityCredential) ([]CountingArea, error) {
	url := fmt.Sprintf("%s/bei/advconn/dynamicdata/v1/countingareas/%s",
		env.SKIDATA_BASE_URL, facilityId)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cred.Username, cred.Password)
	req.Header.Set("Accept", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("counting areas returned %d: %s", resp.StatusCode, string(body))
	}

	var areas []CountingArea
	err = json.NewDecoder(resp.Body).Decode(&areas)
	if err != nil {
		return nil, fmt.Errorf("failed to decode counting areas: %w", err)
	}
	return areas, nil
}

func enableNotifications(facilityId string, cred FacilityCredential, carparkIds []int) error {
	url := fmt.Sprintf("%s/bei/advconn/dynamicdata/v1/notifications/enable/%s",
		env.SKIDATA_BASE_URL, facilityId)

	body, err := json.Marshal(carparkIds)
	if err != nil {
		return fmt.Errorf("failed to marshal carpark ids: %w", err)
	}

	req, err := http.NewRequest(http.MethodPut, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cred.Username, cred.Password)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/text")

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("subscription returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

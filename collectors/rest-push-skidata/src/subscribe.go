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
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"opendatahub.com/rest-push-skidata/skidata"
)

// CountingCategory is re-exported from the shared skidata package so existing
// references in this package keep working.
type CountingCategory = skidata.CountingCategory

var httpClient *http.Client

func init() {
	httpClient = skidata.NewHTTPClient()
}

func SubscribeAll(creds []FacilityCredential) {
	for _, cred := range creds {
		go manageFacility(cred)
	}
}

func manageFacility(cred FacilityCredential) {
	defer tel.FlushOnPanic()

	backoff := time.Second
	for {
		err := healthCheck(cred)
		if err != nil {
			slog.Error("Health check failed", "facility", cred.Facility, "err", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}

		backoff = time.Second
		err = subscribeFacility(cred)
		if err != nil {
			slog.Error("Subscription failed", "facility", cred.Facility, "err", err)
			time.Sleep(backoff)
			backoff = min(backoff*2, 30*time.Second)
			continue
		}

		backoff = time.Second
		slog.Info("Subscribed to push notifications", "facility", cred.Facility)

		// monitoring loop
		for {
			time.Sleep(30 * time.Second)
			err = healthCheck(cred)
			if err != nil {
				slog.Warn("Health check failed, re-subscribing", "facility", cred.Facility, "err", err)
				break
			}
		}
	}
}

func healthCheck(cred FacilityCredential) error {
	url := skidata.ApiURL(env.SKIDATA_BASE_URL, "health")

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.SetBasicAuth(cred.Username, cred.Password)

	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("health check returned %d: %s", resp.StatusCode, string(body))
	}
	return nil
}

func subscribeFacility(cred FacilityCredential) error {
	categories, err := skidata.GetCountingCategories(httpClient, env.SKIDATA_BASE_URL, cred)
	if err != nil {
		return fmt.Errorf("failed to get counting categories: %w", err)
	}

	seen := make(map[int]bool)
	carparkIds := make([]int, 0)
	for _, c := range categories {
		if !seen[c.CarparkId] {
			seen[c.CarparkId] = true
			carparkIds = append(carparkIds, c.CarparkId)
		}
	}

	slog.Info("Fetched counting categories", "facility", cred.Facility, "carparkIds", carparkIds)

	err = enableNotifications(cred, carparkIds)
	if err != nil {
		return fmt.Errorf("failed to enable notifications: %w", err)
	}
	return nil
}

func enableNotifications(cred FacilityCredential, carparkIds []int) error {
	url := skidata.ApiURL(env.SKIDATA_BASE_URL, fmt.Sprintf("notifications/enable/%s", cred.Facility))

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

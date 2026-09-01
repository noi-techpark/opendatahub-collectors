// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"sync"
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

// runFacility is everything one facility needs running, under one context.
//
// The subscription and the category publication are deliberately separate loops
// -- resubscription only happens when a health check fails, which can be days
// apart, so hanging publication off it would let a capacity change sit
// unpublished indefinitely. The supervisor cancels both together, and this
// returns only once both have unwound, so a restarted facility never briefly
// has two of either.
func runFacility(ctx context.Context, cred FacilityCredential) {
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); manageFacility(ctx, cred) }()
	go func() { defer wg.Done(); refreshCategories(ctx, cred) }()
	wg.Wait()
}

// pause sleeps unless ctx ends first, and reports whether to carry on.
//
// Every wait in this file goes through it. A bare time.Sleep is what made a
// credential change require a pod restart: the goroutine could not be told to
// stop, so the only way to stop it was to end the process.
func pause(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func manageFacility(ctx context.Context, cred FacilityCredential) {
	defer tel.FlushOnPanic()

	backoff := time.Second
	for ctx.Err() == nil {
		err := healthCheck(ctx, cred)
		if err != nil {
			slog.Error("Health check failed", "facility", cred.Facility, "err", err)
			if !pause(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}

		backoff = time.Second
		err = subscribeFacility(ctx, cred)
		if err != nil {
			slog.Error("Subscription failed", "facility", cred.Facility, "err", err)
			if !pause(ctx, backoff) {
				return
			}
			backoff = min(backoff*2, 30*time.Second)
			continue
		}

		backoff = time.Second
		slog.Info("Subscribed to push notifications", "facility", cred.Facility)

		// monitoring loop
		for {
			if !pause(ctx, 30*time.Second) {
				return
			}
			if err = healthCheck(ctx, cred); err != nil {
				slog.Warn("Health check failed, re-subscribing", "facility", cred.Facility, "err", err)
				break
			}
		}
	}
}

func healthCheck(ctx context.Context, cred FacilityCredential) error {
	url := skidata.ApiURL(env.SKIDATA_BASE_URL, "health")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
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

func subscribeFacility(ctx context.Context, cred FacilityCredential) error {
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

	err = enableNotifications(ctx, cred, carparkIds)
	if err != nil {
		return fmt.Errorf("failed to enable notifications: %w", err)
	}
	return nil
}

func enableNotifications(ctx context.Context, cred FacilityCredential, carparkIds []int) error {
	url := skidata.ApiURL(env.SKIDATA_BASE_URL, fmt.Sprintf("notifications/enable/%s", cred.Facility))

	body, err := json.Marshal(carparkIds)
	if err != nil {
		return fmt.Errorf("failed to marshal carpark ids: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(body))
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

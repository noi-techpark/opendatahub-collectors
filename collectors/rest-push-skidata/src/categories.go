// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"opendatahub.com/rest-push-skidata/skidata"
)

// Counting categories are a second collection flow, alongside the push events.
//
// The subscription path has always fetched them — it needs the carpark ids to
// enable notifications — and then thrown the rest away. They are the only
// description the provider gives of what a facility contains: its carparks, and
// per counting category the capacity and limits. Downstream that is the
// facility→carpark topology and the total capacity, neither of which can be
// derived from a push event, since an event only ever describes one carpark.
//
// They are published keyed by facility so a consumer can read the current set
// for every facility in one request, instead of replaying the collection.
// One document carries one facility's complete category list: a partial
// document would silently look like a facility that lost a carpark.

// metaKeyFacility is the meta field the documents are keyed by. It has to match
// what the consumer asks the bridge to group on.
const metaKeyFacility = "facility"

// refreshCategories publishes one facility's counting categories immediately,
// then again on every tick.
//
// It is deliberately independent of the subscription loop. Resubscription only
// happens when a health check fails, which can be days apart, so hanging
// publication off it would let a capacity change or a new carpark sit
// unpublished indefinitely.
func refreshCategories(ctx context.Context, cred FacilityCredential) {
	defer tel.FlushOnPanic()

	every := env.SKIDATA_CATEGORY_REFRESH
	if every <= 0 {
		slog.Info("counting category refresh disabled", "facility", cred.Facility)
		return
	}

	publish := func() {
		if err := fetchAndPublishCategories(ctx, cred); err != nil {
			slog.Error("failed publishing counting categories",
				"facility", cred.Facility, "err", err)
			return
		}
	}

	publish()

	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publish()
		}
	}
}

// fetchAndPublishCategories reads the current categories for a facility and
// writes them to the raw data lake as one document.
func fetchAndPublishCategories(ctx context.Context, cred FacilityCredential) error {
	cats, err := skidata.GetCountingCategories(httpClient, env.SKIDATA_BASE_URL, cred)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	// An empty list is not a facility with no carparks, it is a facility we
	// failed to describe. Publishing it would erase a real topology.
	if len(cats) == 0 {
		return fmt.Errorf("provider returned no counting categories; refusing to publish an empty topology")
	}

	body, err := json.Marshal(cats)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	ctx, collection := dc.NewCollection(ctx, env.RAW_WRITER_URL, env.SKIDATA_CATEGORIES_PROVIDER)
	defer collection.End(ctx)

	if err := collection.Publish(ctx, &rdb.RawAny{
		Provider:    env.SKIDATA_CATEGORIES_PROVIDER,
		Timestamp:   time.Now(),
		Rawdata:     body,
		ContentType: "application/json",
		Meta:        map[string]string{metaKeyFacility: cred.Facility},
	}); err != nil {
		return fmt.Errorf("publish: %w", err)
	}

	slog.Info("published counting categories",
		"facility", cred.Facility, "categories", len(cats), "carparks", countCarparks(cats))
	return nil
}

// countCarparks reports how many distinct carparks the categories describe.
func countCarparks(cats []skidata.CountingCategory) int {
	seen := map[int]bool{}
	for _, c := range cats {
		seen[c.CarparkId] = true
	}
	return len(seen)
}

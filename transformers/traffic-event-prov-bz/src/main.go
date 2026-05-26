// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"
)

const (
	SOURCE            = "PROVINCE_BZ"
	ID_TEMPLATE       = "urn:announcements:provincebz"
	PROVIDER_TIMEZONE = "Europe/Rome"
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string
}

var tags clib.TagDefs
var contentClient clib.ContentAPI
var annCache *clib.Cache[odhContentModel.Announcement]
var location *time.Location

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting traffic-event-prov-bz transformer...")

	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error
	location, err = time.LoadLocation(PROVIDER_TIMEZONE)
	ms.FailOnError(context.Background(), err, "failed to load timezone")

	contentClient, err = clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create client")

	annCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhContentModel.Announcement]{
		EntityType:  "Announcement",
		QueryParams: map[string]string{"active": "true", "source": SOURCE, "rawfilter": "isnotnull(Mapping.ProviderProvinceBz.Id)"},
		IDFunc:      func(a odhContentModel.Announcement) string { return *a.ID },
	})
	ms.FailOnError(context.Background(), err, "failed to load announcements")

	// Post-filter: drop already-ended announcements (EndTime <= SyncTime) so
	// they are not held in the cache and re-evaluated on every batch.
	for id, entry := range annCache.Entries() {
		if entry.Entity.EndTime != nil {
			syncTime := entry.Entity.Mapping.ProviderProvinceBz.SyncTime.Truncate(time.Millisecond)
			endTime := entry.Entity.EndTime.Truncate(time.Millisecond)
			if endTime.Before(syncTime) || endTime.Equal(syncTime) {
				annCache.Delete(id)
			}
		}
	}

	tags, err = clib.ReadTagDefs("../resources/tags.json")
	ms.FailOnError(context.Background(), err, "failed to read tags")

	err = clib.SyncTags(context.Background(), contentClient, tags, clib.SyncTagsConfig{
		Source: "announcement",
	})
	ms.FailOnError(context.Background(), err, "failed to sync tags")

	listener := tr.NewTr[string](context.Background(), env.Env)

	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, r *rdb.Raw[[]dto.TrafficEvent]) error {
	logger.Get(ctx).Info("Processing announcements", "count", len(r.Rawdata))

	// Content can't handle nanoseconds; truncate so SyncTime and EndTime
	// share the same precision when compared.
	sourceTime := r.Timestamp.Truncate(time.Millisecond)

	seen := map[string]struct{}{}
	var list []odhContentModel.Announcement

	for _, a := range r.Rawdata {
		id := generateID(a)
		existing, exists := annCache.Get(id)

		ann, err := MapTrafficEventToAnnouncement(tags, a, id)
		if err != nil {
			// Skip a single malformed event instead of failing the whole
			// batch (which would otherwise stall the queue indefinitely).
			logger.Get(ctx).Warn("Failed to map event, skipping", "messageId", a.MessageID.String(), "error", err)
			continue
		}
		ann.Mapping.ProviderProvinceBz.SyncTime = sourceTime

		// Preserve the original start time once it has been established.
		if exists {
			ann.StartTime = existing.Entity.StartTime
		}

		hash, changed, err := annCache.HasChanged(id, ann)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash announcement", "messageId", a.MessageID.String(), "error", err)
			continue
		}

		if changed {
			annCache.Set(id, ann, hash)
			list = append(list, ann)
		}

		seen[id] = struct{}{}
	}

	// Detect ended announcements: cached entries not present in this batch.
	for id, entry := range annCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}
		ann := entry.Entity
		ann.EndTime = &sourceTime
		ann.Mapping.ProviderProvinceBz.SyncTime = sourceTime
		list = append(list, ann)
		annCache.Delete(id)
	}

	logger.Get(ctx).Info("Proceeding updating changed announcements", "count", len(list))

	if len(list) == 0 {
		return nil
	}

	if err := contentClient.PutMultiple(ctx, "Announcement", list); err != nil {
		return fmt.Errorf("failed to update announcements: %w", err)
	}
	return nil
}

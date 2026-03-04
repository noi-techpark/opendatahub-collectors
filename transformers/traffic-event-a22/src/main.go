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
	"opendatahub.com/tr-traffic-event-a22/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22/odh-content-model"
)

const (
	SOURCE      = "A22"
	ID_TEMPLATE = "urn:announcements:a22"
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
var timeNow = time.Now

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting A22 traffic event transformer...")

	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error

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
		QueryParams: map[string]string{"active": "true", "source": SOURCE, "rawfilter": "isnotnull(Mapping.ProviderA22.Id)"},
		IDFunc:      func(a odhContentModel.Announcement) string { return *a.ID },
	})
	ms.FailOnError(context.Background(), err, "failed to load announcements")

	// Post-filter: remove entries where EndTime <= SyncTime
	for id, entry := range annCache.Entries() {
		if entry.Entity.EndTime != nil {
			syncTime := entry.Entity.Mapping.ProviderA22.SyncTime.Truncate(time.Millisecond)
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

func Transform(ctx context.Context, r *rdb.Raw[[]dto.A22Event]) error {
	logger.Get(ctx).Info("Processing A22 events", "count", len(r.Rawdata))

	// content can't handle nanoseconds, truncate to milliseconds
	sourceTime := r.Timestamp.Truncate(time.Millisecond)

	seen := map[string]struct{}{}
	list := []odhContentModel.Announcement{}

	for _, event := range r.Rawdata {
		id := generateID(event)
		existing, exists := annCache.Get(id)

		ann, err := MapA22EventToAnnouncement(tags, event, id)
		if err != nil {
			logger.Get(ctx).Warn("Failed to map A22 event, skipping", "event_id", event.Id, "error", err)
			continue
		}
		ann.Mapping.ProviderA22.SyncTime = sourceTime

		// Preserve existing start time if we've seen this event before
		if exists {
			ann.StartTime = existing.Entity.StartTime
		}

		// Hash to detect changes
		hash, changed, err := annCache.HasChanged(id, ann)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash announcement", "error", err)
			continue
		}

		if changed {
			annCache.Set(id, ann, hash)
			list = append(list, ann)
		}

		seen[id] = struct{}{}
	}

	// Detect ended events: cached announcements not in current batch
	for id, entry := range annCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}

		// Set end time and add to sync list
		ann := entry.Entity
		ann.EndTime = &sourceTime
		ann.Mapping.ProviderA22.SyncTime = sourceTime
		list = append(list, ann)
		annCache.Delete(id)
	}

	logger.Get(ctx).Info("Updating changed announcements", "count", len(list))

	if len(list) == 0 {
		return nil
	}

	err := contentClient.PutMultiple(ctx, "Announcement", list)
	if err != nil {
		return fmt.Errorf("failed to update announcements: %w", err)
	}
	return nil
}

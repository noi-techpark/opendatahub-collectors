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
	"opendatahub.com/tr-traffic-event-a22-opendata/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-a22-opendata/odh-content-model"
)

const (
	SOURCE      = "a22"
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
var rd *roadData
var timeNow = time.Now

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting A22 opendata traffic event transformer...")

	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error

	rd, err = LoadRoad("../resources/a22_road.json")
	ms.FailOnError(context.Background(), err, "failed to load road data")

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
		QueryParams: map[string]string{"active": "true", "source": SOURCE, "rawfilter": "isnotnull(Mapping.ProviderA22Open.Id)"},
		IDFunc:      func(a odhContentModel.Announcement) string { return *a.ID },
	})
	ms.FailOnError(context.Background(), err, "failed to load announcements")

	// Post-filter: remove entries where EndTime <= SyncTime
	for id, entry := range annCache.Entries() {
		if entry.Entity.EndTime != nil {
			syncTime := entry.Entity.Mapping.ProviderA22Open.SyncTime.Truncate(time.Millisecond)
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

type mapperFunc func(*roadData, dto.A22OpendataEvent) (odhContentModel.Announcement, error)

func Transform(ctx context.Context, r *rdb.Raw[dto.Root]) error {
	logger.Get(ctx).Info("Processing A22 opendata events",
		"roadworks", len(r.Rawdata.RoadWorks), "traffic", len(r.Rawdata.Traffic))

	sourceTime := r.Timestamp.Truncate(time.Millisecond)

	seen := map[string]struct{}{}
	var list []odhContentModel.Announcement

	processEvents := func(events []dto.A22OpendataEvent, mapper mapperFunc) {
		for _, event := range events {
			id := generateOpendataID(event)
			existing, exists := annCache.Get(id)

			ann, err := mapper(rd, event)
			if err != nil {
				logger.Get(ctx).Warn("Failed to map event, skipping", "id", event.IDNotizia, "error", err)
				continue
			}
			ann.Mapping.ProviderA22Open.SyncTime = sourceTime

			if exists {
				ann.StartTime = existing.Entity.StartTime
			}

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
	}

	processEvents(r.Rawdata.RoadWorks, MapLavoriToAnnouncement)
	processEvents(r.Rawdata.Traffic, MapTrafficoToAnnouncement)

	// Detect ended events: cached announcements not in current batch
	for id, entry := range annCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}

		ann := entry.Entity
		if ann.EndTime != nil {
			ann.EndTime = &sourceTime
		}
		ann.Mapping.ProviderA22Open.SyncTime = sourceTime
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

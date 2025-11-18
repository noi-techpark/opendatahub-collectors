// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/mitchellh/hashstructure/v2"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentClient "opendatahub.com/tr-traffic-event-prov-bz/odh-content-client"
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

type announcementCache struct {
	ann  odhContentModel.Announcement
	hash uint64
}

var tags Tags
var contentClient *odhContentClient.ContentClient
var annCache map[string]announcementCache
var location *time.Location

func syncTags(ctx context.Context, contentClient *odhContentClient.ContentClient, tags *Tags) error {
	for _, tag := range *tags {
		types := []string{"announcement"}
		if !strings.Contains(tag.ID, "announcement:") {
			types = append(types, "traffic-event")
		}
		err := contentClient.Post(ctx, "Tag", map[string]string{"generateid": "false"}, &odhContentModel.Tag{
			ID:     StringPtr(tag.ID),
			Source: "announcement",
			TagName: map[string]string{
				"it": tag.NameIt,
				"de": tag.NameDe,
				"en": tag.NameEn,
			},
			Types: types,
			LicenseInfo: &odhContentModel.LicenseInfo{
				ClosedData: false,
				License:    StringPtr("CC0"),
			},
		})
		if err != nil && !errors.Is(err, odhContentClient.ErrAlreadyExists) {
			return err
		}
	}
	return nil
}

func loadExistingAnnouncements(ctx context.Context, contentClient *odhContentClient.ContentClient) (map[string]announcementCache, error) {
	cache := map[string]announcementCache{}

	type response struct {
		Items       []odhContentModel.Announcement `json:"Items"`
		TotalPages  int                            `json:"TotalPages"`
		CurrentPage int                            `json:"CurrentPage"`
	}

	currentPage := 1
	totalPage := 99
	for currentPage <= totalPage {
		res := response{}

		err := contentClient.Get(ctx,
			"Announcement",
			map[string]string{
				"active":     "true",
				"source":     SOURCE,
				"pageSize":   "200",
				"pagenumber": fmt.Sprintf("%d", currentPage),
			},
			&res,
		)
		if err != nil {
			return nil, err
		}

		for _, ann := range res.Items {
			// discard all announcements which have an endtime before or equal SyncTime,
			// This prune announcements which are likely to be already ended,
			// we keep only the ones with end null (meaning we haven't find the end call)
			// or end in the future (meaning that the end date was set by the provider but there might be updates).

			// lower time precision when checking for "ended", to much precision could lead to events already ended passing the check
			syncTime := ann.Mapping.ProviderProvinceBz.SyncTime.Truncate(time.Millisecond)
			if ann.EndTime != nil {
				endTime := ann.EndTime.Truncate(time.Millisecond)
				if endTime.Before(syncTime) || endTime.Equal(syncTime) {
					continue

				}
			}

			hash, err := hashstructure.Hash(ann, hashstructure.FormatV2, nil)
			if err != nil {
				return nil, fmt.Errorf("could not hash announcement :%v", err)
			}

			cache[*ann.ID] = announcementCache{
				ann:  ann,
				hash: hash,
			}
		}

		currentPage += 1
		totalPage = res.TotalPages
	}

	for id, c := range cache {
		if id != *c.ann.ID {
			logger.Get(ctx).Info("cache id differs from ann id", "id", id, "ann_id", *c.ann.ID)
		}
	}
	return cache, nil
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting data transformer...")

	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error
	location, err = time.LoadLocation(PROVIDER_TIMEZONE)
	ms.FailOnError(context.Background(), err, "failed to load timezone")

	contentClient, err = odhContentClient.NewContentClient(odhContentClient.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create client")

	annCache, err = loadExistingAnnouncements(context.Background(), contentClient)
	ms.FailOnError(context.Background(), err, "failed to load announcements")

	tags, err = ReadTags("../resources/tags.json")
	ms.FailOnError(context.Background(), err, "failed to read tags")

	err = syncTags(context.Background(), contentClient, &tags)
	ms.FailOnError(context.Background(), err, "failed to sync tag")

	listener := tr.NewTr[string](context.Background(), env.Env)

	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// Helper function to marshal data to JSON and write it to a file
func dumpToJsonFile(data interface{}, prefix string) {
	// Marshal the struct to pretty-printed JSON
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		log.Printf("Error marshaling %s to JSON: %v", prefix, err)
		return
	}

	// Create a unique filename (e.g., "old_announcement_1740924800.json")
	filename := fmt.Sprintf("%s_%d.json", prefix, time.Now().Unix())

	// Write JSON bytes to the file
	err = os.WriteFile(filename, jsonBytes, 0644)
	if err != nil {
		log.Printf("Error writing JSON to file %s: %v", filename, err)
	} else {
		// You would typically see this message in your application logs
		log.Printf("Successfully dumped %s data to %s", prefix, filename)
	}
}

func Transform(ctx context.Context, r *rdb.Raw[[]dto.TrafficEvent]) error {
	logger.Get(ctx).Info("Processing announcements", "count", len(r.Rawdata))

	// content can't handle nanoseconds, therefore we have to truncate to milliseconds to avoid
	// syncTime to have nanoseconds while end time do not
	sourceTime := r.Timestamp.Truncate(time.Millisecond)

	seen := map[string]interface{}{}
	list := []odhContentModel.Announcement{}

	for _, a := range r.Rawdata {
		id := generateID(a)
		existingAnnouncement, exists := annCache[id]

		ann, err := MapTrafficEventToAnnouncement(tags, a, id)
		if err != nil {
			if errors.Is(err, ErrWithoutGeometry) {
				continue
			} else {
				ms.FailOnError(ctx, err, "failed to map announcement")
			}
		}
		ann.Mapping.ProviderProvinceBz.SyncTime = sourceTime

		// inject existing's starting date, if any
		if exists {
			ann.StartTime = existingAnnouncement.ann.StartTime
		}

		// hash to understand if we should update or skip the announcement since it had no changes
		hash, err := hashstructure.Hash(ann, hashstructure.FormatV2, nil)
		ms.FailOnError(ctx, err, "failed to hash announcement")

		if exists {
			if hash != existingAnnouncement.hash {
				// // DUMP OLD (existingAnnouncement.ann) before the object is overwritten
				// dumpToJsonFile(existingAnnouncement.ann, "old_announcement")

				// // DUMP NEW (ann)
				// dumpToJsonFile(ann, "new_announcement")

				annCache[id] = announcementCache{
					ann:  ann,
					hash: hash,
				}

				list = append(list, ann)
			}
		} else {
			// newly seen, add to cache and list
			annCache[id] = announcementCache{
				ann:  ann,
				hash: hash,
			}
			list = append(list, ann)
		}

		if id != *ann.ID {
			logger.Get(ctx).Info("mapped id differs from ann id", "id", id, "ann_id", *ann.ID)
		}

		seen[id] = nil
	}

	// check for "ended" announcements
	for id, c := range annCache {
		if id != *c.ann.ID {
			logger.Get(ctx).Info("cache id differs from ann id", "id", id, "ann_id", *c.ann.ID)
		}
		if _, ok := seen[id]; ok {
			continue
		}

		// override endtime
		c.ann.EndTime = &sourceTime
		// Update sync time
		c.ann.Mapping.ProviderProvinceBz.SyncTime = sourceTime
		// add to sync
		list = append(list, c.ann)
		// remove from cache
		delete(annCache, id)
	}

	logger.Get(ctx).Info("Proceeding updating changed announcements", "count", len(list))

	if len(list) == 0 {
		return nil
	}

	err := contentClient.PutMultiple(ctx, "Announcement", list)
	ms.FailOnError(ctx, err, "failed to transform events")
	return nil
}

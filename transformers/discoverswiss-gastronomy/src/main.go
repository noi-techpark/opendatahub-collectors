// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"opendatahub.com/tr-discoverswiss-gastronomy/dto"
	odhContentModel "opendatahub.com/tr-discoverswiss-gastronomy/odh-content-model"
)

const (
	tagDefsPath            = "../resources/tags.json"
	defaultDeactivateAfter = 12 * time.Hour
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string
	ODH_CORE_REFERER             string
	DEACTIVATE_AFTER             string
}

var (
	contentClient   clib.ContentAPI
	poiCache        *clib.Cache[odhContentModel.ODHActivityPoi]
	tagCatalog      clib.TagDefs
	deactivateAfter = defaultDeactivateAfter
)

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	defer tel.FlushOnPanic()

	var err error
	tagCatalog, err = clib.ReadTagDefs(tagDefsPath)
	ms.FailOnError(context.Background(), err, "failed to read gastronomy tags")

	if env.DEACTIVATE_AFTER != "" {
		deactivateAfter, err = time.ParseDuration(env.DEACTIVATE_AFTER)
		ms.FailOnError(context.Background(), err, "invalid DEACTIVATE_AFTER")
	}

	if env.ODH_CORE_URL != "" {
		client, err := clib.NewContentClient(clib.Config{
			BaseURL:      env.ODH_CORE_URL,
			TokenURL:     env.ODH_CORE_TOKEN_URL,
			ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
			ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
			DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
		}, clib.WithReferer(env.ODH_CORE_REFERER))
		ms.FailOnError(context.Background(), err, "failed to create content client")
		contentClient = client

		if err := clib.SyncTags(context.Background(), contentClient, tagCatalog, clib.SyncTagsConfig{Source: source}); err != nil {
			ms.FailOnError(context.Background(), err, "failed to sync gastronomy tags")
		}

		poiCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhContentModel.ODHActivityPoi]{
			EntityType: "ODHActivityPoi",
			QueryParams: map[string]string{
				"source":    source,
				"tagfilter": gastronomyTag,
			},
			IDFunc: func(p odhContentModel.ODHActivityPoi) string { return mappingID(p.Mapping) },
		})
		ms.FailOnError(context.Background(), err, "failed to load gastronomy POIs")
	} else {
		slog.Info("ODH_CORE_URL not set, starting with an empty cache")
		poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	}

	if len(os.Args) > 1 {
		if os.Args[1] == "--stdin" {
			runStdin()
			return
		}
		runCLI(os.Args[1:])
		return
	}

	slog.Info("Starting gastronomy transformer...")
	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// runStdin reads mongoexport lines ({"rawdata":"<one gastronomy>"}) and imports each one.
func runStdin() {
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	processed := 0
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 || line[0] != '{' {
			continue
		}
		var row struct {
			Rawdata string `json:"rawdata"`
		}
		if err := json.Unmarshal(line, &row); err != nil || row.Rawdata == "" {
			slog.Info("Skipping raw line", "error", err)
			continue
		}
		var raw dto.Gastronomy
		if err := json.Unmarshal([]byte(row.Rawdata), &raw); err != nil {
			slog.Info("Skipping gastronomy that is not valid JSON", "error", err)
			continue
		}
		var err error
		for attempt := 1; attempt <= 3; attempt++ {
			err = processRaw(ctx, raw, now)
			if err == nil {
				break
			}
			slog.Info("Retrying gastronomy after a write failure", "id", raw.Identifier, "lang", raw.ApiCrawlerLang, "attempt", attempt, "error", err)
			time.Sleep(time.Duration(attempt) * 15 * time.Second)
		}
		if err != nil {
			ms.FailOnError(ctx, err, fmt.Sprintf("failed to process %s %s", raw.Identifier, raw.ApiCrawlerLang))
		}
		processed++
		if processed%500 == 0 {
			slog.Info("Import progress", "processed", processed, "id", raw.Identifier, "lang", raw.ApiCrawlerLang)
		}
	}
	ms.FailOnError(ctx, scanner.Err(), "failed while reading raw gastronomies")
	slog.Info("Import finished", "processed", processed)
}

func runCLI(files []string) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)
	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		ms.FailOnError(ctx, err, fmt.Sprintf("failed to read %s", filePath))

		var raw dto.Gastronomy
		err = json.Unmarshal(data, &raw)
		ms.FailOnError(ctx, err, fmt.Sprintf("failed to unmarshal %s", filePath))

		slog.Info("Processing file", "file", filePath, "id", raw.Identifier, "lang", raw.ApiCrawlerLang)
		err = processRaw(ctx, raw, now)
		ms.FailOnError(ctx, err, fmt.Sprintf("failed to process %s", filePath))
	}
}

// Transform handles one raw gastronomy message. A bad item is logged and skipped.
// A write failure is returned so the message is retried.
func Transform(ctx context.Context, r *rdb.Raw[dto.Gastronomy]) error {
	if err := processRaw(ctx, r.Rawdata, r.Timestamp.Truncate(time.Millisecond)); err != nil {
		return fmt.Errorf("process gastronomy: %w", err)
	}
	return nil
}

func processRaw(ctx context.Context, raw dto.Gastronomy, sourceTime time.Time) error {
	if raw.Identifier == "" {
		slog.Info("Skipping gastronomy with no identifier")
		return nil
	}
	if !languageAvailable(raw.AvailableDataLanguage, raw.ApiCrawlerLang) {
		slog.Info("Skipping language that is not published for this gastronomy", "id", raw.Identifier, "lang", raw.ApiCrawlerLang)
		return nil
	}

	poi, err := mapGastronomy(raw, tagCatalog)
	if err != nil {
		slog.Info("Skipping gastronomy that could not be mapped", "id", raw.Identifier, "error", err)
		return nil
	}

	id := mappingID(poi.Mapping)
	setMappingSyncTime(poi.Mapping, sourceTime)

	existing, exists := poiCache.Get(id)
	if exists {
		MergePOI(&existing.Entity, poi)
	} else {
		existing = clib.CacheEntry[odhContentModel.ODHActivityPoi]{Entity: poi}
	}

	hash, changed, err := poiCache.HasChanged(id, existing.Entity)
	if err != nil {
		return fmt.Errorf("hash gastronomy: %w", err)
	}
	if changed && contentClient != nil && existing.Entity.ID != nil {
		if err := uploadWithOwnID(ctx, "ODHActivityPoi", *existing.Entity.ID, existing.Entity); err != nil {
			return err
		}
	}
	poiCache.Set(id, existing.Entity, hash)
	return deactivateStale(ctx, id, sourceTime)
}

func mappingID(mapping map[string]map[string]string) string {
	if m, ok := mapping["discoverswiss"]; ok {
		return m["id"]
	}
	return ""
}

func setMappingSyncTime(mapping map[string]map[string]string, t time.Time) {
	if m, ok := mapping["discoverswiss"]; ok {
		m["synctime"] = t.Format(time.RFC3339Nano)
	}
}

func mappingSyncTime(mapping map[string]map[string]string) (time.Time, bool) {
	raw := mappingIDTime(mapping)
	if raw == "" {
		return time.Time{}, false
	}
	parsed, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return parsed, true
}

func mappingIDTime(mapping map[string]map[string]string) string {
	if m, ok := mapping["discoverswiss"]; ok {
		return m["synctime"]
	}
	return ""
}

func uploadWithOwnID(ctx context.Context, apiPath, id string, payload interface{}) error {
	err := contentClient.Put(ctx, apiPath, id, payload)
	if err == nil {
		slog.Info("Updated gastronomy", "id", id)
		return nil
	}
	if errors.Is(err, clib.ErrNoDataToUpdate) || isNotFound(err) {
		slog.Info("Creating gastronomy", "id", id)
		return contentClient.Post(ctx, apiPath, map[string]string{"generateid": "false"}, payload)
	}
	return fmt.Errorf("PUT %s/%s: %w", apiPath, id, err)
}

func isNotFound(err error) bool {
	var apiErr *clib.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}

// deactivateStale marks gastronomies whose last successful sync is older than
// DEACTIVATE_AFTER. Streaming publishes one restaurant at a time, so a missing
// item cannot be detected by comparing one message with the full list.
func deactivateStale(ctx context.Context, keep string, now time.Time) error {
	if deactivateAfter <= 0 {
		return nil
	}
	cutoff := now.Add(-deactivateAfter)
	for id, entry := range poiCache.Entries() {
		if id == "" || id == keep || !entry.Entity.Active {
			continue
		}
		synced, ok := mappingSyncTime(entry.Entity.Mapping)
		if !ok || !synced.Before(cutoff) {
			continue
		}
		entry.Entity.Active = false
		entry.Entity.SmgActive = boolPtr(false)
		entry.Entity.OdhActive = boolPtr(false)
		if contentClient != nil && entry.Entity.ID != nil {
			if err := uploadWithOwnID(ctx, "ODHActivityPoi", *entry.Entity.ID, entry.Entity); err != nil {
				return fmt.Errorf("deactivate gastronomy %s: %w", id, err)
			}
		}
		hash, _, err := poiCache.HasChanged(id, entry.Entity)
		if err != nil {
			return fmt.Errorf("hash deactivated gastronomy: %w", err)
		}
		poiCache.Set(id, entry.Entity, hash)
		slog.Info("Deactivated gastronomy that disappeared from DiscoverSwiss", "id", id, "lastSync", synced)
	}
	return nil
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
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
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"opendatahub.com/tr-discoverswiss-skiarea/dto"
	odhContentModel "opendatahub.com/tr-discoverswiss-skiarea/odh-content-model"

	_ "time/tzdata"
)

// start
const (
	SOURCE            = "discoverswiss"
	SKIAREA_ID_PREFIX = "urn:skiarea:discoverswiss"
	PROVIDER_TIMEZONE = "Europe/Rome"
)

func generateID(raw dto.SkiArea) string {
	return fmt.Sprintf("%s:%s:%s", SKIAREA_ID_PREFIX, cleanType(raw.Type), raw.Identifier)
}

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string
}

var contentClient clib.ContentAPI
var skiAreaCache *clib.Cache[odhContentModel.SkiArea]
var poiCache *clib.Cache[odhContentModel.ODHActivityPoi]
var mpCache *clib.Cache[odhContentModel.MeasuringpointV2]
var location *time.Location

func main() {
	ms.InitWithEnv(context.Background(), "", &env)

	defer tel.FlushOnPanic()

	var err error
	location, err = time.LoadLocation(PROVIDER_TIMEZONE)
	ms.FailOnError(context.Background(), err, "failed to load timezone")

	client, err := clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create client")
	contentClient = client

	// Load existing entities from ODH API (skip if no URL configured, e.g. dry-run CLI)
	if env.ODH_CORE_URL != "" {
		slog.Info("Loading existing entities...", "url", env.ODH_CORE_URL)

		sourceFilter := map[string]string{
			"active": "true",
			"source": SOURCE,
		}

		skiAreaCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhContentModel.SkiArea]{
			EntityType:  "SkiArea",
			QueryParams: sourceFilter,
			IDFunc:      func(s odhContentModel.SkiArea) string { return getMappingId(s.Mapping) },
		})
		ms.FailOnError(context.Background(), err, "failed to load ski areas")

		poiCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhContentModel.ODHActivityPoi]{
			EntityType:  "ODHActivityPoi",
			QueryParams: sourceFilter,
			IDFunc:      func(p odhContentModel.ODHActivityPoi) string { return getMappingId(p.Mapping) },
		})
		ms.FailOnError(context.Background(), err, "failed to load POIs")

		mpCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhContentModel.MeasuringpointV2]{
			EntityType:  "Weather/Measuringpoint",
			QueryParams: sourceFilter,
			IDFunc:      func(m odhContentModel.MeasuringpointV2) string { return getMappingId(m.Mapping) },
		})
		ms.FailOnError(context.Background(), err, "failed to load measuringpoints")
	} else {
		slog.Info("ODH_CORE_URL not set, starting with empty cache")
		skiAreaCache = clib.NewCache[odhContentModel.SkiArea]()
		poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
		mpCache = clib.NewCache[odhContentModel.MeasuringpointV2]()
	}

	// CLI mode: go run . file1.json [file2.json ...]
	if len(os.Args) > 1 {
		runCLI(os.Args[1:])
		return
	}

	// Normal mode: listen on RabbitMQ
	slog.Info("Starting data transformer...")

	listener := tr.NewTr[string](context.Background(), env.Env)

	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// runCLI reads JSON files and feeds each through processRaw (same flow as RabbitMQ).
func runCLI(files []string) {
	ctx := context.Background()
	now := time.Now().Truncate(time.Millisecond)

	for _, filePath := range files {
		data, err := os.ReadFile(filePath)
		ms.FailOnError(ctx, err, fmt.Sprintf("failed to read %s", filePath))

		var raw dto.SkiArea
		err = json.Unmarshal(data, &raw)
		ms.FailOnError(ctx, err, fmt.Sprintf("failed to unmarshal %s", filePath))

		slog.Info("Processing file", "file", filePath, "id", generateID(raw), "lang", raw.ApiCrawlerLang)

		err = processRaw(ctx, raw, now)
		ms.FailOnError(ctx, err, fmt.Sprintf("failed to process %s", filePath))
	}
}

func getMappingId(mapping map[string]map[string]string) string {
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

// uploadWithOwnId uses the payload's own Id for PUT/POST with generateid=false.
// No lookup needed — the entity is identified by our own ID (e.g. urn:slope:discoverswiss:...).
func uploadWithOwnId(ctx context.Context, apiPath string, id string, payload interface{}) error {
	err := contentClient.Put(ctx, apiPath, id, payload)
	if err == nil {
		slog.Info("Updated entity", "apiPath", apiPath, "id", id)
		return nil
	}
	// clib returns ErrNoDataToUpdate for body "Data to update Not Found",
	// but the API may also return 404 — handle both cases.
	if errors.Is(err, clib.ErrNoDataToUpdate) || isNotFoundError(err) {
		slog.Info("Creating entity", "apiPath", apiPath, "id", id)
		return contentClient.Post(ctx, apiPath, map[string]string{"generateid": "false"}, payload)
	}
	return fmt.Errorf("PUT %s/%s: %w", apiPath, id, err)
}

// isNotFoundError checks if the error is a 404 APIError from clib.
func isNotFoundError(err error) bool {
	var apiErr *clib.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == 404
	}
	return false
}

// processRaw is the shared core: transform, merge into cache, upload if changed.
// Used by both CLI mode and RabbitMQ mode.
func processRaw(ctx context.Context, raw dto.SkiArea, sourceTime time.Time) error {
	id := generateID(raw)
	lang := raw.ApiCrawlerLang

	// Skip if the current language is not in availableDataLanguage
	if !isLanguageAvailable(raw.AvailableDataLanguage, lang) {
		slog.Info("Skipping: language not in availableDataLanguage", "id", id, "lang", lang, "available", raw.AvailableDataLanguage)
		return nil
	}

	result, err := TransformSkiArea(raw, id, lang)
	if err != nil {
		return fmt.Errorf("transform ski area: %w", err)
	}

	setMappingSyncTime(result.SkiArea.Mapping, sourceTime)

	// SkiArea merge + change detection
	existing, exists := skiAreaCache.Get(raw.Identifier)
	if exists {
		MergeSkiArea(&existing.Entity, result.SkiArea)
	} else {
		existing = clib.CacheEntry[odhContentModel.SkiArea]{Entity: result.SkiArea}
	}

	hash, changed, err := skiAreaCache.HasChanged(raw.Identifier, existing.Entity)
	if err != nil {
		return fmt.Errorf("hash ski area: %w", err)
	}
	if changed {
		skiAreaCache.Set(raw.Identifier, existing.Entity, hash)
	}

	// POI merge + change detection
	var changedPOIs []odhContentModel.ODHActivityPoi
	for _, poi := range result.POI {
		mappingId := getMappingId(poi.Mapping)
		setMappingSyncTime(poi.Mapping, sourceTime)

		existingPOI, poiExists := poiCache.Get(mappingId)
		if poiExists {
			MergePOI(&existingPOI.Entity, poi)
		} else {
			existingPOI = clib.CacheEntry[odhContentModel.ODHActivityPoi]{Entity: poi}
		}

		poiHash, poiChanged, err := poiCache.HasChanged(mappingId, existingPOI.Entity)
		if err != nil {
			return fmt.Errorf("hash POI: %w", err)
		}
		if poiChanged {
			poiCache.Set(mappingId, existingPOI.Entity, poiHash)
			changedPOIs = append(changedPOIs, existingPOI.Entity)
		}
	}

	// Measuringpoint merge + change detection
	var changedMPs []odhContentModel.MeasuringpointV2
	for _, mp := range result.Measuringpoints {
		mappingId := getMappingId(mp.Mapping)
		setMappingSyncTime(mp.Mapping, sourceTime)

		existingMP, mpExists := mpCache.Get(mappingId)
		if mpExists {
			MergeMeasuringpoint(&existingMP.Entity, mp)
		} else {
			existingMP = clib.CacheEntry[odhContentModel.MeasuringpointV2]{Entity: mp}
		}

		mpHash, mpChanged, err := mpCache.HasChanged(mappingId, existingMP.Entity)
		if err != nil {
			return fmt.Errorf("hash measuringpoint: %w", err)
		}
		if mpChanged {
			mpCache.Set(mappingId, existingMP.Entity, mpHash)
			changedMPs = append(changedMPs, existingMP.Entity)
		}
	}

	slog.Info("Uploading changed data",
		"skiAreaChanged", changed,
		"changedPOIs", len(changedPOIs),
		"changedMPs", len(changedMPs))

	// Upload ski area (generateid=false — we control the ID)
	if changed {
		if err := uploadWithOwnId(ctx, "SkiArea", *existing.Entity.ID, existing.Entity); err != nil {
			return fmt.Errorf("upload ski area: %w", err)
		}
	}

	// Upload POIs (generateid=false — we control the ID)
	for _, poi := range changedPOIs {
		if err := uploadWithOwnId(ctx, "ODHActivityPoi", *poi.ID, poi); err != nil {
			return fmt.Errorf("upload POI: %w", err)
		}
	}

	// Upload Measuringpoints (generateid=false — we control the ID)
	for _, mp := range changedMPs {
		if err := uploadWithOwnId(ctx, "Weather/Measuringpoint", *mp.ID, mp); err != nil {
			return fmt.Errorf("upload measuringpoint: %w", err)
		}
	}

	return nil
}

// Transform is the RabbitMQ entry point — delegates to processRaw.
func Transform(ctx context.Context, r *rdb.Raw[dto.SkiArea]) error {
	logger.Get(ctx).Info("Processing ski area")
	err := processRaw(ctx, r.Rawdata, r.Timestamp.Truncate(time.Millisecond))
	ms.FailOnError(ctx, err, "failed to process ski area")
	return nil
}

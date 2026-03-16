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

	"github.com/mitchellh/hashstructure/v2"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
	"opendatahub.com/tr-discoverswiss-skiarea/dto"
	odhContentClient "opendatahub.com/tr-discoverswiss-skiarea/odh-content-client"
	odhContentModel "opendatahub.com/tr-discoverswiss-skiarea/odh-content-model"
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

type skiAreaCache struct {
	skiArea odhContentModel.SkiArea
	hash    uint64
}

type poiCache struct {
	poi  odhContentModel.ODHActivityPoi
	hash uint64
}

type measuringpointCache struct {
	mp   odhContentModel.MeasuringpointV2
	hash uint64
}

var contentClient *odhContentClient.ContentClient
var skiAreaCacheMap map[string]skiAreaCache
var poiCacheMap map[string]poiCache
var mpCacheMap map[string]measuringpointCache
var location *time.Location

func loadExistingSkiAreas(ctx context.Context, contentClient *odhContentClient.ContentClient) (map[string]skiAreaCache, error) {
	cache := map[string]skiAreaCache{}

	type response struct {
		Items       []odhContentModel.SkiArea `json:"Items"`
		TotalPages  int                       `json:"TotalPages"`
		CurrentPage int                       `json:"CurrentPage"`
	}

	currentPage := 1
	totalPage := 99
	for currentPage <= totalPage {
		res := response{}

		err := contentClient.Get(ctx,
			"SkiArea",
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

		for _, skiArea := range res.Items {
			hash, err := hashstructure.Hash(skiArea, hashstructure.FormatV2, nil)
			if err != nil {
				return nil, fmt.Errorf("could not hash ski area: %v", err)
			}

			cache[getMappingId(skiArea.Mapping)] = skiAreaCache{
				skiArea: skiArea,
				hash:    hash,
			}
		}

		currentPage += 1
		totalPage = res.TotalPages
	}

	return cache, nil
}

func loadExistingPOIs(ctx context.Context, contentClient *odhContentClient.ContentClient) (map[string]poiCache, error) {
	cache := map[string]poiCache{}

	type response struct {
		Items       []odhContentModel.ODHActivityPoi `json:"Items"`
		TotalPages  int                              `json:"TotalPages"`
		CurrentPage int                              `json:"CurrentPage"`
	}

	currentPage := 1
	totalPage := 99
	for currentPage <= totalPage {
		res := response{}

		err := contentClient.Get(ctx,
			"ODHActivityPoi",
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

		for _, poi := range res.Items {
			hash, err := hashstructure.Hash(poi, hashstructure.FormatV2, nil)
			if err != nil {
				return nil, fmt.Errorf("could not hash POI: %v", err)
			}

			cache[getMappingId(poi.Mapping)] = poiCache{
				poi:  poi,
				hash: hash,
			}
		}

		currentPage += 1
		totalPage = res.TotalPages
	}

	return cache, nil
}

func loadExistingMeasuringpoints(ctx context.Context, contentClient *odhContentClient.ContentClient) (map[string]measuringpointCache, error) {
	cache := map[string]measuringpointCache{}

	type response struct {
		Items       []odhContentModel.MeasuringpointV2 `json:"Items"`
		TotalPages  int                                `json:"TotalPages"`
		CurrentPage int                                `json:"CurrentPage"`
	}

	currentPage := 1
	totalPage := 99
	for currentPage <= totalPage {
		res := response{}

		err := contentClient.Get(ctx,
			"Weather/Measuringpoint",
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

		for _, mp := range res.Items {
			hash, err := hashstructure.Hash(mp, hashstructure.FormatV2, nil)
			if err != nil {
				return nil, fmt.Errorf("could not hash measuringpoint: %v", err)
			}

			cache[getMappingId(mp.Mapping)] = measuringpointCache{
				mp:   mp,
				hash: hash,
			}
		}

		currentPage += 1
		totalPage = res.TotalPages
	}

	return cache, nil
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)

	defer tel.FlushOnPanic()

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

	// Load existing entities from ODH API (skip if no URL configured, e.g. dry-run CLI)
	if env.ODH_CORE_URL != "" {
		slog.Info("Loading existing entities...", "url", env.ODH_CORE_URL)

		skiAreaCacheMap, err = loadExistingSkiAreas(context.Background(), contentClient)
		ms.FailOnError(context.Background(), err, "failed to load ski areas")

		poiCacheMap, err = loadExistingPOIs(context.Background(), contentClient)
		ms.FailOnError(context.Background(), err, "failed to load POIs")

		mpCacheMap, err = loadExistingMeasuringpoints(context.Background(), contentClient)
		ms.FailOnError(context.Background(), err, "failed to load measuringpoints")
	} else {
		slog.Info("ODH_CORE_URL not set, starting with empty cache")
		skiAreaCacheMap = map[string]skiAreaCache{}
		poiCacheMap = map[string]poiCache{}
		mpCacheMap = map[string]measuringpointCache{}
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
	if !errors.Is(err, odhContentClient.ErrNoDataToUpdate) {
		return fmt.Errorf("PUT %s/%s: %w", apiPath, id, err)
	}

	// Entity doesn't exist yet — create with our own ID
	slog.Info("Creating entity", "apiPath", apiPath, "id", id)
	return contentClient.Post(ctx, apiPath, map[string]string{"generateid": "false"}, payload)
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

	// SkiArea merge (cache keyed by DiscoverSwiss mapping ID)
	existing, exists := skiAreaCacheMap[raw.Identifier]
	if exists {
		MergeSkiArea(&existing.skiArea, result.SkiArea)
	} else {
		existing = skiAreaCache{skiArea: result.SkiArea}
	}

	// Hash and compare
	hash, err := hashstructure.Hash(existing.skiArea, hashstructure.FormatV2, nil)
	if err != nil {
		return fmt.Errorf("hash ski area: %w", err)
	}

	skiAreaChanged := hash != existing.hash
	if skiAreaChanged {
		skiAreaCacheMap[raw.Identifier] = skiAreaCache{
			skiArea: existing.skiArea,
			hash:    hash,
		}
	}

	// POI merge (cache keyed by DiscoverSwiss mapping ID)
	var changedPOIs []odhContentModel.ODHActivityPoi
	for _, poi := range result.POI {
		mappingId := getMappingId(poi.Mapping)
		setMappingSyncTime(poi.Mapping, sourceTime)

		existingPOI, poiExists := poiCacheMap[mappingId]
		if poiExists {
			MergePOI(&existingPOI.poi, poi)
		} else {
			existingPOI = poiCache{poi: poi}
		}

		poiHash, err := hashstructure.Hash(existingPOI.poi, hashstructure.FormatV2, nil)
		if err != nil {
			return fmt.Errorf("hash POI: %w", err)
		}

		if poiHash != existingPOI.hash {
			poiCacheMap[mappingId] = poiCache{
				poi:  existingPOI.poi,
				hash: poiHash,
			}
			changedPOIs = append(changedPOIs, existingPOI.poi)
		}
	}

	// Measuringpoint merge (cache keyed by DiscoverSwiss mapping ID)
	var changedMPs []odhContentModel.MeasuringpointV2
	for _, mp := range result.Measuringpoints {
		mappingId := getMappingId(mp.Mapping)
		setMappingSyncTime(mp.Mapping, sourceTime)

		existingMP, mpExists := mpCacheMap[mappingId]
		if mpExists {
			MergeMeasuringpoint(&existingMP.mp, mp)
		} else {
			existingMP = measuringpointCache{mp: mp}
		}

		mpHash, err := hashstructure.Hash(existingMP.mp, hashstructure.FormatV2, nil)
		if err != nil {
			return fmt.Errorf("hash measuringpoint: %w", err)
		}

		if mpHash != existingMP.hash {
			mpCacheMap[mappingId] = measuringpointCache{
				mp:   existingMP.mp,
				hash: mpHash,
			}
			changedMPs = append(changedMPs, existingMP.mp)
		}
	}

	slog.Info("Uploading changed data",
		"skiAreaChanged", skiAreaChanged,
		"changedPOIs", len(changedPOIs),
		"changedMPs", len(changedMPs))

	// Upload ski area (generateid=false — we control the ID)
	if skiAreaChanged {
		if err := uploadWithOwnId(ctx, "SkiArea", *existing.skiArea.ID, existing.skiArea); err != nil {
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

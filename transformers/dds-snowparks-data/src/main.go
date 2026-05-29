// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"

	"opendatahub.com/tr-dss-snowparks/dto"
	odhmodel "opendatahub.com/tr-dss-snowparks/odhmodel"
)

const (
	SOURCE         = "dss"
	ENTITY_TYPE    = "ODHActivityPoi"
	SYNC_INTERFACE = "dsssnowparkbase"
	LICENSE_HOLDER = "https://www.dolomitisuperski.com"
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string
}

var contentClient clib.ContentAPI
var snowparksCache *clib.Cache[odhmodel.ODHActivityPoi]
var nowFunc = func() time.Time { return time.Now().UTC() }

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting DSS Snowpark transformer...")
	defer tel.FlushOnPanic()

	slog.Info("ODH core url", "value", env.ODH_CORE_URL)

	var err error

	contentClient, err = clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create ODH content client")

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing DSS snowpark feed",
		"item_count", len(r.Rawdata.DssSnowparks.Items))

	if snowparksCache == nil {
		var err error
		snowparksCache, err = clib.LoadExisting(ctx, contentClient, clib.LoadConfig[odhmodel.ODHActivityPoi]{
			EntityType:  ENTITY_TYPE,
			QueryParams: map[string]string{"source": SOURCE, "tagfilter": "snowpark"},
			// Normalize legacy uppercase IDs (e.g. "DSS_267") to match buildID output ("dss_267").
			IDFunc: func(p odhmodel.ODHActivityPoi) string {
				return strings.ToLower(*p.Generic.ID)
			},
		})
		if err != nil {
			return fmt.Errorf("failed to load snowpark POI cache: %w", err)
		}
		logger.Get(ctx).Info("Loaded existing snowpark POIs", "count", len(snowparksCache.Entries()))
	}
	defer func() { snowparksCache = nil }()

	seen := map[string]struct{}{}
	pois := map[string]odhmodel.ODHActivityPoi{}

	for _, snowpark := range r.Rawdata.DssSnowparks.Items {
		id := buildID(snowpark)
		seen[id] = struct{}{}

		existing, inCache := snowparksCache.Get(id)
		var base *odhmodel.ODHActivityPoi
		if inCache {
			copy := existing.Entity
			base = &copy
		}

		pois[id] = mapSnowparkToPoi(snowpark, base)
	}

	sortedIDs := make([]string, 0, len(pois))
	for id := range pois {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		poi := pois[id]

		hash, changed, err := snowparksCache.HasChanged(id, poi)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash POI", "id", id, "error", err)
			continue
		}

		_, exists := snowparksCache.Get(id)

		if !exists {
			postErr := contentClient.Post(ctx, ENTITY_TYPE, map[string]string{"generateid": "false"}, poi)
			if postErr == nil {
				snowparksCache.Set(id, poi, hash)
				logger.Get(ctx).Info("Created new snowpark POI", "id", id)
				continue
			}
			if !strings.Contains(postErr.Error(), "data exists already") {
				logger.Get(ctx).Error("API Post failed", "id", id, "error", postErr)
				continue
			}
			logger.Get(ctx).Warn("POST returned 'data exists already', recovering with PUT", "id", id)
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
				logger.Get(ctx).Error("API Put failed (recovery)", "id", id, "error", err)
				continue
			}
			snowparksCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Recovered stale-cache snowpark POI via PUT", "id", id)

		} else if changed {
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", err)
				continue
			}
			snowparksCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Updated snowpark POI", "id", id)
		}
	}

	// ── DEACTIVATION ─────────────────────────────────────────────────────────
	for id := range snowparksCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}
		entry, stillExists := snowparksCache.Get(id)
		if !stillExists {
			continue
		}
		poi := entry.Entity
		poi.Active = false
		poi.SmgActive = false
		poi.OdhActive = false
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
			logger.Get(ctx).Error("Failed to deactivate snowpark POI", "id", id, "error", err)
			continue
		}
		snowparksCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing snowpark POI", "id", id)
	}

	return nil
}

// ── ID ────────────────────────────────────────────────────────────────────────

func buildID(snowpark dto.DssSnowpark) string {
	return fmt.Sprintf("dss_%d", snowpark.Pid)
}

// ── Main mapper ───────────────────────────────────────────────────────────────

func mapSnowparkToPoi(snowpark dto.DssSnowpark, base *odhmodel.ODHActivityPoi) odhmodel.ODHActivityPoi {
	id := buildID(snowpark)
	source := SOURCE
	shortname := stringFromMultilang(snowpark.Name, "de")

	// Snowpark feed has no update-date field — use now as LastChange.
	lastChange := nowFunc()

	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	// Snowparks have no skiresort object — map pid, rid, regionId only.
	mapping := map[string]map[string]string{
		SOURCE: {
			"pid":      strconv.FormatInt(snowpark.Pid, 10),
			"rid":      strconv.FormatInt(snowpark.Rid, 10),
			"regionId": strconv.FormatInt(snowpark.RegionId, 10),
		},
	}

	detail := buildDetail(snowpark)

	// Mirrors C# Convert.ToBoolean(state): any non-zero value = open.
	isOpen := snowpark.State != 0

	gpsInfo, gpsPoints := buildGps(snowpark)

	additionalPoiInfos := map[string]*odhmodel.AdditionalPoiInfo{
		"de": {Novelty: "", Language: "de", Categories: []string{"Snowpark"}},
		"it": {Novelty: "", Language: "it", Categories: []string{"Snowpark"}},
		"en": {Novelty: "", Language: "en", Categories: []string{"Snowpark"}},
	}

	return odhmodel.ODHActivityPoi{
		Generic: odhmodel.Generic{
			ID:          &id,
			Active:      true,
			Source:      &source,
			Shortname:   &shortname,
			HasLanguage: []string{"de", "it", "en"},
			FirstImport: firstImport,
			LastChange:  odhmodel.PtrFlexibleTime(lastChange),
			Mapping:     mapping,
			TagIds:      buildTagIds(),
			SmgTags:     buildSmgTags(),
			GpsInfo:     gpsInfo,
			LicenseInfo: &odhmodel.LicenseInfo{
				Author:        "",
				License:       "CC0",
				LicenseHolder: LICENSE_HOLDER,
				ClosedData:    false,
			},
		},
		Detail:               detail,
		ContactInfos:         map[string]interface{}{},
		AdditionalProperties: map[string]interface{}{},
		PoiProperty:          map[string]interface{}{},
		AdditionalPoiInfos:   additionalPoiInfos,
		LocationInfo:         &odhmodel.LocationInfo{},
		SmgActive:            true,
		OdhActive:            true,
		PublishedOn:          []string{},
		SyncUpdateMode:       "Full",
		SyncSourceInterface:  SYNC_INTERFACE,
		CustomId:             strconv.FormatInt(snowpark.Rid, 10),
		IsOpen:               isOpen,
		// BikeTransport nil — not present in snowpark feed, matches old API null.
		BikeTransport: nil,
		GpsPoints:     gpsPoints,
		// No Number, DistanceLength, DistanceDuration, AltitudeLowestPoint,
		// AltitudeHighestPoint, AltitudeDifference, GpsTrack, OperationSchedule,
		// Difficulty, Ratings — not present in snowpark feed.
	}
}

// ── Tag builders ──────────────────────────────────────────────────────────────

func buildTagIds() []string {
	return []string{
		"activity",
		"snow parks",
		"snowpark",
		"snowparks",
		"winter",
	}
}

func buildSmgTags() []string {
	return []string{
		"winter",
		"snowpark",
		"activity",
	}
}

// ── GPS builder ───────────────────────────────────────────────────────────────

// buildGps mirrors slope GPS logic — single "position" entry.
// Snowpark altitude is data.Altitude: a flat nullable *int (not nested start/end).
func buildGps(snowpark dto.DssSnowpark) ([]odhmodel.GpsInfo, map[string]*odhmodel.GpsInfo) {
	gpsInfo := []odhmodel.GpsInfo{}
	gpsPoints := map[string]*odhmodel.GpsInfo{}

	if snowpark.Location == nil {
		return gpsInfo, gpsPoints
	}

	lat, latOk := safeParseFloat(snowpark.Location.Lat)
	lon, lonOk := safeParseFloat(snowpark.Location.Lon)
	if !latOk || !lonOk {
		return gpsInfo, gpsPoints
	}

	entry := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              snowpark.Data.Altitude,
		AltitudeUnitofMeasure: "m",
	}

	gpsInfo = append(gpsInfo, entry)
	positionCopy := entry
	gpsPoints["position"] = &positionCopy

	return gpsInfo, gpsPoints
}

// ── Detail builder ────────────────────────────────────────────────────────────

// buildDetail maps Name→Title, DetailText→BaseText for each language.
// DetailText is the snowpark equivalent of Description in slopes.
// No AdditionalText — snowpark feed has no info-text field.
func buildDetail(snowpark dto.DssSnowpark) map[string]*clib.DetailGeneric {
	detail := map[string]*clib.DetailGeneric{}
	for _, lang := range []string{"de", "it", "en"} {
		title := stringFromMultilang(snowpark.Name, lang)
		baseText := nilableFromMultilang(snowpark.DetailText, lang)
		langCopy := lang
		detail[lang] = &clib.DetailGeneric{
			Language: &langCopy,
			Title:    &title,
			BaseText: baseText,
		}
	}
	return detail
}

// ── Float safety ──────────────────────────────────────────────────────────────

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

func safeParseFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil || !isFinite(v) {
		return 0, false
	}
	return v, true
}

// ── Multilang helpers ─────────────────────────────────────────────────────────

func stringFromMultilang(m dto.DssMultilang, lang string) string {
	var ptr *string
	switch lang {
	case "de":
		ptr = m.De
	case "it":
		ptr = m.It
	case "en":
		ptr = m.En
	}
	if ptr == nil {
		return ""
	}
	return *ptr
}

func nilableFromMultilang(m dto.DssMultilang, lang string) *string {
	val := stringFromMultilang(m, lang)
	if val == "" {
		return nil
	}
	return &val
}

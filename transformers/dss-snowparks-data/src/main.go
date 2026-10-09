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
	ODH_CORE_REFERER             string
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
	}, clib.WithReferer(env.ODH_CORE_REFERER))
	ms.FailOnError(context.Background(), err, "failed to create ODH content client")

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing DSS snowpark feed",
		"item_count", len(r.Rawdata.DssSnowparks.Items))

	if len(r.Rawdata.DssSnowparks.Items) == 0 {
		// An empty feed is far more likely a crawler/DSS API problem than
		// "no snowparks at all"; the deactivation below would disable every snowpark.
		logger.Get(ctx).Warn("Received DSS snowpark feed without items, skipping processing and deactivation")
		return nil
	}

	if snowparksCache == nil {
		var err error
		snowparksCache, err = clib.LoadExisting(ctx, contentClient, clib.LoadConfig[odhmodel.ODHActivityPoi]{
			EntityType:  ENTITY_TYPE,
			QueryParams: map[string]string{"source": SOURCE, "tagfilter": "snowpark"},
			IDFunc:      func(p odhmodel.ODHActivityPoi) string { return *p.Generic.ID },
		})
		if err != nil {
			return fmt.Errorf("failed to load snowpark POI cache: %w", err)
		}
		logger.Get(ctx).Info("Loaded existing snowpark POIs", "count", len(snowparksCache.Entries()))
	}
	defer func() { snowparksCache = nil }()

	seen := map[string]struct{}{}
	pois := map[string]odhmodel.ODHActivityPoi{}

	lifts := newLiftIndex(r.Rawdata.DssLifts)
	for _, snowpark := range r.Rawdata.DssSnowparks.Items {
		id := buildID(snowpark)
		seen[id] = struct{}{}

		existing, inCache := snowparksCache.Get(id)
		var base *odhmodel.ODHActivityPoi
		if inCache {
			copy := existing.Entity
			base = &copy
		}

		pois[id] = mapSnowparkToPoi(snowpark, base, lifts)
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
		if !entry.Entity.Active && !entry.Entity.SmgActive && !entry.Entity.OdhActive {
			// Already inactive on the API - the cache is rebuilt per message, so
			// re-PUTting it here would repeat on every run.
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

// ── Mapping ───────────────────────────────────────────────────────────────────

// mergeMapping keeps the existing Mapping of the record (other sources and dss
// keys set elsewhere) and only adds/overwrites the given dss keys. Empty values
// are skipped, so a missing DSS field never clears an existing key.
func mergeMapping(base *odhmodel.ODHActivityPoi, dss map[string]string) map[string]map[string]string {
	mapping := map[string]map[string]string{}
	if base != nil {
		for source, values := range base.Mapping {
			copied := make(map[string]string, len(values))
			for k, v := range values {
				copied[k] = v
			}
			mapping[source] = copied
		}
	}
	if mapping[SOURCE] == nil {
		mapping[SOURCE] = map[string]string{}
	}
	for k, v := range dss {
		if v != "" {
			mapping[SOURCE][k] = v
		}
	}
	return mapping
}

// liftIndex links snowparks to lifts. The snowpark feed only has lift names
// (lifts[].rid is always null), so lifts are matched by name within the region.
type liftIndex struct {
	byName     map[string][]dto.DssLift  // "<regionId>|<name>"
	subregions map[int64]map[string]bool // regionId -> subregionIds of its lifts (4a, 4b, ...)
}

func newLiftIndex(feed dto.DssLiftFeed) liftIndex {
	idx := liftIndex{byName: map[string][]dto.DssLift{}, subregions: map[int64]map[string]bool{}}
	for _, lift := range feed.Items {
		key := liftKey(lift.RegionId, stringFromMultilang(lift.Name, "de"))
		idx.byName[key] = append(idx.byName[key], lift)
		if sub := strings.TrimSpace(lift.SubregionId); sub != "" && sub != "all" {
			if idx.subregions[lift.RegionId] == nil {
				idx.subregions[lift.RegionId] = map[string]bool{}
			}
			idx.subregions[lift.RegionId][sub] = true
		}
	}
	return idx
}

func liftKey(regionId int64, name string) string {
	return strconv.FormatInt(regionId, 10) + "|" + strings.ToLower(strings.TrimSpace(name))
}

// resolve returns the pids of the snowpark's lifts (only names that match exactly
// one lift) and its talschaft rid: the regionId, the region's only subregionId
// (6 -> 6a), or for split regions the common subregionId of the matched lifts.
// Without lift data both are empty.
func (idx liftIndex) resolve(snowpark dto.DssSnowpark) (liftPids string, skiAreaRid string) {
	if len(idx.byName) == 0 {
		return "", ""
	}

	names := []string{}
	for _, l := range snowpark.Lifts {
		names = append(names, stringFromMultilang(l.Name, "de"))
	}
	if len(names) == 0 {
		names = append(names, stringFromMultilang(snowpark.Lift, "de"))
	}

	pids := []string{}
	subregions := map[string]bool{}
	for _, name := range names {
		matches := idx.byName[liftKey(snowpark.RegionId, name)]
		if len(matches) == 1 {
			pids = append(pids, strconv.FormatInt(matches[0].Pid, 10))
		}
		for _, lift := range matches {
			if sub := strings.TrimSpace(lift.SubregionId); sub != "" && sub != "all" {
				subregions[sub] = true
			}
		}
	}

	regionSubs := idx.subregions[snowpark.RegionId]
	if len(regionSubs) == 0 {
		skiAreaRid = strconv.FormatInt(snowpark.RegionId, 10)
	} else if len(regionSubs) == 1 {
		subregions = regionSubs
	}
	if skiAreaRid == "" && len(subregions) == 1 {
		for sub := range subregions {
			skiAreaRid = sub
		}
	}
	return strings.Join(pids, ","), skiAreaRid
}

func intPtrString(i *int) string {
	if i == nil {
		return ""
	}
	return strconv.Itoa(*i)
}

// ── ID ────────────────────────────────────────────────────────────────────────

// buildID: urn:odhactivitypoi:dss:snowpark:<regionId>_<pid>. Snowpark pids overlap lift pids
// and are only unique together with regionId (e.g. pid 1 exists in region 5 and 7).
func buildID(snowpark dto.DssSnowpark) string {
	return fmt.Sprintf("urn:odhactivitypoi:%s:snowpark:%d_%d", SOURCE, snowpark.RegionId, snowpark.Pid)
}

// ── Main mapper ───────────────────────────────────────────────────────────────

func mapSnowparkToPoi(snowpark dto.DssSnowpark, base *odhmodel.ODHActivityPoi, lifts liftIndex) odhmodel.ODHActivityPoi {
	id := buildID(snowpark)
	source := SOURCE
	shortname := nameWithFallback(snowpark.Name, "de")

	// Snowpark feed has no update-date field — use now as LastChange.
	lastChange := nowFunc()

	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	liftPids, skiAreaRid := lifts.resolve(snowpark)
	d := snowpark.Data
	mapping := mergeMapping(base, map[string]string{
		"pid":                 strconv.FormatInt(snowpark.Pid, 10),
		"rid":                 strconv.FormatInt(snowpark.Rid, 10),
		"regionId":            strconv.FormatInt(snowpark.RegionId, 10),
		"skiarea_rid":         skiAreaRid,
		"lift_pids":           liftPids,
		"bordercross":         strconv.FormatBool(d.Bordercross),
		"pipe":                strconv.FormatBool(d.Pipe),
		"artificially_snowed": strconv.FormatBool(d.ArtificiallySnowed),
		"is_snowpark":         strconv.FormatBool(d.Snowparks.IsSnowpark),
		"snowpark_overview":   intPtrString(d.Snowparks.SnowparkOverview),
		"snowpark_pro":        intPtrString(d.Snowparks.SnowparkPro),
		"snowpark_med":        intPtrString(d.Snowparks.SnowparkMed),
		"snowpark_easy":       intPtrString(d.Snowparks.SnowparkEasy),
		"snowpark_jib":        intPtrString(d.Snowparks.SnowparkJib),
		"snowpark_nightslope": strconv.FormatBool(d.Snowparks.SnowparkNightslope),
		"is_familyfun":        strconv.FormatBool(d.FamilyFun.IsFamilyFun),
		"familyfun_overview":  intPtrString(d.FamilyFun.FamilyFunOverview),
		"familyfun_curves":    intPtrString(d.FamilyFun.FamilyFunCurves),
		"familyfun_tunnel":    intPtrString(d.FamilyFun.FamilyFunTunnel),
		"familyfun_tools":     intPtrString(d.FamilyFun.FamilyFunTools),
		"is_crossline":        strconv.FormatBool(d.Crossline.IsCrossline),
		"crossline_overview":  intPtrString(d.Crossline.CrosslineOverview),
		"crossline_curves":    intPtrString(d.Crossline.CrosslineCurves),
		"crossline_waves":     intPtrString(d.Crossline.CrosslineWaves),
		"crossline_jumps":     intPtrString(d.Crossline.CrosslineJumps),
	})

	detail, hasLanguage := buildDetail(snowpark)

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
			HasLanguage: hasLanguage,
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
		BikeTransport:  nil,
		GpsPoints:      gpsPoints,
		DistanceLength: snowpark.Data.Length,
		// No Number, DistanceDuration, AltitudeLowestPoint,
		// AltitudeHighestPoint, AltitudeDifference, GpsTrack, OperationSchedule,
		// Difficulty, Ratings — not present in snowpark feed.
	}
}

// ── Tag builders ──────────────────────────────────────────────────────────────

// buildTagIds is kept sorted, as the API stores TagIds sorted (change detection hash).
func buildTagIds() []string {
	return []string{
		"activity",
		"snow parks",
		"snowpark",
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

	// GpsInfo.Altitude is *float64 — ODH API returns floats e.g. 1520.0.
	var altFloat *float64
	if snowpark.Data.Altitude != nil {
		f := float64(*snowpark.Data.Altitude)
		altFloat = &f
	}

	entry := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              altFloat,
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
// It also returns the matching HasLanguage. Languages without title and text are
// skipped: the API drops such entries and removes the language from HasLanguage,
// so sending them would change the hash on every run.
func buildDetail(snowpark dto.DssSnowpark) (map[string]*clib.DetailGeneric, []string) {
	detail := map[string]*clib.DetailGeneric{}
	hasLanguage := []string{}
	for _, lang := range []string{"de", "it", "en"} {
		title := nameWithFallback(snowpark.Name, lang)
		baseText := nilableFromMultilang(snowpark.DetailText, lang)
		if title == "" && baseText == nil {
			continue
		}
		hasLanguage = append(hasLanguage, lang)
		langCopy := lang
		detail[lang] = &clib.DetailGeneric{
			Language: &langCopy,
			Title:    &title,
			BaseText: baseText,
		}
	}
	return detail, hasLanguage
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

// nameWithFallback returns the trimmed name in lang. If it is empty, it falls back to
// the first non-empty name in de, it, en, so every language gets a title.
func nameWithFallback(m dto.DssMultilang, lang string) string {
	if name := strings.TrimSpace(stringFromMultilang(m, lang)); name != "" {
		return name
	}
	for _, l := range []string{"de", "it", "en"} {
		if name := strings.TrimSpace(stringFromMultilang(m, l)); name != "" {
			return name
		}
	}
	return ""
}

// The API trims texts, so they are trimmed here too; otherwise the hash of an
// unchanged record would differ from the loaded one on every run.
func nilableFromMultilang(m dto.DssMultilang, lang string) *string {
	val := strings.TrimSpace(stringFromMultilang(m, lang))
	if val == "" {
		return nil
	}
	return &val
}

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
	_ "time/tzdata" // embed zoneinfo so Europe/Rome always loads

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"

	"opendatahub.com/tr-dss-slopes/dto"
	odhmodel "opendatahub.com/tr-dss-slopes/odhmodel"
)

const (
	SOURCE         = "dss"
	ENTITY_TYPE    = "ODHActivityPoi"
	SYNC_INTERFACE = "dssslopebase"
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
var poiCache *clib.Cache[odhmodel.ODHActivityPoi]
var nowFunc = func() time.Time { return time.Now().UTC() }

// DSS season dates are Unix timestamps of local midnight in Italy.
var romeLocation = func() *time.Location {
	loc, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		panic(err)
	}
	return loc
}()

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting DSS Slope transformer...")
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
	logger.Get(ctx).Info("Processing DSS slope feed",
		"item_count", len(r.Rawdata.DssSlopes.Items))

	if len(r.Rawdata.DssSlopes.Items) == 0 {
		// An empty feed is far more likely a crawler/DSS API problem than
		// "no slopes at all"; the deactivation below would disable every slope.
		logger.Get(ctx).Warn("Received DSS slope feed without items, skipping processing and deactivation")
		return nil
	}

	if poiCache == nil {
		var err error
		poiCache, err = clib.LoadExisting(ctx, contentClient, clib.LoadConfig[odhmodel.ODHActivityPoi]{
			EntityType:  ENTITY_TYPE,
			QueryParams: map[string]string{"source": SOURCE, "tagfilter": "slopes"},
			IDFunc:      func(p odhmodel.ODHActivityPoi) string { return *p.Generic.ID },
		})
		if err != nil {
			return fmt.Errorf("failed to load slope POI cache: %w", err)
		}
		logger.Get(ctx).Info("Loaded existing slope POIs", "count", len(poiCache.Entries()))
	}
	defer func() { poiCache = nil }()

	seen := map[string]struct{}{}
	pois := map[string]odhmodel.ODHActivityPoi{}
	skiAreas := newSkiAreaResolver(r.Rawdata.DssSkiAreas)

	for _, slope := range r.Rawdata.DssSlopes.Items {
		slope = withGeoFileFallback(ctx, slope)
		id := buildID(slope)
		seen[id] = struct{}{}

		existing, inCache := poiCache.Get(id)
		var base *odhmodel.ODHActivityPoi
		if inCache {
			copy := existing.Entity
			base = &copy
		}

		pois[id] = mapSlopeToPoi(slope, base, skiAreas.rid(slope))
	}

	sortedIDs := make([]string, 0, len(pois))
	for id := range pois {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		poi := pois[id]

		hash, changed, err := poiCache.HasChanged(id, poi)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash POI", "id", id, "error", err)
			continue
		}

		_, exists := poiCache.Get(id)

		if !exists {
			postErr := contentClient.Post(ctx, ENTITY_TYPE, map[string]string{"generateid": "false"}, poi)
			if postErr == nil {
				poiCache.Set(id, poi, hash)
				logger.Get(ctx).Info("Created new slope POI", "id", id)
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
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Recovered stale-cache slope POI via PUT", "id", id)

		} else if changed {
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", err)
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Updated slope POI", "id", id)
		}
	}

	// ── DEACTIVATION ─────────────────────────────────────────────────────────
	for id := range poiCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}
		entry, stillExists := poiCache.Get(id)
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
			logger.Get(ctx).Error("Failed to deactivate slope POI", "id", id, "error", err)
			continue
		}
		poiCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing slope POI", "id", id)
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

// skiAreaResolver maps a slope to its DSS talschaft rid (SkiArea Mapping.dss.rid).
type skiAreaResolver struct {
	bySkiresort map[int64]string
	byRegion    map[string][]string // regionId -> talschaft rids ("4" -> 4a, 4b)
}

func newSkiAreaResolver(feed dto.DssSkiAreaFeed) skiAreaResolver {
	r := skiAreaResolver{bySkiresort: map[int64]string{}, byRegion: map[string][]string{}}
	for _, area := range feed.Items {
		region := strings.TrimRight(area.Rid, "abcdefghijklmnopqrstuvwxyz")
		r.byRegion[region] = append(r.byRegion[region], area.Rid)
		for _, resort := range area.Skiresorts {
			if resort.Rid != 0 {
				r.bySkiresort[resort.Rid] = area.Rid
			}
		}
	}
	return r
}

// rid resolves via the slope's skiresort; otherwise it falls back to the only
// talschaft of the slope's region (regions 4, 5 and 8 are split into a/b).
func (r skiAreaResolver) rid(slope dto.DssSlope) string {
	if rid, ok := r.bySkiresort[slope.Skiresort.Rid]; ok {
		return rid
	}
	if areas := r.byRegion[strconv.FormatInt(slope.RegionId, 10)]; len(areas) == 1 {
		return areas[0]
	}
	return ""
}

// ── ID ────────────────────────────────────────────────────────────────────────

// buildID: urn:odhactivitypoi:dss:slope:<regionId>_<pid>.
func buildID(slope dto.DssSlope) string {
	return fmt.Sprintf("urn:odhactivitypoi:%s:slope:%d_%d", SOURCE, slope.RegionId, slope.Pid)
}

// ── Main mapper ───────────────────────────────────────────────────────────────

func mapSlopeToPoi(slope dto.DssSlope, base *odhmodel.ODHActivityPoi, skiAreaRid string) odhmodel.ODHActivityPoi {
	id := buildID(slope)
	source := SOURCE
	shortname := nameWithFallback(slope.Name, "de")
	lastChange := time.Unix(slope.UpdateDate, 0).UTC()

	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	mapping := mergeMapping(base, map[string]string{
		"pid":           strconv.FormatInt(slope.Pid, 10),
		"rid":           strconv.FormatInt(slope.Rid, 10),
		"regionId":      strconv.FormatInt(slope.RegionId, 10),
		"skiresort_rid": strconv.FormatInt(slope.Skiresort.Rid, 10),
		"skiresort_pid": strconv.FormatInt(slope.Skiresort.Pid, 10),
		"skiarea_rid":   skiAreaRid,
		"onlyForExport": strconv.FormatBool(slope.OnlyForExport != 0),
	})

	detail := buildDetail(slope)

	// FIX C: C# uses Convert.ToBoolean(state) = (state != 0)
	isOpen := slope.State != 0

	// FIX B: difficulty values match C# ParseDSSSlopeTypeToODHDifficulty exactly
	// blue=2, red=4, black=6, default=4
	difficulty := parseDifficulty(slope.SlopeType, slope.Slopetype)

	// DistanceDuration: seconds → hours, rounded to 2 decimals; 0 when missing (as C#).
	distDuration := new(float64)
	if slope.Duration != "" {
		if secs, err := strconv.ParseFloat(slope.Duration, 64); err == nil && isFinite(secs) {
			hours := math.RoundToEven((secs/3600.0)*100) / 100 // C# Math.Round rounds midpoints to even
			if isFinite(hours) {
				distDuration = &hours
			}
		}
	}

	gpsInfo, gpsPoints := buildGps(slope)

	gpsTrack := []odhmodel.GpsTrack{}
	if slope.GeoPositionFile != "" {
		gpsTrack = []odhmodel.GpsTrack{
			{
				Id:           nil,
				Type:         "detailed",
				Format:       "kml",
				GpxTrackUrl:  slope.GeoPositionFile,
				GpxTrackDesc: map[string]interface{}{},
			},
		}
	}

	// altitude.start is usually the top of the slope, so order the pair instead of
	// mapping start/end to lowest/highest.
	lowest, highest := slope.Data.Altitude.Start, slope.Data.Altitude.End
	if lowest != nil && highest != nil && *lowest > *highest {
		lowest, highest = highest, lowest
	}

	opSchedule := buildOperationSchedule(slope)
	opSchedules := []odhmodel.OperationSchedule{}
	if opSchedule != nil {
		opSchedules = append(opSchedules, *opSchedule)
	}

	// Full category lists matching old API exactly (3 per language).
	additionalPoiInfos := map[string]*odhmodel.AdditionalPoiInfo{
		"de": {Novelty: stringFromMultilang(slope.InfoText, "de"), Language: "de", Categories: []string{"Ski Alpin", "Skirundtouren & Pisten", "Pisten"}},
		"it": {Novelty: stringFromMultilang(slope.InfoText, "it"), Language: "it", Categories: []string{"sci alpino", "Piste e circuiti sciistici", "Piste"}},
		"en": {Novelty: stringFromMultilang(slope.InfoText, "en"), Language: "en", Categories: []string{"Alpine skiing", "Marked Ski Paths & Slopes", "Slopes"}},
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
		CustomId:             strconv.FormatInt(slope.Rid, 10),
		IsOpen:               isOpen,
		Number:               slope.Number,
		// FIX B+E: set both Difficulty and Ratings.Difficulty — mirrors C# exactly
		Difficulty: difficulty,
		Ratings:    &odhmodel.Ratings{Difficulty: difficulty},
		// BikeTransport nil for slopes — old API has null
		BikeTransport:        nil,
		DistanceLength:       slope.Data.Length,
		DistanceDuration:     distDuration,
		AltitudeLowestPoint:  intToFloat64(lowest),
		AltitudeHighestPoint: intToFloat64(highest),
		AltitudeDifference:   intToFloat64(slope.Data.HeightDifference),
		GpsTrack:             gpsTrack,
		GpsPoints:            gpsPoints,
		OperationSchedule:    opSchedules,
	}
}

// ── Difficulty ────────────────────────────────────────────────────────────────

// parseDifficulty mirrors C# ParseDSSSlopeTypeToODHDifficulty exactly.
// slopeType (color string) takes precedence over slopetype (text).
// Values: blue/easy=2, red/medium=4, black/hard=6, default=4.
func parseDifficulty(slopeType string, slopetype string) *string {
	var d string

	switch strings.ToLower(strings.TrimSpace(slopeType)) {
	case "blue":
		d = "2"
	case "red":
		d = "4"
	case "black":
		d = "6"
	default:
		switch strings.ToLower(strings.TrimSpace(slopetype)) {
		case "easy":
			d = "2"
		case "medium":
			d = "4"
		case "hard":
			d = "6"
		default:
			d = "4" // C# default for unknown types
		}
	}

	return &d
}

// ── Tag builders ──────────────────────────────────────────────────────────────

// buildTagIds — fixed set matching old API exactly. No color tag in TagIds.
// Keep it sorted, as the API stores TagIds sorted (change detection hash).
func buildTagIds() []string {
	return []string{
		"activity",
		"alpine skiing",
		"marked ski paths slopes",
		"other slopes",
		"slope",
		"slopes",
		"winter",
	}
}

// buildSmgTags — fixed set matching old API exactly. No color tag in SmgTags.
func buildSmgTags() []string {
	return []string{
		"winter",
		"skirundtouren pisten",
		"pisten",
		"ski alpin",
		"piste",
		"weitere pisten",
		"activity",
	}
}

// ── GPS builder ───────────────────────────────────────────────────────────────

// withGeoFileFallback fills a missing location with the first point of the
// geoPositionFile track (slope start), which is where DSS places the location.
// DSS coordinates are never overwritten.
func withGeoFileFallback(ctx context.Context, slope dto.DssSlope) dto.DssSlope {
	if hasValidLocation(slope.Location) || slope.GeoPositionFile == "" {
		return slope
	}
	points, err := geoFiles.Points(ctx, slope.GeoPositionFile)
	if err != nil {
		logger.Get(ctx).Warn("No GPS fallback from geoPositionFile", "pid", slope.Pid, "url", slope.GeoPositionFile, "error", err)
		return slope
	}
	slope.Location = &dto.DssSlopeLocation{Lat: formatCoordinate(points[0].Lat), Lon: formatCoordinate(points[0].Lon)}
	return slope
}

func hasValidLocation(loc *dto.DssSlopeLocation) bool {
	if loc == nil {
		return false
	}
	_, latOk := safeParseFloat(loc.Lat)
	_, lonOk := safeParseFloat(loc.Lon)
	return latOk && lonOk
}

// buildGps: single position entry. DSS places the location at the slope start, so it
// gets altitude.start (C# used altitude.end, the bottom of the slope).
func buildGps(slope dto.DssSlope) ([]odhmodel.GpsInfo, map[string]*odhmodel.GpsInfo) {
	gpsInfo := []odhmodel.GpsInfo{}
	gpsPoints := map[string]*odhmodel.GpsInfo{}

	if slope.Location == nil {
		return gpsInfo, gpsPoints
	}

	lat, latOk := safeParseFloat(slope.Location.Lat)
	lon, lonOk := safeParseFloat(slope.Location.Lon)
	if !latOk || !lonOk {
		return gpsInfo, gpsPoints
	}

	// GpsInfo.Altitude is *float64 — ODH API returns 1520.0 not 1520.
	var altFloat *float64
	if slope.Data.Altitude.Start != nil {
		f := float64(*slope.Data.Altitude.Start)
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

// ── OperationSchedule builder ─────────────────────────────────────────────────

// buildOperationSchedule mirrors C# ParseDSSSlopeToODHOperationScheduleFormat.
// C# reads data["seasonStart"] and data["seasonEnd"] — top-level root fields.
// No opening-times slot for slopes (C# parser doesn't add one).
func buildOperationSchedule(slope dto.DssSlope) *odhmodel.OperationSchedule {
	if slope.SeasonStart == nil || slope.SeasonEnd == nil {
		return nil
	}

	const dtFormat = "2006-01-02T00:00:00"
	start := time.Unix(*slope.SeasonStart, 0).In(romeLocation).Format(dtFormat)
	stop := time.Unix(*slope.SeasonEnd, 0).In(romeLocation).Format(dtFormat)

	return &odhmodel.OperationSchedule{
		Stop:  stop,
		Type:  "1",
		Start: start,
		OperationscheduleName: map[string]string{
			"de": "Wintersaison",
			"it": "stagioneinvernale",
			"en": "winterseason",
		},
		// C# slope parser does NOT add OperationScheduleTime — no opening times slot.
	}
}

// ── Detail builder ────────────────────────────────────────────────────────────

// buildDetail mirrors C# detail mapping:
//   - Title + BaseText + AdditionalText (info-text-winter) for de, it, en
//     (C# only set AdditionalText for de)
func buildDetail(slope dto.DssSlope) map[string]*odhmodel.Detail {
	detail := map[string]*odhmodel.Detail{}
	for _, lang := range []string{"de", "it", "en"} {
		title := nameWithFallback(slope.Name, lang)
		baseText := nilableFromMultilang(slope.Description, lang)
		langCopy := lang

		entry := &odhmodel.Detail{
			DetailGeneric: clib.DetailGeneric{
				Language: &langCopy,
				Title:    &title,
				BaseText: baseText,
			},
			AdditionalText: nilableFromMultilang(slope.InfoText, lang),
		}

		detail[lang] = entry
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

// intToFloat64 converts a nullable *int to a nullable *float64.
func intToFloat64(i *int) *float64 {
	if i == nil {
		return nil
	}
	f := float64(*i)
	return &f
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

func nilableFromMultilang(m dto.DssMultilang, lang string) *string {
	val := stringFromMultilang(m, lang)
	if val == "" {
		return nil
	}
	return &val
}

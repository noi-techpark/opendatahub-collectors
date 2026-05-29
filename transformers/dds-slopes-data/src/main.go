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
}

var contentClient clib.ContentAPI
var poiCache *clib.Cache[odhmodel.ODHActivityPoi]
var nowFunc = func() time.Time { return time.Now().UTC() }

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
	})
	ms.FailOnError(context.Background(), err, "failed to create ODH content client")

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing DSS slope feed",
		"item_count", len(r.Rawdata.DssSlopes.Items))

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

	for _, slope := range r.Rawdata.DssSlopes.Items {
		id := buildID(slope)
		seen[id] = struct{}{}

		existing, inCache := poiCache.Get(id)
		var base *odhmodel.ODHActivityPoi
		if inCache {
			copy := existing.Entity
			base = &copy
		}

		pois[id] = mapSlopeToPoi(slope, base)
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

func buildID(slope dto.DssSlope) string {
	return fmt.Sprintf("dss_%d", slope.Pid)
}

func mapSlopeToPoi(slope dto.DssSlope, base *odhmodel.ODHActivityPoi) odhmodel.ODHActivityPoi {
	id := buildID(slope)
	source := SOURCE
	shortname := stringFromMultilang(slope.Name, "de")
	lastChange := time.Unix(slope.UpdateDate, 0).UTC()

	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	mapping := map[string]map[string]string{
		SOURCE: {
			"pid":           strconv.FormatInt(slope.Pid, 10),
			"rid":           strconv.FormatInt(slope.Rid, 10),
			"regionId":      strconv.FormatInt(slope.RegionId, 10),
			"skiresort_rid": strconv.FormatInt(slope.Skiresort.Pid, 10),
			"skiresort_pid": strconv.FormatInt(slope.Skiresort.Rid, 10),
		},
	}

	smgTags := buildSmgTags(slope.SlopeType, slope.Slopetype)
	tagIds := buildTagIds(slope.SlopeType, slope.Slopetype)
	detail := buildDetail(slope)

	// IsOpen: slope has a single state field (not winter/summer)
	isOpen := slope.State == 1

	// Difficulty
	difficulty := parseDifficulty(slope.SlopeType, slope.Slopetype)

	// DistanceDuration
	var distDuration *float64
	if slope.Duration != "" {
		if secs, err := strconv.ParseFloat(slope.Duration, 64); err == nil && isFinite(secs) {
			hours := math.Round((secs/3600.0)*10) / 10
			if isFinite(hours) {
				distDuration = &hours
			}
		}
	}

	gpsInfo, gpsPoints := buildGps(slope)

	var gpsTrack []odhmodel.GpsTrack
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

	opSchedule := buildOperationSchedule(slope)
	var opSchedules []odhmodel.OperationSchedule
	if opSchedule != nil {
		opSchedules = append(opSchedules, *opSchedule)
	}

	additionalPoiInfos := map[string]*odhmodel.AdditionalPoiInfo{
		"de": {Novelty: "", Language: "de", Categories: []string{"Pisten"}},
		"it": {Novelty: "", Language: "it", Categories: []string{"Piste"}},
		"en": {Novelty: "", Language: "en", Categories: []string{"Slopes"}},
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
			TagIds:      tagIds,
			SmgTags:     smgTags,
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
		SmgActive:            true,
		OdhActive:            true,
		PublishedOn:          []string{},
		SyncUpdateMode:       "Full",
		SyncSourceInterface:  SYNC_INTERFACE,
		CustomId:             strconv.FormatInt(slope.Rid, 10),
		IsOpen:               isOpen,
		Number:               slope.Number,
		Difficulty:           difficulty,
		DistanceLength:       slope.Data.Length,
		DistanceDuration:     distDuration,
		AltitudeLowestPoint:  slope.Data.Altitude.Start,
		AltitudeHighestPoint: slope.Data.Altitude.End,
		AltitudeDifference:   slope.Data.HeightDifference,
		GpsTrack:             gpsTrack,
		GpsPoints:            gpsPoints,
		OperationSchedule:    opSchedules,
	}
}

// ── Difficulty ────────────────────────────────────────────────────────────────

// parseDifficulty mirrors C# ParseDSSSlopeTypeToODHDifficulty(slopeType, slopetype).
// slopeType is the color string ("blue","red","black","yellow"),
// slopetype is the numeric/code string ("1","2","3","4").
// Color takes precedence; numeric is the fallback.
func parseDifficulty(slopeType string, slopetype string) *string {
	var d string

	switch strings.ToLower(strings.TrimSpace(slopeType)) {
	case "blue", "1":
		d = "1"
	case "red", "2":
		d = "2"
	case "black", "3":
		d = "3"
	case "yellow", "4":
		d = "4"
	default:
		// Fall back to numeric slopetype field
		switch strings.TrimSpace(slopetype) {
		case "1":
			d = "1"
		case "2":
			d = "2"
		case "3":
			d = "3"
		case "4":
			d = "4"
		default:
			return nil
		}
	}

	return &d
}

// ── Tag builders ──────────────────────────────────────────────────────────────

func buildTagIds(slopeType, slopetype string) []string {
	tags := []string{"activity"}
	if t := difficultyToTagId(slopeType, slopetype); t != "" {
		tags = append(tags, t)
	}
	tags = append(tags, "slopes", "other", "other slopes")
	return tags
}

func buildSmgTags(slopeType, slopetype string) []string {
	tags := []string{
		"winter",
		"skirundtouren pisten",
		"pisten",
		"ski alpin",
		"piste",
		"weitere pisten",
	}
	if t := difficultyToSmgTag(slopeType, slopetype); t != "" {
		tags = append(tags, t)
	}
	tags = append(tags, "activity")
	return tags
}

func difficultyToTagId(slopeType, slopetype string) string {
	d := parseDifficulty(slopeType, slopetype)
	if d == nil {
		return ""
	}
	switch *d {
	case "1":
		return "slope blue"
	case "2":
		return "slope red"
	case "3":
		return "slope black"
	case "4":
		return "slope yellow"
	default:
		return ""
	}
}

func difficultyToSmgTag(slopeType, slopetype string) string {
	d := parseDifficulty(slopeType, slopetype)
	if d == nil {
		return ""
	}
	switch *d {
	case "1":
		return "blaue piste"
	case "2":
		return "rote piste"
	case "3":
		return "schwarze piste"
	case "4":
		return "gelbe piste"
	default:
		return ""
	}
}

// ── GPS builder ───────────────────────────────────────────────────────────────

// buildGps mirrors C# ParseDSSSlopeToODHGpsInfo(location, altitudeend).
// Slopes only have a single location point (valley/position), using altitudeEnd.
// No mountain station point for slopes.
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

	// Slopes use altitudeEnd for the position point (mirrors C# parser)
	entry := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              slope.Data.Altitude.End,
		AltitudeUnitofMeasure: "m",
	}

	gpsInfo = append(gpsInfo, entry)
	positionCopy := entry
	gpsPoints["position"] = &positionCopy

	return gpsInfo, gpsPoints
}

// ── OperationSchedule builder ─────────────────────────────────────────────────

// buildOperationSchedule mirrors C# ParseDSSSlopeToODHOperationScheduleFormat(dssitem).
// Slopes have a single winter schedule only; season dates live on the slope root (not data).
func buildOperationSchedule(slope dto.DssSlope) *odhmodel.OperationSchedule {
	if slope.SeasonWinter.Start == nil || slope.SeasonWinter.End == nil {
		return nil
	}

	rome, err := time.LoadLocation("Europe/Rome")
	if err != nil {
		rome = time.FixedZone("CET", 3600)
	}

	const dtFormat = "2006-01-02T00:00:00"
	start := time.Unix(*slope.SeasonWinter.Start, 0).In(rome).Format(dtFormat)
	stop := time.Unix(*slope.SeasonWinter.End, 0).In(rome).Format(dtFormat)

	os := &odhmodel.OperationSchedule{
		Stop:  stop,
		Type:  "1",
		Start: start,
		OperationscheduleName: map[string]string{
			"de": "Wintersaison",
			"it": "stagioneinvernale",
			"en": "winterseason",
		},
	}

	if slope.OpeningTimes.Start != "" && slope.OpeningTimes.End != "" {
		endTime := formatTimeWithSeconds(slope.OpeningTimes.End)
		if slope.OpeningTimes.EndAfternoon != "" {
			endTime = formatTimeWithSeconds(slope.OpeningTimes.EndAfternoon)
		}
		slot := odhmodel.OperationScheduleTime{
			Start:     formatTimeWithSeconds(slope.OpeningTimes.Start),
			End:       endTime,
			State:     0,
			Timecode:  1,
			Monday:    true,
			Tuesday:   true,
			Wednesday: true,
			Thursday:  true,
			Thuresday: true,
			Friday:    true,
			Saturday:  true,
			Sunday:    true,
		}
		os.OperationScheduleTime = []odhmodel.OperationScheduleTime{slot}
	}

	return os
}

// ── Detail builder ────────────────────────────────────────────────────────────

func buildDetail(slope dto.DssSlope) map[string]*clib.DetailGeneric {
	detail := map[string]*clib.DetailGeneric{}
	for _, lang := range []string{"de", "it", "en"} {
		title := stringFromMultilang(slope.Name, lang)
		baseText := nilableFromMultilang(slope.Description, lang)
		// AdditionalText from info-text-winter not mappable: clib.DetailGeneric
		// only exposes BaseText, Title, Language. No AdditionalText field available.

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

func formatTimeWithSeconds(t string) string {
	if t == "" {
		return t
	}
	if len(strings.Split(t, ":")) == 2 {
		return t + ":00"
	}
	return t
}

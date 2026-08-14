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

	"opendatahub.com/tr-dss-lift/dto"
	odhmodel "opendatahub.com/tr-dss-lift/odhmodel"
)

const (
	SOURCE         = "dss"
	ENTITY_TYPE    = "ODHActivityPoi"
	SYNC_INTERFACE = "dssliftbase"
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
	slog.Info("Starting DSS Lift transformer...")
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

	poiCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhmodel.ODHActivityPoi]{
		EntityType:  ENTITY_TYPE,
		QueryParams: map[string]string{"source": SOURCE, "tagfilter": "lifts"},
		IDFunc:      func(p odhmodel.ODHActivityPoi) string { return *p.Generic.ID },
	})
	ms.FailOnError(context.Background(), err, "failed to load existing lift POIs")

	slog.Info("Loaded existing lift POIs", "count", len(poiCache.Entries()))

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// Transform is called once per raw message from the collector.
func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing DSS lift feed",
		"item_count", len(r.Rawdata.DssLifts.Items))

	seen := map[string]struct{}{}
	pois := map[string]odhmodel.ODHActivityPoi{}

	for _, lift := range r.Rawdata.DssLifts.Items {
		id := buildID(lift)
		seen[id] = struct{}{}

		existing, inCache := poiCache.Get(id)
		var base *odhmodel.ODHActivityPoi
		if inCache {
			copy := existing.Entity
			base = &copy
		}

		pois[id] = mapLiftToPoi(lift, base)
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
				logger.Get(ctx).Info("Created new lift POI", "id", id)
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
			logger.Get(ctx).Info("Recovered stale-cache lift POI via PUT", "id", id)

		} else if changed {
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", err)
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Updated lift POI", "id", id)
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
			logger.Get(ctx).Error("Failed to deactivate lift POI", "id", id, "error", err)
			continue
		}
		poiCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing lift POI", "id", id)
	}

	return nil
}

// ── Mapping ───────────────────────────────────────────────────────────────────

func buildID(lift dto.DssLift) string {
	return fmt.Sprintf("dss_%d", lift.Pid)
}

func mapLiftToPoi(lift dto.DssLift, base *odhmodel.ODHActivityPoi) odhmodel.ODHActivityPoi {
	id := buildID(lift)
	source := SOURCE
	shortname := stringFromMultilang(lift.Name, "de")
	lastChange := time.Unix(lift.UpdateDate, 0).UTC()

	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	// NOTE: skiresort_rid gets Skiresort.Pid and skiresort_pid gets Skiresort.Rid.
	// This intentional swap mirrors the original C# parser exactly.
	mapping := map[string]map[string]string{
		SOURCE: {
			"pid":           strconv.FormatInt(lift.Pid, 10),
			"rid":           strconv.FormatInt(lift.Rid, 10),
			"regionId":      strconv.FormatInt(lift.RegionId, 10),
			"skiresort_rid": strconv.FormatInt(lift.Skiresort.Pid, 10),
			"skiresort_pid": strconv.FormatInt(lift.Skiresort.Rid, 10),
		},
	}

	tagIds := buildTagIds(lift.Lifttype.Rid)
	smgTags := buildSmgTags(lift.Lifttype.Rid)
	detail := buildDetail(lift)
	isOpen := lift.StateWinter == 1 || lift.StateSummer == 1

	// ── DistanceDuration ─────────────────────────────────────────────────────
	// Guard against NaN/Inf: in Go, strconv.ParseFloat("NaN", 64) succeeds and
	// returns math.NaN(), which json.Marshal cannot serialize and will panic.
	var distDuration *float64
	if lift.Duration != "" {
		if secs, err := strconv.ParseFloat(lift.Duration, 64); err == nil && isFinite(secs) {
			hours := math.Round((secs/3600.0)*10) / 10
			if isFinite(hours) {
				distDuration = &hours
			}
		}
	}

	gpsInfo, gpsPoints := buildGps(lift)

	var gpsTrack []odhmodel.GpsTrack
	if lift.GeoPositionFile != "" {
		gpsTrack = []odhmodel.GpsTrack{
			{
				Id:           nil,
				Type:         "detailed",
				Format:       "kml",
				GpxTrackUrl:  lift.GeoPositionFile,
				GpxTrackDesc: map[string]interface{}{},
			},
		}
	}

	var opSchedules []odhmodel.OperationSchedule
	if ws := buildOperationSchedule("winter", lift); ws != nil {
		opSchedules = append(opSchedules, *ws)
	}
	if ss := buildOperationSchedule("summer", lift); ss != nil {
		opSchedules = append(opSchedules, *ss)
	}

	additionalPoiInfos := map[string]*odhmodel.AdditionalPoiInfo{
		"de": {Novelty: "", Language: "de", Categories: []string{"Aufstiegsanlagen"}},
		"it": {Novelty: "", Language: "it", Categories: []string{"Impianti di risalita"}},
		"en": {Novelty: "", Language: "en", Categories: []string{"Lifts"}},
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
		CustomId:             strconv.FormatInt(lift.Rid, 10),
		IsOpen:               isOpen,
		Number:               lift.Number,
		BikeTransport:        &lift.Data.BikeTransport,
		DistanceLength:       lift.Data.Length,
		DistanceDuration:     distDuration,
		AltitudeLowestPoint:  intToFloat64(lift.Data.AltitudeStart),
		AltitudeHighestPoint: intToFloat64(lift.Data.AltitudeEnd),
		AltitudeDifference:   intToFloat64(lift.Data.HeightDifference),
		GpsTrack:             gpsTrack,
		GpsPoints:            gpsPoints,
		OperationSchedule:    opSchedules,
	}
}

// ── Float safety ──────────────────────────────────────────────────────────────

// isFinite returns true only when f is a normal, serializable float64.
// json.Marshal panics on NaN and Inf — both are valid Go float64 values
// returned by strconv.ParseFloat("NaN"/"Inf"/...) without an error.
func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// safeParseFloat parses s and returns (value, true) only when the result is
// finite. Returns (0, false) for empty strings, parse errors, NaN, and Inf.
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

// ── Tag builders ──────────────────────────────────────────────────────────────

func buildTagIds(rid int64) []string {
	tags := []string{"activity", "lifts", "other", "other lifts"}
	if t := lifttypeToTagId(rid); t != "" {
		tags = append(tags, t)
	}
	return tags
}

func buildSmgTags(rid int64) []string {
	tags := []string{"anderes", "aufstiegsanlagen", "weitere aufstiegsanlagen"}
	if t := lifttypeToSmgTag(rid); t != "" {
		tags = append(tags, t)
	}
	tags = append(tags, "activity")
	return tags
}

func lifttypeToTagId(rid int64) string {
	switch rid {
	case 1:
		return "cable car"
	case 3:
		return "gondola"
	case 4:
		return "underground ropeway"
	case 7:
		return "chairlift 2 persons"
	case 8:
		return "chairlift 3 persons"
	case 9:
		return "ski lift"
	case 10:
		return "lift"
	case 13:
		return "cable railway"
	case 14:
		return "skibus"
	case 15:
		return "train"
	case 16:
		return "chairlift 4 persons"
	case 17:
		return "chairlift 6 persons"
	case 19:
		return "moving carpet"
	case 21:
		return "chairlift 4 persons with canopy"
	case 22:
		return "chairlift 6 persons"
	case 23:
		return "chairlift 8 persons with canopy"
	default:
		return ""
	}
}

func lifttypeToSmgTag(rid int64) string {
	switch rid {
	case 1:
		return "seilbahn"
	case 3:
		return "gondelbahn"
	case 4:
		return "unterirdische seilbahn"
	case 7:
		return "2er sessellift"
	case 8:
		return "3er sessellift"
	case 9:
		return "skilift"
	case 10:
		return "lift"
	case 13:
		return "standseilbahn"
	case 14:
		return "skibus"
	case 15:
		return "zug"
	case 16:
		return "4er sessellift"
	case 17:
		return "6er sessellift"
	case 19:
		return "förderband"
	case 21:
		return "4er sessellift kuppelbar"
	case 22:
		return "6er sessellift kuppelbar"
	case 23:
		return "8er sessellift kuppelbar"
	default:
		return ""
	}
}

// ── GPS builder ───────────────────────────────────────────────────────────────

func buildGps(lift dto.DssLift) ([]odhmodel.GpsInfo, map[string]*odhmodel.GpsInfo) {
	gpsInfo := []odhmodel.GpsInfo{}
	gpsPoints := map[string]*odhmodel.GpsInfo{}

	if lift.Location == nil {
		return gpsInfo, gpsPoints
	}

	// safeParseFloat guards against NaN/Inf from malformed DSS coord strings
	lat, latOk := safeParseFloat(lift.Location.Lat)
	lon, lonOk := safeParseFloat(lift.Location.Lon)
	if !latOk || !lonOk {
		return gpsInfo, gpsPoints
	}

	// "position" and "valleystationpoint" both use valley station coords
	// GpsInfo.Altitude is *float64 — ODH API returns floats e.g. 1732.0.
	var altStart *float64
	if lift.Data.AltitudeStart != nil {
		f := float64(*lift.Data.AltitudeStart)
		altStart = &f
	}

	valleyEntry := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              altStart,
		AltitudeUnitofMeasure: "m",
	}
	valleyStation := odhmodel.GpsInfo{
		Gpstype:               "valleystationpoint",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              altStart,
		AltitudeUnitofMeasure: "m",
	}

	gpsInfo = append(gpsInfo, valleyEntry, valleyStation)
	positionCopy := valleyEntry
	valleyStationCopy := valleyStation
	gpsPoints["position"] = &positionCopy
	gpsPoints["valleystationpoint"] = &valleyStationCopy

	if lift.LocationMountain != nil {
		mlat, mlatOk := safeParseFloat(lift.LocationMountain.Lat)
		mlon, mlonOk := safeParseFloat(lift.LocationMountain.Lon)
		if mlatOk && mlonOk {
			var altEnd *float64
			if lift.Data.AltitudeEnd != nil {
				f := float64(*lift.Data.AltitudeEnd)
				altEnd = &f
			}
			mountainEntry := odhmodel.GpsInfo{
				Gpstype:               "mountainstationpoint",
				Latitude:              mlat,
				Longitude:             mlon,
				Altitude:              altEnd,
				AltitudeUnitofMeasure: "m",
			}
			gpsInfo = append(gpsInfo, mountainEntry)
			mountainCopy := mountainEntry
			gpsPoints["mountainstationpoint"] = &mountainCopy
		}
	}

	return gpsInfo, gpsPoints
}

// ── OperationSchedule builder ─────────────────────────────────────────────────

func buildOperationSchedule(season string, lift dto.DssLift) *odhmodel.OperationSchedule {
	var times dto.DssOpeningTimes
	var seasonStart, seasonEnd *int64

	if season == "winter" {
		if !lift.WinterOperation {
			return nil
		}
		times = lift.Data.OpeningTimes
		seasonStart = lift.Data.SeasonWinter.Start
		seasonEnd = lift.Data.SeasonWinter.End
	} else {
		if !lift.SummerOperation {
			return nil
		}
		times = lift.Data.OpeningTimesSummer
		seasonStart = lift.Data.SeasonSummer.Start
		seasonEnd = lift.Data.SeasonSummer.End
	}

	if seasonStart == nil || seasonEnd == nil {
		return nil
	}

	const dtFormat = "2006-01-02T00:00:00"
	start := time.Unix(*seasonStart, 0).UTC().Format(dtFormat)
	stop := time.Unix(*seasonEnd, 0).UTC().Format(dtFormat)

	nameDE, nameIT, nameEN := "Wintersaison", "stagioneinvernale", "winterseason"
	if season == "summer" {
		nameDE, nameIT, nameEN = "Sommersaison", "stagioneestiva", "summerseason"
	}

	os := &odhmodel.OperationSchedule{
		Stop:  stop,
		Type:  "1",
		Start: start,
		OperationscheduleName: map[string]string{
			"de": nameDE,
			"it": nameIT,
			"en": nameEN,
		},
	}

	if times.Start != "" && times.End != "" {
		endTime := formatTimeWithSeconds(times.End)
		if times.EndAfternoon != "" {
			endTime = formatTimeWithSeconds(times.EndAfternoon)
		}
		slot := odhmodel.OperationScheduleTime{
			Start:     formatTimeWithSeconds(times.Start),
			End:       endTime,
			State:     0,
			Timecode:  1,
			Monday:    true,
			Tuesday:   true,
			Wednesday: true,
			Thursday:  true,
			Thuresday: true, // ODH typo — must be set alongside Thursday
			Friday:    true,
			Saturday:  true,
			Sunday:    true,
		}
		os.OperationScheduleTime = []odhmodel.OperationScheduleTime{slot}
	}

	return os
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

// ── Detail builder ────────────────────────────────────────────────────────────

func buildDetail(lift dto.DssLift) map[string]*clib.DetailGeneric {
	detail := map[string]*clib.DetailGeneric{}
	for _, lang := range []string{"de", "it", "en"} {
		title := stringFromMultilang(lift.Name, lang)
		baseText := nilableFromMultilang(lift.Description, lang)

		// AdditionalText (info-text / info-text-summer) is set in the C# parser
		// but clib.DetailGeneric only exposes BaseText, Title, and Language.
		// Cannot be mapped until the SDK struct is extended.

		langCopy := lang
		detail[lang] = &clib.DetailGeneric{
			Language: &langCopy,
			Title:    &title,
			BaseText: baseText,
		}
	}
	return detail
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

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
	}, clib.WithReferer(env.ODH_CORE_REFERER))
	ms.FailOnError(context.Background(), err, "failed to create ODH content client")

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

// Transform is called once per raw message from the collector.
func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing DSS lift feed",
		"item_count", len(r.Rawdata.DssLifts.Items))

	// Load the existing lifts on every run, so changes made in the API since the
	// last run (manual edits, other importers, deletions) are detected.
	if poiCache == nil {
		var err error
		poiCache, err = clib.LoadExisting(ctx, contentClient, clib.LoadConfig[odhmodel.ODHActivityPoi]{
			EntityType:  ENTITY_TYPE,
			QueryParams: map[string]string{"source": SOURCE, "tagfilter": "lifts"},
			IDFunc:      func(p odhmodel.ODHActivityPoi) string { return *p.Generic.ID },
		})
		if err != nil {
			return fmt.Errorf("failed to load lift POI cache: %w", err)
		}
		logger.Get(ctx).Info("Loaded existing lift POIs", "count", len(poiCache.Entries()))
	}
	defer func() { poiCache = nil }()

	seen := map[string]struct{}{}
	pois := map[string]odhmodel.ODHActivityPoi{}

	for _, lift := range r.Rawdata.DssLifts.Items {
		lift = withGeoFileFallback(ctx, lift)
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

// buildID: urn:odhactivitypoi:dss:lift:<regionId>_<pid>. pid alone is not unique across DSS
// exports (snowpark pids overlap lift pids), so the type is part of the ID.
func buildID(lift dto.DssLift) string {
	return fmt.Sprintf("urn:odhactivitypoi:%s:lift:%d_%d", SOURCE, lift.RegionId, lift.Pid)
}

func mapLiftToPoi(lift dto.DssLift, base *odhmodel.ODHActivityPoi) odhmodel.ODHActivityPoi {
	id := buildID(lift)
	source := SOURCE
	shortname := nameWithFallback(lift.Name, "de")
	lastChange := time.Unix(lift.UpdateDate, 0).UTC()

	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	mapping := mergeMapping(base, map[string]string{
		"pid":                         strconv.FormatInt(lift.Pid, 10),
		"rid":                         strconv.FormatInt(lift.Rid, 10),
		"regionId":                    strconv.FormatInt(lift.RegionId, 10),
		"skiresort_rid":               strconv.FormatInt(lift.Skiresort.Rid, 10),
		"skiresort_pid":               strconv.FormatInt(lift.Skiresort.Pid, 10),
		"skiarea_rid":                 skiAreaRid(lift),
		"lifttype_rid":                nonZeroInt(lift.Lifttype.Rid),
		"datacenterId":                strings.TrimSpace(lift.DatacenterId),
		"capacity":                    intPtrString(lift.Data.Capacity),
		"capacity_per_hour":           intPtrString(lift.Data.CapacityPerHour),
		"summercard_points_up":        intPtrString(lift.Data.SummercardPoints.Up),
		"summercard_points_down":      intPtrString(lift.Data.SummercardPoints.Down),
		"summercard_points_roundtrip": intPtrString(lift.Data.SummercardPoints.Roundtrip),
	})

	tagIds := buildTagIds(lift.Lifttype.Rid)
	smgTags := buildSmgTags(lift.Lifttype.Rid)
	detail := buildDetail(lift)
	isOpen := lift.StateWinter == 1 || lift.StateSummer == 1

	// ── DistanceDuration ─────────────────────────────────────────────────────
	// Guard against NaN/Inf: in Go, strconv.ParseFloat("NaN", 64) succeeds and
	// returns math.NaN(), which json.Marshal cannot serialize and will panic.
	// Seconds → hours, rounded to 2 decimals; 0 when missing (as C#).
	distDuration := new(float64)
	if lift.Duration != "" {
		if secs, err := strconv.ParseFloat(lift.Duration, 64); err == nil && isFinite(secs) {
			hours := math.RoundToEven((secs/3600.0)*100) / 100 // C# Math.Round rounds midpoints to even
			if isFinite(hours) {
				distDuration = &hours
			}
		}
	}

	gpsInfo, gpsPoints := buildGps(lift)

	gpsTrack := []odhmodel.GpsTrack{}
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

	opSchedules := []odhmodel.OperationSchedule{}
	if ws := buildOperationSchedule("winter", lift); ws != nil {
		opSchedules = append(opSchedules, *ws)
	}
	if ss := buildOperationSchedule("summer", lift); ss != nil {
		opSchedules = append(opSchedules, *ss)
	}

	additionalPoiInfos := map[string]*odhmodel.AdditionalPoiInfo{
		"de": {Novelty: stringFromMultilang(lift.InfoText, "de"), Language: "de", Categories: []string{"Aufstiegsanlagen"}},
		"it": {Novelty: stringFromMultilang(lift.InfoText, "it"), Language: "it", Categories: []string{"Impianti di risalita"}},
		"en": {Novelty: stringFromMultilang(lift.InfoText, "en"), Language: "en", Categories: []string{"Lifts"}},
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

// skiAreaRid returns the DSS talschaft rid (SkiArea Mapping.dss.rid) of a lift:
// subregionId for the split regions (e.g. "4a"), otherwise the regionId.
func skiAreaRid(lift dto.DssLift) string {
	if sub := strings.TrimSpace(lift.SubregionId); sub != "" && sub != "all" {
		return sub
	}
	return nonZeroInt(lift.RegionId)
}

func nonZeroInt(i int64) string {
	if i == 0 {
		return ""
	}
	return strconv.FormatInt(i, 10)
}

func intPtrString(i *int) string {
	if i == nil {
		return ""
	}
	return strconv.Itoa(*i)
}

// ── Tag builders ──────────────────────────────────────────────────────────────

// liftTypeTags maps DSS lifttype.rid to its SmgTag (ODHTag) and TagIds, as the
// C# importer produced them. All of them exist on the ODHTag / Tag endpoints.
var liftTypeTags = map[int64]struct {
	smgTag string
	tagIds []string
}{
	1:  {"seilbahn", []string{"ropeway"}},
	3:  {"kabinenbahn", []string{"cabinet train", "gondola lift"}},
	4:  {"unterirdische bahn", []string{"underground train"}},
	7:  {"sessellift", []string{"chairlift"}}, // Sessellift 2
	8:  {"sessellift", []string{"chairlift"}}, // Sessellift 3
	9:  {"skilift", []string{"ski lift"}},
	10: {"schrägaufzug", []string{"inclined elevator"}},
	11: {"klein-skilift", []string{"small ski lift"}},
	12: {"telemix", []string{"telemix"}},
	13: {"standseilbahn zahnradbahn", []string{"funicular railwaycog railway"}},
	14: {"skibus", []string{"skibus"}},
	15: {"zug", []string{"train"}},
	16: {"sessellift", []string{"chairlift"}}, // Sessellift 4
	17: {"sessellift", []string{"chairlift"}}, // Sessellift 6
	18: {"sessellift", []string{"chairlift"}}, // Sessellift 8
	19: {"förderband", []string{"moving carpet"}},
	21: {"4er sessellift kuppelbar", []string{"chairlift 4 persons"}},
	22: {"6er sessellift kuppelbar", []string{"chairlift 6 persons"}},
	23: {"8er sessellift kuppelbar", []string{"chairlift 8 persons"}},
	24: {"seilbahn", []string{"ropeway"}}, // 3S Bahn
}

// buildTagIds returns the tags sorted, as the API stores them; otherwise the
// hash of an unchanged lift would differ from the loaded one on every run.
func buildTagIds(rid int64) []string {
	tags := []string{"activity", "lifts", "other", "other lifts"}
	if t, ok := liftTypeTags[rid]; ok {
		tags = append(tags, t.tagIds...)
	}
	sort.Strings(tags)
	return tags
}

func buildSmgTags(rid int64) []string {
	tags := []string{"anderes", "aufstiegsanlagen", "weitere aufstiegsanlagen"}
	if t, ok := liftTypeTags[rid]; ok {
		tags = append(tags, t.smgTag)
	}
	tags = append(tags, "activity")
	return tags
}

// ── GPS builder ───────────────────────────────────────────────────────────────

// withGeoFileFallback fills a missing location from the geoPositionFile track:
// first point = valley station, last point = mountain station (only if that is
// missing too). DSS coordinates are never overwritten.
func withGeoFileFallback(ctx context.Context, lift dto.DssLift) dto.DssLift {
	if hasValidLocation(lift.Location) || lift.GeoPositionFile == "" {
		return lift
	}
	points, err := geoFiles.Points(ctx, lift.GeoPositionFile)
	if err != nil {
		logger.Get(ctx).Warn("No GPS fallback from geoPositionFile", "pid", lift.Pid, "url", lift.GeoPositionFile, "error", err)
		return lift
	}
	lift.Location = toDssLocation(points[0])
	if !hasValidLocation(lift.LocationMountain) {
		lift.LocationMountain = toDssLocation(points[len(points)-1])
	}
	return lift
}

func hasValidLocation(loc *dto.DssLocation) bool {
	if loc == nil {
		return false
	}
	_, latOk := safeParseFloat(loc.Lat)
	_, lonOk := safeParseFloat(loc.Lon)
	return latOk && lonOk
}

func toDssLocation(p geoPoint) *dto.DssLocation {
	return &dto.DssLocation{Lat: formatCoordinate(p.Lat), Lon: formatCoordinate(p.Lon)}
}

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
	start := time.Unix(*seasonStart, 0).In(romeLocation).Format(dtFormat)
	stop := time.Unix(*seasonEnd, 0).In(romeLocation).Format(dtFormat)

	nameDE, nameIT, nameEN := "Wintersaison", "stagioneinvernale", "winterseason"
	if season == "summer" {
		nameDE, nameIT, nameEN = "Sommersaison", "stagioneestiva", "summerseason"
	}

	os := &odhmodel.OperationSchedule{
		Stop:                  stop,
		OperationScheduleTime: []odhmodel.OperationScheduleTime{},
		Type:                  "1",
		Start:                 start,
		OperationscheduleName: map[string]string{
			"de": nameDE,
			"it": nameIT,
			"en": nameEN,
		},
	}

	// Lifts with a lunch break have a second (afternoon) slot; "00:00" means not set.
	if times.Start != "" && times.End != "" {
		os.OperationScheduleTime = append(os.OperationScheduleTime, buildOperationScheduleTime(times.Start, times.End))
	}
	if times.StartAfternoon != "" && times.EndAfternoon != "" &&
		times.StartAfternoon != "00:00" && times.EndAfternoon != "00:00" {
		os.OperationScheduleTime = append(os.OperationScheduleTime, buildOperationScheduleTime(times.StartAfternoon, times.EndAfternoon))
	}

	return os
}

func buildOperationScheduleTime(start, end string) odhmodel.OperationScheduleTime {
	return odhmodel.OperationScheduleTime{
		Start:     formatTimeWithSeconds(start),
		End:       formatTimeWithSeconds(end),
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

func buildDetail(lift dto.DssLift) map[string]*odhmodel.Detail {
	detail := map[string]*odhmodel.Detail{}
	for _, lang := range []string{"de", "it", "en"} {
		title := nameWithFallback(lift.Name, lang)
		baseText := nilableFromMultilang(lift.Description, lang)

		// AdditionalText: winter info-text, falling back to the summer one.
		additionalText := nilableFromMultilang(lift.InfoText, lang)
		if additionalText == nil {
			additionalText = nilableFromMultilang(lift.InfoTextSummer, lang)
		}

		langCopy := lang
		detail[lang] = &odhmodel.Detail{
			DetailGeneric: clib.DetailGeneric{
				Language: &langCopy,
				Title:    &title,
				BaseText: baseText,
			},
			AdditionalText: additionalText,
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

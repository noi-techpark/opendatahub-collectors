// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
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

	"opendatahub.com/tr-dss-skiareas/dto"
	odhmodel "opendatahub.com/tr-dss-skiareas/odhmodel"
)

const (
	SOURCE         = "dss"
	ENTITY_TYPE    = "SkiArea"
	SYNC_INTERFACE = "dssskiarea"
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
var skiAreaCache *clib.Cache[odhmodel.SkiArea]
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
	slog.Info("Starting DSS SkiArea transformer...")
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

// Transform processes the full talschaften feed.
// For each DSS skiarea:
//   - Query ODH by Mapping.dss.rid
//   - If found (0..N): update OperationSchedule only, preserve everything else
//   - If not found:    create new SkiArea with DSS name, contact, seasons
func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing DSS skiarea feed",
		"item_count", len(r.Rawdata.DssSkiAreas.Items))

	for _, dssArea := range r.Rawdata.DssSkiAreas.Items {
		if err := processSkiArea(ctx, dssArea); err != nil {
			// Log and continue — one bad area must not abort the whole feed
			logger.Get(ctx).Error("Failed to process skiarea",
				"rid", dssArea.Rid, "error", err)
		}
	}

	return nil
}

// processSkiArea handles the upsert logic for one DSS talschaft.
func processSkiArea(ctx context.Context, dssArea dto.DssSkiArea) error {
	log := logger.Get(ctx).With("rid", dssArea.Rid)

	existing, err := findByDssRid(ctx, dssArea.Rid)
	if err != nil {
		return fmt.Errorf("ODH lookup failed: %w", err)
	}

	opSchedules := buildOperationSchedules(dssArea)

	if len(existing) == 0 {
		// ── CREATE ────────────────────────────────────────────────────────────
		log.Info("No existing SkiArea found — creating new")
		newArea := buildNewSkiArea(dssArea, opSchedules)
		applyRegionGeo(ctx, &newArea, dssArea.RegionMap)
		if err := contentClient.Post(ctx, ENTITY_TYPE,
			map[string]string{"generateid": "false"}, newArea); err != nil {
			return fmt.Errorf("POST failed: %w", err)
		}
		log.Info("Created new SkiArea", "id", *newArea.Id)
		return nil
	}

	// ── UPDATE — apply to all matched SkiAreas ────────────────────────────
	// Per senior: "if 2 skiareas found, do this on both"
	// We replace ONLY OperationSchedule, Mapping (dss keys), Geo track and LastChange.
	// ContactInfos is json.RawMessage — round-tripped unchanged ✓
	// PublishedOn, LicenseInfo — preserved from existing record ✓
	// Detail, TagIds, SmgTags, GpsInfo etc. — preserved via struct fields ✓
	//
	// Change detection: the record is hashed as loaded from the API and again after
	// our changes; it is only PUT (with a new LastChange) when the hashes differ.
	for _, area := range existing {
		id := *area.Id

		before, err := clib.HashEntity(area)
		if err != nil {
			log.Error("Failed to hash SkiArea", "id", id, "error", err)
			continue
		}

		area.OperationSchedule = opSchedules
		applyRegionGeo(ctx, &area, dssArea.RegionMap)

		// Keep existing Mapping (idm etc.) and existing dss keys, only add/overwrite ours.
		area.Mapping = mergeMapping(area.Mapping, dssMapping(dssArea))

		after, err := clib.HashEntity(area)
		if err != nil {
			log.Error("Failed to hash SkiArea", "id", id, "error", err)
			continue
		}
		if before == after {
			continue
		}

		area.LastChange = odhmodel.PtrFlexibleTime(nowFunc())
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, area); err != nil {
			log.Error("PUT failed", "id", id, "error", err)
			continue
		}
		log.Info("Updated SkiArea OperationSchedule", "id", id)
	}

	return nil
}

// applyRegionGeo adds the regionMap outline to the SkiArea:
//   - Geo["track"]: the outline as WKT (LINESTRING / MULTILINESTRING)
//   - only if GpsInfo is empty: the centroid of the outline as GpsInfo "position"
//     and Geo["position"], both as the default entry
//
// Existing GpsInfo and Geo entries are kept unchanged. The API requires exactly
// one default Geo entry, so the track only becomes the default when no other
// entry is one. On any download/parse error the SkiArea is left unchanged.
func applyRegionGeo(ctx context.Context, area *odhmodel.SkiArea, regionMap string) {
	if regionMap == "" {
		return
	}
	track, err := geoFiles.Track(ctx, regionMap)
	if err != nil {
		logger.Get(ctx).Warn("No Geo track from regionMap", "url", regionMap, "error", err)
		return
	}

	geo := make(map[string]json.RawMessage, len(area.Geo)+2)
	for key, value := range area.Geo {
		geo[key] = value
	}

	if !hasGpsInfo(area.GpsInfo) {
		position := map[string]any{
			"Gpstype":   "position",
			"Latitude":  track.Center.Lat,
			"Longitude": track.Center.Lon,
			"Geometry":  "POINT (" + formatCoordinate(track.Center.Lon) + " " + formatCoordinate(track.Center.Lat) + ")",
			"Default":   true,
		}
		gpsInfo, errGps := json.Marshal([]any{position})
		rawPosition, errPos := json.Marshal(position)
		if errGps == nil && errPos == nil {
			area.GpsInfo = gpsInfo
			geo["position"] = rawPosition
		}
	}

	hasDefault := false
	for key, raw := range geo {
		var entry struct {
			Default *bool `json:"Default"`
		}
		if key != "track" && json.Unmarshal(raw, &entry) == nil && entry.Default != nil && *entry.Default {
			hasDefault = true
		}
	}

	// Keep the stored track when it is unchanged: the API stores it with extra null
	// keys, so the newly marshaled entry would differ byte-wise on every run.
	if !sameTrack(geo["track"], track.WKT, !hasDefault) {
		trackEntry := map[string]any{"Geometry": track.WKT}
		if !hasDefault {
			trackEntry["Default"] = true
		}
		if raw, err := json.Marshal(trackEntry); err == nil {
			geo["track"] = raw
		}
	}
	area.Geo = geo
}

// sameTrack reports whether the raw Geo track entry has the given geometry and default flag.
func sameTrack(raw json.RawMessage, wkt string, isDefault bool) bool {
	var entry struct {
		Default  *bool  `json:"Default"`
		Geometry string `json:"Geometry"`
	}
	if raw == nil || json.Unmarshal(raw, &entry) != nil {
		return false
	}
	return entry.Geometry == wkt && (entry.Default != nil && *entry.Default) == isDefault
}

// hasGpsInfo reports whether the raw GpsInfo holds at least one entry.
func hasGpsInfo(raw json.RawMessage) bool {
	var entries []json.RawMessage
	return json.Unmarshal(raw, &entries) == nil && len(entries) > 0
}

// findByDssRid queries ODH SkiArea filtered by Mapping.dss.rid.
// Uses rawfilter + pagenumber=1 as required by the SkiArea endpoint.
func findByDssRid(ctx context.Context, rid string) ([]odhmodel.SkiArea, error) {
	rawFilter := fmt.Sprintf("eq(Mapping.dss.rid,'%s')", rid)

	var page odhmodel.SkiAreaPage
	if err := contentClient.Get(ctx, ENTITY_TYPE,
		map[string]string{
			"rawfilter":  rawFilter,
			"pagenumber": "1",
		}, &page); err != nil {
		return nil, fmt.Errorf("GET SkiArea rawfilter failed: %w", err)
	}

	return page.Items, nil
}

// buildOperationSchedules produces winter + summer OperationSchedule entries.
// Skips any season where start or end is nil.
func buildOperationSchedules(dssArea dto.DssSkiArea) []odhmodel.OperationSchedule {
	const dtFormat = "2006-01-02T00:00:00"
	var schedules []odhmodel.OperationSchedule

	if dssArea.SeasonWinter.Start != nil && dssArea.SeasonWinter.End != nil {
		start := time.Unix(*dssArea.SeasonWinter.Start, 0).In(romeLocation).Format(dtFormat)
		stop := time.Unix(*dssArea.SeasonWinter.End, 0).In(romeLocation).Format(dtFormat)
		schedules = append(schedules, odhmodel.OperationSchedule{
			Type:                  "1",
			Start:                 start,
			Stop:                  stop,
			OperationScheduleTime: []odhmodel.OperationScheduleTime{},
			OperationscheduleName: map[string]string{
				"de": "Wintersaison",
				"it": "stagioneinvernale",
				"en": "winterseason",
			},
		})
	}

	if dssArea.SeasonSummer.Start != nil && dssArea.SeasonSummer.End != nil {
		start := time.Unix(*dssArea.SeasonSummer.Start, 0).In(romeLocation).Format(dtFormat)
		stop := time.Unix(*dssArea.SeasonSummer.End, 0).In(romeLocation).Format(dtFormat)
		schedules = append(schedules, odhmodel.OperationSchedule{
			Type:                  "1",
			Start:                 start,
			Stop:                  stop,
			OperationScheduleTime: []odhmodel.OperationScheduleTime{},
			OperationscheduleName: map[string]string{
				"de": "Sommersaison",
				"it": "stagioneestiva",
				"en": "summerseason",
			},
		})
	}

	return schedules
}

// buildNewSkiArea constructs a full new SkiArea for the CREATE path.
// Sets name (de/it/en), contact info, seasons, source="dss".
// Does NOT set rich Detail beyond Title — per senior instructions.
func buildNewSkiArea(dssArea dto.DssSkiArea, opSchedules []odhmodel.OperationSchedule) odhmodel.SkiArea {
	id := buildID(dssArea)
	source := SOURCE
	shortname := stringVal(dssArea.Name.De)

	// HasLanguage + Detail: only for languages where DSS name is non-empty
	hasLanguage := []string{}
	detail := map[string]*clib.DetailGeneric{}
	for _, lang := range []string{"de", "it", "en"} {
		name := stringFromMultilang(dssArea.Name, lang)
		if name == "" {
			continue
		}
		hasLanguage = append(hasLanguage, lang)
		langCopy := lang
		detail[lang] = &clib.DetailGeneric{
			Language: &langCopy,
			Title:    &name,
		}
	}

	// ContactInfos: marshal our minimal ContactInfo into json.RawMessage
	// so the field type is consistent with the UPDATE path (which uses RawMessage).
	contactInfos := map[string]json.RawMessage{}
	for _, lang := range hasLanguage {
		ci := odhmodel.ContactInfo{
			Language:    lang,
			Email:       strings.TrimSpace(dssArea.Email.TouristBoard),
			Phonenumber: strings.TrimSpace(dssArea.Phone),
		}
		raw, err := json.Marshal(ci)
		if err != nil {
			continue
		}
		contactInfos[lang] = raw
	}

	now := nowFunc()

	return odhmodel.SkiArea{
		Id:                &id,
		Active:            true,
		Source:            &source,
		Shortname:         &shortname,
		HasLanguage:       hasLanguage,
		FirstImport:       odhmodel.PtrFlexibleTime(now),
		LastChange:        odhmodel.PtrFlexibleTime(now),
		Mapping:           mergeMapping(nil, dssMapping(dssArea)),
		Detail:            detail,
		ContactInfos:      contactInfos,
		OperationSchedule: opSchedules,
		SmgActive:         true,
		OdhActive:         true,
		// PublishedOn: [] on new DSS record (not published on idm-marketplace)
		PublishedOn:         []string{},
		SyncUpdateMode:      "Full",
		SyncSourceInterface: SYNC_INTERFACE,
		// LicenseInfo: set on CREATE only; preserved from existing record on UPDATE
		LicenseInfo: &odhmodel.LicenseInfo{
			Author:        "",
			License:       "CC0",
			LicenseHolder: LICENSE_HOLDER,
			ClosedData:    false,
		},
	}
}

// dssMapping returns the dss Mapping keys of a talschaft.
func dssMapping(dssArea dto.DssSkiArea) map[string]string {
	resorts := []string{}
	resortPids := []string{}
	for _, resort := range dssArea.Skiresorts {
		if resort.Rid != 0 {
			resorts = append(resorts, strconv.FormatInt(resort.Rid, 10))
		}
		if resort.Pid != 0 {
			resortPids = append(resortPids, strconv.FormatInt(resort.Pid, 10))
		}
	}
	return map[string]string{
		"rid":            dssArea.Rid,
		"pid":            strings.TrimSpace(dssArea.Pid),
		"skiresort_rids": strings.Join(resorts, ","),
		"skiresort_pids": strings.Join(resortPids, ","),
		"activeWinter":   strconv.FormatBool(dssArea.ActiveWinter != 0),
		"activeBike":     strconv.FormatBool(dssArea.ActiveBike != 0),
		"activeHike":     strconv.FormatBool(dssArea.ActiveHike != 0),
		"email_lifts":    strings.TrimSpace(dssArea.Email.Lifts),
	}
}

// mergeMapping keeps the existing Mapping of the record (other sources and dss
// keys set elsewhere) and only adds/overwrites the given dss keys. Empty values
// are skipped, so a missing DSS field never clears an existing key.
func mergeMapping(existing map[string]map[string]string, dss map[string]string) map[string]map[string]string {
	mapping := map[string]map[string]string{}
	for source, values := range existing {
		copied := make(map[string]string, len(values))
		for k, v := range values {
			copied[k] = v
		}
		mapping[source] = copied
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

// buildID generates a deterministic ODH ID for new DSS SkiArea records.
// Format: "dss_skiarea_<rid>" — rid kept as-is (string, e.g. "4a").
func buildID(dssArea dto.DssSkiArea) string {
	return fmt.Sprintf("dss_skiarea_%s", dssArea.Rid)
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
	return stringVal(ptr)
}

func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

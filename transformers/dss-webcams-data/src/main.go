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

	"opendatahub.com/tr-dss-webcams/dto"
	odhmodel "opendatahub.com/tr-dss-webcams/odhmodel"
)

const (
	SOURCE         = "dss"
	ENTITY_TYPE    = "WebcamInfo"
	SYNC_INTERFACE = "dsswebcambase"
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
var webcamCache *clib.Cache[odhmodel.WebcamInfo]
var nowFunc = func() time.Time { return time.Now().UTC() }

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting DSS Webcam transformer...")
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
	logger.Get(ctx).Info("Processing DSS webcam feed",
		"item_count", len(r.Rawdata.DssWebcams.Items))

	if webcamCache == nil {
		var err error
		webcamCache, err = clib.LoadExisting(ctx, contentClient, clib.LoadConfig[odhmodel.WebcamInfo]{
			EntityType:  ENTITY_TYPE,
			QueryParams: map[string]string{"source": SOURCE},
			IDFunc:      func(w odhmodel.WebcamInfo) string { return strings.ToUpper(*w.Id) },
		})
		if err != nil {
			return fmt.Errorf("failed to load webcam cache: %w", err)
		}
		logger.Get(ctx).Info("Loaded existing webcams", "count", len(webcamCache.Entries()))
	}
	defer func() { webcamCache = nil }()

	seen := map[string]struct{}{}
	webcams := map[string]odhmodel.WebcamInfo{}

	skiAreas := newSkiAreaResolver(r.Rawdata.DssSkiAreas)
	for _, cam := range r.Rawdata.DssWebcams.Items {
		id := buildID(cam)
		seen[id] = struct{}{}

		existing, inCache := webcamCache.Get(id)
		var base *odhmodel.WebcamInfo
		if inCache {
			copy := existing.Entity
			base = &copy
		}

		webcams[id] = mapWebcamToODH(cam, base, skiAreas.rid(cam))
	}

	// Stable iteration order
	sortedIDs := make([]string, 0, len(webcams))
	for id := range webcams {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		cam := webcams[id]

		hash, changed, err := webcamCache.HasChanged(id, cam)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash webcam", "id", id, "error", err)
			continue
		}

		_, exists := webcamCache.Get(id)

		if !exists {
			postErr := contentClient.Post(ctx, ENTITY_TYPE, map[string]string{"generateid": "false"}, cam)
			if postErr == nil {
				webcamCache.Set(id, cam, hash)
				logger.Get(ctx).Info("Created new webcam", "id", id)
				continue
			}
			if !strings.Contains(postErr.Error(), "data exists already") {
				logger.Get(ctx).Error("API Post failed", "id", id, "error", postErr)
				continue
			}
			logger.Get(ctx).Warn("POST returned 'data exists already', recovering with PUT", "id", id)
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, cam); err != nil {
				logger.Get(ctx).Error("API Put failed (recovery)", "id", id, "error", err)
				continue
			}
			webcamCache.Set(id, cam, hash)
			logger.Get(ctx).Info("Recovered stale-cache webcam via PUT", "id", id)

		} else if changed {
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, cam); err != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", err)
				continue
			}
			webcamCache.Set(id, cam, hash)
			logger.Get(ctx).Info("Updated webcam", "id", id)
		}
	}

	// ── DEACTIVATION ─────────────────────────────────────────────────────────
	for id := range webcamCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}
		entry, stillExists := webcamCache.Get(id)
		if !stillExists {
			continue
		}
		cam := entry.Entity
		cam.Active = false
		cam.SmgActive = false
		cam.OdhActive = false
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, cam); err != nil {
			logger.Get(ctx).Error("Failed to deactivate webcam", "id", id, "error", err)
			continue
		}
		webcamCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing webcam", "id", id)
	}

	return nil
}

// ── ID ────────────────────────────────────────────────────────────────────────

// buildID mirrors the C# parser: "DSS_" + pid. The API stores WebcamInfo Ids in
// uppercase, so the ID must be uppercase too, otherwise no feed webcam matches the
// cache and the deactivation step deactivates every webcam.
func buildID(cam dto.DssWebcam) string {
	return fmt.Sprintf("DSS_%d", cam.Pid)
}

// ── Main mapper ───────────────────────────────────────────────────────────────

func mapWebcamToODH(cam dto.DssWebcam, base *odhmodel.WebcamInfo, skiAreaRid string) odhmodel.WebcamInfo {
	id := buildID(cam)
	source := SOURCE

	// Shortname: first non-empty name across de/it/en
	shortname := firstNonEmpty(
		stringVal(cam.Name.De),
		stringVal(cam.Name.It),
		stringVal(cam.Name.En),
	)

	// Preserve FirstImport from cache; set now only on first create
	var firstImport *odhmodel.FlexibleTime
	if base != nil && base.FirstImport != nil {
		firstImport = base.FirstImport
	} else {
		firstImport = odhmodel.PtrFlexibleTime(nowFunc())
	}

	// ── Mapping ───────────────────────────────────────────────────────────────
	// C# parser:
	//   - always adds pid
	//   - only adds rid if rid != 0 (all live records have rid=0, so skipped)
	//   - only adds feratelId if non-empty
	//   - always adds skiresort (plain string)
	//   - existing Mapping entries are kept, dss keys are only added/overwritten
	dssMap := map[string]string{
		"pid":         strconv.FormatInt(cam.Pid, 10),
		"skiresort":   cam.Skiresort,
		"regionId":    strconv.FormatInt(cam.RegionId, 10),
		"skiarea_rid": skiAreaRid,
		"webcamType":  cam.WebcamType,
	}
	if cam.Rid != 0 {
		dssMap["rid"] = strconv.FormatInt(cam.Rid, 10)
	}
	if cam.FeratelId != "" {
		dssMap["feratelId"] = cam.FeratelId
	}
	dssMap["showOnSummer"] = strconv.FormatBool(cam.ShowOnSummer)
	var baseMapping map[string]map[string]string
	if base != nil {
		baseMapping = base.Mapping
	}
	mapping := mergeMapping(baseMapping, dssMap)

	// ── Detail + HasLanguage + Webcamname ─────────────────────────────────────
	// C# parser: only adds Detail entry if name is non-empty.
	detail := map[string]*clib.DetailGeneric{}
	webcamname := map[string]string{}
	hasLanguage := []string{}

	for _, lang := range []string{"de", "it", "en"} {
		name := stringFromMultilang(cam.Name, lang)
		if name == "" {
			continue
		}
		langCopy := lang
		detail[lang] = &clib.DetailGeneric{
			Language: &langCopy,
			Title:    &name,
		}
		webcamname[lang] = name
		hasLanguage = append(hasLanguage, lang)
	}

	// ── GPS ───────────────────────────────────────────────────────────────────
	gpsInfo, gpsPoints := buildGps(cam)

	// ── Image gallery ─────────────────────────────────────────────────────────
	// C# parser: adds one entry with webcamurl as ImageUrl, name as ImageName.
	var imageGallery []odhmodel.ImageGalleryEntry
	if cam.OriginalImage != "" {
		listPos := 0
		imageGallery = []odhmodel.ImageGalleryEntry{
			{
				ImageUrl:     cam.OriginalImage,
				ImageName:    shortname,
				ImageDesc:    map[string]string{},
				ImageTitle:   map[string]string{},
				ImageAltText: map[string]string{},
				ImageSource:  SOURCE,
				IsInGallery:  true,
				ListPosition: &listPos,
			},
		}
	}

	// ── WebCamProperties ─────────────────────────────────────────────────────
	// WebcamUrl  → original-image (static snapshot)
	// StreamUrl  → iframe["it"] if non-empty (C# parser uses only "it")
	var webcamUrl, streamUrl *string
	if cam.OriginalImage != "" {
		webcamUrl = &cam.OriginalImage
	}
	iframeIt := stringVal(cam.Iframe.It)
	if iframeIt != "" {
		streamUrl = &iframeIt
	}

	props := &odhmodel.WebCamProperties{
		WebcamUrl: webcamUrl,
		StreamUrl: streamUrl,
	}

	// Top-level convenience mirrors (match ODH WebcamInfo shape)
	var webcamUrlTop, streamUrlTop *string
	if webcamUrl != nil {
		u := *webcamUrl
		webcamUrlTop = &u
	}
	if streamUrl != nil {
		s := *streamUrl
		streamUrlTop = &s
	}

	return odhmodel.WebcamInfo{
		Id:               &id,
		WebcamId:         id,
		Shortname:        &shortname,
		Source:           &source,
		Active:           true,
		SmgActive:        true,
		OdhActive:        true,
		FirstImport:      firstImport,
		HasLanguage:      hasLanguage,
		Mapping:          mapping,
		Detail:           detail,
		Webcamname:       webcamname,
		ContactInfos:     map[string]interface{}{},
		Webcamurl:        webcamUrlTop,
		Streamurl:        streamUrlTop,
		GpsInfo:          gpsInfo,
		GpsPoints:        gpsPoints,
		ImageGallery:     imageGallery,
		WebCamProperties: props,
		PublishedOn:      []string{},
		LicenseInfo: &odhmodel.LicenseInfo{
			Author:        "",
			License:       "CC0",
			LicenseHolder: LICENSE_HOLDER,
			ClosedData:    false,
		},
	}
}

// ── GPS builder ───────────────────────────────────────────────────────────────

// buildGps builds GpsInfo and GpsPoints for a webcam.
// Webcam location lat/lon are already float64 (unlike lifts/slopes which are strings).
// Uses cam.Altitude (int) for the altitude field.
func buildGps(cam dto.DssWebcam) ([]odhmodel.GpsInfo, map[string]*odhmodel.GpsInfo) {
	gpsInfo := []odhmodel.GpsInfo{}
	gpsPoints := map[string]*odhmodel.GpsInfo{}

	if cam.Location == nil {
		return gpsInfo, gpsPoints
	}

	// Webcam coords are already float64 — still guard against NaN/Inf
	if !isFinite(cam.Location.Lat) || !isFinite(cam.Location.Lon) {
		return gpsInfo, gpsPoints
	}

	// GpsInfo.Altitude is *float64 — ODH API returns floats e.g. 1520.0.
	altFloat := float64(cam.Altitude)
	entry := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              cam.Location.Lat,
		Longitude:             cam.Location.Lon,
		Altitude:              &altFloat,
		AltitudeUnitofMeasure: "m",
	}

	gpsInfo = append(gpsInfo, entry)
	positionCopy := entry
	gpsPoints["position"] = &positionCopy

	return gpsInfo, gpsPoints
}

// ── Float safety ──────────────────────────────────────────────────────────────

func isFinite(f float64) bool {
	return !math.IsNaN(f) && !math.IsInf(f, 0)
}

// ── Multilang helpers ─────────────────────────────────────────────────────────

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

// skiAreaResolver maps a webcam to its DSS talschaft rid (SkiArea Mapping.dss.rid).
type skiAreaResolver struct {
	byRegion map[string][]dto.DssSkiArea // regionId -> talschaften ("4" -> 4a, 4b)
}

func newSkiAreaResolver(feed dto.DssSkiAreaFeed) skiAreaResolver {
	r := skiAreaResolver{byRegion: map[string][]dto.DssSkiArea{}}
	for _, area := range feed.Items {
		region := strings.TrimRight(area.Rid, "abcdefghijklmnopqrstuvwxyz")
		r.byRegion[region] = append(r.byRegion[region], area)
	}
	return r
}

// rid returns the only talschaft of the webcam's region; for split regions
// (4, 5, 8) the one whose name (de/it/en) occurs in the webcam's skiresort text.
func (r skiAreaResolver) rid(cam dto.DssWebcam) string {
	areas := r.byRegion[strconv.FormatInt(cam.RegionId, 10)]
	if len(areas) == 1 {
		return areas[0].Rid
	}
	skiresort := strings.ToLower(cam.Skiresort)
	match := ""
	for _, area := range areas {
		for _, lang := range []string{"de", "it", "en"} {
			name := strings.ToLower(strings.TrimSpace(stringFromMultilang(area.Name, lang)))
			if name != "" && strings.Contains(skiresort, name) {
				if match != "" && match != area.Rid {
					return "" // ambiguous
				}
				match = area.Rid
			}
		}
	}
	return match
}

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

// stringVal safely dereferences a *string, returning "" for nil.
func stringVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

// safeParseFloat is kept for consistency but not used for webcam coords
// (which are already float64 in the JSON).
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

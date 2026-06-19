// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"fmt"
	"log/slog"
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

	odhmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-panomax/odh-content-model"
)

const (
	SOURCE      = "panomax"
	ENTITY_TYPE = "WebcamInfo"
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string
}

var contentClient clib.ContentAPI
var webcamCache *clib.Cache[odhmodel.WebcamInfo]
var timeNow = time.Now

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Panomax webcam transformer...")
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

	webcamCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhmodel.WebcamInfo]{
		EntityType:  ENTITY_TYPE,
		QueryParams: map[string]string{"source": SOURCE},
		IDFunc:      func(w odhmodel.WebcamInfo) string { return w.Id },
	})
	ms.FailOnError(context.Background(), err, "failed to load existing webcams")

	slog.Info("Loaded existing webcams", "count", len(webcamCache.Entries()))

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, r *rdb.Raw[[]PanomaxCamera]) error {
	logger.Get(ctx).Info("Processing Panomax webcam feed", "item_count", len(r.Rawdata))

	seen := map[string]struct{}{}
	webcams := map[string]odhmodel.WebcamInfo{}

	for _, cam := range r.Rawdata {
		id := buildID(cam.Id)
		seen[id] = struct{}{}

		existing, inCache := webcamCache.Get(id)
		var base *odhmodel.WebcamInfo
		if inCache {
			copy := existing.Entity
			base = &copy
		}
		if alreadyParsed, ok := webcams[id]; ok {
			base = &alreadyParsed
		}

		webcams[id] = mapToODH(cam, base, id)
	}

	sortedIDs := make([]string, 0, len(webcams))
	for id := range webcams {
		sortedIDs = append(sortedIDs, id)
	}
	sort.Strings(sortedIDs)

	for _, id := range sortedIDs {
		webcam := webcams[id]

		hash, changed, err := webcamCache.HasChanged(id, webcam)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash Webcam", "id", id, "error", err)
			continue
		}

		_, exists := webcamCache.Get(id)

		if !exists {
			postErr := contentClient.Post(ctx, ENTITY_TYPE, map[string]string{"generateid": "false"}, webcam)
			if postErr == nil {
				webcamCache.Set(id, webcam, hash)
				logger.Get(ctx).Info("Created new webcam", "id", id)
				continue
			}
			if !strings.Contains(postErr.Error(), "data exists already") {
				logger.Get(ctx).Error("API Post failed", "id", id, "error", postErr)
				continue
			}
			logger.Get(ctx).Warn("POST returned 'data exists already', recovering with PUT", "id", id)
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, webcam); err != nil {
				logger.Get(ctx).Error("API Put failed (recovery)", "id", id, "error", err)
				continue
			}
			webcamCache.Set(id, webcam, hash)
			logger.Get(ctx).Info("Recovered stale-cache webcam via PUT", "id", id)

		} else if changed {
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, webcam); err != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", err)
				continue
			}
			webcamCache.Set(id, webcam, hash)
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
		webcam := entry.Entity
		webcam.Active = false
		webcam.SmgActive = false
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, webcam); err != nil {
			logger.Get(ctx).Error("Failed to deactivate webcam", "id", id, "error", err)
			continue
		}
		webcamCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing webcam", "id", id)
	}

	return nil
}

func buildID(locationId int) string {
	return "PANOMAX_" + strconv.Itoa(locationId)
}

func mapToODH(cam PanomaxCamera, base *odhmodel.WebcamInfo, odhid string) odhmodel.WebcamInfo {
	var webcam odhmodel.WebcamInfo
	if base != nil {
		webcam = *base
		if webcam.Detail == nil {
			webcam.Detail = map[string]odhmodel.Detail{}
		}
		if webcam.ContactInfos == nil {
			webcam.ContactInfos = map[string]odhmodel.ContactInfo{}
		}
		if webcam.Mapping == nil {
			webcam.Mapping = map[string]map[string]string{}
		}
		if webcam.VideoItems == nil {
			webcam.VideoItems = map[string][]odhmodel.VideoItem{}
		}
		if webcam.HasLanguage == nil {
			webcam.HasLanguage = []string{}
		}
		if webcam.GpsPoints == nil {
			webcam.GpsPoints = map[string]odhmodel.GpsInfo{}
		}
	} else {
		webcam = odhmodel.WebcamInfo{
			Source:           "panomax",
			Id:               odhid,
			WebCamProperties: odhmodel.WebCamProperties{},
			Detail:           map[string]odhmodel.Detail{},
			ContactInfos:     map[string]odhmodel.ContactInfo{},
			Mapping:          map[string]map[string]string{},
			VideoItems:       map[string][]odhmodel.VideoItem{},
			HasLanguage:      []string{},
			GpsPoints:        map[string]odhmodel.GpsInfo{},
		}
	}

	webcam.Active = true
	webcam.SmgActive = true
	webcam.OdhActive = true

	webcam.WebCamProperties.HtmlEmbed = fmt.Sprintf("<script type=\"text/javascript\" src=\"https://static.panomax.com/front/thumbnail/js/pmaxthumbnail.js\"></script><script type=\"text/javascript\">PmaxThumbnail.place({ instance: %d});</script>", cam.Id)
	webcam.WebCamProperties.WebcamUrl = cam.WebcamUrl
	webcam.WebCamProperties.ZeroDirection = cam.ZeroDirection
	webcam.WebCamProperties.ViewAngleDegree = strconv.FormatFloat(cam.ViewAngleDegree, 'f', -1, 64)
	webcam.Webcamurl = cam.WebcamUrl

	webcam.LastChange = timeNow().UTC()

	webcam.Shortname = cam.Name

	languages := []string{"de", "it", "en"}

	for _, lang := range languages {
		hasLang := false
		for _, l := range webcam.HasLanguage {
			if l == lang {
				hasLang = true
				break
			}
		}
		if !hasLang {
			webcam.HasLanguage = append(webcam.HasLanguage, lang)
		}

		// Detail
		webcam.Detail[lang] = odhmodel.Detail{
			Title:    cam.Name,
			Language: lang,
		}

		// ContactInfos
		webcam.ContactInfos[lang] = odhmodel.ContactInfo{
			Region:   "IT-BZ",
			Language: lang,
		}
	}

	lat, _ := strconv.ParseFloat(cam.Latitude, 64)
	lon, _ := strconv.ParseFloat(cam.Longitude, 64)
	alt, _ := strconv.ParseFloat(cam.Elevation, 64)

	gpsinfo := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              alt,
		AltitudeUnitofMeasure: "m",
	}
	webcam.GpsInfo = []odhmodel.GpsInfo{gpsinfo}
	webcam.GpsPoints["position"] = gpsinfo

	// Images
	webcam.ImageGallery = []odhmodel.ImageGallery{}
	for _, img := range cam.Images {
		w, _ := strconv.Atoi(img.Width)
		h, _ := strconv.Atoi(img.Height)

		image := odhmodel.ImageGallery{
			Width:       w,
			Height:      h,
			ImageUrl:    img.Url,
			ImageSource: "panomax",
			ImageDesc:   map[string]string{},
			ImageTitle:  map[string]string{},
			ImageAltText: map[string]string{},
		}
		webcam.ImageGallery = append(webcam.ImageGallery, image)
	}

	// Mapping
	if _, ok := webcam.Mapping["panomax"]; !ok {
		webcam.Mapping["panomax"] = map[string]string{}
	}
	webcam.Mapping["panomax"]["id"] = strconv.Itoa(cam.Id)
	webcam.Mapping["panomax"]["camId"] = strconv.Itoa(cam.CamId)
	webcam.Mapping["panomax"]["customerId"] = strconv.Itoa(cam.CustomerId)

	webcam.WebcamId = strconv.Itoa(cam.CamId)

	return webcam
}

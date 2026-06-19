// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
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

	odhmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-panocloud/odh-content-model"
)

const (
	SOURCE      = "panocloud"
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
	slog.Info("Starting Panocloud webcam transformer...")
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

func Transform(ctx context.Context, r *rdb.Raw[PanocloudResponse]) error {
	logger.Get(ctx).Info("Processing Panocloud webcam feed", "item_count", len(r.Rawdata.LiveCam))

	seen := map[string]struct{}{}
	webcams := map[string]odhmodel.WebcamInfo{}

	for _, cam := range r.Rawdata.LiveCam {
		id := buildID(cam.Attributes.LocationId, cam.Attributes.GeoAlt)
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

func buildID(locationId string, geoAlt string) string {
	return "PANOCLOUD_" + locationId + "_" + geoAlt
}

func mapToODH(cam PanocloudCamera, base *odhmodel.WebcamInfo, odhid string) odhmodel.WebcamInfo {
	attr := cam.Attributes

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
	} else {
		webcam = odhmodel.WebcamInfo{
			Source:           "panocloud",
			Id:               odhid,
			WebCamProperties: odhmodel.WebCamProperties{},
			Detail:           map[string]odhmodel.Detail{},
			ContactInfos:     map[string]odhmodel.ContactInfo{},
			Mapping:          map[string]map[string]string{},
			VideoItems:       map[string][]odhmodel.VideoItem{},
			HasLanguage:      []string{},
		}
	}

	if attr.CameraStatus == "active" {
		webcam.Active = true
	} else {
		webcam.Active = false
	}
	webcam.SmgActive = webcam.Active

	if attr.Full360 == "yes" {
		webcam.WebCamProperties.ViewAngleDegree = "360"
	} else {
		webcam.WebCamProperties.ViewAngleDegree = ""
	}

	if attr.HasVR == "yes" {
		webcam.WebCamProperties.HasVR = true
	} else {
		webcam.WebCamProperties.HasVR = false
	}

	webcam.WebCamProperties.ViewerType = attr.ViewerType
	if attr.Url != "" {
		webcam.WebCamProperties.WebcamUrl = addHttpsPrefixIfNotPresent(attr.Url)
	}

	t, _ := time.Parse(time.RFC3339, attr.LastModified) // Or just leave empty
	if t.IsZero() {
		t, _ = time.Parse("2006-01-02T15:04:05-07:00", attr.LastModified)
	}
	webcam.LastChange = t.UTC()

	webcam.Shortname = attr.Name

	defaultlanguage := attr.DefaultLang
	if defaultlanguage == "" {
		defaultlanguage = "de"
	}

	hasLang := false
	for _, l := range webcam.HasLanguage {
		if l == defaultlanguage {
			hasLang = true
			break
		}
	}
	if !hasLang {
		webcam.HasLanguage = append(webcam.HasLanguage, defaultlanguage)
	}

	// Detail
	baseText := attr.LongDescription
	webcam.Detail[defaultlanguage] = odhmodel.Detail{
		Title:     attr.Name,
		IntroText: attr.Description,
		BaseText:  baseText,
		Language:  defaultlanguage,
	}

	// ContactInfo
	contactinfo := odhmodel.ContactInfo{
		Region:   attr.GeoRegion,
		Language: defaultlanguage,
	}
	// No logos in my simplified dto for now, skip logo
	webcam.ContactInfos[defaultlanguage] = contactinfo

	// GpsInfo
	lat, _ := strconv.ParseFloat(attr.GeoLat, 64)
	lon, _ := strconv.ParseFloat(attr.GeoLong, 64)
	alt, _ := strconv.ParseFloat(attr.GeoAlt, 64)

	gpsinfo := odhmodel.GpsInfo{
		Gpstype:               "position",
		Latitude:              lat,
		Longitude:             lon,
		Altitude:              alt,
		AltitudeUnitofMeasure: "m",
	}
	webcam.GpsInfo = []odhmodel.GpsInfo{gpsinfo}

	// Images
	webcam.ImageGallery = []odhmodel.ImageGallery{}
	if len(cam.Images.Image) > 0 {
		for _, imagetoparse := range cam.Images.Image {
			iAttr := imagetoparse.Attributes

			w, _ := strconv.Atoi(iAttr.ImgWidth)
			h, _ := strconv.Atoi(iAttr.ImgHeight)

			image := odhmodel.ImageGallery{
				Width:       w,
				Height:      h,
				ImageName:   iAttr.FileName,
				ImageUrl:    addHttpsPrefixIfNotPresent(iAttr.FileUrl),
				ImageSource: "panocloud",
				IsInGallery: true,
				ImageTags:   []string{},
			}

			image.ImageTags = append(image.ImageTags, iAttr.FileType)
			image.ImageTags = append(image.ImageTags, iAttr.MimeType)

			if iAttr.FileType == "thumbnail" {
				image.ListPosition = 0
			}

			if iAttr.Panorama == "yes" {
				image.ImageTags = append(image.ImageTags, "panorama")
			}

			if iAttr.FileType != "" {
				found := false
				for _, tag := range image.ImageTags {
					if tag == iAttr.FileType {
						found = true
						break
					}
				}
				if !found {
					image.ImageTags = append(image.ImageTags, iAttr.FileType)
				}
			}

			webcam.ImageGallery = append(webcam.ImageGallery, image)
		}

		sort.Slice(webcam.ImageGallery, func(i, j int) bool {
			return webcam.ImageGallery[i].ListPosition > webcam.ImageGallery[j].ListPosition
		})
	}

	// Videos
	if attrVideos := cam.Videos; attrVideos.Video.Attributes.VideoClipUrl != "" {
		vAttr := attrVideos.Video.Attributes
		dur, _ := strconv.ParseFloat(vAttr.Duration, 64)
		bitrate, _ := strconv.ParseInt(vAttr.VideoBitRate, 10, 64)
		res, _ := strconv.ParseInt(vAttr.Resolution, 10, 64)

		video := odhmodel.VideoItem{
			Url:             addHttpsPrefixIfNotPresent(vAttr.VideoClipUrl),
			StreamingSource: "panocloud",
			Active:          true,
			Resolution:      int(res),
			Definition:      vAttr.Definition,
			Bitrate:         int(bitrate),
			Duration:        dur,
			VideoType:       vAttr.MimeType,
		}
		webcam.VideoItems[defaultlanguage] = []odhmodel.VideoItem{video}
	}

	// Mapping
	if _, ok := webcam.Mapping["panocloud"]; !ok {
		webcam.Mapping["panocloud"] = map[string]string{}
	}
	webcam.Mapping["panocloud"]["locationId"] = attr.LocationId

	return webcam
}

func addHttpsPrefixIfNotPresent(url string) string {
	if url == "" {
		return url
	}
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return "https://" + url
	}
	return url
}

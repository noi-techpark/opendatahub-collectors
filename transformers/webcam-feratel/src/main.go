// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/xml"
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

	contentmodel "github.com/noi-techpark/opendatahub-collectors/transformers/webcam-feratel/content-model"
)

const (
	SOURCE      = "feratel"
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
var webcamCache *clib.Cache[contentmodel.WebcamInfo]
var timeNow = time.Now

// Feratel XML Models
type FeratelResponse struct {
	XMLName xml.Name       `xml:"feratel"`
	Content FeratelContent `xml:"content"`
}

type FeratelContent struct {
	Portal FeratelPortal `xml:"portal"`
}

type FeratelPortal struct {
	Links struct {
		Links []FeratelLink `xml:"link"`
	} `xml:"links"`
}

type FeratelLink struct {
	ID       string      `xml:"id,attr"`
	Location Location    `xml:"location"`
	Region   string      `xml:"region"`
	Village  Village     `xml:"village"`
	Country  Country     `xml:"country"`
	Keywords string      `xml:"keywords"`
	Cams     FeratelCams `xml:"cams"`
}

type Location struct {
	Value string `xml:",chardata"`
	H     string `xml:"h,attr"`
	X     string `xml:"x,attr"`
	Y     string `xml:"y,attr"`
	Addon string `xml:"addon,attr"`
	Zip   string `xml:"zip,attr"`
}

type Village struct {
	Value string `xml:",chardata"`
	K     string `xml:"k,attr"`
}

type Country struct {
	Value string `xml:",chardata"`
	Lkz   string `xml:"lkz,attr"`
	Ioc   string `xml:"ioc,attr"`
}

type FeratelCams struct {
	Count string       `xml:"count,attr"`
	Cams  []FeratelCam `xml:"cam"`
}

type FeratelCam struct {
	PanID string  `xml:"panid,attr"`
	Stat  string  `xml:"stat,attr"`
	L     string  `xml:"l,attr"`
	H     string  `xml:"h,attr"`
	X     string  `xml:"x,attr"`
	Y     string  `xml:"y,attr"`
	URLs  URLList `xml:"urllist"`
}

type URLList struct {
	DURLs []DURL `xml:"durl"`
}

type DURL struct {
	T string `xml:"t,attr"`
	V string `xml:"v,attr"`
	K string `xml:"k,attr"`
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Feratel webcam transformer...")
	defer tel.FlushOnPanic()

	var err error

	contentClient, err = clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create ODH content client")

	webcamCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[contentmodel.WebcamInfo]{
		EntityType:  ENTITY_TYPE,
		QueryParams: map[string]string{"source": SOURCE},
		IDFunc:      func(w contentmodel.WebcamInfo) string { return w.Id },
	})
	ms.FailOnError(context.Background(), err, "failed to load existing webcams")

	slog.Info("Loaded existing webcams", "count", len(webcamCache.Entries()))

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), Transform)
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func Transform(ctx context.Context, r *rdb.Raw[string]) error {
	logger.Get(ctx).Info("Processing Feratel webcam feed")

	var raw FeratelResponse
	err := xml.Unmarshal([]byte(r.Rawdata), &raw)
	if err != nil {
		logger.Get(ctx).Error("failed to unmarshal xml", "error", err)
		return err
	}

	seen := map[string]struct{}{}
	webcams := map[string]contentmodel.WebcamInfo{}

	for _, link := range raw.Content.Portal.Links.Links {
		for _, cam := range link.Cams.Cams {
			id := "FERATEL_" + link.ID + "_" + cam.PanID
			seen[id] = struct{}{}

			existing, inCache := webcamCache.Get(id)
			var base *contentmodel.WebcamInfo
			if inCache {
				copy := existing.Entity
				base = &copy
			}
			if alreadyParsed, ok := webcams[id]; ok {
				base = &alreadyParsed
			}

			webcams[id] = mapToCore(link, cam, base, id)
		}
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

func mapToCore(link FeratelLink, cam FeratelCam, base *contentmodel.WebcamInfo, odhid string) contentmodel.WebcamInfo {
	var res contentmodel.WebcamInfo
	if base != nil {
		res = *base
		if res.Detail == nil {
			res.Detail = map[string]contentmodel.Detail{}
		}
		if res.ContactInfos == nil {
			res.ContactInfos = map[string]contentmodel.ContactInfo{}
		}
		if res.Mapping == nil {
			res.Mapping = map[string]map[string]string{}
		}
		if res.VideoItems == nil {
			res.VideoItems = map[string][]contentmodel.VideoItem{}
		}
		if res.HasLanguage == nil {
			res.HasLanguage = []string{}
		}
	} else {
		res = contentmodel.WebcamInfo{
			Source:           "feratel",
			Id:               odhid,
			WebCamProperties: contentmodel.WebCamProperties{},
			Detail:           map[string]contentmodel.Detail{},
			ContactInfos:     map[string]contentmodel.ContactInfo{},
			Mapping:          map[string]map[string]string{},
			VideoItems:       map[string][]contentmodel.VideoItem{},
			HasLanguage:      []string{},
		}
	}

	res.Active = true
	res.SmgActive = true
	res.OdhActive = true
	res.WebcamId = cam.PanID
	res.LastChange = timeNow().UTC()

	// GPS Info
	gps := contentmodel.GpsInfo{
		Gpstype:               "position",
		AltitudeUnitofMeasure: "m",
	}
	if h, err := strconv.ParseFloat(cam.H, 64); err == nil {
		gps.Altitude = h
	}
	if x, err := strconv.ParseFloat(cam.X, 64); err == nil {
		gps.Latitude = x
	}
	if y, err := strconv.ParseFloat(cam.Y, 64); err == nil {
		gps.Longitude = y
	}
	res.GpsInfo = []contentmodel.GpsInfo{gps}

	languages := []string{"de", "it", "en"}

	for _, lang := range languages {
		hasLang := false
		for _, l := range res.HasLanguage {
			if l == lang {
				hasLang = true
				break
			}
		}
		if !hasLang {
			res.HasLanguage = append(res.HasLanguage, lang)
		}

		// ContactInfo
		contact := contentmodel.ContactInfo{
			Language:    lang,
			ZipCode:     link.Location.Zip,
			City:        link.Location.Value,
			Area:        link.Village.Value,
			Region:      link.Region,
			CountryCode: link.Country.Ioc,
			CountryName: link.Country.Value,
		}

		// URLs
		for _, url := range cam.URLs.DURLs {
			if url.T == "feratel.com" {
				contact.Url = url.V
			}
		}
		res.ContactInfos[lang] = contact

		// Detail
		detail := contentmodel.Detail{
			Title:    cam.L,
			Language: lang,
		}
		res.Shortname = cam.L
		if link.Keywords != "" {
			parts := strings.Split(link.Keywords, ",")
			var kws []string
			for _, part := range parts {
				part = strings.TrimSpace(part)
				if part != "" {
					kws = append(kws, part)
				}
			}
			detail.Keywords = kws
		}
		res.Detail[lang] = detail
	}

	// ImageGallery
	res.ImageGallery = []contentmodel.ImageGallery{}
	for _, url := range cam.URLs.DURLs {
		if url.T == "MediaPlayer Thumbnails" ||
			url.T == "MediaPlayer Thumbnails 38" ||
			url.T == "MediaPlayer Thumbnails 36" ||
			url.T == "MediaPlayer Thumbnail 360" {

			image := contentmodel.ImageGallery{
				ImageName:   url.T,
				ImageUrl:    url.V,
				ImageSource: "feratel",
				IsInGallery: true,
			}
			if url.T == "MediaPlayer Thumbnails 38" {
				image.ListPosition = 0
			} else {
				image.ListPosition = 1
			}
			res.ImageGallery = append(res.ImageGallery, image)
		}
	}
	sort.Slice(res.ImageGallery, func(i, j int) bool {
		return res.ImageGallery[i].ListPosition > res.ImageGallery[j].ListPosition
	})

	// WebcamProperties
	props := contentmodel.WebCamProperties{}
	for _, url := range cam.URLs.DURLs {
		if url.T == "MediaPlayer v5" {
			props.WebcamUrl = url.V
		}
	}
	res.WebCamProperties = props

	// Mapping
	res.Mapping["feratel"] = map[string]string{
		"link_id": link.ID,
		"panid":   cam.PanID,
	}

	return res
}

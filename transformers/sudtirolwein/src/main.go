// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

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
	"opendatahub.com/tr-sudtirolwein/dto"
	odhContentModel "opendatahub.com/tr-sudtirolwein/odh-content-model"
)

const (
	SOURCE         = "suedtirolwein"
	ENTITY_TYPE    = "ODHActivityPoi"
	LICENSE_HOLDER = "https://www.suedtirolwein.com/"
)

var env struct {
	tr.Env

	ODH_CORE_URL                 string
	ODH_CORE_TOKEN_CLIENT_ID     string
	ODH_CORE_TOKEN_CLIENT_SECRET string
	ODH_CORE_TOKEN_URL           string
}

var contentClient clib.ContentAPI
var poiCache *clib.Cache[odhContentModel.ODHActivityPoi]

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Sudtirol Wine transformer...")
	defer tel.FlushOnPanic()

	slog.Info("core url", "value", env.ODH_CORE_URL)

	var err error

	contentClient, err = clib.NewContentClient(clib.Config{
		BaseURL:      env.ODH_CORE_URL,
		TokenURL:     env.ODH_CORE_TOKEN_URL,
		ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
		ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
		DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create client")

	poiCache, err = clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[odhContentModel.ODHActivityPoi]{
		EntityType:  ENTITY_TYPE,
		QueryParams: map[string]string{"source": SOURCE},
		IDFunc:      func(p odhContentModel.ODHActivityPoi) string { return *p.Generic.ID },
	})
	ms.FailOnError(context.Background(), err, "failed to load existing POIs")

	slog.Info("Loaded existing POIs", "count", len(poiCache.Entries()))

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

type langBatch struct {
	lang      string
	companies []dto.WineCompany
}

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing wine company data")

	batches := []langBatch{
		{"de", companiesFromLang(r.Rawdata.De)},
		{"it", companiesFromLang(r.Rawdata.It)},
		{"en", companiesFromLang(r.Rawdata.En)},
		{"ru", companiesFromLang(r.Rawdata.Ru)},
		{"jp", companiesFromLang(r.Rawdata.Jp)},
		{"us", companiesFromLang(r.Rawdata.Us)},
	}

	pois := map[string]odhContentModel.ODHActivityPoi{}
	seen := map[string]struct{}{}

	// DE lookup for GPS + images (C# parser always uses DE for these fields)
	deCompanies := companiesFromLang(r.Rawdata.De)
	deByID := make(map[string]dto.WineCompany, len(deCompanies))
	for _, c := range deCompanies {
		deByID[c.ID] = c
	}

	for _, batch := range batches {
		if len(batch.companies) == 0 {
			continue
		}
		for _, company := range batch.companies {
			id := buildID(company)
			if id == "" {
				logger.Get(ctx).Warn("Skipping company with empty ID", "name", company.Title)
				continue
			}
			seen[id] = struct{}{}

			if existing, ok := pois[id]; ok {
				mergeLang(&existing, company, batch.lang)
				pois[id] = existing
			} else {
				deCopy, hasDe := deByID[company.ID]
				if !hasDe {
					deCopy = company
				}
				pois[id] = mapToPoi(company, batch.lang, deCopy, r.Timestamp)
			}
		}
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
				logger.Get(ctx).Info("Created new POI", "id", id)
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
			logger.Get(ctx).Info("Recovered stale-cache POI via PUT", "id", id)
		} else if changed {
			if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", err)
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Updated existing POI", "id", id)
		}
	}

	// Deactivate records no longer in source
	cacheIDs := make([]string, 0, len(poiCache.Entries()))
	for id := range poiCache.Entries() {
		cacheIDs = append(cacheIDs, id)
	}
	for _, id := range cacheIDs {
		if _, ok := seen[id]; ok {
			continue
		}
		entry, stillExists := poiCache.Get(id)
		if !stillExists {
			continue
		}
		poi := entry.Entity
		poi.Active = false
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, poi); err != nil {
			logger.Get(ctx).Error("Failed to deactivate POI", "id", id, "error", err)
			continue
		}
		poiCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing wine company", "id", id)
	}

	return nil
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func companiesFromLang(ld *dto.LangData) []dto.WineCompany {
	if ld == nil {
		return nil
	}
	return ld.Companies.Items()
}

func buildID(c dto.WineCompany) string {
	if c.ID != "" {
		return fmt.Sprintf("smgpoi%ssuedtirolwein", c.ID)
	}
	return ""
}

func mapToPoi(c dto.WineCompany, lang string, de dto.WineCompany, ts time.Time) odhContentModel.ODHActivityPoi {
	id := buildID(c)
	source := SOURCE
	shortname := c.Title

	return odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:          &id,
			Active:      parseBool(c.Active),
			Source:      &source,
			Shortname:   &shortname,
			HasLanguage: []string{lang},
			LastChange:  odhContentModel.PtrFlexibleTime(ts),
			Mapping: map[string]map[string]string{
				SOURCE: buildMapping(c),
			},
			TagIds:  []string{"wine:company"},
			SmgTags: []string{"poi", "business", "wine", "winery"},
			LicenseInfo: &odhContentModel.LicenseInfo{
				License:       "CC0",
				LicenseHolder: LICENSE_HOLDER,
			},
			GpsInfo: buildGpsInfo(de),
		},
		SmgActive:           true,
		PublishedOn:         []string{},
		SyncUpdateMode:      "Full",
		SyncSourceInterface: "suedtirolweincompany",
		Detail:              map[string]*clib.DetailGeneric{lang: buildDetail(c, lang)},
		ContactInfos:        map[string]*odhContentModel.ContactInfo{lang: buildContactInfo(c, lang)},
		ImageGallery:        buildImageGallery(de),
		PoiProperty:         map[string][]odhContentModel.PoiPropertyEntry{lang: buildPoiProperty(c)},
	}
}

func mergeLang(poi *odhContentModel.ODHActivityPoi, c dto.WineCompany, lang string) {
	found := false
	for _, l := range poi.Generic.HasLanguage {
		if l == lang {
			found = true
			break
		}
	}
	if !found {
		poi.Generic.HasLanguage = append(poi.Generic.HasLanguage, lang)
	}

	if poi.Detail == nil {
		poi.Detail = map[string]*clib.DetailGeneric{}
	}
	poi.Detail[lang] = buildDetail(c, lang)

	if poi.ContactInfos == nil {
		poi.ContactInfos = map[string]*odhContentModel.ContactInfo{}
	}
	poi.ContactInfos[lang] = buildContactInfo(c, lang)

	if poi.PoiProperty == nil {
		poi.PoiProperty = map[string][]odhContentModel.PoiPropertyEntry{}
	}
	poi.PoiProperty[lang] = buildPoiProperty(c)
}

// ── Field builders ────────────────────────────────────────────────────────────

func buildMapping(c dto.WineCompany) map[string]string {
	return map[string]string{
		"id":      c.ID,
		"name":    c.Title,
		"address": c.Address,
		"city":    c.Place,
		"zipcode": c.ZipCode,
		"email":   c.Email,
		"website": c.Homepage,
	}
}

// buildDetail uses only fields available in clib.DetailGeneric: Title, BaseText, Language.
// Slogan, Subtitle, Quote are stored in PoiProperty.
func buildDetail(c dto.WineCompany, lang string) *clib.DetailGeneric {
	return &clib.DetailGeneric{
		Language: &lang,
		Title:    ptrOf(c.Title),
		BaseText: ptrOf(c.CompanyDescription),
	}
}

func buildContactInfo(c dto.WineCompany, lang string) *odhContentModel.ContactInfo {
	website := c.Homepage
	if website != "" && !strings.Contains(website, "http") {
		website = "http://" + website
	}

	countryName := "Italy"
	switch lang {
	case "de":
		countryName = "Italien"
	case "it":
		countryName = "Italia"
	}

	return &odhContentModel.ContactInfo{
		Language:    lang,
		CompanyName: c.Title,
		Address:     c.Address,
		ZipCode:     c.ZipCode,
		City:        c.Place,
		CountryCode: "IT",
		Area:        countryName,
		Phonenumber: c.Phone,
		Url:         website,
		Email:       c.Email,
	}
}

func buildGpsInfo(de dto.WineCompany) []odhContentModel.GpsData {
	if strings.Contains(de.Latitude, "°") || strings.Contains(de.Longitude, "°") {
		return nil
	}
	lat := parseCoord(de.Latitude)
	lon := parseCoord(de.Longitude)
	if lat == 0 && lon == 0 {
		return nil
	}
	return []odhContentModel.GpsData{
		{
			Gpstype:   ptrOf("position"),
			Latitude:  lat,
			Longitude: lon,
		},
	}
}

func buildImageGallery(de dto.WineCompany) []odhContentModel.ImageGalleryEntry {
	var gallery []odhContentModel.ImageGalleryEntry
	seen := map[string]bool{}

	if de.Media != "" && !seen[de.Media] {
		seen[de.Media] = true
		entry := odhContentModel.ImageGalleryEntry{
			ImageUrl:     de.Media,
			ImageSource:  "SuedtirolWein",
			IsInGallery:  true,
			ListPosition: 0,
		}
		if de.ImageMetaTitle != "" {
			entry.ImageDesc = map[string]string{"de": de.ImageMetaTitle}
		}
		gallery = append(gallery, entry)
	}

	if de.MediaDetail != "" && !seen[de.MediaDetail] {
		seen[de.MediaDetail] = true
		gallery = append(gallery, odhContentModel.ImageGalleryEntry{
			ImageUrl:     de.MediaDetail,
			ImageSource:  "SuedtirolWein",
			IsInGallery:  true,
			ListPosition: 1,
		})
	}

	return gallery
}

func buildPoiProperty(c dto.WineCompany) []odhContentModel.PoiPropertyEntry {
	var props []odhContentModel.PoiPropertyEntry
	add := func(name, value string) {
		if value != "" {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: name, Value: value})
		}
	}

	// Content fields that don't fit DetailGeneric
	add("slogan", c.Slogan)
	add("subtitle", c.Subtitle)
	add("quote", c.Quote)
	add("quoteauthor", c.QuoteAuthor)
	add("h1", c.H1)
	add("h2", c.H2)
	add("region", c.Region)
	add("farmname", c.FarmName)
	add("openingtimeswineshop", c.OpeningTimesWineShop)
	add("openingtimesguides", c.OpeningTimesGuides)
	add("openingtimesgastronomie", c.OpeningTimesGastronomy)
	add("companyholiday", c.CompanyHoliday)

	// Feature flags
	add("hasvisits", c.HasVisits)
	add("hasovernights", c.HasOvernights)
	add("hasbiowine", c.HasBioWine)
	add("hasaccomodation", c.HasAccomodation)
	add("isvinumhotel", c.IsVinumHotel)
	add("isanteprima", c.IsAnteprima)
	add("iswinestories", c.IsWineStories)
	add("iswinesummit", c.IsWineSummit)
	add("issparklingwineassociation", c.IsSparklingWineAssociation)
	add("iswinery", c.IsWinery)
	add("iswineryassociation", c.IsWineryAssociation)
	add("hasonlineshop", c.HasOnlineShop)
	add("hasdeliveryservice", c.HasDeliveryService)
	add("onlineshopurl", c.OnlineShopURL)
	add("deliveryserviceurl", c.DeliveryServiceURL)
	add("hasdirectsales", c.HasDirectSales)
	add("isskyalpspartner", c.IsSkyAlpsPartner)
	add("wines", c.Wines)

	// Sparkling wine producer
	add("descriptionsparklingwineproducer", c.DescriptionSparklingWineProducer)
	add("h1sparklingwineproducer", c.H1SparklingWineProducer)
	add("h2sparklingwineproducer", c.H2SparklingWineProducer)
	add("imagesparklingwineproducer", c.ImageSparklingWineProducer)

	// Socials
	add("socialsinstagram", c.SocialsInstagram)
	add("socialsfacebook", c.SocialsFacebook)
	add("socialslinkedIn", c.SocialsLinkedIn)
	add("socialspinterest", c.SocialsPinterest)
	add("socialstiktok", c.SocialsTikTok)
	add("socialsyoutube", c.SocialsYouTube)
	add("socialstwitter", c.SocialsTwitter)

	return props
}

// ── Utility ───────────────────────────────────────────────────────────────────

func parseBool(s string) bool {
	return strings.EqualFold(s, "true")
}

func parseCoord(s string) float64 {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func ptrOf[T any](v T) *T { return &v }

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"bytes"
	"context"
	"encoding/json"
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
	COPYRIGHT      = "Suedtirol Wein"
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
		IDFunc: func(p odhContentModel.ODHActivityPoi) string {
			if p.Generic.ID == nil {
				return ""
			}
			return *p.Generic.ID
		},
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

func noEscapeJSON(v interface{}) (json.RawMessage, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

func sanitizeHTML(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "<br>", "<br />")
	s = strings.ReplaceAll(s, "<br/>", "<br />")
	s = strings.ReplaceAll(s, "<hr>", "<hr />")
	s = strings.ReplaceAll(s, "<hr/>", "<hr />")

	// Strip raw backslash escape symbols from input strings
	s = strings.ReplaceAll(s, "\\n", " ")
	s = strings.ReplaceAll(s, "\\r", "")

	// Strip structural/literal line breaks
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", "")

	return s
}

// cleanCachedPOI scrubs old cached entries loaded from the ODH API before PUTting
// them back for deactivation. Old entries often contain null collections or
// literal newlines (\n) that fail the modern ASP.NET Core validation rules.
func cleanCachedPOI(poi *odhContentModel.ODHActivityPoi) {
	if poi.GpsInfo == nil {
		poi.GpsInfo = []odhContentModel.GpsData{}
	}
	if poi.ImageGallery == nil {
		poi.ImageGallery = []odhContentModel.ImageGalleryEntry{}
	}
	if poi.PoiServices == nil {
		poi.PoiServices = []string{}
	}
	if poi.SmgTags == nil {
		poi.SmgTags = []string{}
	}
	if poi.TagIds == nil {
		poi.TagIds = []string{}
	}
	if poi.PublishedOn == nil {
		poi.PublishedOn = []string{}
	}
	if poi.Generic.HasLanguage == nil {
		poi.Generic.HasLanguage = []string{}
	}
	if poi.PoiProperty == nil {
		poi.PoiProperty = map[string][]odhContentModel.PoiPropertyEntry{}
	}

	// Clean text fields of old newline escapes
	if poi.Detail != nil {
		for _, det := range poi.Detail {
			if det.BaseText != nil {
				c := sanitizeHTML(*det.BaseText)
				det.BaseText = &c
			}
			if det.IntroText != nil {
				c := sanitizeHTML(*det.IntroText)
				det.IntroText = &c
			}
			if det.Header != nil {
				c := sanitizeHTML(*det.Header)
				det.Header = &c
			}
			if det.SubHeader != nil {
				c := sanitizeHTML(*det.SubHeader)
				det.SubHeader = &c
			}
			if det.Title != nil {
				c := sanitizeHTML(*det.Title)
				det.Title = &c
			}
		}
	}

	if poi.PoiProperty != nil {
		for lang, props := range poi.PoiProperty {
			for i, prop := range props {
				props[i].Value = sanitizeHTML(prop.Value)
			}
			poi.PoiProperty[lang] = props
		}
	}

	if poi.AdditionalProperties != nil && poi.AdditionalProperties.SuedtirolWeinCompanyDataProperties != nil {
		p := poi.AdditionalProperties.SuedtirolWeinCompanyDataProperties
		cleanMap := func(m odhContentModel.FlexibleMap) {
			if m != nil {
				for k, v := range m {
					m[k] = sanitizeHTML(v)
				}
			}
		}
		cleanMap(p.H1)
		cleanMap(p.H2)
		cleanMap(p.Quote)
		cleanMap(p.QuoteAuthor)
		cleanMap(p.Slogan)
		cleanMap(p.FarmName)
		cleanMap(p.OpeningTimesWineShop)
		cleanMap(p.OpeningTimesGuides)
		cleanMap(p.OpeningTimesGastronomie)
		cleanMap(p.CompanyHoliday)
	}
}

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing wine company data")

	batches := []langBatch{
		{"de", companiesFromLang(r.Rawdata.De)},
		{"it", companiesFromLang(r.Rawdata.It)},
		{"en", companiesFromLang(r.Rawdata.En)},
		{"ru", companiesFromLang(r.Rawdata.Ru)},
	}

	pois := map[string]odhContentModel.ODHActivityPoi{}
	seen := map[string]struct{}{}

	deCompanies := companiesFromLang(r.Rawdata.De)
	deBySlug := make(map[string]dto.WineCompany, len(deCompanies))
	for _, c := range deCompanies {
		if c.Slug != "" {
			deBySlug[c.Slug] = c
		}
	}

	allLangsBySlug := map[string]map[string]dto.WineCompany{}
	for _, batch := range batches {
		for _, company := range batch.companies {
			if company.Slug == "" {
				continue
			}
			if allLangsBySlug[company.Slug] == nil {
				allLangsBySlug[company.Slug] = map[string]dto.WineCompany{}
			}
			allLangsBySlug[company.Slug][batch.lang] = company
		}
	}

	for _, batch := range batches {
		if len(batch.companies) == 0 {
			continue
		}
		for _, company := range batch.companies {
			if company.Slug == "" {
				logger.Get(ctx).Warn("Skipping company with empty slug", "name", company.Title)
				continue
			}

			if !company.Active {
				continue
			}

			deCopy, hasDe := deBySlug[company.Slug]
			var id string
			if hasDe {
				id = deCopy.ID
			} else {
				id = company.ID
			}
			if id == "" {
				logger.Get(ctx).Warn("Skipping company with empty ID", "name", company.Title)
				continue
			}

			seen[id] = struct{}{}

			if existing, ok := pois[id]; ok {
				mergeLang(&existing, company, batch.lang)
				pois[id] = existing
			} else {
				if !hasDe {
					deCopy = company
				}
				poi := mapToPoi(id, company, batch.lang, deCopy, allLangsBySlug[company.Slug], r.Timestamp)
				poi.AdditionalProperties = &odhContentModel.AdditionalProperties{
					SuedtirolWeinCompanyDataProperties: buildAdditionalProperties(allLangsBySlug[company.Slug]),
				}
				pois[id] = poi
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
			payload, serErr := noEscapeJSON(poi)
			if serErr != nil {
				logger.Get(ctx).Error("Failed to serialize POI", "id", id, "error", serErr)
				continue
			}
			postErr := contentClient.Post(ctx, ENTITY_TYPE, map[string]string{"generateid": "false"}, payload)
			if postErr == nil {
				poiCache.Set(id, poi, hash)
				logger.Get(ctx).Info("Created new wine company", "id", id)
				continue
			}
			logger.Get(ctx).Warn("POST failed, recovering via PUT", "id", id, "postErr", postErr)
			if putErr := contentClient.Put(ctx, ENTITY_TYPE, id, payload); putErr != nil {
				logger.Get(ctx).Error("API Put failed (recovery)", "id", id, "error", putErr, "payload", string(payload))
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Recovered wine company via PUT", "id", id)

		} else if changed {
			payload, serErr := noEscapeJSON(poi)
			if serErr != nil {
				logger.Get(ctx).Error("Failed to serialize POI", "id", id, "error", serErr)
				continue
			}
			if putErr := contentClient.Put(ctx, ENTITY_TYPE, id, payload); putErr != nil {
				logger.Get(ctx).Error("API Put failed", "id", id, "error", putErr, "payload", string(payload))
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Updated wine company", "id", id)
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

		// SCRUB THE CACHED ENTITY BEFORE SENDING IT BACK TO ODH API!
		cleanCachedPOI(&poi)

		payload, serErr := noEscapeJSON(poi)
		if serErr != nil {
			logger.Get(ctx).Error("Failed to serialize POI for deactivation", "id", id, "error", serErr)
			continue
		}
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, payload); err != nil {
			logger.Get(ctx).Error("Failed to deactivate wine company", "id", id, "error", err, "payload", string(payload))
			continue
		}
		poiCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing wine company", "id", id)
	}

	return nil
}

func companiesFromLang(ld *dto.LangData) []dto.WineCompany {
	if ld == nil {
		return nil
	}
	return ld.Items()
}

func mapToPoi(id string, c dto.WineCompany, lang string, de dto.WineCompany, byLang map[string]dto.WineCompany, ts time.Time) odhContentModel.ODHActivityPoi {
	source := SOURCE
	shortname := c.Title

	mapping := map[string]map[string]string{
		SOURCE: {
			"id": c.ID,
		},
	}
	if c.Slug != "" {
		mapping[SOURCE]["slug"] = c.Slug
	}
	if c.OriginID != "" {
		mapping[SOURCE]["origin_id"] = c.OriginID
	}
	if de.LegacyNumber != "" {
		mapping[SOURCE]["legacyid"] = de.LegacyNumber
	} else if c.LegacyNumber != "" {
		mapping[SOURCE]["legacyid"] = c.LegacyNumber
	}

	return odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:          &id,
			Active:      c.Active,
			Source:      &source,
			Shortname:   &shortname,
			HasLanguage: []string{lang},
			LastChange:  odhContentModel.PtrFlexibleTime(ts),
			Mapping:     mapping,
			TagIds: []string{
				"28CDEF87206E464D9B179FBCAF506457",
				"6EFED925DF3B4EF5B69495E994F446AC",
				"eating drinking",
				"gastronomy",
				"wineries",
			},
			SmgTags: []string{"gastronomy", "essen trinken", "weinkellereien"},
			LicenseInfo: &odhContentModel.LicenseInfo{
				License:       "CC0",
				LicenseHolder: LICENSE_HOLDER,
			},
			GpsInfo: gpsFromAnyLang(de, byLang),
		},
		SmgActive:           true,
		OdhActive:           true,
		PublishedOn:         []string{},
		SyncUpdateMode:      "full",
		SyncSourceInterface: "suedtirolweincompany",
		PoiServices:         []string{},
		PoiProperty:         map[string][]odhContentModel.PoiPropertyEntry{},
		Detail:              map[string]*odhContentModel.DetailGeneric{lang: buildDetail(c, lang)},
		ContactInfos:        map[string]*odhContentModel.ContactInfo{lang: buildContactInfo(c, lang)},
		AdditionalContact:   buildAdditionalContacts(c, lang),
		ImageGallery:        imageGalleryFromAnyLang(de, byLang),
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
		poi.Detail = map[string]*odhContentModel.DetailGeneric{}
	}
	poi.Detail[lang] = buildDetail(c, lang)

	if poi.ContactInfos == nil {
		poi.ContactInfos = map[string]*odhContentModel.ContactInfo{}
	}
	poi.ContactInfos[lang] = buildContactInfo(c, lang)

	if poi.AdditionalContact == nil {
		poi.AdditionalContact = map[string][]odhContentModel.AdditionalContact{}
	}
	if contacts := buildAdditionalContacts(c, lang); contacts != nil {
		poi.AdditionalContact[lang] = contacts[lang]
	}

	if len(poi.Generic.GpsInfo) == 0 || (len(poi.Generic.GpsInfo) == 1 && poi.Generic.GpsInfo[0].Latitude == 0 && poi.Generic.GpsInfo[0].Longitude == 0) {
		if gps := gpsFrom(c); gps != nil {
			poi.Generic.GpsInfo = gps
		}
	}

	if len(poi.ImageGallery) == 0 || (len(poi.ImageGallery) == 1 && poi.ImageGallery[0].ImageUrl == "") {
		if gallery := buildImageGallery(c, c); len(gallery) > 0 {
			poi.ImageGallery = gallery
		}
	}
}

func buildDetail(c dto.WineCompany, lang string) *odhContentModel.DetailGeneric {
	return &odhContentModel.DetailGeneric{
		DetailGeneric: clib.DetailGeneric{
			Language: &lang,
			Title:    ptrOfStr(c.Title),
			BaseText: ptrOfStr(sanitizeHTML(c.CompanyDescription)),
		},
		Header:    ptrOfStr(sanitizeHTML(c.Slogan)),
		SubHeader: ptrOfStr(sanitizeHTML(c.Subtitle)),
		IntroText: ptrOfStr(sanitizeHTML(c.Quote)),
	}
}

func buildContactInfo(c dto.WineCompany, lang string) *odhContentModel.ContactInfo {
	website := c.Homepage
	if website != "" && !strings.Contains(website, "http") {
		website = "http://" + website
	}
	if !isValidURL(website) {
		website = ""
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
		CountryName: countryName,
		Phonenumber: c.Phone,
		Url:         website,
		Email:       c.Email,
		LogoUrl:     dto.AssetURL(c.Logo),
	}
}

func buildAdditionalContacts(c dto.WineCompany, lang string) map[string][]odhContentModel.AdditionalContact {
	if c.Importers == nil {
		return nil
	}
	importers := c.Importers.Importers()
	if len(importers) == 0 {
		return nil
	}

	var contacts []odhContentModel.AdditionalContact
	for _, imp := range importers {
		website := imp.ImporterHomepage
		if website != "" && !strings.Contains(website, "http") {
			website = "http://" + website
		}
		if !isValidURL(website) {
			website = ""
		}
		contacts = append(contacts, odhContentModel.AdditionalContact{
			Type:        "wineimporter",
			Description: imp.ImporterContactPerson,
			ContactInfo: &odhContentModel.ContactInfo{
				Language:    lang,
				CompanyName: imp.ImporterName,
				Address:     imp.ImporterAddress,
				ZipCode:     imp.ImporterZipCode,
				City:        imp.ImporterPlace,
				Phonenumber: imp.ImporterPhone,
				Email:       imp.ImporterEmail,
				Url:         website,
			},
		})
	}
	return map[string][]odhContentModel.AdditionalContact{lang: contacts}
}

func gpsFrom(c dto.WineCompany) []odhContentModel.GpsData {
	if c.Latitude == nil || c.Longitude == nil {
		return nil
	}
	lat := strings.TrimSpace(*c.Latitude)
	lon := strings.TrimSpace(*c.Longitude)

	if lat == "" || lon == "" || lat == "0" || lon == "0" {
		return nil
	}
	if strings.Contains(lat, "°") || strings.Contains(lon, "°") {
		return nil
	}

	latF := parseCoord(lat)
	lonF := parseCoord(lon)
	if latF == 0 && lonF == 0 {
		return nil
	}

	return []odhContentModel.GpsData{
		{Gpstype: ptrOf("position"), Latitude: latF, Longitude: lonF},
	}
}

func gpsFromAnyLang(preferred dto.WineCompany, byLang map[string]dto.WineCompany) []odhContentModel.GpsData {
	if gps := gpsFrom(preferred); gps != nil {
		return gps
	}
	for _, c := range byLang {
		if gps := gpsFrom(c); gps != nil {
			return gps
		}
	}
	return []odhContentModel.GpsData{}
}

func buildImageGallery(geoSource dto.WineCompany, c dto.WineCompany) []odhContentModel.ImageGalleryEntry {
	var gallery []odhContentModel.ImageGalleryEntry
	seen := map[string]bool{}
	pos := 0

	addImage := func(raw interface{}) {
		url := dto.AssetURL(raw)
		if url == "" || seen[url] {
			return
		}
		seen[url] = true
		gallery = append(gallery, odhContentModel.ImageGalleryEntry{
			ImageUrl:      url,
			ImageSource:   "suedtirolwein",
			CopyRight:     COPYRIGHT,
			LicenseHolder: LICENSE_HOLDER,
			IsInGallery:   true,
			ListPosition:  pos,
			ImageTitle:    nil,
			ImageDesc:     nil,
			ImageAltText:  nil,
		})
		pos++
	}

	if dto.AssetURL(geoSource.Media) != "" {
		addImage(geoSource.Media)
	} else {
		addImage(c.Media)
	}
	if dto.AssetURL(geoSource.MediaDetail) != "" {
		addImage(geoSource.MediaDetail)
	} else {
		addImage(c.MediaDetail)
	}
	if dto.AssetURL(geoSource.Logo) != "" {
		addImage(geoSource.Logo)
	} else {
		addImage(c.Logo)
	}

	return gallery
}

func imageGalleryFromAnyLang(preferred dto.WineCompany, byLang map[string]dto.WineCompany) []odhContentModel.ImageGalleryEntry {
	if gallery := buildImageGallery(preferred, preferred); len(gallery) > 0 {
		return gallery
	}
	for _, c := range byLang {
		if gallery := buildImageGallery(c, c); len(gallery) > 0 {
			return gallery
		}
	}
	return []odhContentModel.ImageGalleryEntry{}
}

func buildAdditionalProperties(byLang map[string]dto.WineCompany) *odhContentModel.SuedtirolWeinCompanyDataProperties {
	if len(byLang) == 0 {
		return nil
	}

	p := &odhContentModel.SuedtirolWeinCompanyDataProperties{
		H1:                      map[string]string{},
		H2:                      map[string]string{},
		Quote:                   map[string]string{},
		QuoteAuthor:             map[string]string{},
		Slogan:                  map[string]string{},
		FarmName:                map[string]string{},
		OpeningTimesWineShop:    map[string]string{},
		OpeningTimesGuides:      map[string]string{},
		OpeningTimesGastronomie: map[string]string{},
		CompanyHoliday:          map[string]string{},
	}

	if de, ok := byLang["de"]; ok {
		p.HasVisits = de.HasVisits
		p.HasOvernights = de.HasOvernights
		p.HasBiowine = de.HasBioWine
		p.HasAccommodation = ptrOf(de.HasAccomodation)
		p.HasOnlineshop = de.HasOnlineShop
		p.HasDeliveryservice = de.HasDeliveryService
		p.HasDirectSales = de.HasDirectSales
		p.IsVinumHotel = de.IsVinumHotel
		p.IsWineStories = de.IsWineStories
		p.IsWineSummit = de.IsWineSummit
		p.IsSparklingWineassociation = de.IsSparklingWineAssociation
		p.IsWinery = de.IsWinery
		p.IsWineryAssociation = de.IsWineryAssociation
		p.IsSkyalpsPartner = de.IsSkyAlpsPartner

		if de.OnlineShopURL != nil && isValidURL(*de.OnlineShopURL) {
			p.OnlineShopurl = ptrStringToFlexibleMap(de.OnlineShopURL)
		}
		if de.DeliveryServiceURL != nil && isValidURL(*de.DeliveryServiceURL) {
			p.DeliveryServiceUrl = ptrStringToFlexibleMap(de.DeliveryServiceURL)
		}
		if de.SocialsInstagram != nil && isValidURL(*de.SocialsInstagram) {
			p.SocialsInstagram = *de.SocialsInstagram
		}
		if de.SocialsFacebook != nil && isValidURL(*de.SocialsFacebook) {
			p.SocialsFacebook = *de.SocialsFacebook
		}
		if de.SocialsLinkedIn != nil && isValidURL(*de.SocialsLinkedIn) {
			p.SocialsLinkedIn = *de.SocialsLinkedIn
		}
		if de.SocialsPinterest != nil && isValidURL(*de.SocialsPinterest) {
			p.SocialsPinterest = *de.SocialsPinterest
		}
		if de.SocialsTikTok != nil && isValidURL(*de.SocialsTikTok) {
			p.SocialsTiktok = *de.SocialsTikTok
		}
		if de.SocialsYouTube != nil && isValidURL(*de.SocialsYouTube) {
			p.SocialsYoutube = *de.SocialsYouTube
		}
		if de.SocialsTwitter != nil && isValidURL(*de.SocialsTwitter) {
			p.SocialsTwitter = *de.SocialsTwitter
		}

		p.H1SparklingWineproducer = ptrStringToFlexibleMap(de.H1SparklingWineProducer)
		p.H2SparklingWineproducer = ptrStringToFlexibleMap(de.H2SparklingWineProducer)
		p.DescriptionSparklingWineproducer = ptrStringToFlexibleMap(de.DescriptionSparklingWineProducer)
	}

	for lang, c := range byLang {
		if c.H1 != "" {
			p.H1[lang] = sanitizeHTML(c.H1)
		}
		if c.Subtitle != "" {
			p.H2[lang] = sanitizeHTML(c.Subtitle)
		}
		if c.Quote != "" {
			p.Quote[lang] = sanitizeHTML(c.Quote)
		}
		if c.QuoteAuthor != "" {
			p.QuoteAuthor[lang] = sanitizeHTML(c.QuoteAuthor)
		}
		if c.Slogan != "" {
			p.Slogan[lang] = sanitizeHTML(c.Slogan)
		}
		if c.FarmName != "" {
			p.FarmName[lang] = sanitizeHTML(c.FarmName)
		}
		if c.OpeningTimesWineShop != nil && *c.OpeningTimesWineShop != "" {
			p.OpeningTimesWineShop[lang] = sanitizeHTML(*c.OpeningTimesWineShop)
		}
		if c.OpeningTimesGuides != nil && *c.OpeningTimesGuides != "" {
			p.OpeningTimesGuides[lang] = sanitizeHTML(*c.OpeningTimesGuides)
		}
		if c.OpeningTimesGastronomy != nil && *c.OpeningTimesGastronomy != "" {
			p.OpeningTimesGastronomie[lang] = sanitizeHTML(*c.OpeningTimesGastronomy)
		}
		if c.CompanyHoliday != nil && *c.CompanyHoliday != "" {
			p.CompanyHoliday[lang] = sanitizeHTML(*c.CompanyHoliday)
		}
	}

	nilIfEmpty := func(m odhContentModel.FlexibleMap) odhContentModel.FlexibleMap {
		if len(m) == 0 {
			return nil
		}
		return m
	}
	p.H1 = nilIfEmpty(p.H1)
	p.H2 = nilIfEmpty(p.H2)
	p.Quote = nilIfEmpty(p.Quote)
	p.QuoteAuthor = nilIfEmpty(p.QuoteAuthor)
	p.Slogan = nilIfEmpty(p.Slogan)
	p.FarmName = nilIfEmpty(p.FarmName)
	p.OpeningTimesWineShop = nilIfEmpty(p.OpeningTimesWineShop)
	p.OpeningTimesGuides = nilIfEmpty(p.OpeningTimesGuides)
	p.OpeningTimesGastronomie = nilIfEmpty(p.OpeningTimesGastronomie)
	p.CompanyHoliday = nilIfEmpty(p.CompanyHoliday)

	return p
}

func isValidURL(s string) bool {
	s = strings.TrimSpace(s)
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
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

func ptrOfStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrStringToFlexibleMap(s *string) odhContentModel.FlexibleMap {
	if s == nil || *s == "" {
		return nil
	}
	return odhContentModel.FlexibleMap{"de": *s}
}

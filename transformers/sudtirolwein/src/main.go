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

// noEscapeJSON serializes v to JSON without HTML-escaping (<, >, &).
// The ODH API stores and expects literal HTML tags (e.g. <br />) in text
// fields; using the default encoder would turn them into \u003cbr /\u003e
// which the API rejects with a 400.
func noEscapeJSON(v interface{}) (json.RawMessage, error) {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	// Encode appends a trailing newline; trim it.
	return json.RawMessage(bytes.TrimRight(buf.Bytes(), "\n")), nil
}

// sanitizeHTML normalizes HTML from the Statamic source to XHTML that the
// ODH API accepts. The API stores and expects self-closing tags like <br />
// rather than HTML5 <br> or <br/>, and literal & rather than &amp;.
func sanitizeHTML(s string) string {
	if s == "" {
		return s
	}
	s = strings.ReplaceAll(s, "&amp;", "&")
	s = strings.ReplaceAll(s, "<br>", "<br />")
	s = strings.ReplaceAll(s, "<br/>", "<br />")
	s = strings.ReplaceAll(s, "<hr>", "<hr />")
	s = strings.ReplaceAll(s, "<hr/>", "<hr />")
	return s
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
	deByID := make(map[string]dto.WineCompany, len(deCompanies))
	for _, c := range deCompanies {
		deByID[c.ID] = c
	}

	allLangsByID := map[string]map[string]dto.WineCompany{}
	for _, batch := range batches {
		for _, company := range batch.companies {
			if company.ID == "" {
				continue
			}
			if allLangsByID[company.ID] == nil {
				allLangsByID[company.ID] = map[string]dto.WineCompany{}
			}
			allLangsByID[company.ID][batch.lang] = company
		}
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
				poi := mapToPoi(id, company, batch.lang, deCopy, allLangsByID[company.ID], r.Timestamp)
				poi.AdditionalProperties = &odhContentModel.AdditionalProperties{
					SuedtirolWeinCompanyDataProperties: buildAdditionalProperties(allLangsByID[company.ID]),
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
			// The API returns a plain-text error body on conflict (not JSON),
			// so we cannot reliably detect "data exists already" by string match.
			// Always recover via PUT on any POST failure.
			logger.Get(ctx).Warn("POST failed, recovering via PUT", "id", id, "postErr", postErr)
			if putErr := contentClient.Put(ctx, ENTITY_TYPE, id, payload); putErr != nil {
				logger.Get(ctx).Error("API Put failed (recovery)", "id", id, "error", putErr)
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
				logger.Get(ctx).Error("API Put failed", "id", id, "error", putErr)
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Updated wine company", "id", id)
		}
		// exists && !changed — skip silently
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

		payload, serErr := noEscapeJSON(poi)
		if serErr != nil {
			logger.Get(ctx).Error("Failed to serialize POI for deactivation", "id", id, "error", serErr)
			continue
		}
		if err := contentClient.Put(ctx, ENTITY_TYPE, id, payload); err != nil {
			logger.Get(ctx).Error("Failed to deactivate wine company", "id", id, "error", err)
			continue
		}
		poiCache.Delete(id)
		logger.Get(ctx).Info("Deactivated missing wine company", "id", id)
	}

	return nil
}

// companiesFromLang returns the company list for a given language payload.
// LangData.Items() handles the nil check internally.
func companiesFromLang(ld *dto.LangData) []dto.WineCompany {
	if ld == nil {
		return nil
	}
	return ld.Items()
}

func buildID(c dto.WineCompany) string {
	return c.ID
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

	if len(poi.Generic.GpsInfo) == 0 {
		if gps := gpsFrom(c); gps != nil {
			poi.Generic.GpsInfo = gps
		}
	}

	if len(poi.ImageGallery) == 0 {
		if gallery := buildImageGallery(c, c); len(gallery) > 0 {
			poi.ImageGallery = gallery
		}
	}
}

// buildDetail maps a WineCompany to the ODH DetailGeneric structure.
// All string fields that may contain HTML are passed through sanitizeHTML so
// that tags like <br> are normalised to the self-closing <br /> form the API
// expects. noEscapeJSON (used when serialising the whole POI) then ensures
// those angle brackets are written as literal characters, not \u003c escapes.
func buildDetail(c dto.WineCompany, lang string) *odhContentModel.DetailGeneric {
	var baseText *string
	if c.CompanyDescription != "" {
		s := sanitizeHTML(c.CompanyDescription)
		baseText = &s
	}

	var header *string
	if c.Slogan != "" {
		s := sanitizeHTML(c.Slogan)
		header = &s
	}

	var subHeader *string
	if c.Subtitle != "" {
		s := sanitizeHTML(c.Subtitle)
		subHeader = &s
	}

	var introText *string
	if c.Quote != "" {
		s := sanitizeHTML(c.Quote)
		introText = &s
	}

	return &odhContentModel.DetailGeneric{
		DetailGeneric: clib.DetailGeneric{
			Language: &lang,
			Title:    ptrOf(c.Title),
			BaseText: baseText,
		},
		Header:    header,
		SubHeader: subHeader,
		IntroText: introText,
	}
}

// buildContactInfo maps a WineCompany to the ODH ContactInfo structure.
// URLs are validated: bare hostnames are prefixed with http://, and anything
// that still doesn't start with http:// or https:// is dropped to avoid
// sending a malformed URL that would cause a 400 from the API.
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

// buildAdditionalContacts maps wine importers to ODH AdditionalContact entries.
// Importer URLs receive the same http-prefix and validity check as the main
// contact URL so that no bare hostname reaches the API.
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

// gpsFrom extracts GPS coordinates from a single WineCompany.
// Returns nil when coordinates are missing, zero, or in non-decimal format.
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

// gpsFromAnyLang returns GPS coordinates preferring the DE version of the
// company, falling back to any other language that has valid coordinates.
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

// buildImageGallery constructs the image gallery for a POI.
// geoSource is used for Media/MediaDetail/Logo URL resolution (prefer DE);
// c is the same company and is kept as a parameter for symmetry with
// gpsFromAnyLang — the new DTO no longer carries per-language image metadata
// fields (ImageMetaTitle/Description/Alt were removed), so all entries get
// empty multilingual maps which other languages can populate via mergeLang.
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
			ImageTitle:    map[string]string{},
			ImageDesc:     map[string]string{},
			ImageAltText:  map[string]string{},
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

// imageGalleryFromAnyLang returns the image gallery preferring the DE
// company for URL resolution, falling back to any other language.
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
		// All boolean fields are native bool in the new DTO — assign directly.
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
			p.OnlineShopurl = &odhContentModel.FlexibleString{Value: *de.OnlineShopURL}
		}
		if de.DeliveryServiceURL != nil && isValidURL(*de.DeliveryServiceURL) {
			p.DeliveryServiceUrl = &odhContentModel.FlexibleString{Value: *de.DeliveryServiceURL}
		}
		if de.SocialsInstagram != nil && isValidURL(*de.SocialsInstagram) {
			p.SocialsInstagram = &odhContentModel.FlexibleString{Value: *de.SocialsInstagram}
		}
		if de.SocialsFacebook != nil && isValidURL(*de.SocialsFacebook) {
			p.SocialsFacebook = &odhContentModel.FlexibleString{Value: *de.SocialsFacebook}
		}
		if de.SocialsLinkedIn != nil && isValidURL(*de.SocialsLinkedIn) {
			p.SocialsLinkedIn = &odhContentModel.FlexibleString{Value: *de.SocialsLinkedIn}
		}
		if de.SocialsPinterest != nil && isValidURL(*de.SocialsPinterest) {
			p.SocialsPinterest = &odhContentModel.FlexibleString{Value: *de.SocialsPinterest}
		}
		if de.SocialsTikTok != nil && isValidURL(*de.SocialsTikTok) {
			p.SocialsTiktok = &odhContentModel.FlexibleString{Value: *de.SocialsTikTok}
		}
		if de.SocialsYouTube != nil && isValidURL(*de.SocialsYouTube) {
			p.SocialsYoutube = &odhContentModel.FlexibleString{Value: *de.SocialsYouTube}
		}
		if de.SocialsTwitter != nil && isValidURL(*de.SocialsTwitter) {
			p.SocialsTwitter = &odhContentModel.FlexibleString{Value: *de.SocialsTwitter}
		}
		if de.H1SparklingWineProducer != nil {
			p.H1SparklingWineproducer = &odhContentModel.FlexibleString{Value: *de.H1SparklingWineProducer}
		}
		if de.H2SparklingWineProducer != nil {
			p.H2SparklingWineproducer = &odhContentModel.FlexibleString{Value: *de.H2SparklingWineProducer}
		}
		if de.DescriptionSparklingWineProducer != nil {
			p.DescriptionSparklingWineproducer = &odhContentModel.FlexibleString{Value: *de.DescriptionSparklingWineProducer}
		}
	}

	for lang, c := range byLang {
		if c.H1 != "" {
			p.H1[lang] = c.H1
		}
		// H2 maps to Subtitle in the new DTO (the "subtitle" JSON field).
		if c.Subtitle != "" {
			p.H2[lang] = c.Subtitle
		}
		if c.Quote != "" {
			p.Quote[lang] = c.Quote
		}
		if c.QuoteAuthor != "" {
			p.QuoteAuthor[lang] = c.QuoteAuthor
		}
		if c.Slogan != "" {
			p.Slogan[lang] = c.Slogan
		}
		if c.FarmName != "" {
			p.FarmName[lang] = c.FarmName
		}
		// Opening times and holiday are *string in the new DTO.
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

// isValidURL returns true only for URLs that start with http:// or https://.
// Bare hostnames and other schemes are rejected to prevent the ODH API from
// returning a 400 on malformed URL fields.
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

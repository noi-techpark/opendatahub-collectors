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

	// allLangsByID collects all language versions per company for AdditionalProperties
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
				poi := mapToPoi(company, batch.lang, deCopy, r.Timestamp)
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

	// PoiServices from DE wines (language-neutral list)
	var poiServices []string
	if de.Wines != "" {
		poiServices = strings.Split(de.Wines, ",")
	}

	return odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:          &id,
			Active:      parseBool(c.Active),
			Source:      &source,
			Shortname:   &shortname,
			HasLanguage: []string{lang},
			LastChange:  odhContentModel.PtrFlexibleTime(ts),
			Mapping: map[string]map[string]string{
				SOURCE: {"id": c.ID},
			},
			TagIds: []string{
				"28CDEF87206E464D9B179FBCAF506457",
				"6EFED925DF3B4EF5B69495E994F446AC",
				"eating drinking",
				"gastronomy",
				"wineries",
			},
			// SmgTags is an obsolete field that must be filled for now for backwards compatibility
			SmgTags: []string{"gastronomy", "essen trinken", "weinkellereien"},
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
		PoiServices:         poiServices,
		Detail:              map[string]*odhContentModel.DetailGeneric{lang: buildDetail(c, lang)},
		ContactInfos:        map[string]*odhContentModel.ContactInfo{lang: buildContactInfo(c, lang)},
		AdditionalContact:   buildAdditionalContacts(c, lang),
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
		poi.Detail = map[string]*odhContentModel.DetailGeneric{}
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

	mergeAdditionalContacts(poi, buildAdditionalContacts(c, lang))

	if c.ImageMetaTitle != "" && len(poi.ImageGallery) > 0 {
		if poi.ImageGallery[0].ImageTitle == nil {
			poi.ImageGallery[0].ImageTitle = map[string]string{}
		}
		poi.ImageGallery[0].ImageTitle[lang] = c.ImageMetaTitle
	}
	if c.ImageMetaDescription != "" && len(poi.ImageGallery) > 0 {
		if poi.ImageGallery[0].ImageDesc == nil {
			poi.ImageGallery[0].ImageDesc = map[string]string{}
		}
		poi.ImageGallery[0].ImageDesc[lang] = c.ImageMetaDescription
	}
	if c.ImageMetaAlt != "" && len(poi.ImageGallery) > 0 {
		if poi.ImageGallery[0].ImageAltText == nil {
			poi.ImageGallery[0].ImageAltText = map[string]string{}
		}
		poi.ImageGallery[0].ImageAltText[lang] = c.ImageMetaAlt
	}
}

// ── Field builders ────────────────────────────────────────────────────────────

func buildMapping(c dto.WineCompany) map[string]string {
	return map[string]string{
		"id": c.ID,
	}
}

// buildDetail maps to our extended DetailGeneric which adds Header, SubHeader, IntroText.
// Header = slogan, SubHeader = subtitle, IntroText = quote (matching C# parser).
func buildDetail(c dto.WineCompany, lang string) *odhContentModel.DetailGeneric {
	return &odhContentModel.DetailGeneric{
		DetailGeneric: clib.DetailGeneric{
			Language: &lang,
			Title:    nullableString(c.Title),
			BaseText: nullableString(c.CompanyDescription),
		},
		Header:    ptrOf(c.Slogan),
		SubHeader: ptrOf(c.Subtitle),
		IntroText: nullableString(c.Quote),
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
		CountryName: countryName,
		Phonenumber: c.Phone,
		Url:         website,
		Email:       c.Email,
		LogoUrl:     c.Logo,
	}
}

// buildAdditionalContacts maps importer data into AdditionalContact entries.
func buildAdditionalContacts(c dto.WineCompany, lang string) []odhContentModel.AdditionalContact {
	if c.Importers == nil {
		return nil
	}
	importers := c.Importers.Importers()
	if len(importers) == 0 {
		return nil
	}

	contacts := make([]odhContentModel.AdditionalContact, 0, len(importers))
	seen := make(map[string]struct{}, len(importers))
	for _, imp := range importers {
		website := imp.ImporterHomepage
		if website != "" && !strings.Contains(website, "http") {
			website = "http://" + website
		}
		key := strings.Join([]string{
			strings.TrimSpace(imp.ImporterName),
			strings.TrimSpace(imp.ImporterAddress),
			strings.TrimSpace(imp.ImporterZipCode),
			strings.TrimSpace(imp.ImporterPlace),
			strings.TrimSpace(imp.ImporterPhone),
			strings.TrimSpace(imp.ImporterEmail),
			strings.TrimSpace(website),
		}, "|")
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
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
	return contacts
}

func mergeAdditionalContacts(poi *odhContentModel.ODHActivityPoi, contacts []odhContentModel.AdditionalContact) {
	if len(contacts) == 0 {
		return
	}
	if poi.AdditionalContact == nil {
		poi.AdditionalContact = []odhContentModel.AdditionalContact{}
	}
	seen := make(map[string]struct{}, len(poi.AdditionalContact))
	for _, existing := range poi.AdditionalContact {
		seen[additionalContactKey(existing)] = struct{}{}
	}
	for _, contact := range contacts {
		key := additionalContactKey(contact)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		poi.AdditionalContact = append(poi.AdditionalContact, contact)
	}
}

func additionalContactKey(c odhContentModel.AdditionalContact) string {
	var companyName, address, zipCode, city, phone, email, url, desc string
	if c.ContactInfo != nil {
		companyName = c.ContactInfo.CompanyName
		address = c.ContactInfo.Address
		zipCode = c.ContactInfo.ZipCode
		city = c.ContactInfo.City
		phone = c.ContactInfo.Phonenumber
		email = c.ContactInfo.Email
		url = c.ContactInfo.Url
	}
	desc = c.Description
	return strings.Join([]string{c.Type, desc, companyName, address, zipCode, city, phone, email, url}, "|")
}

// buildGpsInfo uses DE company data with validation (matching C# parser checks).
func buildGpsInfo(de dto.WineCompany) []odhContentModel.GpsData {
	lat, okLat := parseCoordWithOK(de.Latitude)
	lon, okLon := parseCoordWithOK(de.Longitude)

	if !okLat || !okLon || !isValidCoordinate(lat, lon) {
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

// buildImageGallery uses DE company data for images.
// imageMeta is a map of lang->title/desc/alt for multilingual image metadata.
func buildImageGallery(de dto.WineCompany) []odhContentModel.ImageGalleryEntry {
	var gallery []odhContentModel.ImageGalleryEntry
	seen := map[string]bool{}

	if de.Media != "" && !seen[de.Media] {
		seen[de.Media] = true
		entry := odhContentModel.ImageGalleryEntry{
			ImageUrl:      de.Media,
			ImageSource:   "SuedtirolWein",
			CopyRight:     COPYRIGHT,
			LicenseHolder: LICENSE_HOLDER,
			IsInGallery:   true,
			ListPosition:  0,
		}
		// Seed DE image meta — other languages added via mergeLang
		if de.ImageMetaTitle != "" {
			entry.ImageTitle = map[string]string{"de": de.ImageMetaTitle}
		}
		if de.ImageMetaDescription != "" {
			entry.ImageDesc = map[string]string{"de": de.ImageMetaDescription}
		}
		if de.ImageMetaAlt != "" {
			entry.ImageAltText = map[string]string{"de": de.ImageMetaAlt}
		}
		gallery = append(gallery, entry)
	}

	if de.MediaDetail != "" && !seen[de.MediaDetail] {
		seen[de.MediaDetail] = true
		gallery = append(gallery, odhContentModel.ImageGalleryEntry{
			ImageUrl:      de.MediaDetail,
			ImageSource:   "SuedtirolWein",
			CopyRight:     COPYRIGHT,
			LicenseHolder: LICENSE_HOLDER,
			IsInGallery:   true,
			ListPosition:  1,
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
	add("descriptionsparklingwineproducer", c.DescriptionSparklingWineProducer)
	add("h1sparklingwineproducer", c.H1SparklingWineProducer)
	add("h2sparklingwineproducer", c.H2SparklingWineProducer)
	add("imagesparklingwineproducer", c.ImageSparklingWineProducer)
	add("socialsinstagram", c.SocialsInstagram)
	add("socialsfacebook", c.SocialsFacebook)
	add("socialslinkedin", c.SocialsLinkedIn)
	add("socialspinterest", c.SocialsPinterest)
	add("socialstiktok", c.SocialsTikTok)
	add("socialsyoutube", c.SocialsYouTube)
	add("socialstwitter", c.SocialsTwitter)

	return props
}

// buildAdditionalProperties builds the structured SuedtirolWein-specific properties.
func buildAdditionalProperties(byLang map[string]dto.WineCompany) *odhContentModel.SuedtirolWeinCompanyDataProperties {
	if len(byLang) == 0 {
		return nil
	}

	p := &odhContentModel.SuedtirolWeinCompanyDataProperties{
		H1:                      map[string]string{},
		H2:                      map[string]string{},
		Quote:                   map[string]string{},
		QuoteAuthor:             map[string]string{},
		OpeningTimesWineShop:    map[string]string{},
		OpeningTimesGuides:      map[string]string{},
		OpeningTimesGastronomie: map[string]string{},
		CompanyHoliday:          map[string]string{},
	}

	// Use DE for boolean flags and language-neutral fields
	if de, ok := byLang["de"]; ok {
		p.HasVisits = parseBool(de.HasVisits)
		p.HasOvernights = parseBool(de.HasOvernights)
		p.HasBiowine = parseBool(de.HasBioWine)
		p.HasAccommodation = parseBool(de.HasAccomodation)
		p.HasOnlineshop = parseBool(de.HasOnlineShop)
		p.HasDeliveryservice = parseBool(de.HasDeliveryService)
		p.HasDirectSales = parseBool(de.HasDirectSales)
		p.IsVinumHotel = parseBool(de.IsVinumHotel)
		p.IsAnteprima = parseBool(de.IsAnteprima)
		p.IsWineStories = parseBool(de.IsWineStories)
		p.IsWineSummit = parseBool(de.IsWineSummit)
		p.IsSparklingWineassociation = parseBool(de.IsSparklingWineAssociation)
		p.IsWinery = parseBool(de.IsWinery)
		p.IsWineryAssociation = parseBool(de.IsWineryAssociation)
		p.IsSkyalpsPartner = parseBool(de.IsSkyAlpsPartner)

		if de.Wines != "" {
			p.Wines = strings.Split(de.Wines, ",")
		}

		p.OnlineShopurl = nullableString(de.OnlineShopURL)
		p.DeliveryServiceUrl = nullableString(de.DeliveryServiceURL)
		p.SocialsInstagram = nullableString(de.SocialsInstagram)
		p.SocialsFacebook = nullableString(de.SocialsFacebook)
		p.SocialsLinkedIn = nullableString(de.SocialsLinkedIn)
		p.SocialsPinterest = nullableString(de.SocialsPinterest)
		p.SocialsTiktok = nullableString(de.SocialsTikTok)
		p.SocialsYoutube = nullableString(de.SocialsYouTube)
		p.SocialsTwitter = nullableString(de.SocialsTwitter)
		p.H1SparklingWineproducer = nullableString(de.H1SparklingWineProducer)
		p.H2SparklingWineproducer = nullableString(de.H2SparklingWineProducer)
		p.ImageSparklingWineproducer = nullableString(de.ImageSparklingWineProducer)
		p.DescriptionSparklingWineproducer = nullableString(de.DescriptionSparklingWineProducer)
	}

	// Populate multilingual maps from all languages
	for lang, c := range byLang {
		if c.H1 != "" {
			p.H1[lang] = c.H1
		}
		if c.H2 != "" {
			p.H2[lang] = c.H2
		}
		if c.Quote != "" {
			p.Quote[lang] = c.Quote
		}
		if c.QuoteAuthor != "" {
			p.QuoteAuthor[lang] = c.QuoteAuthor
		}
		if c.OpeningTimesWineShop != "" {
			p.OpeningTimesWineShop[lang] = c.OpeningTimesWineShop
		}
		if c.OpeningTimesGuides != "" {
			p.OpeningTimesGuides[lang] = c.OpeningTimesGuides
		}
		if c.OpeningTimesGastronomy != "" {
			p.OpeningTimesGastronomie[lang] = c.OpeningTimesGastronomy
		}
		if c.CompanyHoliday != "" {
			p.CompanyHoliday[lang] = c.CompanyHoliday
		}
	}

	// Nil out empty maps for clean JSON
	if len(p.H1) == 0 {
		p.H1 = nil
	}
	if len(p.H2) == 0 {
		p.H2 = nil
	}
	if len(p.Quote) == 0 {
		p.Quote = nil
	}
	if len(p.QuoteAuthor) == 0 {
		p.QuoteAuthor = nil
	}
	if len(p.OpeningTimesWineShop) == 0 {
		p.OpeningTimesWineShop = nil
	}
	if len(p.OpeningTimesGuides) == 0 {
		p.OpeningTimesGuides = nil
	}
	if len(p.OpeningTimesGastronomie) == 0 {
		p.OpeningTimesGastronomie = nil
	}
	if len(p.CompanyHoliday) == 0 {
		p.CompanyHoliday = nil
	}

	return p
}

// ── Utility ───────────────────────────────────────────────────────────────────

func isValidCoordinate(lat, lon float64) bool {
	if math.IsNaN(lat) || math.IsNaN(lon) || math.IsInf(lat, 0) || math.IsInf(lon, 0) {
		return false
	}
	if lat == 0 && lon == 0 {
		return false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	return true
}

func parseBool(s string) bool {
	return strings.EqualFold(s, "true")
}

func parseCoordWithOK(s string) (float64, bool) {
	s = strings.TrimSpace(strings.ReplaceAll(s, ",", "."))
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseCoord(s string) float64 {
	v, _ := parseCoordWithOK(s)
	return v
}

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func ptrOf[T any](v T) *T { return &v }

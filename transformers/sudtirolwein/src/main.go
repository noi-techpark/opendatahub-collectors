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

	// Use a minimal stub to avoid time-parsing failures on ODH's non-RFC3339
	// timestamps (e.g. "2023-05-24T14:26:57.3413155" without timezone).
	type poiStub struct {
		ID *string `json:"Id"`
	}
	stubCache, err := clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[poiStub]{
		EntityType:  ENTITY_TYPE,
		QueryParams: map[string]string{"source": SOURCE},
		IDFunc: func(p poiStub) string {
			if p.ID == nil {
				return ""
			}
			return *p.ID
		},
	})
	ms.FailOnError(context.Background(), err, "failed to load existing POIs")

	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	for id := range stubCache.Entries() {
		poiCache.Set(id, odhContentModel.ODHActivityPoi{}, 0)
	}
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

	var toUpsert []odhContentModel.ODHActivityPoi
	for _, id := range sortedIDs {
		poi := pois[id]
		hash, changed, err := poiCache.HasChanged(id, poi)
		if err != nil {
			logger.Get(ctx).Error("Failed to hash POI", "id", id, "error", err)
			continue
		}
		if changed {
			poiCache.Set(id, poi, hash)
			toUpsert = append(toUpsert, poi)
		}
	}

	if len(toUpsert) > 0 {
		logger.Get(ctx).Info("Upserting changed POIs", "count", len(toUpsert))
		if err := contentClient.PutMultiple(ctx, ENTITY_TYPE, toUpsert); err != nil {
			return fmt.Errorf("failed to upsert POIs: %w", err)
		}
	}

	// Deactivate records no longer in source
	var toDeactivate []odhContentModel.ODHActivityPoi
	for id, entry := range poiCache.Entries() {
		if _, ok := seen[id]; ok {
			continue
		}
		poi := entry.Entity
		poi.Active = false
		toDeactivate = append(toDeactivate, poi)
		poiCache.Delete(id)
		logger.Get(ctx).Info("Deactivating missing wine company", "id", id)
	}
	if len(toDeactivate) > 0 {
		if err := contentClient.PutMultiple(ctx, ENTITY_TYPE, toDeactivate); err != nil {
			return fmt.Errorf("failed to deactivate POIs: %w", err)
		}
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
		return fmt.Sprintf("%s", c.ID)
	}
	return ""
}

func mapToPoi(c dto.WineCompany, lang string, de dto.WineCompany, ts time.Time) odhContentModel.ODHActivityPoi {
	id := buildID(c)
	source := SOURCE
	shortname := c.Title

	poiServices := []string{}
	if de.Wines != "" {
		poiServices = strings.Split(de.Wines, ",")
	}

	additionalContacts := buildAdditionalContacts(c, lang)
	if additionalContacts == nil {
		additionalContacts = []odhContentModel.AdditionalContact{}
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
		AdditionalContact:   additionalContacts,
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

	poi.AdditionalContact = append(poi.AdditionalContact, buildAdditionalContacts(c, lang)...)

	if len(poi.ImageGallery) > 0 {
		if c.ImageMetaTitle != "" {
			if poi.ImageGallery[0].ImageTitle == nil {
				poi.ImageGallery[0].ImageTitle = map[string]string{}
			}
			poi.ImageGallery[0].ImageTitle[lang] = c.ImageMetaTitle
		}
		if c.ImageMetaDescription != "" {
			if poi.ImageGallery[0].ImageDesc == nil {
				poi.ImageGallery[0].ImageDesc = map[string]string{}
			}
			poi.ImageGallery[0].ImageDesc[lang] = c.ImageMetaDescription
		}
		if c.ImageMetaAlt != "" {
			if poi.ImageGallery[0].ImageAltText == nil {
				poi.ImageGallery[0].ImageAltText = map[string]string{}
			}
			poi.ImageGallery[0].ImageAltText[lang] = c.ImageMetaAlt
		}
	}
}

func buildDetail(c dto.WineCompany, lang string) *odhContentModel.DetailGeneric {
	return &odhContentModel.DetailGeneric{
		DetailGeneric: clib.DetailGeneric{
			Language: &lang,
			Title:    ptrOf(c.Title),
			BaseText: ptrOf(c.CompanyDescription),
		},
		Header:    ptrOf(c.Slogan),
		SubHeader: ptrOf(c.Subtitle),
		IntroText: ptrOf(c.Quote),
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

func buildAdditionalContacts(c dto.WineCompany, lang string) []odhContentModel.AdditionalContact {
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
		contacts = append(contacts, odhContentModel.AdditionalContact{
			Type:        "wineimporter",
			Description: imp.ImporterDescription,
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

func buildGpsInfo(de dto.WineCompany) []odhContentModel.GpsData {
	lat := de.Latitude
	lon := de.Longitude

	if lat == "" || lon == "" || lat == "0" || lon == "0" {
		return []odhContentModel.GpsData{}
	}
	if strings.Contains(lat, "°") || strings.Contains(lon, "°") {
		return []odhContentModel.GpsData{}
	}

	latF := parseCoord(lat)
	lonF := parseCoord(lon)
	if latF == 0 && lonF == 0 {
		return []odhContentModel.GpsData{}
	}

	return []odhContentModel.GpsData{
		{Gpstype: ptrOf("position"), Latitude: latF, Longitude: lonF},
	}
}

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
			ImageTitle:    map[string]string{},
			ImageDesc:     map[string]string{},
			ImageAltText:  map[string]string{},
		}
		if de.ImageMetaTitle != "" {
			entry.ImageTitle["de"] = de.ImageMetaTitle
		}
		if de.ImageMetaDescription != "" {
			entry.ImageDesc["de"] = de.ImageMetaDescription
		}
		if de.ImageMetaAlt != "" {
			entry.ImageAltText["de"] = de.ImageMetaAlt
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
	add("socialslinkedIn", c.SocialsLinkedIn)
	add("socialspinterest", c.SocialsPinterest)
	add("socialstiktok", c.SocialsTikTok)
	add("socialsyoutube", c.SocialsYouTube)
	add("socialstwitter", c.SocialsTwitter)

	return props
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
		OpeningTimesWineShop:    map[string]string{},
		OpeningTimesGuides:      map[string]string{},
		OpeningTimesGastronomie: map[string]string{},
		CompanyHoliday:          map[string]string{},
	}

	if de, ok := byLang["de"]; ok {
		p.HasVisits = parseBool(de.HasVisits)
		p.HasOvernights = parseBool(de.HasOvernights)
		p.HasBiowine = parseBool(de.HasBioWine)
		p.HasAccommodation = nullableBool(de.HasAccomodation)
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

func nullableString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func nullableBool(s string) *bool {
	if s == "" {
		return nil
	}
	v := parseBool(s)
	return &v
}

func ptrOf[T any](v T) *T { return &v }

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

// langBatch pairs a language code with its slice of companies.
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

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	batches := []langBatch{
		{"de", r.Rawdata.De.Items()},
		{"it", r.Rawdata.It.Items()},
		{"en", r.Rawdata.En.Items()},
		{"ru", r.Rawdata.Ru.Items()},
	}

	logger.Get(ctx).Info("Processing wine company data")

	// Index DE entries by slug for use as the authoritative source for
	// GPS, images, and boolean flags (language-neutral fields).
	deBySlug := make(map[string]dto.WineCompany)
	for _, c := range r.Rawdata.De.Items() {
		deBySlug[c.Slug] = c
	}

	// Build a full index of all languages keyed by slug for AdditionalProperties.
	allLangsBySlug := map[string]map[string]dto.WineCompany{}
	for _, batch := range batches {
		for _, c := range batch.companies {
			if c.Slug == "" {
				continue
			}
			if allLangsBySlug[c.Slug] == nil {
				allLangsBySlug[c.Slug] = map[string]dto.WineCompany{}
			}
			allLangsBySlug[c.Slug][batch.lang] = c
		}
	}

	pois := map[string]odhContentModel.ODHActivityPoi{}
	seen := map[string]struct{}{}

	for _, batch := range batches {
		for _, company := range batch.companies {
			if company.Slug == "" {
				logger.Get(ctx).Warn("Skipping company with empty slug", "title", company.Title)
				continue
			}

			// Only process published entries.
			if !company.Published {
				continue
			}

			id := company.Slug
			seen[id] = struct{}{}

			if existing, ok := pois[id]; ok {
				mergeLang(&existing, company, batch.lang)
				pois[id] = existing
			} else {
				de, hasDe := deBySlug[company.Slug]
				if !hasDe {
					de = company
				}
				poi := mapToPoi(company, batch.lang, de, r.Timestamp)
				poi.AdditionalProperties = &odhContentModel.AdditionalProperties{
					SuedtirolWeinCompanyDataProperties: buildAdditionalProperties(allLangsBySlug[company.Slug]),
				}
				pois[id] = poi
			}
		}
	}

	// Sort for deterministic processing order.
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
			if !strings.Contains(postErr.Error(), "data exists already") {
				logger.Get(ctx).Error("API Post failed", "id", id, "error", postErr)
				continue
			}
			logger.Get(ctx).Warn("POST conflict, recovering with PUT", "id", id)
			putPayload, serErr := noEscapeJSON(poi)
			if serErr != nil {
				logger.Get(ctx).Error("Failed to serialize POI for PUT recovery", "id", id, "error", serErr)
				continue
			}
			if putErr := contentClient.Put(ctx, ENTITY_TYPE, id, putPayload); putErr != nil {
				logger.Get(ctx).Error("API Put failed (recovery)", "id", id, "error", putErr)
				continue
			}
			poiCache.Set(id, poi, hash)
			logger.Get(ctx).Info("Recovered stale-cache wine company via PUT", "id", id)

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
	}

	// Deactivate records no longer present in source.
	for id := range poiCache.Entries() {
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

func mapToPoi(c dto.WineCompany, lang string, de dto.WineCompany, ts time.Time) odhContentModel.ODHActivityPoi {
	id := c.Slug
	source := SOURCE
	shortname := c.Title

	// Build wine services list from the DE entry (language-neutral).
	poiServices := []string{}
	if de.WineryVisits {
		poiServices = append(poiServices, "winery_visits")
	}
	if de.DeliveryService {
		poiServices = append(poiServices, "delivery_service")
	}
	if de.OnlineShop {
		poiServices = append(poiServices, "online_shop")
	}
	if de.DirectSales {
		poiServices = append(poiServices, "direct_sales")
	}
	if de.Catering {
		poiServices = append(poiServices, "catering")
	}
	if de.OvernightStay {
		poiServices = append(poiServices, "overnight_stay")
	}
	if de.Skyalps {
		poiServices = append(poiServices, "skyalps")
	}

	// Build mapping — include legacyNumber for matching existing ODH records.
	mapping := map[string]map[string]string{
		SOURCE: {
			"slug": c.Slug,
		},
	}
	if de.LegacyNumber != "" {
		mapping[SOURCE]["legacyid"] = de.LegacyNumber
	}

	return odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:          &id,
			Active:      c.Published,
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
			GpsInfo: buildGpsInfo(de),
		},
		SmgActive:           true,
		PublishedOn:         []string{},
		SyncUpdateMode:      "full",
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

	if poi.AdditionalContact == nil {
		poi.AdditionalContact = map[string][]odhContentModel.AdditionalContact{}
	}
	if contacts := buildAdditionalContacts(c, lang); contacts != nil {
		poi.AdditionalContact[lang] = contacts[lang]
	}
}

func buildDetail(c dto.WineCompany, lang string) *odhContentModel.DetailGeneric {
	return &odhContentModel.DetailGeneric{
		DetailGeneric: clib.DetailGeneric{
			Language: &lang,
			Title:    ptrOf(c.Title),
			BaseText: ptrOf(c.Intro),
		},
		Header:    ptrOf(c.Headline),
		SubHeader: ptrOf(c.Subtitle),
		IntroText: ptrOf(c.QuoteText),
	}
}

func buildContactInfo(c dto.WineCompany, lang string) *odhContentModel.ContactInfo {
	website := c.Website
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
		City:        c.Location,
		CountryCode: "IT",
		CountryName: countryName,
		Phonenumber: c.Phone,
		Url:         website,
		Email:       c.Email,
		LogoUrl:     dto.AssetURL(c.Logo),
	}
}

func buildAdditionalContacts(c dto.WineCompany, lang string) map[string][]odhContentModel.AdditionalContact {
	if len(c.Importers.Importers()) == 0 {
		return nil
	}

	var contacts []odhContentModel.AdditionalContact
	for _, imp := range c.Importers.Importers() {
		website := imp.ImporterHomepage
		if website != "" && !strings.Contains(website, "http") {
			website = "http://" + website
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

func buildGpsInfo(de dto.WineCompany) []odhContentModel.GpsData {
	if de.Latitude == nil || de.Longitude == nil {
		return []odhContentModel.GpsData{}
	}
	lat := strings.TrimSpace(*de.Latitude)
	lon := strings.TrimSpace(*de.Longitude)
	if lat == "" || lon == "" || lat == "0" || lon == "0" {
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

	addImage := func(raw interface{}, pos int) {
		url := dto.AssetURL(raw)
		if url == "" {
			return
		}
		gallery = append(gallery, odhContentModel.ImageGalleryEntry{
			ImageUrl:      url,
			ImageSource:   "SuedtirolWein",
			CopyRight:     COPYRIGHT,
			LicenseHolder: LICENSE_HOLDER,
			IsInGallery:   true,
			ListPosition:  pos,
			ImageTitle:    map[string]string{},
			ImageDesc:     map[string]string{},
			ImageAltText:  map[string]string{},
		})
	}

	addImage(de.ImageHeader, 0)
	addImage(de.ImagePreview, 1)

	return gallery
}

func buildPoiProperty(c dto.WineCompany) []odhContentModel.PoiPropertyEntry {
	var props []odhContentModel.PoiPropertyEntry
	add := func(name, value string) {
		if value != "" {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: name, Value: value})
		}
	}
	addBool := func(name string, value bool) {
		props = append(props, odhContentModel.PoiPropertyEntry{Name: name, Value: strconv.FormatBool(value)})
	}
	addPtr := func(name string, value *string) {
		if value != nil && *value != "" {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: name, Value: *value})
		}
	}

	add("slogan", c.Slogan)
	add("subtitle", c.Subtitle)
	add("quote", c.QuoteText)
	add("quoteauthor", c.QuoteAuthor)
	add("h1", c.Headline)
	add("h2", c.Subtitle)
	add("farmname", c.Hofname)
	addPtr("openingtimeswineshop", c.OpeningHoursWineSales)
	addPtr("openingtimesguides", c.OpeningHoursCellarTours)
	addPtr("openingtimesgastronomie", c.OpeningHoursRestaurant)
	addPtr("companyholiday", c.Holiday)
	addBool("hasvisits", c.WineryVisits)
	addBool("hasovernights", c.OvernightStay)
	addBool("hasbiowine", c.OrganicWine)
	addBool("hasaccomodation", c.Catering)
	addBool("hasonlineshop", c.OnlineShop)
	addBool("hasdeliveryservice", c.DeliveryService)
	addBool("hasdirectsales", c.DirectSales)
	addBool("isvinumhotel", c.VinumHotel)
	addBool("iswinestories", c.WineStories)
	addBool("iswinesummit", c.WineSummit)
	addBool("issparklingwineassociation", c.SparklingWineAssociation)
	addBool("iswinery", c.Winery)
	addBool("iswineryassociation", c.WineryAssociation)
	addBool("isskyalpspartner", c.Skyalps)
	addPtr("onlineshopurl", c.URLOnlineShop)
	addPtr("deliveryserviceurl", c.URLDeliveryService)
	add("socialsinstagram", ptrStr(c.Instagram))
	add("socialsfacebook", ptrStr(c.Facebook))
	add("socialslinkedIn", ptrStr(c.LinkedIn))
	add("socialspinterest", ptrStr(c.Pinterest))
	add("socialstiktok", ptrStr(c.TikTok))
	add("socialsyoutube", ptrStr(c.YouTube))
	add("socialstwitter", ptrStr(c.Twitter))
	addPtr("h1sparklingwineproducer", c.SparklingWineProducerHeadline)
	addPtr("h2sparklingwineproducer", c.SparklingWineProducerSubheadline)
	addPtr("descriptionsparklingwineproducer", c.SparklingWineProducerText)

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
		p.HasVisits = de.WineryVisits
		p.HasOvernights = de.OvernightStay
		p.HasBiowine = de.OrganicWine
		p.HasAccommodation = &de.Catering
		p.HasOnlineshop = de.OnlineShop
		p.HasDeliveryservice = de.DeliveryService
		p.HasDirectSales = de.DirectSales
		p.IsVinumHotel = de.VinumHotel
		p.IsWineStories = de.WineStories
		p.IsWineSummit = de.WineSummit
		p.IsSparklingWineassociation = de.SparklingWineAssociation
		p.IsWinery = de.Winery
		p.IsWineryAssociation = de.WineryAssociation
		p.IsSkyalpsPartner = de.Skyalps
		if de.URLOnlineShop != nil {
			p.OnlineShopurl = de.URLOnlineShop
		}
		if de.URLDeliveryService != nil {
			p.DeliveryServiceUrl = de.URLDeliveryService
		}
		p.SocialsInstagram = de.Instagram
		p.SocialsFacebook = de.Facebook
		p.SocialsLinkedIn = de.LinkedIn
		p.SocialsPinterest = de.Pinterest
		p.SocialsTiktok = de.TikTok
		p.SocialsYoutube = de.YouTube
		p.SocialsTwitter = de.Twitter
		if de.SparklingWineProducerHeadline != nil {
			p.H1SparklingWineproducer = de.SparklingWineProducerHeadline
		}
		if de.SparklingWineProducerSubheadline != nil {
			p.H2SparklingWineproducer = de.SparklingWineProducerSubheadline
		}
		if de.SparklingWineProducerText != nil {
			p.DescriptionSparklingWineproducer = de.SparklingWineProducerText
		}
	}

	for lang, c := range byLang {
		if c.Headline != "" {
			p.H1[lang] = c.Headline
		}
		if c.Subtitle != "" {
			p.H2[lang] = c.Subtitle
		}
		if c.QuoteText != "" {
			p.Quote[lang] = c.QuoteText
		}
		if c.QuoteAuthor != "" {
			p.QuoteAuthor[lang] = c.QuoteAuthor
		}
		if c.OpeningHoursWineSales != nil && *c.OpeningHoursWineSales != "" {
			p.OpeningTimesWineShop[lang] = *c.OpeningHoursWineSales
		}
		if c.OpeningHoursCellarTours != nil && *c.OpeningHoursCellarTours != "" {
			p.OpeningTimesGuides[lang] = *c.OpeningHoursCellarTours
		}
		if c.OpeningHoursRestaurant != nil && *c.OpeningHoursRestaurant != "" {
			p.OpeningTimesGastronomie[lang] = *c.OpeningHoursRestaurant
		}
		if c.Holiday != nil && *c.Holiday != "" {
			p.CompanyHoliday[lang] = *c.Holiday
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
	p.OpeningTimesWineShop = nilIfEmpty(p.OpeningTimesWineShop)
	p.OpeningTimesGuides = nilIfEmpty(p.OpeningTimesGuides)
	p.OpeningTimesGastronomie = nilIfEmpty(p.OpeningTimesGastronomie)
	p.CompanyHoliday = nilIfEmpty(p.CompanyHoliday)

	return p
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

func ptrStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

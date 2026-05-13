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
	"opendatahub.com/tr-siag-museum/dto"
	odhContentModel "opendatahub.com/tr-siag-museum/odh-content-model"
)

const (
	SOURCE         = "siag"
	ENTITY_TYPE    = "ODHActivityPoi"
	LICENSE_HOLDER = "http://www.provinz.bz.it/kunst-kultur/museen"
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
	slog.Info("Starting SIAG museum transformer...")
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

func Transform(ctx context.Context, r *rdb.Raw[dto.RawData]) error {
	logger.Get(ctx).Info("Processing museum data")

	pois := map[string]odhContentModel.ODHActivityPoi{}
	// seen tracks every id present in the current source batch.
	seen := map[string]struct{}{}

	// tagDefs accumulates all unique tags encountered in this batch so they
	// can be synced to the Tag endpoint before we touch any POI records.
	// We use a map keyed by tag ID to deduplicate, then convert to a slice
	// for SyncTags (which expects clib.TagDefs, i.e. []clib.TagDef).
	tagDefsMap := map[string]clib.TagDef{}

	for _, batch := range []struct {
		lang    string
		museums []dto.SiagMuseum
	}{
		{"de", r.Rawdata.De},
		{"it", r.Rawdata.It},
		{"en", r.Rawdata.En},
	} {
		for _, museum := range batch.museums {
			id := buildID(museum)
			seen[id] = struct{}{}

			collectTagDefs(museum.Elements, tagDefsMap)

			if existing, ok := pois[id]; ok {
				mergeLang(&existing, museum, batch.lang)
				pois[id] = existing
			} else {
				pois[id] = mapToPoi(museum, batch.lang)
			}
		}
	}

	// Convert deduplicated map to the slice type clib.SyncTags expects.
	tagDefs := make(clib.TagDefs, 0, len(tagDefsMap))
	for _, def := range tagDefsMap {
		tagDefs = append(tagDefs, def)
	}

	// Sync all tags discovered in this batch to the ODH Tag endpoint.
	// clib.SyncTags POSTs each tag and ignores ErrAlreadyExists, so it is
	// safe to call on every run — new tags get created, existing ones are
	// left untouched.
	if err := clib.SyncTags(ctx, contentClient, tagDefs, clib.SyncTagsConfig{
		Source: SOURCE,
	}); err != nil {
		// Log and continue: a tag-sync failure must not block POI updates.
		logger.Get(ctx).Error("Failed to sync tags", "error", err)
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
			// Attempt POST; if the API reports the record already exists
			// (stale cache), fall through to PUT to reconcile.
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
		// exists && !changed → nothing to do
	}

	// DEACTIVATION: Items in cache that are absent from the current batch.
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
		logger.Get(ctx).Info("Deactivated missing museum", "id", id)
	}

	return nil
}

// collectTagDefs inspects a museum's elements and merges every tag it carries
// into the provided deduplication map.  The map key is the tag ID; values are
// clib.TagDef structs using the NameDe/NameIt/NameEn fields that clib expects.
func collectTagDefs(e dto.SiagElements, defs map[string]clib.TagDef) {
	// addDef handles taxonomy tags (categories, services, offerings) whose
	// display name is pipe-separated: "de|it|de|en".
	addDef := func(codename, name string) {
		id := "siag:museum:" + codename
		if _, exists := defs[id]; exists {
			return
		}
		defs[id] = clib.TagDef{
			ID:     id,
			NameDe: langName(name, "de"),
			NameIt: langName(name, "it"),
			NameEn: langName(name, "en"),
			Types:  []string{"MuseumData"},
		}
	}

	// addSimple handles boolean-flag tags (paramuseum, provincial_museum,
	// museum_association) that share the same label in every language.
	addSimple := func(id, displayName string) {
		if _, exists := defs[id]; exists {
			return
		}
		defs[id] = clib.TagDef{
			ID:     id,
			NameDe: displayName,
			NameIt: displayName,
			NameEn: displayName,
			Types:  []string{"MuseumData"},
		}
	}

	for _, t := range e.MuseumCategories.Value {
		addDef(t.Codename, t.Name)
	}
	for _, t := range e.MuseumServices.Value {
		addDef(t.Codename, t.Name)
	}
	for _, t := range e.MuseumOfferings.Value {
		addDef(t.Codename, t.Name)
	}

	if choiceIsYes(e.Paramuseum) {
		addSimple("siag:museum:paramuseum", "Paramuseum")
	}
	if choiceIsYes(e.ProvincialMuseum) {
		addSimple("siag:museum:provincial_museum", "Provincial museum")
	}
	if choiceIsYes(e.MuseumAssociation) {
		addSimple("siag:museum:museum_association", "Museum association")
	}
}

// buildID generates the ODH Id: "smgpoi{numericId}siag"
// Falls back to system.codename when numeric id is null.
func buildID(m dto.SiagMuseum) string {
	if m.Elements.Id.Value != nil {
		return fmt.Sprintf("smgpoi%dsiag", int(*m.Elements.Id.Value))
	}
	return m.System.Codename
}

func mapToPoi(m dto.SiagMuseum, lang string) odhContentModel.ODHActivityPoi {
	e := m.Elements
	id := buildID(m)
	source := SOURCE
	shortname := e.Title.Value
	lon := parseCoord(e.GeoCoordX.Value)
	lat := parseCoord(e.GeoCoordY.Value)

	lastChange := parseLastModified(m.System.LastModified)
	tagIds, smgTags := buildTagIds(e)

	entryVal := ""
	if len(e.MuseumOfferings.Value) > 0 {
		idx := 0
		if len(e.MuseumOfferings.Value) > 1 {
			idx = 1
		}
		entryVal = langName(e.MuseumOfferings.Value[idx].Name, lang)
	}

	return odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:          &id,
			Active:      isActive(m),
			Source:      &source,
			Shortname:   &shortname,
			HasLanguage: []string{lang},
			LastChange:  odhContentModel.PtrFlexibleTime(lastChange),
			Mapping: map[string]map[string]string{
				SOURCE: museumMapping(m),
			},
			TagIds:  tagIds,
			SmgTags: smgTags,
			LicenseInfo: &odhContentModel.LicenseInfo{
				License:       "CC0",
				LicenseHolder: LICENSE_HOLDER,
			},
			GpsInfo: []odhContentModel.GpsData{
				{
					Gpstype:   ptrOf("position"),
					Latitude:  lat,
					Longitude: lon,
				},
			},
		},
		SmgActive:           true,
		PublishedOn:         []string{},
		SyncUpdateMode:      "full",
		SyncSourceInterface: "museumdata",
		HasFreeEntrance:     hasFreeEntrance(e),
		Detail:              map[string]*clib.DetailGeneric{lang: buildDetail(e, lang)},
		ContactInfos:        map[string]*odhContentModel.ContactInfo{lang: buildContactInfo(e, lang)},
		ImageGallery:        buildImageGallery(e),
		PoiProperty:         map[string][]odhContentModel.PoiPropertyEntry{lang: buildPoiProperty(e, lang)},

		AdditionalProperties: &odhContentModel.AdditionalProperties{
			SiagMuseumDataProperties: &odhContentModel.SiagMuseumDataProperties{
				Entry:        map[string]string{lang: entryVal},
				OpeningTimes: map[string]string{lang: e.OpeningHours.Value},
			},
		},
	}
}

func mergeLang(poi *odhContentModel.ODHActivityPoi, m dto.SiagMuseum, lang string) {
	e := m.Elements

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
	poi.Detail[lang] = buildDetail(e, lang)

	if poi.ContactInfos == nil {
		poi.ContactInfos = map[string]*odhContentModel.ContactInfo{}
	}
	poi.ContactInfos[lang] = buildContactInfo(e, lang)

	if poi.PoiProperty == nil {
		poi.PoiProperty = map[string][]odhContentModel.PoiPropertyEntry{}
	}
	poi.PoiProperty[lang] = buildPoiProperty(e, lang)

	if poi.AdditionalProperties == nil {
		poi.AdditionalProperties = &odhContentModel.AdditionalProperties{}
	}
	if poi.AdditionalProperties.SiagMuseumDataProperties == nil {
		poi.AdditionalProperties.SiagMuseumDataProperties = &odhContentModel.SiagMuseumDataProperties{
			Entry:        map[string]string{},
			OpeningTimes: map[string]string{},
		}
	}

	p := poi.AdditionalProperties.SiagMuseumDataProperties

	if len(e.MuseumOfferings.Value) > 0 {
		idx := 0
		if len(e.MuseumOfferings.Value) > 1 {
			idx = 1
		}
		p.Entry[lang] = langName(e.MuseumOfferings.Value[idx].Name, lang)
	} else {
		p.Entry[lang] = ""
	}

	p.OpeningTimes[lang] = e.OpeningHours.Value
}

// ── Field builders ────────────────────────────────────────────────────────────

func museumMapping(m dto.SiagMuseum) map[string]string {
	mapping := map[string]string{
		"system.id":            m.System.Id,
		"system.codename":      m.System.Codename,
		"system.type":          m.System.Type,
		"system.last_modified": m.System.LastModified,
	}
	if m.Elements.Id.Value != nil {
		mapping["museId"] = strconv.Itoa(int(*m.Elements.Id.Value))
	}
	return mapping
}

func buildTagIds(e dto.SiagElements) (tagIds []string, smgTags []string) {
	for _, t := range e.MuseumCategories.Value {
		tagIds = append(tagIds, "siag:museum:"+t.Codename)
	}
	for _, t := range e.MuseumServices.Value {
		tagIds = append(tagIds, "siag:museum:"+t.Codename)
	}
	for _, t := range e.MuseumOfferings.Value {
		tagIds = append(tagIds, "siag:museum:"+t.Codename)
	}
	if choiceIsYes(e.Paramuseum) {
		tagIds = append(tagIds, "siag:museum:paramuseum")
	}
	if choiceIsYes(e.ProvincialMuseum) {
		tagIds = append(tagIds, "siag:museum:provincial_museum")
	}
	if choiceIsYes(e.MuseumAssociation) {
		tagIds = append(tagIds, "siag:museum:museum_association")
	}

	tagIds, smgTags = addCompatibilityTags(tagIds)
	return tagIds, smgTags
}

func addCompatibilityTags(tags []string) (tagIds []string, smgTags []string) {
	tagIds = tags

	has := func(tag string) bool {
		for _, t := range tagIds {
			if t == tag {
				return true
			}
		}
		return false
	}
	addTag := func(tag string) {
		if !has(tag) {
			tagIds = append(tagIds, tag)
		}
	}

	smgTags = []string{"poi", "kultur sehenswürdigkeiten", "museen"}

	if has("siag:museum:culture") {
		addTag("museums culture")
		smgTags = append(smgTags, "museen kultur")
	}
	if has("siag:museum:nature") {
		addTag("museums nature")
		smgTags = append(smgTags, "museen natur")
	}
	if has("siag:museum:technology") {
		addTag("museums technology")
		smgTags = append(smgTags, "museen technik")
	}
	if has("siag:museum:art") {
		addTag("museums art")
		smgTags = append(smgTags, "museen kunst")
	}
	if has("siag:museum:mine") {
		addTag("mines")
		smgTags = append(smgTags, "bergwerke")
	}
	if has("siag:museum:natureparks") {
		addTag("nature park visitors centres")
		smgTags = append(smgTags, "naturparkhäuser")
	}
	if has("siag:museum:barrier_free") {
		addTag("barrierfree")
		smgTags = append(smgTags, "barrierefrei")
	}
	if has("siag:museum:offers_for_schools") {
		smgTags = append(smgTags, "familientip")
	}

	return tagIds, smgTags
}

func buildDetail(e dto.SiagElements, lang string) *clib.DetailGeneric {
	return &clib.DetailGeneric{
		Language: &lang,
		Title:    &e.Title.Value,
		BaseText: &e.Description.Value,
	}
}

func buildContactInfo(e dto.SiagElements, lang string) *odhContentModel.ContactInfo {
	area := ""
	if len(e.Districts.Value) > 0 {
		area = langName(e.Districts.Value[0].Name, lang)
	}

	info := &odhContentModel.ContactInfo{
		Language:    lang,
		Email:       e.Email.Value,
		Phonenumber: firstNonEmpty(e.Phone.Value, e.Phone2.Value),
		Url:         e.Web.Value,
		City:        e.Municipality.Value,
		ZipCode:     e.ZipCode.Value,
		CountryCode: "IT",
		Area:        area,
	}
	if lang == "de" || lang == "it" {
		info.Address = e.Street.Value
		info.CompanyName = e.Title.Value
	}
	return info
}

func buildImageGallery(e dto.SiagElements) []odhContentModel.ImageGalleryEntry {
	var gallery []odhContentModel.ImageGalleryEntry
	seen := map[string]bool{}
	for i, asset := range append(e.MainImage.Value, e.PhotoGallery.Value...) {
		if asset.Url == "" || seen[asset.Url] {
			continue
		}
		seen[asset.Url] = true
		gallery = append(gallery, odhContentModel.ImageGalleryEntry{
			ImageUrl:     asset.Url,
			ImageName:    asset.Name,
			ImageDesc:    map[string]string{"de": asset.Description, "it": asset.Description, "en": asset.Description},
			IsInGallery:  true,
			ListPosition: i,
			Width:        asset.Width,
			Height:       asset.Height,
			ImageSource:  SOURCE,
			ImageLicence: "",
		})
	}
	return gallery
}

func buildPoiProperty(e dto.SiagElements, lang string) []odhContentModel.PoiPropertyEntry {
	var props []odhContentModel.PoiPropertyEntry

	if e.OpeningHours.Value != "" {
		props = append(props, odhContentModel.PoiPropertyEntry{Name: "openingtimes", Value: e.OpeningHours.Value})
	}
	if len(e.MuseumOfferings.Value) > 0 {
		val := e.MuseumOfferings.Value[0].Name
		props = append(props, odhContentModel.PoiPropertyEntry{
			Name:  "entry",
			Value: langName(val, lang),
		})
	}
	if len(e.MuseumCategories.Value) > 0 {
		names := make([]string, len(e.MuseumCategories.Value))
		for i, c := range e.MuseumCategories.Value {
			names[i] = langName(c.Name, lang)
		}
		props = append(props, odhContentModel.PoiPropertyEntry{Name: "categories", Value: strings.Join(names, ", ")})
	}
	if len(e.MuseumServices.Value) > 0 {
		names := make([]string, len(e.MuseumServices.Value))
		for i, s := range e.MuseumServices.Value {
			names[i] = langName(s.Name, lang)
		}
		props = append(props, odhContentModel.PoiPropertyEntry{Name: "services", Value: strings.Join(names, ", ")})
	}
	if len(e.Paramuseum.Value) >= 0 {
		names := make([]string, len(e.Paramuseum.Value))
		for i, s := range e.Paramuseum.Value {
			names[i] = langName(s.Name, lang)
		}
		if strings.Join(names, ", ") == "Yes" {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: "paramuseum", Value: "Paramuseum"})
		} else {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: "paramuseum", Value: "Paramuseum"})
		}
	}
	if len(e.MuseumAssociation.Value) >= 0 {
		names := make([]string, len(e.MuseumAssociation.Value))
		for i, s := range e.MuseumAssociation.Value {
			names[i] = langName(s.Name, lang)
		}
		if strings.Join(names, ", ") == "Yes" {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: "museum_association", Value: "Museum association"})
		} else {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: "museum_association", Value: "Museum association"})
		}
	}
	if len(e.ProvincialMuseum.Value) >= 0 {
		names := make([]string, len(e.ProvincialMuseum.Value))
		for i, s := range e.ProvincialMuseum.Value {
			names[i] = langName(s.Name, lang)
		}
		if strings.Join(names, ", ") == "Yes" {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: "provincial_museum", Value: "Provincial museum"})
		} else {
			props = append(props, odhContentModel.PoiPropertyEntry{Name: "provincial_museum", Value: "Provincial museum"})
		}
	}
	return props
}

// ── Utility helpers ───────────────────────────────────────────────────────────

// langName extracts language-specific part from pipe-separated taxonomy name.
// Format: "de|it|de|en" — index 0=de, 1=it, 3=en
func langName(name, lang string) string {
	parts := strings.Split(name, "|")
	idx := map[string]int{"de": 0, "it": 1, "en": 3}
	if i, ok := idx[lang]; ok && i < len(parts) {
		return strings.TrimSpace(parts[i])
	}
	return name
}

func choiceIsYes(f dto.SiagChoiceField) bool {
	for _, v := range f.Value {
		if v.Codename == "yes" {
			return true
		}
	}
	return false
}

func hasFreeEntrance(e dto.SiagElements) bool {
	for _, o := range e.MuseumOfferings.Value {
		if o.Codename == "free_entry" {
			return true
		}
	}
	return false
}

func isActive(m dto.SiagMuseum) bool {
	return m.System.WorkflowStep == "published"
}

func parseLastModified(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		return time.Now()
	}
	return t
}

func parseCoord(s string) float64 {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return v
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

func ptrOf[T any](v T) *T { return &v }

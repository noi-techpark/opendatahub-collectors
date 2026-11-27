// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/stretchr/testify/require"
	"opendatahub.com/tr-discoverswiss-gastronomy/dto"
	odhContentModel "opendatahub.com/tr-discoverswiss-gastronomy/odh-content-model"
)

func TestPunctumMergesFourLanguagesIntoOnePOI(t *testing.T) {
	tags := testTags(t)
	langs := []string{"en", "it", "fr", "de"}
	var poi odhContentModel.ODHActivityPoi
	for i, lang := range langs {
		raw := loadPunctum(t, lang)
		mapped, err := mapGastronomy(raw, tags)
		require.NoError(t, err)
		if i == 0 {
			poi = mapped
			continue
		}
		MergePOI(&poi, mapped)
	}

	require.Equal(t, "urn:poi:discoverswiss:foo_4kq_dcbfgbeb", *poi.ID)
	require.Equal(t, "discoverswiss", *poi.Source)
	require.Equal(t, "Punctum – Restaurant & Apéro Bar", *poi.Shortname)
	require.Equal(t, []string{"Restaurant", "gastronomy"}, poi.TagIds)
	require.ElementsMatch(t, []string{"de", "en", "fr", "it"}, poi.HasLanguage)
	require.Len(t, poi.Detail, 4)
	require.Contains(t, *poi.Detail["de"].BaseText, "Churer Altstadt")
	require.Equal(t, "punctum@gmx.ch", *poi.ContactInfos["de"].Email)
	require.Equal(t, "Graubünden", poi.LocationInfo.RegionInfo.Name["de"])
	require.Equal(t, "Grisons", poi.LocationInfo.RegionInfo.Name["en"])
	require.Equal(t, "POINT (9.533141 46.848501)", *poi.GpsInfo[0].Geometry)
	require.True(t, poi.Active)

	again := poi
	MergePOI(&poi, again)
	first, err := clib.HashEntity(poi)
	require.NoError(t, err)
	second, err := clib.HashEntity(again)
	require.NoError(t, err)
	require.Equal(t, first, second)
}

func TestReplayDoesNotTreatTheSameGastronomyAsChanged(t *testing.T) {
	tags := testTags(t)
	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	contentClient = &countingAPI{}
	tagCatalog = tags
	deactivateAfter = 12 * time.Hour

	raw := loadPunctum(t, "de")
	now := time.Date(2026, 11, 25, 10, 0, 0, 0, time.UTC)
	require.NoError(t, processRaw(context.Background(), raw, now))
	require.Equal(t, 1, contentClient.(*countingAPI).writes)

	require.NoError(t, processRaw(context.Background(), raw, now.Add(time.Minute)))
	require.Equal(t, 1, contentClient.(*countingAPI).writes)
}

func TestReplayAfterTheApiDropsLocationIsNotAChange(t *testing.T) {
	tags := testTags(t)
	var poi odhContentModel.ODHActivityPoi
	for i, lang := range []string{"de", "en", "fr", "it"} {
		mapped, err := mapGastronomy(loadPunctum(t, lang), tags)
		require.NoError(t, err)
		if i == 0 {
			poi = mapped
			continue
		}
		MergePOI(&poi, mapped)
	}
	poi.LocationInfo = nil

	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	contentClient = &countingAPI{}
	tagCatalog = tags
	deactivateAfter = 12 * time.Hour
	store(t, poi)

	require.NoError(t, processRaw(context.Background(), loadPunctum(t, "de"), time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)))
	require.Equal(t, 0, contentClient.(*countingAPI).writes)
}

func TestUnknownCategoryAndUnavailableLanguageAreSkipped(t *testing.T) {
	tags := testTags(t)
	raw := loadPunctum(t, "de")
	raw.Category = []dto.Category{{Identifier: "sui_root"}, {Identifier: "not_a_real_tag", Name: "Nope"}}
	poi, err := mapGastronomy(raw, tags)
	require.NoError(t, err)
	require.NotContains(t, poi.TagIds, "sui_root")
	require.NotContains(t, poi.TagIds, "not_a_real_tag")

	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	contentClient = nil
	tagCatalog = tags
	raw.ApiCrawlerLang = "rm"
	raw.AvailableDataLanguage = []string{"de", "en", "fr", "it"}
	require.NoError(t, processRaw(context.Background(), raw, time.Now()))
	require.Empty(t, poiCache.Entries())
}

func TestStaleSyncTimeIsDeactivatedOnce(t *testing.T) {
	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	contentClient = &countingAPI{}
	deactivateAfter = 12 * time.Hour
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

	stale := samplePOI("gone", now.Add(-13*time.Hour))
	fresh := samplePOI("still-there", now)
	store(t, stale)
	store(t, fresh)

	require.NoError(t, deactivateStale(context.Background(), "still-there", now))
	gone, ok := poiCache.Get("gone")
	require.True(t, ok)
	require.False(t, gone.Entity.Active)
	kept, ok := poiCache.Get("still-there")
	require.True(t, ok)
	require.True(t, kept.Entity.Active)
	require.Equal(t, 1, contentClient.(*countingAPI).writes)

	require.NoError(t, deactivateStale(context.Background(), "still-there", now))
	require.Equal(t, 1, contentClient.(*countingAPI).writes)
}

func TestEmptyPayloadDoesNotDeactivate(t *testing.T) {
	poiCache = clib.NewCache[odhContentModel.ODHActivityPoi]()
	contentClient = &countingAPI{}
	deactivateAfter = 12 * time.Hour
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	store(t, samplePOI("gone", now.Add(-13*time.Hour)))

	require.NoError(t, processRaw(context.Background(), dto.Gastronomy{}, now))
	gone, ok := poiCache.Get("gone")
	require.True(t, ok)
	require.True(t, gone.Entity.Active)
	require.Equal(t, 0, contentClient.(*countingAPI).writes)
}

func testTags(t *testing.T) clib.TagDefs {
	t.Helper()
	tags, err := clib.ReadTagDefs("../resources/tags.json")
	require.NoError(t, err)
	return tags
}

func loadPunctum(t *testing.T, lang string) dto.Gastronomy {
	t.Helper()
	data, err := os.ReadFile("../testdata/punctum-" + lang + ".json")
	require.NoError(t, err)
	var raw dto.Gastronomy
	require.NoError(t, json.Unmarshal(data, &raw))
	return raw
}

func samplePOI(id string, synced time.Time) odhContentModel.ODHActivityPoi {
	active := true
	return odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:        stringPtr("urn:poi:discoverswiss:" + id),
			Active:    true,
			SmgActive: &active,
			Source:    stringPtr(source),
			Mapping: map[string]map[string]string{
				"discoverswiss": {"id": id, "synctime": synced.Format(time.RFC3339Nano)},
			},
		},
	}
}

func store(t *testing.T, poi odhContentModel.ODHActivityPoi) {
	t.Helper()
	id := mappingID(poi.Mapping)
	hash, _, err := poiCache.HasChanged(id, poi)
	require.NoError(t, err)
	poiCache.Set(id, poi, hash)
}

type countingAPI struct {
	writes int
}

func (c *countingAPI) Get(context.Context, string, map[string]string, interface{}) error { return nil }
func (c *countingAPI) Post(context.Context, string, map[string]string, interface{}) error {
	c.writes++
	return nil
}
func (c *countingAPI) Put(context.Context, string, string, interface{}) error {
	c.writes++
	return nil
}
func (c *countingAPI) PutMultiple(context.Context, string, interface{}) error { return nil }

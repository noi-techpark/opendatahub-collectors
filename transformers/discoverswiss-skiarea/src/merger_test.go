// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"opendatahub.com/tr-discoverswiss-skiarea/dto"
	odhContentModel "opendatahub.com/tr-discoverswiss-skiarea/odh-content-model"
)

func loadTestSkiArea(t *testing.T, path string) dto.SkiArea {
	data, err := os.ReadFile(path)
	require.NoError(t, err, "Failed to read %s", path)

	var raw dto.SkiArea
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err, "Failed to unmarshal %s", path)

	return raw
}

func TestMergeSkiArea(t *testing.T) {
	rawDE := loadTestSkiArea(t, "../test/data/skiarea-merge-de.json")
	rawIT := loadTestSkiArea(t, "../test/data/skiarea-merge-it.json")

	id := generateID(rawDE)

	// Transform both records
	resultDE, err := TransformSkiArea(rawDE, id, "de")
	require.NoError(t, err)

	resultIT, err := TransformSkiArea(rawIT, id, "it")
	require.NoError(t, err)

	// Merge: start with DE, overlay IT
	merged := resultDE.SkiArea
	MergeSkiArea(&merged, resultIT.SkiArea)

	// Verify Detail has both languages with different values
	assert.Len(t, merged.Detail, 2, "Detail should have 2 language entries after merge")
	assert.Contains(t, merged.Detail, "de")
	assert.Contains(t, merged.Detail, "it")
	assert.Equal(t, "Skigebiet San Bernardino in den Graubündner Alpen", *merged.Detail["de"].BaseText)
	assert.Equal(t, "Comprensorio sciistico San Bernardino nelle Alpi grigionesi", *merged.Detail["it"].BaseText)

	// Verify ContactInfos has both languages
	assert.Len(t, merged.ContactInfos, 2, "ContactInfos should have 2 language entries after merge")
	assert.Contains(t, merged.ContactInfos, "de")
	assert.Contains(t, merged.ContactInfos, "it")

	// Verify HasLanguage is the union
	assert.Contains(t, merged.HasLanguage, "de")
	assert.Contains(t, merged.HasLanguage, "it")
	assert.Len(t, merged.HasLanguage, 2)

	// Verify LocationInfo names have both languages
	assert.NotNil(t, merged.LocationInfo)
	assert.NotNil(t, merged.LocationInfo.RegionInfo)
	assert.Equal(t, "Graubünden", merged.LocationInfo.RegionInfo.Name["de"])
	assert.Equal(t, "Grigioni", merged.LocationInfo.RegionInfo.Name["it"])

	// Verify Mapping has individual fields
	dsMapping := merged.Mapping["discoverswiss"]
	assert.NotEmpty(t, dsMapping, "Mapping discoverswiss should not be empty after merge")
	assert.Contains(t, dsMapping, "type")
	assert.Contains(t, dsMapping, "autoTranslatedData")
	assert.Contains(t, dsMapping, "seasonStart")
	assert.Equal(t, "2025-12-01", dsMapping["seasonStart"])
	assert.Contains(t, dsMapping, "maxElevation")

	jsonResult, err := json.MarshalIndent(merged, "", "  ")
	require.NoError(t, err)
	t.Logf("Merged SkiArea:\n%s", string(jsonResult))
}

func TestMergePOI(t *testing.T) {
	rawDE := loadTestSkiArea(t, "../test/data/skiarea-merge-de.json")
	rawIT := loadTestSkiArea(t, "../test/data/skiarea-merge-it.json")

	id := generateID(rawDE)

	resultDE, err := TransformSkiArea(rawDE, id, "de")
	require.NoError(t, err)

	resultIT, err := TransformSkiArea(rawIT, id, "it")
	require.NoError(t, err)

	require.NotEmpty(t, resultDE.POI, "DE should have POIs")
	require.NotEmpty(t, resultIT.POI, "IT should have POIs")

	// Find matching POIs by ID (take the first lift from each)
	poiDE := resultDE.POI[0]
	poiIT := resultIT.POI[0]

	assert.Equal(t, *poiDE.ID, *poiIT.ID, "POI IDs should match")

	// Merge: start with DE, overlay IT
	merged := poiDE
	MergePOI(&merged, poiIT)

	// Verify Detail has both languages
	assert.Len(t, merged.Detail, 2, "POI Detail should have 2 language entries after merge")
	assert.Contains(t, merged.Detail, "de")
	assert.Contains(t, merged.Detail, "it")
	assert.Equal(t, "Sesselbahn Confin", *merged.Detail["de"].Title)
	assert.Equal(t, "Seggiovia Confin", *merged.Detail["it"].Title)

	// Verify HasLanguage
	assert.Contains(t, merged.HasLanguage, "de")
	assert.Contains(t, merged.HasLanguage, "it")

	// Verify LocationInfo
	assert.NotNil(t, merged.LocationInfo)
	assert.NotNil(t, merged.LocationInfo.RegionInfo)
	assert.Equal(t, "Graubünden", merged.LocationInfo.RegionInfo.Name["de"])
	assert.Equal(t, "Grigioni", merged.LocationInfo.RegionInfo.Name["it"])

	// Verify mapping merge for slope POI (has language-dependent parking with dot notation)
	// Find the slope POI
	for i, poiDE := range resultDE.POI {
		if getMappingId(poiDE.Mapping) != "slope_001" {
			continue
		}
		poiIT := resultIT.POI[i]
		mergedPOI := poiDE
		MergePOI(&mergedPOI, poiIT)

		dsMap := mergedPOI.Mapping["discoverswiss"]
		assert.NotEmpty(t, dsMap, "Slope Mapping should not be empty")
		assert.Equal(t, "Grosser Parkplatz an der Talstation", dsMap["parking.de"])
		assert.Equal(t, "Grande parcheggio alla stazione a valle", dsMap["parking.it"])
		break
	}
}

func TestMergeSkiAreaOverwritesNonLangFields(t *testing.T) {
	base := odhContentModel.SkiArea{
		Generic: odhContentModel.Generic{
			Active: true,
			Source: StringPtr(SOURCE),
			Geo: map[string]odhContentModel.GpsInfo{
				"position": {
					Latitude:  Float64Ptr(46.0),
					Longitude: Float64Ptr(9.0),
				},
			},
		},
		Detail: map[string]odhContentModel.Detail{
			"de": {Title: StringPtr("Original DE Title")},
		},
		GpsInfo: []odhContentModel.GpsInfo{
			{Latitude: Float64Ptr(46.0), Longitude: Float64Ptr(9.0)},
		},
		AltitudeFrom:  IntPtr(1000),
		AltitudeTo:    IntPtr(2000),
		TotalSlopeKm:  StringPtr("50.0"),
		LiftCount:     StringPtr("10"),
		SkiAreaMapURL: StringPtr("http://old-map.example.com"),
		CustomId:      StringPtr("old-id"),
		OperationSchedule: []odhContentModel.OperationSchedule{
			{Start: StringPtr("2025-01-01")},
		},
	}

	overlay := odhContentModel.SkiArea{
		Generic: odhContentModel.Generic{
			Active: false,
			Source: StringPtr(SOURCE),
			Geo: map[string]odhContentModel.GpsInfo{
				"position": {
					Latitude:  Float64Ptr(47.0),
					Longitude: Float64Ptr(10.0),
				},
			},
			HasLanguage: []string{"it"},
		},
		Detail: map[string]odhContentModel.Detail{
			"it": {Title: StringPtr("IT Title")},
		},
		GpsInfo: []odhContentModel.GpsInfo{
			{Latitude: Float64Ptr(47.0), Longitude: Float64Ptr(10.0)},
		},
		AltitudeFrom:  IntPtr(1200),
		AltitudeTo:    IntPtr(2500),
		TotalSlopeKm:  StringPtr("75.0"),
		LiftCount:     StringPtr("15"),
		SkiAreaMapURL: StringPtr("http://new-map.example.com"),
		CustomId:      StringPtr("new-id"),
		OperationSchedule: []odhContentModel.OperationSchedule{
			{Start: StringPtr("2025-12-01"), Stop: StringPtr("2026-04-01")},
		},
	}

	MergeSkiArea(&base, overlay)

	// Non-language fields should be overwritten
	assert.False(t, base.Active, "Active should be overwritten to false")
	assert.Equal(t, 47.0, *base.Geo["position"].Latitude, "GPS should be overwritten")
	assert.Equal(t, 10.0, *base.Geo["position"].Longitude, "GPS should be overwritten")

	// New SkiArea-specific fields should be overwritten
	assert.Equal(t, 47.0, *base.GpsInfo[0].Latitude, "GpsInfo should be overwritten")
	assert.Equal(t, 1200, *base.AltitudeFrom, "AltitudeFrom should be overwritten")
	assert.Equal(t, 2500, *base.AltitudeTo, "AltitudeTo should be overwritten")
	assert.Equal(t, "75.0", *base.TotalSlopeKm, "TotalSlopeKm should be overwritten")
	assert.Equal(t, "15", *base.LiftCount, "LiftCount should be overwritten")
	assert.Equal(t, "http://new-map.example.com", *base.SkiAreaMapURL, "SkiAreaMapURL should be overwritten")
	assert.Equal(t, "new-id", *base.CustomId, "CustomId should be overwritten")
	assert.Equal(t, "2025-12-01", *base.OperationSchedule[0].Start, "OperationSchedule should be overwritten")
	assert.Equal(t, "2026-04-01", *base.OperationSchedule[0].Stop, "OperationSchedule Stop should be overwritten")

	// Language fields should be merged
	assert.Contains(t, base.Detail, "de", "DE detail should be preserved")
	assert.Contains(t, base.Detail, "it", "IT detail should be added")
	assert.Equal(t, "Original DE Title", *base.Detail["de"].Title)
	assert.Equal(t, "IT Title", *base.Detail["it"].Title)
}

func TestMergeSkiAreaPreservesOtherLanguages(t *testing.T) {
	base := odhContentModel.SkiArea{
		Generic: odhContentModel.Generic{
			HasLanguage: []string{"de", "en"},
		},
		Detail: map[string]odhContentModel.Detail{
			"de": {Title: StringPtr("DE Title"), BaseText: StringPtr("DE Text")},
			"en": {Title: StringPtr("EN Title"), BaseText: StringPtr("EN Text")},
		},
		ContactInfos: map[string]odhContentModel.ContactInfos{
			"de": {Language: StringPtr("de"), City: StringPtr("Berlin")},
			"en": {Language: StringPtr("en"), City: StringPtr("London")},
		},
	}

	// Merge IT overlay - should not destroy DE or EN
	overlay := odhContentModel.SkiArea{
		Generic: odhContentModel.Generic{
			HasLanguage: []string{"it"},
		},
		Detail: map[string]odhContentModel.Detail{
			"it": {Title: StringPtr("IT Title"), BaseText: StringPtr("IT Text")},
		},
		ContactInfos: map[string]odhContentModel.ContactInfos{
			"it": {Language: StringPtr("it"), City: StringPtr("Roma")},
		},
	}

	MergeSkiArea(&base, overlay)

	// All three languages should be present
	assert.Len(t, base.Detail, 3)
	assert.Equal(t, "DE Title", *base.Detail["de"].Title)
	assert.Equal(t, "EN Title", *base.Detail["en"].Title)
	assert.Equal(t, "IT Title", *base.Detail["it"].Title)

	assert.Len(t, base.ContactInfos, 3)
	assert.Equal(t, "Berlin", *base.ContactInfos["de"].City)
	assert.Equal(t, "London", *base.ContactInfos["en"].City)
	assert.Equal(t, "Roma", *base.ContactInfos["it"].City)

	// HasLanguage should be union of all
	assert.Contains(t, base.HasLanguage, "de")
	assert.Contains(t, base.HasLanguage, "en")
	assert.Contains(t, base.HasLanguage, "it")
	assert.Len(t, base.HasLanguage, 3)
}

func TestMergeMappings(t *testing.T) {
	t.Run("nil base returns overlay", func(t *testing.T) {
		overlay := map[string]map[string]string{"discoverswiss": {"id": "abc"}}
		result := mergeMappings(nil, overlay)
		assert.Equal(t, "abc", result["discoverswiss"]["id"])
	})

	t.Run("nil overlay returns base", func(t *testing.T) {
		base := map[string]map[string]string{"discoverswiss": {"id": "abc"}}
		result := mergeMappings(base, nil)
		assert.Equal(t, "abc", result["discoverswiss"]["id"])
	})

	t.Run("overlay keys are merged into base", func(t *testing.T) {
		base := map[string]map[string]string{
			"discoverswiss": {"id": "abc", "parking.de": "Parkplatz"},
		}
		overlay := map[string]map[string]string{
			"discoverswiss": {"id": "abc", "parking.it": "Parcheggio", "type": "SkiResort"},
		}
		result := mergeMappings(base, overlay)
		assert.Equal(t, "Parkplatz", result["discoverswiss"]["parking.de"])
		assert.Equal(t, "Parcheggio", result["discoverswiss"]["parking.it"])
		assert.Equal(t, "SkiResort", result["discoverswiss"]["type"])
	})
}

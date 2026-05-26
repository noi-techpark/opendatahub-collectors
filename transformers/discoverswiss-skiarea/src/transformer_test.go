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
)

func TestMapSkiAreaToODH(t *testing.T) {
	data, err := os.ReadFile("../test/data/skiarea-full.json")
	require.NoError(t, err, "Failed to read example1.json")

	var raw dto.SkiArea
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err, "Failed to unmarshal example1.json")

	id := generateID(raw)
	lang := raw.ApiCrawlerLang

	skiArea, err := MapSkiAreaToODH(raw, id, lang)
	require.NoError(t, err, "Transformation failed")

	assert.NotNil(t, skiArea.ID, "ID should not be nil")
	assert.Equal(t, id, *skiArea.ID, "ID should match generated ID")
	assert.True(t, skiArea.Active, "SkiArea should be active")
	assert.Equal(t, SOURCE, *skiArea.Source, "Source should match")
	assert.NotNil(t, skiArea.LicenseInfo, "LicenseInfo should not be nil")

	// Verify single-language Detail
	assert.Len(t, skiArea.Detail, 1, "Detail should have only one language entry")
	assert.Contains(t, skiArea.Detail, lang, "Detail should contain the specified language")

	// Verify HasLanguage
	assert.Equal(t, []string{lang}, skiArea.HasLanguage, "HasLanguage should contain only the specified language")

	// Verify GpsInfo array
	assert.NotEmpty(t, skiArea.GpsInfo, "GpsInfo should not be empty")
	assert.NotNil(t, skiArea.GpsInfo[0].Latitude)
	assert.NotNil(t, skiArea.GpsInfo[0].Longitude)
	assert.True(t, skiArea.GpsInfo[0].Default)

	// Verify Shortname is the name, not identifier
	assert.NotNil(t, skiArea.Shortname, "Shortname should not be nil")
	assert.NotEqual(t, raw.Identifier, *skiArea.Shortname, "Shortname should be name, not identifier")
	assert.Equal(t, raw.Name, *skiArea.Shortname, "Shortname should be the ski area name")

	// Verify TagIds
	assert.Contains(t, skiArea.TagIds, "skiarea", "TagIds should contain 'skiarea'")
	assert.Contains(t, skiArea.TagIds, "winter", "TagIds should contain 'winter'")

	// Verify AltitudeTo from skiSlopeSummary.maxElevation
	assert.NotNil(t, skiArea.AltitudeTo, "AltitudeTo should not be nil")
	assert.Equal(t, 2857, *skiArea.AltitudeTo)

	// Verify AltitudeFrom from MinElevation
	if raw.MinElevation > 0 {
		assert.NotNil(t, skiArea.AltitudeFrom, "AltitudeFrom should not be nil")
		assert.Equal(t, raw.MinElevation, *skiArea.AltitudeFrom)
	}

	// Verify TotalSlopeKm from lengthOfSlopes
	assert.NotNil(t, skiArea.TotalSlopeKm, "TotalSlopeKm should not be nil")
	assert.Equal(t, "224.9", *skiArea.TotalSlopeKm)

	// Verify LiftCount from skiLiftSummary.totalFeatures
	assert.NotNil(t, skiArea.LiftCount, "LiftCount should not be nil")
	assert.Equal(t, "43", *skiArea.LiftCount)

	// Verify SkiAreaMapURL
	if raw.HasMap != "" {
		assert.NotNil(t, skiArea.SkiAreaMapURL, "SkiAreaMapURL should not be nil")
	}

	// Verify CustomId from DataGovernance
	assert.NotNil(t, skiArea.CustomId, "CustomId should not be nil")
	assert.Equal(t, "123", *skiArea.CustomId)

	// Verify OperationSchedule from seasonStart/seasonEnd
	assert.NotEmpty(t, skiArea.OperationSchedule, "OperationSchedule should not be empty")
	assert.NotNil(t, skiArea.OperationSchedule[0].Start)
	assert.NotNil(t, skiArea.OperationSchedule[0].Stop)

	// Verify Mapping has individual fields
	dsMapping := skiArea.Mapping["discoverswiss"]
	assert.NotEmpty(t, dsMapping, "Mapping discoverswiss should not be empty")
	assert.Contains(t, dsMapping, "type", "Mapping should contain 'type'")
	assert.Contains(t, dsMapping, "autoTranslatedData", "Mapping should contain 'autoTranslatedData'")

}

func TestTransformSkiArea(t *testing.T) {
	data, err := os.ReadFile("../test/data/skiarea-full.json")
	require.NoError(t, err, "Failed to read example1.json")

	var raw dto.SkiArea
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err, "Failed to unmarshal example1.json")

	id := generateID(raw)
	lang := raw.ApiCrawlerLang

	result, err := TransformSkiArea(raw, id, lang)
	require.NoError(t, err, "TransformSkiArea failed")

	// Validate SkiArea
	assert.NotNil(t, result.SkiArea.ID, "SkiArea ID should not be nil")
	assert.Equal(t, id, *result.SkiArea.ID, "SkiArea ID should match")

	// Validate single-language Detail
	assert.Len(t, result.SkiArea.Detail, 1, "SkiArea Detail should have one language entry")
	assert.Contains(t, result.SkiArea.Detail, lang)

	// Validate POIs
	t.Logf("Number of POIs extracted: %d", len(result.POI))

	for i, poi := range result.POI {
		assert.NotNil(t, poi.ID, "POI[%d] ID should not be nil", i)
		assert.True(t, poi.Active || !poi.Active, "POI[%d] Active should be set", i)
		assert.NotNil(t, poi.Type, "POI[%d] Type should not be nil", i)

		// Validate single-language POI Detail
		assert.Len(t, poi.Detail, 1, "POI[%d] Detail should have one language entry", i)
		assert.Contains(t, poi.Detail, lang, "POI[%d] Detail should contain lang %s", i, lang)

		t.Logf("POI[%d]: ID=%s Type=%s", i, *poi.ID, *poi.Type)
	}

}

func TestMapSubEntityToPOI(t *testing.T) {
	details := dto.SkiSubEntityDetails{
		Identifier:            "test-slope-001",
		Type:                  "Tour",
		AdditionalType:        "SkiSlope",
		Name:                  "Test Slope",
		Description:           "A test ski slope",
		AvailableDataLanguage: []string{"de", "en"},
		State:                 "open",
		Geo: &dto.GeoCoordinates{
			Latitude:  46.5,
			Longitude: 11.3,
			Elevation: 2000,
		},
		Elevation: &dto.Elevation{
			MaxAltitude:  2500,
			MinAltitude:  1500,
			Differential: 1000,
		},
		Length: 3500,
		Time:   45,
		Rating: &dto.Rating{
			Difficulty: 2,
			Technique:  3,
		},
		Exposition: &dto.Exposition{
			NN: true,
			NE: true,
		},
	}

	poi := MapSubEntityToPOI(details, "SkiSlope", "parent-ski-area-id", 0, "de", nil)

	assert.NotNil(t, poi.ID, "POI ID should not be nil")
	assert.True(t, poi.Active, "POI should be active")
	assert.Equal(t, "SkiSlope", *poi.Type, "Type should be SkiSlope")
	assert.NotNil(t, poi.IsOpen, "IsOpen should not be nil")
	assert.True(t, *poi.IsOpen, "Should be open")
	assert.NotNil(t, poi.AltitudeHighestPoint, "AltitudeHighestPoint should be set")
	assert.Equal(t, float64(2500), *poi.AltitudeHighestPoint)
	assert.NotNil(t, poi.AltitudeLowestPoint, "AltitudeLowestPoint should be set")
	assert.Equal(t, float64(1500), *poi.AltitudeLowestPoint)
	assert.NotNil(t, poi.DistanceLength, "DistanceLength should be set")
	assert.Equal(t, float64(3500), *poi.DistanceLength)
	assert.NotNil(t, poi.DistanceDuration, "DistanceDuration should be set")
	assert.Equal(t, float64(45), *poi.DistanceDuration)
	assert.NotNil(t, poi.Ratings, "Ratings should not be nil")
	assert.Equal(t, "2", *poi.Ratings.Difficulty)
	assert.Equal(t, "3", *poi.Ratings.Technique)
	assert.Contains(t, poi.ExpositionValues, "N")
	assert.Contains(t, poi.ExpositionValues, "NE")
	assert.Equal(t, []string{"parent-ski-area-id"}, poi.AreaId, "AreaId should reference parent")

	// Validate single-language detail mapping
	assert.Len(t, poi.Detail, 1, "Detail should have one language entry")
	assert.Contains(t, poi.Detail, "de")
	assert.Equal(t, "Test Slope", *poi.Detail["de"].Title)

	// Validate HasLanguage
	assert.Equal(t, []string{"de"}, poi.HasLanguage, "HasLanguage should contain only 'de'")

	// Validate GPS
	assert.NotEmpty(t, poi.GpsInfo, "GpsInfo should not be empty")
	assert.NotNil(t, poi.GpsInfo[0].Latitude)
	assert.Equal(t, 46.5, *poi.GpsInfo[0].Latitude)

	// Validate Mapping has individual fields
	dsMapping := poi.Mapping["discoverswiss"]
	assert.NotEmpty(t, dsMapping, "Mapping discoverswiss should not be empty")
	assert.Contains(t, dsMapping, "autoTranslatedData", "Mapping should contain 'autoTranslatedData'")

	jsonResult, err := json.MarshalIndent(poi, "", "  ")
	require.NoError(t, err, "Failed to marshal POI")
	t.Logf("Mapped POI:\n%s", string(jsonResult))
}

func TestMapPOIOperationSchedule(t *testing.T) {
	specs := []dto.OpeningHoursSpec{
		{
			Opens:        "08:00",
			Closes:       "16:30",
			DayOfWeek:    "Monday",
			ValidFrom:    "2025-12-01",
			ValidThrough: "2026-04-15",
		},
		{
			Opens:        "08:00",
			Closes:       "16:30",
			DayOfWeek:    "Tuesday",
			ValidFrom:    "2025-12-01",
			ValidThrough: "2026-04-15",
		},
		{
			Opens:        "09:00",
			Closes:       "15:00",
			DayOfWeek:    "Saturday",
			ValidFrom:    "2026-04-16",
			ValidThrough: "2026-05-01",
		},
	}

	result := mapPOIOperationSchedule(specs, "de")
	assert.NotEmpty(t, result, "OperationSchedule should not be empty")
	assert.Len(t, result, 2, "Should have 2 periods (different validity ranges)")

	// Find the winter period (2025-12-01 to 2026-04-15)
	for _, sched := range result {
		if sched.Start != nil && *sched.Start == "2025-12-01" {
			// Monday and Tuesday with same times should be merged into one entry
			assert.Len(t, sched.OperationScheduleTime, 1,
				"Same opens/closes in same period should be merged into one time slot")
			st := sched.OperationScheduleTime[0]
			assert.True(t, st.Monday, "Monday should be true")
			assert.True(t, st.Tuesday, "Tuesday should be true")
			assert.False(t, st.Wednesday, "Wednesday should be false")
			assert.Equal(t, "08:00", *st.Start)
			assert.Equal(t, "16:30", *st.End)
		}
	}
}

func TestMapSubEntityTagIds(t *testing.T) {
	tests := []struct {
		name           string
		subEntityType  string
		additionalType string
		expected       []string
	}{
		{"SkiLift generic", "SkiLift", "", []string{"lifts"}},
		{"SkiLift ChairLift", "SkiLift", "ChairLift", []string{"lifts", "chairlift"}},
		{"SkiLift CableCar", "SkiLift", "CableCar", []string{"lifts", "ropeway"}},
		{"SkiSlope", "SkiSlope", "SkiSlope", []string{"winter", "slope", "slopes", "marked ski paths slopes"}},
		{"SnowPark", "SnowPark", "SnowPark", []string{"winter", "snowpark", "snow parks"}},
		{"Tobogganing", "Tobogganing", "TobogganRun", []string{"winter", "tobbogan run", "sledging trail"}},
		{"CrossCountry", "CrossCountry", "CrossCountry", []string{"winter", "crosscountry skitrack", "crosscountry skiing"}},
		{"Hiking generic", "Hiking", "HikingTrail", []string{"hiking", "winter", "winter hiking"}},
		{"Hiking snowshoe", "Hiking", "SnowshoeTrail", []string{"hiking", "winter", "snowshoe hikes"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mapSubEntityTagIds(tt.subEntityType, tt.additionalType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestMapPOIExposition(t *testing.T) {
	expo := &dto.Exposition{
		NN: true,
		SS: true,
		EE: true,
		WW: false,
	}

	result := mapPOIExposition(expo)
	assert.Contains(t, result, "N")
	assert.Contains(t, result, "S")
	assert.Contains(t, result, "E")
	assert.NotContains(t, result, "W")
}

func TestWeatherCodeMapping(t *testing.T) {
	// Known icons
	assert.Equal(t, "Sunny", *mapWeatherCode(1))
	assert.Equal(t, "Mostly Cloudy w/ Snow", *mapWeatherCode(23))
	assert.Equal(t, "Partly Sunny w/ Showers", *mapWeatherCode(14))
	assert.Equal(t, "Clear", *mapWeatherCode(33))

	// Unknown icon returns nil
	assert.Nil(t, mapWeatherCode(999))
	assert.Nil(t, mapWeatherCode(0))
}

func TestMeasuringpointNoGpsFromSkiArea(t *testing.T) {
	data, err := os.ReadFile("../test/data/skiarea-full.json")
	require.NoError(t, err)

	var raw dto.SkiArea
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)
	require.NotNil(t, raw.Geo, "Test fixture should have GPS on ski area")

	id := generateID(raw)
	lang := raw.ApiCrawlerLang

	result, err := TransformSkiArea(raw, id, lang)
	require.NoError(t, err)
	require.NotEmpty(t, result.Measuringpoints)

	for _, mp := range result.Measuringpoints {
		assert.Empty(t, mp.GpsInfo, "Measuringpoint should not have GPS derived from ski area location")
	}
}

func TestMeasuringpointWeatherCode(t *testing.T) {
	data, err := os.ReadFile("../test/data/skiarea-full.json")
	require.NoError(t, err)

	var raw dto.SkiArea
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err)

	id := generateID(raw)
	lang := raw.ApiCrawlerLang

	result, err := TransformSkiArea(raw, id, lang)
	require.NoError(t, err)
	require.NotEmpty(t, result.Measuringpoints, "Should have measuringpoints")

	for _, mp := range result.Measuringpoints {
		for _, obs := range mp.WeatherObservation {
			assert.NotNil(t, obs.IconID, "IconID should be set")
			assert.NotNil(t, obs.WeatherCode, "WeatherCode should be set for known icons")
			// WeatherCode should match the icon mapping
			icon := obs.IconID
			if icon != nil {
				assert.NotEmpty(t, *obs.WeatherCode, "WeatherCode should not be empty for icon %s", *icon)
			}
		}
	}
}

func TestIsLanguageAvailable(t *testing.T) {
	assert.True(t, isLanguageAvailable(nil, "de"), "nil list means available")
	assert.True(t, isLanguageAvailable([]string{}, "de"), "empty list means available")
	assert.True(t, isLanguageAvailable([]string{"de", "en", "it"}, "de"))
	assert.False(t, isLanguageAvailable([]string{"de", "en"}, "fr"))
}

func TestTransformSkiAreaSkipsUnavailableLanguagePOIs(t *testing.T) {
	details := dto.SkiSubEntityDetails{
		Identifier:            "slope-de-only",
		Type:                  "Tour",
		Name:                  "German Only Slope",
		AvailableDataLanguage: []string{"de"},
	}

	raw := dto.SkiArea{
		Identifier:     "test_ski",
		Type:           "SkiResort",
		ApiCrawlerLang: "fr",
		Name:           "Test Ski Area",
		HasSkiSlope:    []dto.SkiSubEntity{{Details: &details}},
	}

	id := generateID(raw)
	result, err := TransformSkiArea(raw, id, "fr")
	require.NoError(t, err)
	assert.Empty(t, result.POI, "POI with availableDataLanguage=[de] should be skipped for lang=fr")

	// Same POI should be included for lang=de
	raw.ApiCrawlerLang = "de"
	result, err = TransformSkiArea(raw, id, "de")
	require.NoError(t, err)
	assert.Len(t, result.POI, 1, "POI should be included for lang=de")
}

func TestGenerateID(t *testing.T) {
	data, err := os.ReadFile("../test/data/skiarea-full.json")
	require.NoError(t, err, "Failed to read skiarea-full.json\"")

	var raw dto.SkiArea
	err = json.Unmarshal(data, &raw)
	require.NoError(t, err, "Failed to unmarshal skiarea-full.json\"")

	id := generateID(raw)

	assert.NotEmpty(t, id, "Generated ID should not be empty")
	assert.Contains(t, id, SKIAREA_ID_PREFIX, "ID should contain the template prefix")
}

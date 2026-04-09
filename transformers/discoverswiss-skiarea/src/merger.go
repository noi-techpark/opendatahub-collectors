// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	odhContentModel "opendatahub.com/tr-discoverswiss-skiarea/odh-content-model"
)

// MergeSkiArea merges overlay into base.
// Language-dependent fields (Detail, ContactInfos, LocationInfo names, HasLanguage)
// are merged per-language key. Non-language fields are overwritten from overlay.
func MergeSkiArea(base *odhContentModel.SkiArea, overlay odhContentModel.SkiArea) {
	// Overwrite non-language fields
	base.Active = overlay.Active
	base.Source = overlay.Source
	base.LicenseInfo = overlay.LicenseInfo
	base.Geo = overlay.Geo
	base.ImageGallery = mergeImageGallery(base.ImageGallery, overlay.ImageGallery)
	base.SmgTags = overlay.SmgTags
	base.Mapping = mergeMappings(base.Mapping, overlay.Mapping)
	base.Shortname = overlay.Shortname
	base.LastChange = overlay.LastChange
	base.FirstImport = overlay.FirstImport
	base.TagIds = overlay.TagIds
	if overlay.SkiRegionName != nil {
		if base.SkiRegionName == nil {
			base.SkiRegionName = make(map[string]string)
		}
		for lang, name := range overlay.SkiRegionName {
			base.SkiRegionName[lang] = name
		}
	}
	base.GpsInfo = overlay.GpsInfo
	base.TotalSlopeKm = overlay.TotalSlopeKm
	base.SlopeKmBlue = overlay.SlopeKmBlue
	base.SlopeKmRed = overlay.SlopeKmRed
	base.SlopeKmBlack = overlay.SlopeKmBlack
	base.LiftCount = overlay.LiftCount
	base.AltitudeFrom = overlay.AltitudeFrom
	base.AltitudeTo = overlay.AltitudeTo
	base.SkiAreaMapURL = overlay.SkiAreaMapURL
	base.CustomId = overlay.CustomId
	// SkiArea OperationSchedule comes from seasonStart/seasonEnd (no name, single period) — overwrite
	base.OperationSchedule = overlay.OperationSchedule

	// Merge HasLanguage (union)
	base.HasLanguage = mergeLanguages(base.HasLanguage, overlay.HasLanguage)

	// Merge Detail per language key
	if base.Detail == nil {
		base.Detail = make(map[string]odhContentModel.Detail)
	}
	for lang, detail := range overlay.Detail {
		base.Detail[lang] = detail
	}

	// Merge ContactInfos per language key
	if base.ContactInfos == nil {
		base.ContactInfos = make(map[string]odhContentModel.ContactInfos)
	}
	for lang, contact := range overlay.ContactInfos {
		base.ContactInfos[lang] = contact
	}

	// Merge LocationInfo names per language key
	mergeLocationInfo(base, overlay)
}

// MergePOI merges overlay into base.
// Language-dependent fields (Detail, ContactInfos, HasLanguage)
// are merged per-language key. Non-language fields are overwritten from overlay.
func MergePOI(base *odhContentModel.ODHActivityPoi, overlay odhContentModel.ODHActivityPoi) {
	// Overwrite non-language fields
	base.Active = overlay.Active
	base.Source = overlay.Source
	base.LicenseInfo = overlay.LicenseInfo
	base.Geo = overlay.Geo
	base.ImageGallery = mergeImageGallery(base.ImageGallery, overlay.ImageGallery)
	base.SmgTags = overlay.SmgTags
	base.Mapping = mergeMappings(base.Mapping, overlay.Mapping)
	base.Shortname = overlay.Shortname
	base.LastChange = overlay.LastChange
	base.FirstImport = overlay.FirstImport
	base.TagIds = overlay.TagIds

	base.Type = overlay.Type
	base.SubType = overlay.SubType
	base.GpsInfo = overlay.GpsInfo
	base.AltitudeHighestPoint = overlay.AltitudeHighestPoint
	base.AltitudeLowestPoint = overlay.AltitudeLowestPoint
	base.AltitudeSumUp = overlay.AltitudeSumUp
	base.AltitudeSumDown = overlay.AltitudeSumDown
	base.AltitudeDifference = overlay.AltitudeDifference
	base.DistanceLength = overlay.DistanceLength
	base.DistanceDuration = overlay.DistanceDuration
	base.Ratings = overlay.Ratings
	base.ExpositionValues = overlay.ExpositionValues
	base.IsOpen = overlay.IsOpen
	base.HasFreeEntrance = overlay.HasFreeEntrance
	base.Highlight = overlay.Highlight
	base.OperationSchedule = mergeOperationSchedules(base.OperationSchedule, overlay.OperationSchedule)
	base.Tags = overlay.Tags
	base.ODHActivityPoiTypes = overlay.ODHActivityPoiTypes
	base.AreaId = overlay.AreaId
	base.CustomId = overlay.CustomId
	base.GpsTrack = overlay.GpsTrack

	// Merge HasLanguage (union)
	base.HasLanguage = mergeLanguages(base.HasLanguage, overlay.HasLanguage)

	// Merge Detail per language key
	if base.Detail == nil {
		base.Detail = make(map[string]odhContentModel.Detail)
	}
	for lang, detail := range overlay.Detail {
		base.Detail[lang] = detail
	}

	// Merge ContactInfos per language key
	if base.ContactInfos == nil {
		base.ContactInfos = make(map[string]odhContentModel.ContactInfos)
	}
	for lang, contact := range overlay.ContactInfos {
		base.ContactInfos[lang] = contact
	}

	// Merge LocationInfo names per language key
	mergePOILocationInfo(base, overlay)
}

// MergeMeasuringpoint merges overlay into base.
// Language-dependent fields (Detail, HasLanguage, LocationInfo names) are merged per-language key.
// Non-language fields (weather data, GPS, snow) are overwritten from overlay.
func MergeMeasuringpoint(base *odhContentModel.MeasuringpointV2, overlay odhContentModel.MeasuringpointV2) {
	// Overwrite non-language fields
	base.Active = overlay.Active
	base.Source = overlay.Source
	base.LicenseInfo = overlay.LicenseInfo
	base.Mapping = mergeMappings(base.Mapping, overlay.Mapping)
	base.Shortname = overlay.Shortname
	base.LastChange = overlay.LastChange
	base.FirstImport = overlay.FirstImport
	base.TagIds = overlay.TagIds
	base.GpsInfo = overlay.GpsInfo
	base.SkiAreaIds = overlay.SkiAreaIds
	base.AreaIds = overlay.AreaIds
	base.Tags = overlay.Tags

	// Weather data — always overwrite (not language-dependent)
	base.SnowHeight = overlay.SnowHeight
	base.NewSnowHeight = overlay.NewSnowHeight
	base.Temperature = overlay.Temperature
	base.LastSnowDate = overlay.LastSnowDate
	base.WeatherObservation = overlay.WeatherObservation

	// Merge HasLanguage (union)
	base.HasLanguage = mergeLanguages(base.HasLanguage, overlay.HasLanguage)

	// Merge Detail per language key
	if base.Detail == nil {
		base.Detail = make(map[string]odhContentModel.DetailGeneric)
	}
	for lang, detail := range overlay.Detail {
		base.Detail[lang] = detail
	}

	// Merge LocationInfo names per language key
	mergeMPLocationInfo(base, overlay)
}

// mergeMPLocationInfo merges LocationInfo name maps per language for MeasuringpointV2
func mergeMPLocationInfo(base *odhContentModel.MeasuringpointV2, overlay odhContentModel.MeasuringpointV2) {
	if overlay.LocationInfo == nil {
		return
	}
	if base.LocationInfo == nil {
		base.LocationInfo = &odhContentModel.LocationInfo{}
	}

	if overlay.LocationInfo.RegionInfo != nil {
		if base.LocationInfo.RegionInfo == nil {
			base.LocationInfo.RegionInfo = &odhContentModel.RegionInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.RegionInfo.ID = overlay.LocationInfo.RegionInfo.ID
		for lang, name := range overlay.LocationInfo.RegionInfo.Name {
			base.LocationInfo.RegionInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.MunicipalityInfo != nil {
		if base.LocationInfo.MunicipalityInfo == nil {
			base.LocationInfo.MunicipalityInfo = &odhContentModel.MunicipalityInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.MunicipalityInfo.ID = overlay.LocationInfo.MunicipalityInfo.ID
		for lang, name := range overlay.LocationInfo.MunicipalityInfo.Name {
			base.LocationInfo.MunicipalityInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.TvInfo != nil {
		if base.LocationInfo.TvInfo == nil {
			base.LocationInfo.TvInfo = &odhContentModel.TvInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.TvInfo.ID = overlay.LocationInfo.TvInfo.ID
		for lang, name := range overlay.LocationInfo.TvInfo.Name {
			base.LocationInfo.TvInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.DistrictInfo != nil {
		if base.LocationInfo.DistrictInfo == nil {
			base.LocationInfo.DistrictInfo = &odhContentModel.DistrictInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.DistrictInfo.ID = overlay.LocationInfo.DistrictInfo.ID
		for lang, name := range overlay.LocationInfo.DistrictInfo.Name {
			base.LocationInfo.DistrictInfo.Name[lang] = name
		}
	}
}

// mergeLanguages returns the union of two language slices without duplicates
func mergeLanguages(base, overlay []string) []string {
	seen := make(map[string]bool, len(base)+len(overlay))
	for _, lang := range base {
		seen[lang] = true
	}
	for _, lang := range overlay {
		seen[lang] = true
	}
	result := make([]string, 0, len(seen))
	// Preserve order: base first, then new from overlay
	for _, lang := range base {
		if seen[lang] {
			result = append(result, lang)
			delete(seen, lang)
		}
	}
	for _, lang := range overlay {
		if seen[lang] {
			result = append(result, lang)
			delete(seen, lang)
		}
	}
	return result
}

// mergeLocationInfo merges LocationInfo name maps per language for SkiArea
func mergeLocationInfo(base *odhContentModel.SkiArea, overlay odhContentModel.SkiArea) {
	if overlay.LocationInfo == nil {
		return
	}
	if base.LocationInfo == nil {
		base.LocationInfo = &odhContentModel.LocationInfo{}
	}

	if overlay.LocationInfo.RegionInfo != nil {
		if base.LocationInfo.RegionInfo == nil {
			base.LocationInfo.RegionInfo = &odhContentModel.RegionInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.RegionInfo.ID = overlay.LocationInfo.RegionInfo.ID
		for lang, name := range overlay.LocationInfo.RegionInfo.Name {
			base.LocationInfo.RegionInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.MunicipalityInfo != nil {
		if base.LocationInfo.MunicipalityInfo == nil {
			base.LocationInfo.MunicipalityInfo = &odhContentModel.MunicipalityInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.MunicipalityInfo.ID = overlay.LocationInfo.MunicipalityInfo.ID
		for lang, name := range overlay.LocationInfo.MunicipalityInfo.Name {
			base.LocationInfo.MunicipalityInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.TvInfo != nil {
		if base.LocationInfo.TvInfo == nil {
			base.LocationInfo.TvInfo = &odhContentModel.TvInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.TvInfo.ID = overlay.LocationInfo.TvInfo.ID
		for lang, name := range overlay.LocationInfo.TvInfo.Name {
			base.LocationInfo.TvInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.DistrictInfo != nil {
		if base.LocationInfo.DistrictInfo == nil {
			base.LocationInfo.DistrictInfo = &odhContentModel.DistrictInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.DistrictInfo.ID = overlay.LocationInfo.DistrictInfo.ID
		for lang, name := range overlay.LocationInfo.DistrictInfo.Name {
			base.LocationInfo.DistrictInfo.Name[lang] = name
		}
	}
}

// mergeMappings merges overlay mapping into base mapping.
// Each provider's inner map keys are merged (overlay values win).
func mergeMappings(base, overlay map[string]map[string]string) map[string]map[string]string {
	if len(overlay) == 0 {
		return base
	}
	if base == nil {
		return overlay
	}
	for provider, overlayMap := range overlay {
		if base[provider] == nil {
			base[provider] = map[string]string{}
		}
		for key, val := range overlayMap {
			base[provider][key] = val
		}
	}
	return base
}

// mergeImageGallery merges image galleries by matching on ImageUrl.
// ImageTitle and ImageDesc are merged per-language key.
func mergeImageGallery(base, overlay []odhContentModel.ImageGallery) []odhContentModel.ImageGallery {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}

	// Index base images by URL
	byURL := map[string]int{}
	for i, img := range base {
		if img.ImageUrl != nil {
			byURL[*img.ImageUrl] = i
		}
	}

	for _, oImg := range overlay {
		if oImg.ImageUrl == nil {
			continue
		}
		if idx, ok := byURL[*oImg.ImageUrl]; ok {
			// Merge language maps into existing entry
			for lang, val := range oImg.ImageTitle {
				if base[idx].ImageTitle == nil {
					base[idx].ImageTitle = map[string]string{}
				}
				base[idx].ImageTitle[lang] = val
			}
			for lang, val := range oImg.ImageDesc {
				if base[idx].ImageDesc == nil {
					base[idx].ImageDesc = map[string]string{}
				}
				base[idx].ImageDesc[lang] = val
			}
		} else {
			base = append(base, oImg)
			byURL[*oImg.ImageUrl] = len(base) - 1
		}
	}

	return base
}

// mergeOperationSchedules merges OperationSchedule slices by matching Start/Stop periods.
// OperationscheduleName is merged per-language. Other fields are overwritten from overlay.
func mergeOperationSchedules(base, overlay []odhContentModel.OperationSchedule) []odhContentModel.OperationSchedule {
	if len(base) == 0 {
		return overlay
	}
	if len(overlay) == 0 {
		return base
	}

	// Index base by start+stop
	type periodKey struct{ start, stop string }
	ptrStr := func(s *string) string {
		if s == nil {
			return ""
		}
		return *s
	}
	byPeriod := map[periodKey]int{}
	for i, s := range base {
		byPeriod[periodKey{ptrStr(s.Start), ptrStr(s.Stop)}] = i
	}

	for _, o := range overlay {
		key := periodKey{ptrStr(o.Start), ptrStr(o.Stop)}
		if idx, ok := byPeriod[key]; ok {
			// Merge OperationscheduleName per-language into existing
			if len(o.OperationscheduleName) > 0 {
				if base[idx].OperationscheduleName == nil {
					base[idx].OperationscheduleName = make(map[string]string)
				}
				for lang, name := range o.OperationscheduleName {
					base[idx].OperationscheduleName[lang] = name
				}
			}
			// Overwrite time slots from overlay
			base[idx].OperationScheduleTime = o.OperationScheduleTime
		} else {
			base = append(base, o)
			byPeriod[key] = len(base) - 1
		}
	}

	return base
}

// mergePOILocationInfo merges LocationInfo name maps per language for POI
func mergePOILocationInfo(base *odhContentModel.ODHActivityPoi, overlay odhContentModel.ODHActivityPoi) {
	if overlay.LocationInfo == nil {
		return
	}
	if base.LocationInfo == nil {
		base.LocationInfo = &odhContentModel.LocationInfo{}
	}

	if overlay.LocationInfo.RegionInfo != nil {
		if base.LocationInfo.RegionInfo == nil {
			base.LocationInfo.RegionInfo = &odhContentModel.RegionInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.RegionInfo.ID = overlay.LocationInfo.RegionInfo.ID
		for lang, name := range overlay.LocationInfo.RegionInfo.Name {
			base.LocationInfo.RegionInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.MunicipalityInfo != nil {
		if base.LocationInfo.MunicipalityInfo == nil {
			base.LocationInfo.MunicipalityInfo = &odhContentModel.MunicipalityInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.MunicipalityInfo.ID = overlay.LocationInfo.MunicipalityInfo.ID
		for lang, name := range overlay.LocationInfo.MunicipalityInfo.Name {
			base.LocationInfo.MunicipalityInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.TvInfo != nil {
		if base.LocationInfo.TvInfo == nil {
			base.LocationInfo.TvInfo = &odhContentModel.TvInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.TvInfo.ID = overlay.LocationInfo.TvInfo.ID
		for lang, name := range overlay.LocationInfo.TvInfo.Name {
			base.LocationInfo.TvInfo.Name[lang] = name
		}
	}

	if overlay.LocationInfo.DistrictInfo != nil {
		if base.LocationInfo.DistrictInfo == nil {
			base.LocationInfo.DistrictInfo = &odhContentModel.DistrictInfo{
				Name: make(map[string]string),
			}
		}
		base.LocationInfo.DistrictInfo.ID = overlay.LocationInfo.DistrictInfo.ID
		for lang, name := range overlay.LocationInfo.DistrictInfo.Name {
			base.LocationInfo.DistrictInfo.Name[lang] = name
		}
	}
}

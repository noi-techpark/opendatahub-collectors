// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"sort"

	odhContentModel "opendatahub.com/tr-discoverswiss-gastronomy/odh-content-model"
)

// MergePOI fills language-specific fields from overlay into base.
// One crawler message is one language, so repeated calls build one POI.
func MergePOI(base *odhContentModel.ODHActivityPoi, overlay odhContentModel.ODHActivityPoi) {
	base.Active = overlay.Active
	base.SmgActive = overlay.SmgActive
	base.OdhActive = overlay.OdhActive
	base.Source = overlay.Source
	base.LicenseInfo = overlay.LicenseInfo
	base.GpsInfo = overlay.GpsInfo
	base.PublishedOn = overlay.PublishedOn
	base.Type = overlay.Type
	base.SubType = overlay.SubType
	base.LastChange = overlay.LastChange
	base.FirstImport = overlay.FirstImport
	base.CustomId = overlay.CustomId
	base.TagIds = overlay.TagIds
	base.SmgTags = overlay.SmgTags
	base.OperationSchedule = overlay.OperationSchedule
	base.Shortname = mergeShortname(base.Shortname, overlay.Shortname, overlay.HasLanguage)
	base.ImageGallery = mergeImages(base.ImageGallery, overlay.ImageGallery)
	base.Mapping = mergeMappings(base.Mapping, overlay.Mapping)
	base.HasLanguage = mergeLanguages(base.HasLanguage, overlay.HasLanguage)

	if base.Detail == nil {
		base.Detail = map[string]odhContentModel.Detail{}
	}
	for lang, detail := range overlay.Detail {
		base.Detail[lang] = detail
	}
	if base.ContactInfos == nil {
		base.ContactInfos = map[string]odhContentModel.ContactInfos{}
	}
	for lang, contact := range overlay.ContactInfos {
		base.ContactInfos[lang] = contact
	}
	mergeLocation(base, overlay)
}

func mergeShortname(base, overlay *string, overlayLangs []string) *string {
	if overlay == nil {
		return base
	}
	if base == nil || deref(base) == "" {
		return overlay
	}
	for _, lang := range overlayLangs {
		if lang == shortnameLang {
			return overlay
		}
	}
	return base
}

func mergeLanguages(base, overlay []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, lang := range append(append([]string{}, base...), overlay...) {
		if lang == "" || seen[lang] {
			continue
		}
		seen[lang] = true
		out = append(out, lang)
	}
	return out
}

func mergeMappings(base, overlay map[string]map[string]string) map[string]map[string]string {
	if base == nil {
		base = map[string]map[string]string{}
	}
	for key, values := range overlay {
		if base[key] == nil {
			base[key] = map[string]string{}
		}
		for field, value := range values {
			base[key][field] = value
		}
	}
	return base
}

func mergeImages(base, overlay []odhContentModel.ImageGallery) []odhContentModel.ImageGallery {
	byURL := map[string]odhContentModel.ImageGallery{}
	var order []string
	add := func(img odhContentModel.ImageGallery) {
		url := deref(img.ImageUrl)
		if url == "" {
			return
		}
		existing, ok := byURL[url]
		if !ok {
			order = append(order, url)
			byURL[url] = img
			return
		}
		if existing.ImageTitle == nil {
			existing.ImageTitle = map[string]string{}
		}
		for lang, title := range img.ImageTitle {
			existing.ImageTitle[lang] = title
		}
		byURL[url] = existing
	}
	for _, img := range base {
		add(img)
	}
	for _, img := range overlay {
		add(img)
	}
	out := make([]odhContentModel.ImageGallery, 0, len(order))
	for _, url := range order {
		out = append(out, byURL[url])
	}
	sort.Slice(out, func(i, j int) bool {
		return deref(out[i].ImageUrl) < deref(out[j].ImageUrl)
	})
	for i := range out {
		pos := i
		out[i].ListPosition = &pos
	}
	return out
}

func mergeLocation(base *odhContentModel.ODHActivityPoi, overlay odhContentModel.ODHActivityPoi) {
	if overlay.LocationInfo == nil {
		return
	}
	if base.LocationInfo == nil {
		base.LocationInfo = &odhContentModel.LocationInfo{}
	}
	base.LocationInfo.RegionInfo = mergeNamedRef(base.LocationInfo.RegionInfo, overlay.LocationInfo.RegionInfo)
	base.LocationInfo.MunicipalityInfo = mergeNamedRef(base.LocationInfo.MunicipalityInfo, overlay.LocationInfo.MunicipalityInfo)
	base.LocationInfo.TvInfo = mergeNamedRef(base.LocationInfo.TvInfo, overlay.LocationInfo.TvInfo)
	base.LocationInfo.DistrictInfo = mergeNamedRef(base.LocationInfo.DistrictInfo, overlay.LocationInfo.DistrictInfo)
}

func mergeNamedRef(base, overlay *odhContentModel.NamedRef) *odhContentModel.NamedRef {
	if overlay == nil {
		return base
	}
	if base == nil {
		copied := *overlay
		copied.Name = copyNames(overlay.Name)
		return &copied
	}
	base.ID = overlay.ID
	if base.Name == nil {
		base.Name = map[string]string{}
	}
	for lang, name := range overlay.Name {
		base.Name[lang] = name
	}
	return base
}

func copyNames(names map[string]string) map[string]string {
	out := map[string]string{}
	for lang, name := range names {
		out[lang] = name
	}
	return out
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"fmt"
	"log/slog"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"opendatahub.com/tr-discoverswiss-gastronomy/dto"
	odhContentModel "opendatahub.com/tr-discoverswiss-gastronomy/odh-content-model"
)

const (
	source         = "discoverswiss"
	idPrefix       = "urn:poi:discoverswiss"
	licenseHolder  = "Schweiz Tourismus"
	gastronomyTag  = "gastronomy"
	shortnameLang  = "de"
	skippedRootTag = "sui_root"
)

func mapGastronomy(raw dto.Gastronomy, tags clib.TagDefs) (odhContentModel.ODHActivityPoi, error) {
	if raw.Identifier == "" {
		return odhContentModel.ODHActivityPoi{}, fmt.Errorf("food establishment has no identifier")
	}

	active := !raw.Removed
	license, closed := mapLicense(raw.License)
	poi := odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			ID:        stringPtr(fmt.Sprintf("%s:%s", idPrefix, raw.Identifier)),
			Active:    active,
			SmgActive: boolPtr(active),
			OdhActive: boolPtr(active),
			Source:    stringPtr(source),
			LicenseInfo: &odhContentModel.LicenseInfo{
				License:       ifNotEmpty(license),
				LicenseHolder: stringPtr(licenseHolder),
				ClosedData:    closed,
			},
			HasLanguage: []string{raw.ApiCrawlerLang},
			PublishedOn: []string{},
		},
		Type:    stringPtr("Gastronomy"),
		SubType: ifNotEmpty(raw.AdditionalType),
	}
	if raw.DataGovernance != nil && raw.DataGovernance.Author != nil {
		poi.LicenseInfo.Author = ifNotEmpty(raw.DataGovernance.Author.FamilyName)
	}

	poi.Shortname = ifNotEmpty(raw.Name)
	poi.Detail = mapDetail(raw)
	poi.ContactInfos = mapContact(raw)
	poi.ImageGallery = mapImages(raw)
	poi.LocationInfo = mapLocation(raw.ContainedInPlace, raw.ApiCrawlerLang)
	poi.OperationSchedule = mapOpeningHours(raw.OpeningHoursSpecification)
	poi.TagIds = mapTagIDs(raw, tags)
	poi.SmgTags = append([]string(nil), poi.TagIds...)

	if raw.Geo != nil {
		gps := odhContentModel.GpsInfo{
			Latitude:  floatPtr(raw.Geo.Latitude),
			Longitude: floatPtr(raw.Geo.Longitude),
			Gpstype:   stringPtr("position"),
			Geometry:  stringPtr(fmt.Sprintf("POINT (%s %s)", formatFloat(raw.Geo.Longitude), formatFloat(raw.Geo.Latitude))),
			Default:   true,
		}
		if raw.Geo.Elevation > 0 {
			gps.Altitude = floatPtr(raw.Geo.Elevation)
			gps.AltitudeUnitofMeasure = stringPtr("m")
		}
		poi.GpsInfo = []odhContentModel.GpsInfo{gps}
	}

	if parsed, ok := parseTime(raw.LastModified); ok {
		poi.LastChange = stringPtr(parsed.Format(time.RFC3339))
	}
	if raw.DataGovernance != nil && len(raw.DataGovernance.Origin) > 0 {
		origin := raw.DataGovernance.Origin[0]
		if parsed, ok := parseTime(origin.Created); ok {
			poi.FirstImport = stringPtr(parsed.Format(time.RFC3339))
		}
		poi.CustomId = ifNotEmpty(origin.SourceID)
	}

	mapping := map[string]string{
		"id":                 raw.Identifier,
		"autoTranslatedData": strconv.FormatBool(raw.AutoTranslatedData),
	}
	if raw.AtID != "" {
		mapping["atId"] = raw.AtID
	}
	for _, prop := range raw.AdditionalProperty {
		if prop.PropertyID == "google_place_id" && prop.Value != "" {
			mapping["googlePlaceId"] = string(prop.Value)
		}
	}
	poi.Mapping = map[string]map[string]string{"discoverswiss": mapping}
	return poi, nil
}

func mapDetail(raw dto.Gastronomy) map[string]odhContentModel.Detail {
	lang := raw.ApiCrawlerLang
	detail := odhContentModel.Detail{
		Language:       stringPtr(lang),
		Title:          ifNotEmpty(raw.Name),
		BaseText:       ifNotEmpty(raw.Description),
		Header:         ifNotEmpty(raw.TitleTeaser),
		IntroText:      ifNotEmpty(raw.TextTeaser),
		AdditionalText: ifNotEmpty(raw.DisambiguatingDescription),
	}
	getThere := raw.GettingThere
	if getThere == "" {
		getThere = raw.Directions
	}
	detail.GetThereText = ifNotEmpty(getThere)
	return map[string]odhContentModel.Detail{lang: detail}
}

func mapContact(raw dto.Gastronomy) map[string]odhContentModel.ContactInfos {
	contact := odhContentModel.ContactInfos{
		Language:    stringPtr(raw.ApiCrawlerLang),
		CompanyName: ifNotEmpty(raw.Name),
		Url:         ifNotEmpty(raw.URL),
		Faxnumber:   ifNotEmpty(raw.FaxNumber),
	}
	if raw.Address != nil {
		contact.Address = ifNotEmpty(raw.Address.StreetAddress)
		contact.City = ifNotEmpty(raw.Address.AddressLocality)
		contact.ZipCode = ifNotEmpty(raw.Address.PostalCode)
		contact.CountryCode = ifNotEmpty(raw.Address.AddressCountry)
		contact.Email = ifNotEmpty(raw.Address.Email)
		contact.Phonenumber = ifNotEmpty(raw.Address.Telephone)
	}
	if contact.Phonenumber == nil {
		contact.Phonenumber = ifNotEmpty(raw.Telephone)
	}
	return map[string]odhContentModel.ContactInfos{raw.ApiCrawlerLang: contact}
}

func mapImages(raw dto.Gastronomy) []odhContentModel.ImageGallery {
	seen := map[string]bool{}
	var images []odhContentModel.ImageGallery
	add := func(img dto.Image) {
		if img.ContentURL == "" || seen[img.ContentURL] {
			return
		}
		seen[img.ContentURL] = true
		title := img.Caption
		if title == "" {
			title = img.Name
		}
		entry := odhContentModel.ImageGallery{
			ImageUrl:    stringPtr(img.ContentURL),
			ImageName:   ifNotEmpty(img.Name),
			CopyRight:   ifNotEmpty(img.CopyrightNotice),
			License:     ifNotEmpty(raw.License),
			IsInGallery: boolPtr(true),
		}
		if title != "" {
			entry.ImageTitle = map[string]string{raw.ApiCrawlerLang: title}
		}
		images = append(images, entry)
	}
	if raw.Image != nil {
		add(*raw.Image)
	}
	for _, photo := range raw.Photo {
		add(photo)
	}
	sort.Slice(images, func(i, j int) bool {
		return deref(images[i].ImageUrl) < deref(images[j].ImageUrl)
	})
	for i := range images {
		pos := i
		images[i].ListPosition = &pos
	}
	return images
}

func mapLocation(places []dto.AdministrativeArea, lang string) *odhContentModel.LocationInfo {
	if len(places) == 0 {
		return nil
	}
	loc := &odhContentModel.LocationInfo{}
	for _, place := range places {
		ref := &odhContentModel.NamedRef{ID: ifNotEmpty(place.Identifier), Name: map[string]string{lang: place.Name}}
		switch strings.ToLower(place.AdditionalType) {
		case "state":
			if loc.RegionInfo == nil {
				loc.RegionInfo = ref
			}
		case "city":
			if loc.MunicipalityInfo == nil {
				loc.MunicipalityInfo = ref
			}
		case "tourismarea":
			if loc.TvInfo == nil {
				loc.TvInfo = ref
			}
		case "district":
			if loc.DistrictInfo == nil {
				loc.DistrictInfo = ref
			}
		}
	}
	if loc.RegionInfo == nil && loc.MunicipalityInfo == nil && loc.TvInfo == nil && loc.DistrictInfo == nil {
		return nil
	}
	return loc
}

func mapOpeningHours(specs []dto.OpeningHoursSpec) []odhContentModel.OperationSchedule {
	if len(specs) == 0 {
		return nil
	}
	slots := make([]odhContentModel.OperationScheduleTime, 0, len(specs))
	for _, spec := range specs {
		slot := odhContentModel.OperationScheduleTime{
			Start: ifNotEmpty(spec.Opens),
			End:   ifNotEmpty(spec.Closes),
			State: 2,
		}
		switch strings.ToLower(spec.DayOfWeek) {
		case "monday":
			slot.Monday = true
		case "tuesday":
			slot.Tuesday = true
		case "wednesday":
			slot.Wednesday = true
		case "thursday":
			slot.Thursday = true
		case "friday":
			slot.Friday = true
		case "saturday":
			slot.Saturday = true
		case "sunday":
			slot.Sunday = true
		}
		slots = append(slots, slot)
	}
	schedule := odhContentModel.OperationSchedule{
		Type:                  stringPtr("weekly"),
		OperationScheduleTime: slots,
	}
	if specs[0].ValidFrom != "" {
		schedule.Start = stringPtr(specs[0].ValidFrom)
	}
	if specs[0].ValidThrough != "" {
		schedule.Stop = stringPtr(specs[0].ValidThrough)
	}
	return []odhContentModel.OperationSchedule{schedule}
}

func mapTagIDs(raw dto.Gastronomy, tags clib.TagDefs) []string {
	ids := []string{gastronomyTag}
	if raw.AdditionalType != "" {
		if tags.FindById(raw.AdditionalType) != nil {
			ids = append(ids, raw.AdditionalType)
		} else {
			slog.Info("skipping unknown gastronomy type", "id", raw.AdditionalType, "identifier", raw.Identifier)
		}
	}
	for _, cat := range raw.Category {
		if cat.Identifier == "" || cat.Identifier == skippedRootTag {
			continue
		}
		if tags.FindById(cat.Identifier) == nil {
			slog.Info("skipping unknown gastronomy category", "id", cat.Identifier, "identifier", raw.Identifier)
			continue
		}
		ids = append(ids, cat.Identifier)
	}
	return uniqueSorted(ids)
}

func parseTime(value string) (time.Time, bool) {
	if value == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func languageAvailable(available []string, lang string) bool {
	if len(available) == 0 {
		return true
	}
	for _, candidate := range available {
		if candidate == lang {
			return true
		}
	}
	return false
}

func mapLicense(value string) (string, bool) {
	switch value {
	case "CC 0":
		return "CC0", false
	case "C-All-Rights-Reserved":
		return "Closed", true
	default:
		return value, false
	}
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func stringPtr(value string) *string { return &value }

func ifNotEmpty(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func boolPtr(value bool) *bool { return &value }

func floatPtr(value float64) *float64 { return &value }

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func formatFloat(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

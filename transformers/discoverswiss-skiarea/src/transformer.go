// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"opendatahub.com/tr-discoverswiss-skiarea/dto"
	odhContentModel "opendatahub.com/tr-discoverswiss-skiarea/odh-content-model"
)

// weatherIconToCode maps DiscoverSwiss/AccuWeather icon numbers to weather description codes.
var weatherIconToCode = map[int]string{
	1:  "Sunny",
	2:  "Mostly Sunny",
	3:  "Partly Sunny",
	4:  "Intermittent Clouds",
	5:  "Hazy Sunshine",
	6:  "Mostly Cloudy",
	7:  "Cloudy",
	8:  "Dreary (Overcast)",
	11: "Fog",
	12: "Showers",
	13: "Mostly Cloudy w/ Showers",
	14: "Partly Sunny w/ Showers",
	15: "T-Storms",
	16: "Mostly Cloudy w/ T-Storms",
	17: "Partly Sunny w/ T-Storms",
	18: "Rain",
	19: "Flurries",
	20: "Mostly Cloudy w/ Flurries",
	21: "Partly Sunny w/ Flurries",
	22: "Snow",
	23: "Mostly Cloudy w/ Snow",
	24: "Ice",
	25: "Sleet",
	26: "Freezing Rain",
	29: "Rain and Snow",
	30: "Hot",
	31: "Cold",
	32: "Windy",
	33: "Clear",
	34: "Mostly Clear",
	35: "Partly Cloudy",
	36: "Intermittent Clouds",
	37: "Hazy Moonlight",
	38: "Mostly Cloudy",
	39: "Partly Cloudy w/ Showers",
	40: "Mostly Cloudy w/ Showers",
	41: "Partly Cloudy w/ T-Storms",
	42: "Mostly Cloudy w/ T-Storms",
	43: "Mostly Cloudy w/ Flurries",
	44: "Mostly Cloudy w/ Snow",
}

// mapWeatherCode converts a DiscoverSwiss weather icon number to a weather code string.
func mapWeatherCode(icon int) *string {
	if code, ok := weatherIconToCode[icon]; ok {
		return &code
	}
	return nil
}

// mapLicense maps DiscoverSwiss license strings to ODH license values.
// ODH accepts: "CC0", "CC-BY", "Closed".
func mapLicense(dsLicense string) (license string, closedData bool) {
	switch dsLicense {
	case "CC 0":
		return "CC0", false
	case "CC-BY-ND-SA":
		return "CC BY-ND-SA", false
	case "CC BY", "CC BY-SA", "CC BY-NC-SA":
		return dsLicense, false
	case "C-All-Rights-Reserved":
		return "Closed", true
	default:
		if dsLicense != "" {
			return dsLicense, false
		}
		return "", false
	}
}

// cleanType strips the "schema.org/" prefix from DiscoverSwiss type strings.
// e.g. "schema.org/TransportationSystem" → "TransportationSystem"
func cleanType(t string) string {
	return strings.TrimPrefix(t, "schema.org/")
}

func StringPtr(s string) *string {
	return &s
}

func IfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func BoolPtr(b bool) *bool {
	return &b
}

func Float64Ptr(f float64) *float64 {
	return &f
}

func IntPtr(i int) *int {
	return &i
}

// flattenJSONToMap parses raw JSON and flattens it into dot-notation keys in the target map.
// Objects use dot: prefix.key.subkey
// Arrays use brackets: prefix[0].field
func flattenJSONToMap(m map[string]string, prefix string, data json.RawMessage) {
	if len(data) == 0 {
		return
	}

	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return
	}
	flattenValue(m, prefix, v)
}

func flattenValue(m map[string]string, prefix string, v any) {
	switch val := v.(type) {
	case map[string]any:
		for key, sub := range val {
			flattenValue(m, prefix+"."+key, sub)
		}
	case []any:
		for i, sub := range val {
			flattenValue(m, fmt.Sprintf("%s[%d]", prefix, i), sub)
		}
	case string:
		m[prefix] = val
	case float64:
		if val == float64(int64(val)) {
			m[prefix] = strconv.FormatInt(int64(val), 10)
		} else {
			m[prefix] = strconv.FormatFloat(val, 'f', -1, 64)
		}
	case bool:
		m[prefix] = strconv.FormatBool(val)
	case nil:
		// skip null values
	}
}

// summaryData is a partial parse of DiscoverSwiss summary objects
type summaryData struct {
	TotalFeatures struct {
		Value string `json:"value"`
	} `json:"totalFeatures"`
	MaxElevation       int `json:"maxElevation"`
	AdditionalProperty []struct {
		PropertyID string `json:"propertyId"`
		Value      string `json:"value"`
	} `json:"additionalProperty"`
}

// parseSummary extracts structured data from a json.RawMessage summary
func parseSummary(raw json.RawMessage) *summaryData {
	if len(raw) == 0 {
		return nil
	}
	var s summaryData
	if err := json.Unmarshal(raw, &s); err != nil {
		return nil
	}
	return &s
}

// getSummaryProperty extracts a property value from additionalProperty by propertyId
func getSummaryProperty(s *summaryData, propertyID string) string {
	if s == nil {
		return ""
	}
	for _, p := range s.AdditionalProperty {
		if p.PropertyID == propertyID {
			return p.Value
		}
	}
	return ""
}

// isLanguageAvailable checks if lang is listed in availableDataLanguage.
// If availableDataLanguage is empty/nil, the language is considered available.
func isLanguageAvailable(availableDataLanguage []string, lang string) bool {
	if len(availableDataLanguage) == 0 {
		return true
	}
	for _, l := range availableDataLanguage {
		if l == lang {
			return true
		}
	}
	return false
}

// TransformSkiArea converts raw SkiArea data from DiscoverSwiss to a TransformResult
// containing the SkiArea and all associated POIs (lifts, slopes, parks, tobogganing).
// The lang parameter specifies the language of the current record.
func TransformSkiArea(raw dto.SkiArea, id string, lang string) (odhContentModel.TransformResult, error) {
	skiArea, err := MapSkiAreaToODH(raw, id, lang)
	if err != nil {
		return odhContentModel.TransformResult{}, fmt.Errorf("mapping ski area: %w", err)
	}

	var pois []odhContentModel.ODHActivityPoi

	subEntityGroups := []struct {
		entities      []dto.SkiSubEntity
		subEntityType string
	}{
		{raw.HasSkiLift, "SkiLift"},
		{raw.HasSkiSlope, "SkiSlope"},
		{raw.HasSnowPark, "SnowPark"},
		{raw.HasTobogganing, "Tobogganing"},
		{raw.HasCrossCountry, "CrossCountry"},
		{raw.HasHiking, "Hiking"},
	}

	for _, group := range subEntityGroups {
		for i, entity := range group.entities {
			if entity.Details == nil {
				continue
			}
			// Skip sub-entity if lang is not in its availableDataLanguage
			if !isLanguageAvailable(entity.Details.AvailableDataLanguage, lang) {
				continue
			}
			poi := MapSubEntityToPOI(*entity.Details, group.subEntityType, id, i, lang, raw.ContainedInPlace)
			pois = append(pois, poi)
		}
	}

	// Extract weather measuring points
	measuringpoints := MapWeatherToMeasuringpoints(raw, id, lang)

	return odhContentModel.TransformResult{
		SkiArea:         skiArea,
		POI:             pois,
		Measuringpoints: measuringpoints,
	}, nil
}

// MapSubEntityToPOI converts a DiscoverSwiss SkiSubEntityDetails to an ODH ODHActivityPoi.
// The lang parameter specifies the language for this record's text fields.
func MapSubEntityToPOI(raw dto.SkiSubEntityDetails, subEntityType string, parentID string, index int, lang string, parentPlaces []dto.AdministrativeArea) odhContentModel.ODHActivityPoi {
	poi := odhContentModel.ODHActivityPoi{
		Generic: odhContentModel.Generic{
			Active: !raw.Removed,
			Source: StringPtr(SOURCE),
			LicenseInfo: &odhContentModel.LicenseInfo{
				ClosedData: false,
			},
			Geo: make(map[string]odhContentModel.GpsInfo),
		},
	}

	// Generate POI ID from identifier or compose from parent + index
	poiID := generatePOIID(raw, parentID, subEntityType, index)
	poi.ID = StringPtr(poiID)

	// Set shortname from alternateName or name
	if raw.AlternateName != "" {
		poi.Shortname = IfNotEmpty(raw.AlternateName)
	} else if raw.Name != "" {
		poi.Shortname = IfNotEmpty(raw.Name)
	}

	// Map type classification
	poi.Type = IfNotEmpty(subEntityType)
	poi.SubType = IfNotEmpty(raw.AdditionalType)

	// Map detail (name, description, additional texts)
	poi.Detail = mapPOIDetailData(raw, lang)

	// Map contact information
	poi.ContactInfos = mapPOIContactInfo(raw, lang)

	// Map GPS coordinates
	if raw.Geo != nil {
		gpsInfo := odhContentModel.GpsInfo{
			Latitude:  Float64Ptr(raw.Geo.Latitude),
			Longitude: Float64Ptr(raw.Geo.Longitude),
			Gpstype:   StringPtr("position"),
			Default:   true,
		}
		if raw.Geo.Elevation > 0 {
			gpsInfo.Altitude = Float64Ptr(raw.Geo.Elevation)
			gpsInfo.AltitudeUnitofMeasure = StringPtr("m")
		}
		poi.Geo["position"] = gpsInfo

		// Also populate GpsInfo array for ODHActivityPoi
		poi.GpsInfo = append(poi.GpsInfo, gpsInfo)
	}

	// Map images
	poi.ImageGallery = mapPOIImageGallery(raw, lang)

	// Map elevation data
	if raw.Elevation != nil {
		if raw.Elevation.MaxAltitude > 0 {
			poi.AltitudeHighestPoint = Float64Ptr(float64(raw.Elevation.MaxAltitude))
		}
		if raw.Elevation.MinAltitude > 0 {
			poi.AltitudeLowestPoint = Float64Ptr(float64(raw.Elevation.MinAltitude))
		}
		if raw.Elevation.Ascent > 0 {
			poi.AltitudeSumUp = Float64Ptr(float64(raw.Elevation.Ascent))
		}
		if raw.Elevation.Descent > 0 {
			poi.AltitudeSumDown = Float64Ptr(float64(raw.Elevation.Descent))
		}
		if raw.Elevation.Differential > 0 {
			poi.AltitudeDifference = Float64Ptr(float64(raw.Elevation.Differential))
		}
	}

	// Map length and duration
	if raw.Length > 0 {
		poi.DistanceLength = Float64Ptr(raw.Length)
	}
	if raw.Time > 0 {
		poi.DistanceDuration = Float64Ptr(float64(raw.Time))
	}

	// Map ratings
	if raw.Rating != nil {
		poi.Ratings = mapPOIRatings(raw.Rating)
	}

	// Map difficulty from difficulties.difficulty[] (overrides rating.difficulty if present)
	if raw.Difficulties != nil && len(raw.Difficulties.Difficulty) > 0 {
		if poi.Ratings == nil {
			poi.Ratings = &odhContentModel.Ratings{}
		}
		d := raw.Difficulties.Difficulty[0]
		poi.Ratings.Difficulty = StringPtr(fmt.Sprintf("%s %s", d.Type, d.Value))
	}

	// Map exposition
	if raw.Exposition != nil {
		poi.ExpositionValues = mapPOIExposition(raw.Exposition)
	}

	// Map state (open/closed)
	if raw.State != "" {
		isOpen := strings.EqualFold(raw.State, "open")
		poi.IsOpen = BoolPtr(isOpen)
	}

	// Map boolean flags
	if raw.IsAccessibleForFree != nil {
		poi.HasFreeEntrance = raw.IsAccessibleForFree
	}
	if raw.Highlight != nil {
		poi.Highlight = raw.Highlight
	}

	// Map license information
	if raw.License != "" {
		license, closedData := mapLicense(raw.License)
		poi.LicenseInfo.License = StringPtr(license)
		poi.LicenseInfo.ClosedData = closedData
	}
	if raw.DataGovernance != nil && raw.DataGovernance.Source != nil {
		poi.LicenseInfo.LicenseHolder = IfNotEmpty(raw.DataGovernance.Source.Name)
	}

	// Set HasLanguage to the current language
	poi.HasLanguage = []string{lang}

	// Map temporal data
	if raw.LastModified != "" {
		if lastModTime, err := time.Parse(time.RFC3339, raw.LastModified); err == nil {
			poi.LastChange = StringPtr(lastModTime.Format(time.RFC3339))
		}
	}

	// Map data governance to first import
	if raw.DataGovernance != nil && len(raw.DataGovernance.Origin) > 0 {
		if created := raw.DataGovernance.Origin[0].Created; created != "" {
			if createdTime, err := time.Parse(time.RFC3339, created); err == nil {
				poi.FirstImport = StringPtr(createdTime.Format(time.RFC3339))
			}
		}
		// Map custom/source ID
		if raw.DataGovernance.Origin[0].SourceID != "" {
			poi.CustomId = IfNotEmpty(raw.DataGovernance.Origin[0].SourceID)
		}
	}

	// Map tags
	poi.Tags = mapPOITags(raw.Tag)

	// Map categories to SmgTags
	poi.SmgTags = mapCategoriesToTags(raw.Category)

	// Map ODHActivityPoiTypes from type and additionalType
	poi.ODHActivityPoiTypes = mapPOITypes(raw, subEntityType)

	// Map location info: use sub-entity's own containedInPlace, fall back to parent's
	places := raw.ContainedInPlace
	if len(places) == 0 {
		places = parentPlaces
	}
	poi.LocationInfo = mapPlacesToLocationInfo(places, lang)

	// Map operation schedule from opening hours
	poi.OperationSchedule = mapPOIOperationSchedule(raw.OpeningHoursSpecification, lang)

	// Map GPS tracks from download links
	poi.GpsTrack = mapGpsTracks(raw.Link)

	// Map TagIds for API filtering (tagfilter parameter)
	poi.TagIds = mapSubEntityTagIds(subEntityType, raw.AdditionalType)

	// Link to parent ski area
	poi.AreaId = []string{parentID}

	// Build mapping with individual fields
	m := map[string]string{
		"id":                 raw.Identifier,
		"autoTranslatedData": strconv.FormatBool(raw.AutoTranslatedData),
	}

	if len(raw.Video) > 0 {
		videoBytes, _ := json.Marshal(raw.Video)
		flattenJSONToMap(m, "video", videoBytes)
	}
	if raw.Duration != "" {
		m["duration"] = raw.Duration
	}
	if raw.LengthOpen > 0 {
		m["lengthOpen"] = strconv.FormatFloat(raw.LengthOpen, 'f', -1, 64)
	}
	if raw.StateTimestamp != "" {
		m["stateTimestamp"] = raw.StateTimestamp
	}

	// Extract label from additionalProperty (e.g. slope number)
	for _, prop := range raw.AdditionalProperty {
		if prop.PropertyID == "label" && len(prop.Value) > 0 {
			m["label"] = prop.ValueString()
			break
		}
	}

	// json.RawMessage fields flattened with dot notation
	if len(raw.LocatedAt) > 0 {
		flattenJSONToMap(m, "locatedAt", raw.LocatedAt)
	}
	if len(raw.PotentialAction) > 0 {
		flattenJSONToMap(m, "potentialAction", raw.PotentialAction)
	}

	// Language-dependent text fields with dot notation
	if raw.Parking != "" {
		m["parking."+lang] = raw.Parking
	}
	if raw.PublicTransport != "" {
		m["publicTransport."+lang] = raw.PublicTransport
	}
	if raw.Equipment != "" {
		m["equipment."+lang] = raw.Equipment
	}
	if raw.SafetyGuidelines != "" {
		m["safetyGuidelines."+lang] = raw.SafetyGuidelines
	}

	poi.Mapping = map[string]map[string]string{"discoverswiss": m}

	return poi
}

// mapGpsTracks converts download links (GPX, KML) to ODH GpsTrack entries
func mapGpsTracks(links []dto.Link) []odhContentModel.GpsTrack {
	var tracks []odhContentModel.GpsTrack
	for _, link := range links {
		if link.URL == "" {
			continue
		}
		switch link.Type {
		case "DownloadGpx":
			tracks = append(tracks, odhContentModel.GpsTrack{
				GpxTrackUrl: StringPtr(link.URL),
				Type:        StringPtr("gpx"),
			})
		case "DownloadKml":
			tracks = append(tracks, odhContentModel.GpsTrack{
				GpxTrackUrl: StringPtr(link.URL),
				Type:        StringPtr("kml"),
			})
		}
	}
	return tracks
}

// mapSubEntityTagIds returns the TagIds for a given sub-entity type and additionalType.
// These are used by the ODH API tagfilter parameter for filtering POIs.
func mapSubEntityTagIds(subEntityType string, additionalType string) []string {
	switch subEntityType {
	case "SkiLift":
		tags := []string{"lifts"}
		switch additionalType {
		case "ChairLift":
			tags = append(tags, "chairlift")
		case "CableCar":
			tags = append(tags, "ropeway")
		}
		return tags
	case "SkiSlope":
		return []string{"winter", "slope", "slopes", "marked ski paths slopes"}
	case "SnowPark":
		return []string{"winter", "snowpark", "snow parks"}
	case "Tobogganing":
		return []string{"winter", "tobbogan run", "sledging trail"}
	case "CrossCountry":
		return []string{"winter", "crosscountry skitrack", "crosscountry skiing"}
	case "Hiking":
		tags := []string{"hiking"}
		switch additionalType {
		case "SnowshoeTrail":
			tags = append(tags, "winter", "snowshoe hikes")
		case "HikingTrail":
			// could be winter or summer, add winter by default since it's in a ski area
			tags = append(tags, "winter", "winter hiking")
		default:
			tags = append(tags, "winter", "winter hiking")
		}
		return tags
	default:
		return []string{"winter"}
	}
}

// generatePOIID creates an ID for a POI from its details.
// Format: urn:odhactivitypoi:discoverswiss:<ds_type>:<identifier>
// ds_type comes from raw.Type (e.g. Tour, LocalBusiness, TransportationSystem)
func generatePOIID(raw dto.SkiSubEntityDetails, parentID string, subEntityType string, index int) string {
	dsType := cleanType(raw.Type)
	if dsType == "" {
		dsType = subEntityType
	}
	if raw.Identifier != "" {
		return fmt.Sprintf("urn:odhactivitypoi:%s:%s:%s", SOURCE, dsType, raw.Identifier)
	}
	return fmt.Sprintf("urn:odhactivitypoi:%s:%s:%s_%d", SOURCE, dsType, parentID, index)
}

// mapPOIDetailData maps name and description from sub-entity to a single-language Detail entry
func mapPOIDetailData(raw dto.SkiSubEntityDetails, lang string) map[string]odhContentModel.Detail {
	details := make(map[string]odhContentModel.Detail)

	detail := odhContentModel.Detail{
		Language: StringPtr(lang),
		Title:    IfNotEmpty(raw.Name),
		BaseText: IfNotEmpty(raw.Description),
	}

	if raw.DisambiguatingDescription != "" {
		detail.AdditionalText = IfNotEmpty(raw.DisambiguatingDescription)
	}
	if raw.AdditionalInformation != "" {
		detail.AdditionalText = IfNotEmpty(raw.AdditionalInformation)
	}
	if raw.TextTeaser != "" {
		detail.IntroText = IfNotEmpty(raw.TextTeaser)
	}
	if raw.TitleTeaser != "" {
		detail.Header = IfNotEmpty(raw.TitleTeaser)
	}
	if raw.Directions != "" || raw.GettingThere != "" {
		text := raw.GettingThere
		if text == "" {
			text = raw.Directions
		}
		detail.GetThereText = IfNotEmpty(text)
	}

	details[lang] = detail

	return details
}

// mapPOIContactInfo maps address and contact data from sub-entity to a single-language entry
func mapPOIContactInfo(raw dto.SkiSubEntityDetails, lang string) map[string]odhContentModel.ContactInfos {
	if raw.Address == nil && raw.Telephone == "" && raw.FaxNumber == "" && raw.URL == "" {
		return nil
	}

	contactInfos := make(map[string]odhContentModel.ContactInfos)

	contact := odhContentModel.ContactInfos{
		Language: StringPtr(lang),
	}

	if raw.Address != nil {
		contact.Address = IfNotEmpty(raw.Address.StreetAddress)
		contact.City = IfNotEmpty(raw.Address.AddressLocality)
		contact.ZipCode = IfNotEmpty(raw.Address.PostalCode)
		contact.CountryCode = IfNotEmpty(raw.Address.AddressCountry)

		if raw.Address.Email != "" {
			contact.Email = StringPtr(raw.Address.Email)
		}
		if raw.Address.Telephone != "" {
			contact.Phonenumber = StringPtr(raw.Address.Telephone)
		}
	}

	if contact.Phonenumber == nil && raw.Telephone != "" {
		contact.Phonenumber = StringPtr(raw.Telephone)
	}
	if raw.FaxNumber != "" {
		contact.Faxnumber = StringPtr(raw.FaxNumber)
	}
	if raw.URL != "" {
		contact.Url = StringPtr(raw.URL)
	}

	contactInfos[lang] = contact

	return contactInfos
}

// mapPOIImageGallery maps images from sub-entity to ODH ImageGallery
func mapPOIImageGallery(raw dto.SkiSubEntityDetails, lang string) []odhContentModel.ImageGallery {
	var gallery []odhContentModel.ImageGallery

	images := raw.ParseImages()

	for i, img := range images {
		if img.ContentURL == "" {
			continue
		}
		entry := mapSingleImage(img, i, i == 0, lang)
		gallery = append(gallery, entry)
	}

	for i, photo := range raw.Photo {
		if photo.ContentURL == "" {
			continue
		}
		// Check for duplicates with image array
		duplicate := false
		for _, img := range images {
			if photo.ContentURL == img.ContentURL {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		entry := mapSingleImage(photo, len(images)+i, false, lang)
		gallery = append(gallery, entry)
	}

	return gallery
}

// mapPOIRatings maps DiscoverSwiss rating to ODH Ratings
func mapPOIRatings(rating *dto.Rating) *odhContentModel.Ratings {
	r := &odhContentModel.Ratings{}
	if rating.Difficulty > 0 {
		r.Difficulty = StringPtr(strconv.Itoa(rating.Difficulty))
	}
	if rating.Technique > 0 {
		r.Technique = StringPtr(strconv.Itoa(rating.Technique))
	}
	if rating.Condition > 0 {
		r.Stamina = StringPtr(strconv.Itoa(rating.Condition))
	}
	if rating.QualityOfExperience > 0 {
		r.Experience = StringPtr(strconv.Itoa(rating.QualityOfExperience))
	}
	if rating.Landscape > 0 {
		r.Landscape = StringPtr(strconv.Itoa(rating.Landscape))
	}
	return r
}

// mapPOIExposition maps DiscoverSwiss Exposition booleans to ODH Exposition array
func mapPOIExposition(expo *dto.Exposition) odhContentModel.Exposition {
	var result odhContentModel.Exposition
	if expo.NN {
		result = append(result, "N")
	}
	if expo.NE {
		result = append(result, "NE")
	}
	if expo.EE {
		result = append(result, "E")
	}
	if expo.SE {
		result = append(result, "SE")
	}
	if expo.SS {
		result = append(result, "S")
	}
	if expo.SW {
		result = append(result, "SW")
	}
	if expo.WW {
		result = append(result, "W")
	}
	if expo.NW {
		result = append(result, "NW")
	}
	return result
}

// mapPOITags maps DiscoverSwiss tags to ODH Tags
func mapPOITags(tags []dto.Tag) []odhContentModel.Tag {
	if len(tags) == 0 {
		return nil
	}

	var result []odhContentModel.Tag
	for _, t := range tags {
		// Use Identifier (e.g. "seasonality-winter") as the tag ID
		tagID := t.Identifier
		if tagID == "" {
			tagID = t.ID
		}
		if tagID == "" {
			continue
		}
		odhTag := odhContentModel.Tag{
			ID:     StringPtr(tagID),
			Source: StringPtr(SOURCE),
			Name:   IfNotEmpty(t.Name),
			Type:   IfNotEmpty(t.Type),
		}
		result = append(result, odhTag)
	}
	return result
}

// mapPOITypes generates ODHActivityPoiTypes from the sub-entity type info
func mapPOITypes(raw dto.SkiSubEntityDetails, subEntityType string) []odhContentModel.ODHActivityPoiType {
	var types []odhContentModel.ODHActivityPoiType

	// Primary type based on sub-entity kind
	primaryType := odhContentModel.ODHActivityPoiType{
		Type: StringPtr(subEntityType),
	}
	if raw.Type != "" {
		primaryType.Key = IfNotEmpty(raw.Type)
	}
	types = append(types, primaryType)

	// Additional type if present
	if raw.AdditionalType != "" {
		additionalType := odhContentModel.ODHActivityPoiType{
			Type: StringPtr(raw.AdditionalType),
			Key:  IfNotEmpty(raw.AdditionalType),
		}
		types = append(types, additionalType)
	}

	return types
}

// mapPlacesToLocationInfo extracts location info from containedInPlace entries for a single language.
// Used by both SkiArea and POI mapping.
func mapPlacesToLocationInfo(places []dto.AdministrativeArea, lang string) *odhContentModel.LocationInfo {
	if len(places) == 0 {
		return nil
	}

	locInfo := &odhContentModel.LocationInfo{}

	for _, place := range places {
		switch strings.ToLower(place.AdditionalType) {
		case "state":
			locInfo.RegionInfo = &odhContentModel.RegionInfo{
				Name: make(map[string]string),
			}
			if place.Identifier != "" {
				locInfo.RegionInfo.ID = IfNotEmpty(place.Identifier)
			}
			locInfo.RegionInfo.Name[lang] = place.Name

		case "city":
			locInfo.MunicipalityInfo = &odhContentModel.MunicipalityInfo{
				Name: make(map[string]string),
			}
			if place.Identifier != "" {
				locInfo.MunicipalityInfo.ID = IfNotEmpty(place.Identifier)
			}
			locInfo.MunicipalityInfo.Name[lang] = place.Name

		case "tourismarea":
			locInfo.TvInfo = &odhContentModel.TvInfo{
				Name: make(map[string]string),
			}
			if place.Identifier != "" {
				locInfo.TvInfo.ID = IfNotEmpty(place.Identifier)
			}
			locInfo.TvInfo.Name[lang] = place.Name

		case "district":
			locInfo.DistrictInfo = &odhContentModel.DistrictInfo{
				Name: make(map[string]string),
			}
			if place.Identifier != "" {
				locInfo.DistrictInfo.ID = IfNotEmpty(place.Identifier)
			}
			locInfo.DistrictInfo.Name[lang] = place.Name
		}
	}

	return locInfo
}

// mapPOIOperationSchedule maps DiscoverSwiss openingHoursSpecification to ODH OperationSchedule.
// Entries with the same validity period and same opens/closes times are merged into
// a single OperationScheduleTime with multiple day flags set.
// Names are aggregated per period: equal names are deduplicated, different ones joined with " - ".
func mapPOIOperationSchedule(specs []dto.OpeningHoursSpec, lang string) []odhContentModel.OperationSchedule {
	if len(specs) == 0 {
		return nil
	}

	type periodKey struct {
		ValidFrom    string
		ValidThrough string
	}
	type timeKey struct {
		Opens  string
		Closes string
	}

	// period -> timeSlot -> merged OperationScheduleTime
	type periodData struct {
		schedule *odhContentModel.OperationSchedule
		times    map[timeKey]*odhContentModel.OperationScheduleTime
		names    []string // unique names collected from specs in this period
	}
	periods := make(map[periodKey]*periodData)
	// Preserve insertion order
	var periodOrder []periodKey

	for _, spec := range specs {
		pKey := periodKey{
			ValidFrom:    spec.ValidFrom,
			ValidThrough: spec.ValidThrough,
		}

		pd, exists := periods[pKey]
		if !exists {
			pd = &periodData{
				schedule: &odhContentModel.OperationSchedule{
					Start: IfNotEmpty(spec.ValidFrom),
					Stop:  IfNotEmpty(spec.ValidThrough),
				},
				times: make(map[timeKey]*odhContentModel.OperationScheduleTime),
			}
			periods[pKey] = pd
			periodOrder = append(periodOrder, pKey)
		}

		// Collect unique names per period
		if spec.Name != "" {
			found := false
			for _, n := range pd.names {
				if n == spec.Name {
					found = true
					break
				}
			}
			if !found {
				pd.names = append(pd.names, spec.Name)
			}
		}

		tKey := timeKey{Opens: spec.Opens, Closes: spec.Closes}
		schedTime, texists := pd.times[tKey]
		if !texists {
			schedTime = &odhContentModel.OperationScheduleTime{
				Start: IfNotEmpty(spec.Opens),
				End:   IfNotEmpty(spec.Closes),
				State: 2, // open
			}
			pd.times[tKey] = schedTime
		}

		// Merge day flag into existing entry
		switch strings.ToLower(spec.DayOfWeek) {
		case "monday":
			schedTime.Monday = true
		case "tuesday":
			schedTime.Tuesday = true
		case "wednesday":
			schedTime.Wednesday = true
		case "thursday":
			schedTime.Thursday = true
		case "friday":
			schedTime.Friday = true
		case "saturday":
			schedTime.Saturday = true
		case "sunday":
			schedTime.Sunday = true
		}
	}

	var result []odhContentModel.OperationSchedule
	for _, pKey := range periodOrder {
		pd := periods[pKey]
		for _, st := range pd.times {
			pd.schedule.OperationScheduleTime = append(pd.schedule.OperationScheduleTime, *st)
		}
		if len(pd.names) > 0 {
			pd.schedule.OperationscheduleName = map[string]string{
				lang: strings.Join(pd.names, " - "),
			}
		}
		result = append(result, *pd.schedule)
	}

	return result
}

// MapSkiAreaToODH converts raw SkiArea data from DiscoverSwiss to OpenDataHub SkiArea model.
// The lang parameter specifies the language for this record's text fields.
func MapSkiAreaToODH(raw dto.SkiArea, id string, lang string) (odhContentModel.SkiArea, error) {
	skiArea := odhContentModel.SkiArea{
		Generic: odhContentModel.Generic{
			Active: !raw.Removed,
			Source: StringPtr(SOURCE),
			LicenseInfo: &odhContentModel.LicenseInfo{
				ClosedData: false,
			},
			Geo: make(map[string]odhContentModel.GpsInfo),
		},
	}

	// Set ID
	skiArea.ID = StringPtr(id)

	// Set shortname from name (more descriptive than identifier)
	if raw.Name != "" {
		skiArea.Shortname = IfNotEmpty(raw.Name)
	} else if raw.Identifier != "" {
		skiArea.Shortname = IfNotEmpty(raw.Identifier)
	}

	// Map basic names and descriptions
	skiArea.Detail = mapDetailData(raw, lang)

	// Map contact information
	skiArea.ContactInfos = mapContactInfo(raw, lang)

	// Map GPS coordinates
	if raw.Geo != nil {
		gpsInfo := odhContentModel.GpsInfo{
			Latitude:  Float64Ptr(raw.Geo.Latitude),
			Longitude: Float64Ptr(raw.Geo.Longitude),
			Gpstype:   StringPtr("position"),
			Default:   true,
		}

		if raw.Geo.Elevation > 0 {
			gpsInfo.Altitude = Float64Ptr(raw.Geo.Elevation)
			gpsInfo.AltitudeUnitofMeasure = StringPtr("m")
		}

		skiArea.Geo["position"] = gpsInfo
		// SkiArea uses GpsInfo array (not Geo map) in the ODH API
		skiArea.GpsInfo = []odhContentModel.GpsInfo{gpsInfo}
	}

	// Map images
	skiArea.ImageGallery = mapImageGallery(raw, lang)

	// Map license information
	if raw.License != "" {
		license, closedData := mapLicense(raw.License)
		skiArea.LicenseInfo.License = StringPtr(license)
		skiArea.LicenseInfo.ClosedData = closedData
	}

	// Map data governance to license holder
	if raw.DataGovernance != nil && raw.DataGovernance.Source != nil {
		skiArea.LicenseInfo.LicenseHolder = IfNotEmpty(raw.DataGovernance.Source.Name)
	}

	// Map logo URL to ContactInfos
	if len(raw.Logo) > 0 {
		var logo dto.ImageObject
		if err := json.Unmarshal(raw.Logo, &logo); err == nil && logo.ContentURL != "" {
			if contact, ok := skiArea.ContactInfos[lang]; ok {
				contact.LogoUrl = StringPtr(logo.ContentURL)
				skiArea.ContactInfos[lang] = contact
			}
		}
	}

	// Set HasLanguage to the current language
	skiArea.HasLanguage = []string{lang}

	// Map temporal data
	if raw.LastModified != "" {
		if lastModTime, err := time.Parse(time.RFC3339, raw.LastModified); err == nil {
			skiArea.LastChange = StringPtr(lastModTime.Format(time.RFC3339))
		}
	}

	// Map data governance to first import if available
	if raw.DataGovernance != nil && len(raw.DataGovernance.Origin) > 0 {
		if created := raw.DataGovernance.Origin[0].Created; created != "" {
			if createdTime, err := time.Parse(time.RFC3339, created); err == nil {
				skiArea.FirstImport = StringPtr(createdTime.Format(time.RFC3339))
			}
		}
	}

	// Map tags from categories
	skiArea.SmgTags = mapCategoriesToTags(raw.Category)

	// Map region information from containedInPlace
	skiArea.LocationInfo = mapLocationInfo(raw, lang)

	// Map SkiRegionName from containedInPlace MountainArea
	for _, place := range raw.ContainedInPlace {
		if strings.EqualFold(place.AdditionalType, "MountainArea") {
			skiArea.SkiRegionName = map[string]string{lang: place.Name}
			break
		}
	}

	// Map AltitudeFrom from MinElevation
	if raw.MinElevation > 0 {
		skiArea.AltitudeFrom = IntPtr(raw.MinElevation)
	}

	// Map SkiAreaMapURL from HasMap
	skiArea.SkiAreaMapURL = IfNotEmpty(raw.HasMap)

	// Map CustomId from DataGovernance
	if raw.DataGovernance != nil && len(raw.DataGovernance.Origin) > 0 {
		skiArea.CustomId = IfNotEmpty(raw.DataGovernance.Origin[0].SourceID)
	}

	// Parse summaries for structured data
	slopeSummary := parseSummary(raw.SkiSlopeSummary)
	liftSummary := parseSummary(raw.SkiLiftSummary)

	// AltitudeTo from skiSlopeSummary.maxElevation
	if slopeSummary != nil && slopeSummary.MaxElevation > 0 {
		skiArea.AltitudeTo = IntPtr(slopeSummary.MaxElevation)
	} else if raw.MaxElevation > 0 {
		skiArea.AltitudeTo = IntPtr(raw.MaxElevation)
	}

	// TotalSlopeKm from lengthOfSlopes (convert meters to km)
	if lengthStr := getSummaryProperty(slopeSummary, "lengthOfSlopes"); lengthStr != "" {
		if lengthM, err := strconv.Atoi(lengthStr); err == nil && lengthM > 0 {
			km := float64(lengthM) / 1000.0
			skiArea.TotalSlopeKm = StringPtr(strconv.FormatFloat(km, 'f', 1, 64))
		}
	}

	// LiftCount from skiLiftSummary.totalFeatures.value
	if liftSummary != nil && liftSummary.TotalFeatures.Value != "" {
		skiArea.LiftCount = IfNotEmpty(liftSummary.TotalFeatures.Value)
	}

	// OperationSchedule from seasonStart/seasonEnd
	if raw.SeasonStart != "" || raw.SeasonEnd != "" {
		schedule := odhContentModel.OperationSchedule{
			Start: IfNotEmpty(raw.SeasonStart),
			Stop:  IfNotEmpty(raw.SeasonEnd),
		}
		skiArea.OperationSchedule = []odhContentModel.OperationSchedule{schedule}
	}

	// TagIds for SkiArea API filtering
	skiArea.TagIds = []string{"skiarea", "winter"}

	// Build mapping with individual fields
	m := map[string]string{
		"id":                 raw.Identifier,
		"autoTranslatedData": strconv.FormatBool(raw.AutoTranslatedData),
	}

	if raw.Type != "" {
		m["type"] = raw.Type
	}
	if raw.OsmID != "" {
		m["osmId"] = raw.OsmID
	}

	// Complex objects flattened with dot notation
	for key, rm := range map[string]json.RawMessage{
		"additionalProperty":  raw.AdditionalProperty,
		"crossCountrySummary": raw.CrossCountrySummary,
		"hikingSummary":       raw.HikingSummary,
		"skiLiftSummary":      raw.SkiLiftSummary,
		"skiSlopeSummary":     raw.SkiSlopeSummary,
		"snowConditions":      raw.SnowConditions,
		"snowConditionsSlope": raw.SnowConditionsSlope,
		"snowboardSummary":    raw.SnowboardSummary,
		"tobogganingSummary":  raw.TobogganingSummary,
		"weatherMountain":     raw.WeatherMountain,
		"weatherValley":       raw.WeatherValley,
		"touristInformation":  raw.TouristInformation,
		"potentialAction":     raw.PotentialAction,
		"logo":                raw.Logo,
	} {
		if len(rm) > 0 {
			flattenJSONToMap(m, key, rm)
		}
	}

	// Simple fields
	if raw.SeasonStart != "" {
		m["seasonStart"] = raw.SeasonStart
	}
	if raw.SeasonEnd != "" {
		m["seasonEnd"] = raw.SeasonEnd
	}
	if raw.MinElevation > 0 {
		m["minElevation"] = strconv.Itoa(raw.MinElevation)
	}
	if raw.MaxElevation > 0 {
		m["maxElevation"] = strconv.Itoa(raw.MaxElevation)
	}
	if raw.HasMap != "" {
		m["hasMap"] = raw.HasMap
	}
	if raw.HasShuttleBus != nil {
		m["hasShuttleBus"] = strconv.FormatBool(*raw.HasShuttleBus)
	}

	skiArea.Mapping = map[string]map[string]string{"discoverswiss": m}

	return skiArea, nil
}

// mapDetailData maps name and description to a single-language Detail entry
func mapDetailData(raw dto.SkiArea, lang string) map[string]odhContentModel.Detail {
	details := make(map[string]odhContentModel.Detail)

	detail := odhContentModel.Detail{
		Language: StringPtr(lang),
		Title:    IfNotEmpty(raw.Name),
		BaseText: IfNotEmpty(raw.Description),
	}

	if raw.DisambiguatingDescription != "" {
		detail.Header = IfNotEmpty(raw.DisambiguatingDescription)
	}

	details[lang] = detail

	return details
}

// mapContactInfo maps address and contact data to a single-language ContactInfos entry
func mapContactInfo(raw dto.SkiArea, lang string) map[string]odhContentModel.ContactInfos {
	contactInfos := make(map[string]odhContentModel.ContactInfos)

	contact := odhContentModel.ContactInfos{
		Language: StringPtr(lang),
	}

	if raw.Address != nil {
		contact.Address = IfNotEmpty(raw.Address.StreetAddress)
		contact.City = IfNotEmpty(raw.Address.AddressLocality)
		contact.ZipCode = IfNotEmpty(raw.Address.PostalCode)
		contact.CountryCode = IfNotEmpty(raw.Address.AddressCountry)

		if raw.Address.Email != "" {
			contact.Email = StringPtr(raw.Address.Email)
		}
		if raw.Address.Telephone != "" {
			contact.Phonenumber = StringPtr(raw.Address.Telephone)
		}
	}

	if contact.Email == nil && raw.Address != nil && raw.Address.Email != "" {
		contact.Email = StringPtr(raw.Address.Email)
	}
	if contact.Phonenumber == nil && raw.Telephone != "" {
		contact.Phonenumber = StringPtr(raw.Telephone)
	}

	if raw.URL != "" {
		contact.Url = StringPtr(raw.URL)
	} else if len(raw.Link) > 0 {
		for _, link := range raw.Link {
			if link.Type == "WebHomepage" {
				contact.Url = StringPtr(link.URL)
				break
			}
		}
	}

	contactInfos[lang] = contact

	return contactInfos
}

// mapImageGallery maps DiscoverSwiss image objects to ODH ImageGallery
func mapImageGallery(raw dto.SkiArea, lang string) []odhContentModel.ImageGallery {
	var gallery []odhContentModel.ImageGallery

	if raw.Image != nil && raw.Image.ContentURL != "" {
		img := mapSingleImage(*raw.Image, 0, true, lang)
		gallery = append(gallery, img)
	}

	for i, photo := range raw.Photo {
		if raw.Image != nil && photo.ContentURL == raw.Image.ContentURL {
			continue
		}

		img := mapSingleImage(photo, i+1, false, lang)
		gallery = append(gallery, img)
	}

	return gallery
}

// mapSingleImage converts a single DiscoverSwiss ImageObject to ODH ImageGallery entry
func mapSingleImage(img dto.ImageObject, position int, isMain bool, lang string) odhContentModel.ImageGallery {
	gallery := odhContentModel.ImageGallery{
		ImageUrl:     IfNotEmpty(img.ContentURL),
		ImageName:    IfNotEmpty(img.Name),
		CopyRight:    IfNotEmpty(img.CopyrightNotice),
		IsInGallery:  BoolPtr(true),
		ListPosition: &position,
	}
	if img.Name != "" {
		gallery.ImageTitle = map[string]string{lang: img.Name}
	}
	if img.Caption != "" {
		gallery.ImageDesc = map[string]string{lang: img.Caption}
	}

	if img.License != "" {
		mapped, _ := mapLicense(img.License)
		gallery.License = StringPtr(mapped)
	}

	// Map license from data governance
	if img.DataGovernance != nil && len(img.DataGovernance.Origin) > 0 {
		if gallery.License == nil {
			mapped, _ := mapLicense(img.DataGovernance.Origin[0].License)
			gallery.License = StringPtr(mapped)
		}
	}

	// Map image source
	if img.DataGovernance != nil && img.DataGovernance.Source != nil {
		gallery.ImageSource = IfNotEmpty(img.DataGovernance.Source.Name)
	}

	// Map width/height if available
	if img.Width != "" {
		if w, err := strconv.Atoi(img.Width); err == nil {
			gallery.Width = IntPtr(w)
		}
	}
	if img.Height != "" {
		if h, err := strconv.Atoi(img.Height); err == nil {
			gallery.Height = IntPtr(h)
		}
	}

	return gallery
}

// mapCategoriesToTags converts DiscoverSwiss categories to tag strings
func mapCategoriesToTags(categories []dto.Category) []string {
	if len(categories) == 0 {
		return nil
	}

	tags := make([]string, 0, len(categories))
	for _, cat := range categories {
		if cat.Identifier == "sui_root" {
			continue
		}
		if cat.Identifier != "" {
			tags = append(tags, cat.Identifier)
		}
	}

	return tags
}

// mapLocationInfo extracts region and municipality information from containedInPlace for a single language
func mapLocationInfo(raw dto.SkiArea, lang string) *odhContentModel.LocationInfo {
	return mapPlacesToLocationInfo(raw.ContainedInPlace, lang)
}

// MapWeatherToMeasuringpoints creates up to 2 MeasuringpointV2 from a ski area:
// one for mountain (weatherMountain + snowConditions) and one for valley (weatherValley + snowConditionsSlope).
func MapWeatherToMeasuringpoints(raw dto.SkiArea, skiAreaID string, lang string) []odhContentModel.MeasuringpointV2 {
	var weatherMountain []dto.WeatherEntry
	var weatherValley []dto.WeatherEntry
	var snowConditions *dto.SnowCondition
	var snowConditionsSlope *dto.SnowCondition

	json.Unmarshal(raw.WeatherMountain, &weatherMountain)
	json.Unmarshal(raw.WeatherValley, &weatherValley)
	json.Unmarshal(raw.SnowConditions, &snowConditions)
	json.Unmarshal(raw.SnowConditionsSlope, &snowConditionsSlope)

	var mps []odhContentModel.MeasuringpointV2

	if len(weatherMountain) > 0 || snowConditions != nil {
		mp := mapMeasuringpoint(
			fmt.Sprintf("urn:measuringpoint:%s:weatherMountain:%s", SOURCE, raw.Identifier),
			raw, skiAreaID, lang,
			weatherMountain, snowConditions,
			"Mountain",
		)
		mps = append(mps, mp)
	}

	if len(weatherValley) > 0 || snowConditionsSlope != nil {
		mp := mapMeasuringpoint(
			fmt.Sprintf("urn:measuringpoint:%s:weatherValley:%s", SOURCE, raw.Identifier),
			raw, skiAreaID, lang,
			weatherValley, snowConditionsSlope,
			"Valley",
		)
		mps = append(mps, mp)
	}

	return mps
}

// mapMeasuringpoint builds a single MeasuringpointV2 from weather and snow data.
func mapMeasuringpoint(
	id string,
	raw dto.SkiArea,
	skiAreaID string,
	lang string,
	weather []dto.WeatherEntry,
	snow *dto.SnowCondition,
	suffix string,
) odhContentModel.MeasuringpointV2 {
	name := raw.Name
	if name != "" {
		name = name + " (" + suffix + ")"
	}

	mp := odhContentModel.MeasuringpointV2{
		Generic: odhContentModel.Generic{
			ID:     StringPtr(id),
			Active: !raw.Removed,
			Source: StringPtr(SOURCE),
			LicenseInfo: &odhContentModel.LicenseInfo{
				ClosedData: false,
			},
			Shortname:   IfNotEmpty(name),
			HasLanguage: []string{lang},
		},
		SkiAreaIds: []string{skiAreaID},
	}

	// License
	if raw.License != "" {
		license, closedData := mapLicense(raw.License)
		mp.LicenseInfo.License = StringPtr(license)
		mp.LicenseInfo.ClosedData = closedData
	}
	if raw.DataGovernance != nil && raw.DataGovernance.Source != nil {
		mp.LicenseInfo.LicenseHolder = IfNotEmpty(raw.DataGovernance.Source.Name)
	}

	// Detail (language-keyed title)
	if name != "" {
		mp.Detail = map[string]odhContentModel.DetailGeneric{
			lang: {
				Title:    StringPtr(name),
				Language: StringPtr(lang),
			},
		}
	}

	// GPS: not set — weather data has no own coordinates, and copying from ski area location
	// would be misleading. Only set GPS if the source data provides weather-specific coordinates.

	// LocationInfo from ski area
	mp.LocationInfo = mapPlacesToLocationInfo(raw.ContainedInPlace, lang)

	// Snow conditions
	if snow != nil {
		mp.SnowHeight = IfNotEmpty(snow.MaxSnowHeight.Value)
		mp.NewSnowHeight = IfNotEmpty(snow.FreshFallenSnow.Value)
		mp.LastSnowDate = IfNotEmpty(snow.LastSnowfall)
	}

	// Temperature from first weather entry (today)
	if len(weather) > 0 {
		mp.Temperature = StringPtr(strconv.FormatFloat(weather[0].Temperature, 'f', -1, 64))
	}

	// Weather observations from forecast array
	for _, w := range weather {
		obs := odhContentModel.WeatherObservation{
			Date:        IfNotEmpty(w.Date),
			IconID:      StringPtr(strconv.Itoa(w.Icon)),
			WeatherCode: mapWeatherCode(w.Icon),
		}
		mp.WeatherObservation = append(mp.WeatherObservation, obs)
	}

	// Mapping
	mp.Mapping = map[string]map[string]string{
		"discoverswiss": {
			"id": raw.Identifier + ":" + strings.ToLower(suffix),
		},
	}

	// TagIds
	mp.TagIds = []string{"weather", "measuringpoint", "winter"}

	return mp
}

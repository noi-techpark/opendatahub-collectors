// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

import "encoding/json"

// RawData is the top-level message written by the crawler.
type RawData struct {
	De *LangData `json:"de,omitempty"`
	It *LangData `json:"it,omitempty"`
	En *LangData `json:"en,omitempty"`
	Ru *LangData `json:"ru,omitempty"`
}

// LangData holds the flat list of entries for one language.
type LangData struct {
	Data []WineCompany `json:"data"`
}

func (l *LangData) Items() []WineCompany {
	if l == nil {
		return nil
	}
	return l.Data
}

// WineCompany maps one entry from the Statamic API.
type WineCompany struct {
	ID       string `json:"id"`
	Slug     string `json:"slug"`
	Locale   string `json:"locale"`
	Status   string `json:"status"`
	OriginID string `json:"origin_id"`

	LegacyNumber string `json:"legacyNumber"`

	Title    string `json:"title"`
	Business string `json:"business"`
	Hofname  string `json:"hofname"`

	Address  string      `json:"address"`
	ZipCode  string      `json:"zip_code"`
	Location string      `json:"location"`
	Phone    string      `json:"phone"`
	Email    string      `json:"email"`
	Website  string      `json:"website"`
	Logo     interface{} `json:"logo"`

	Latitude  *string `json:"latitude"`
	Longitude *string `json:"longitude"`

	Headline    string `json:"headline"`
	Subtitle    string `json:"subtitle"`
	Intro       string `json:"intro"`
	Slogan      string `json:"slogan"`
	QuoteText   string `json:"quote_text"`
	QuoteAuthor string `json:"quote_author"`

	OpeningHoursWineSales   *string `json:"opening_hours_wine_sales"`
	OpeningHoursCellarTours *string `json:"opening_hours_cellar_tours"`
	OpeningHoursRestaurant  *string `json:"opening_hours_restaurant"`
	Holiday                 *string `json:"holiday"`

	ImageHeader  interface{} `json:"image_header"`
	ImagePreview interface{} `json:"image_preview"`

	Published                bool `json:"published"`
	Catering                 bool `json:"catering"`
	DeliveryService          bool `json:"delivery_service"`
	DirectSales              bool `json:"direct_sales"`
	OnlineShop               bool `json:"online_shop"`
	OrganicWine              bool `json:"organic_wine"`
	OvernightStay            bool `json:"overnight_stay"`
	Sale                     bool `json:"sale"`
	Skyalps                  bool `json:"skyalps"`
	SparklingWineAssociation bool `json:"sparkling_wine_association"`
	VinumHotel               bool `json:"vinum_hotel"`
	WineStories              bool `json:"wine_stories"`
	WineSummit               bool `json:"wine_summit"`
	Winery                   bool `json:"winery"`
	WineryAssociation        bool `json:"winery_association"`
	WineryVisits             bool `json:"winery_visits"`
	Dws                      bool `json:"dws"`

	URLOnlineShop      *string `json:"url_online_shop"`
	URLDeliveryService *string `json:"url_delivery_service"`

	SparklingWineProducerHeadline    *string `json:"sparkling_wine_producer_headline"`
	SparklingWineProducerSubheadline *string `json:"sparkling_wine_producer_subheadline"`
	SparklingWineProducerText        *string `json:"sparkling_wine_producer_text"`

	Facebook  *string `json:"facebook"`
	Instagram *string `json:"instagram"`
	LinkedIn  *string `json:"linkedin"`
	Pinterest *string `json:"pinterest"`
	TikTok    *string `json:"tiktok"`
	Twitter   *string `json:"twitter"`
	YouTube   *string `json:"youtube"`

	// Importers uses a custom unmarshaler — the new API returns null, a plain
	// string, a single object, or an array depending on the entry.
	Importers *ImportersWrapper `json:"importers,omitempty"`
}

// ImportersWrapper handles every shape the API returns for the importers field:
//   - null            → zero items
//   - ""  (string)    → zero items (legacy text blob, ignored)
//   - {...} (object)  → one importer
//   - [{...}] (array) → N importers
type ImportersWrapper struct {
	items []Importer
}

func (iw *ImportersWrapper) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		// plain string — not structured data, ignore
		return nil
	}
	if b[0] == '[' {
		return json.Unmarshal(b, &iw.items)
	}
	if b[0] == '{' {
		var single Importer
		if err := json.Unmarshal(b, &single); err != nil {
			return err
		}
		iw.items = []Importer{single}
		return nil
	}
	return nil
}

func (iw *ImportersWrapper) Importers() []Importer {
	if iw == nil {
		return nil
	}
	return iw.items
}

type Importer struct {
	ImporterName          string `json:"importername"`
	ImporterAddress       string `json:"importeraddress"`
	ImporterZipCode       string `json:"importerzipcode"`
	ImporterPlace         string `json:"importerplace"`
	ImporterPhone         string `json:"importerphone"`
	ImporterEmail         string `json:"importeremail"`
	ImporterHomepage      string `json:"importerhomepage"`
	ImporterContactPerson string `json:"importercontactperson"`
	ImporterDescription   string `json:"importerdescription"`
}

// AssetURL extracts a plain URL string from a Statamic asset field, which may
// be returned as null, a plain string, or an object with a "url" key.
func AssetURL(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case map[string]interface{}:
		if u, ok := val["url"].(string); ok {
			return u
		}
	}
	return ""
}

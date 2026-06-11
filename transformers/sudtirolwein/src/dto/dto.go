// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

import "encoding/json"

// RawData represents the wine company data as received from the api-crawler.
// jp/us removed — the new interface no longer provides them.
type RawData struct {
	De *LangData `json:"de,omitempty"`
	It *LangData `json:"it,omitempty"`
	En *LangData `json:"en,omitempty"`
	Ru *LangData `json:"ru,omitempty"`
}

// LangData wraps the entries list for a single language as returned by
// /api/collections/winzers/entries?filter[site]=<lang>.
type LangData struct {
	Data []WineCompany `json:"data"`
}

// Items returns the company list for this language.
func (l *LangData) Items() []WineCompany {
	if l == nil {
		return nil
	}
	return l.Data
}

// WineCompany maps to the fields returned by the new Statamic-based
// SüdtirolWein API (/api/collections/winzers/entries).
//
// Field-name mapping vs the old XML-based API (kept as comments for
// migration traceability):
//
//	old field                 -> new field
//	id                        -> id (still present, but slug is now the stable key)
//	latidude (typo)           -> latitude
//	longitude                 -> longitude
//	companydescription        -> intro
//	quote                     -> quote_text
//	quoteauthor               -> quote_author
//	h1                        -> headline
//	h2                        -> subtitle
//	homepage                  -> website
//	place                     -> location
//	zipcode                   -> zip_code
//	farmname                  -> hofname
//	media / mediadetail       -> image_header / image_preview (asset objects)
//	active                    -> published (now bool)
//	hasvisits                 -> winery_visits (bool)
//	hasovernights             -> overnight_stay (bool)
//	hasbiowine                -> organic_wine (bool)
//	hasaccomodation           -> catering (bool)
//	hasonlineshop             -> online_shop (bool)
//	hasdeliveryservice        -> delivery_service (bool)
//	hasdirectsales            -> direct_sales (bool)
//	isvinumhotel              -> vinum_hotel (bool)
//	iswinestories             -> wine_stories (bool)
//	iswinesummit              -> wine_summit (bool)
//	issparklingwineassociation-> sparkling_wine_association (bool)
//	iswinery                  -> winery (bool)
//	iswineryassociation       -> winery_association (bool)
//	isskyalpspartner          -> skyalps (bool)
//	onlineshopurl             -> url_online_shop
//	deliveryserviceurl        -> url_delivery_service
//	openingtimeswineshop      -> opening_hours_wine_sales
//	openingtimesguides        -> opening_hours_cellar_tours
//	openingtimesgastronomie   -> opening_hours_restaurant
//	companyholiday            -> holiday
//	socials*                  -> facebook/instagram/linkedin/pinterest/tiktok/youtube/twitter
//	importers                 -> importers (now null | string | object | array)
//
// Removed (no equivalent in new API): region, isanteprima, hassale, wines,
// wineids, sort, imagemetatitle/description/alt, sparklingwineproducer image.
type WineCompany struct {
	ID     string `json:"id"`
	Slug   string `json:"slug"`
	Locale string `json:"locale"`
	Status string `json:"status"`

	// LegacyNumber is the old numeric ID from the consisto.net API,
	// used to match against pre-existing ODH records during migration.
	LegacyNumber string `json:"legacyNumber"`

	Latitude  *string `json:"latitude"`
	Longitude *string `json:"longitude"`

	Title    string `json:"title"`
	FarmName string `json:"hofname"`
	Address  string `json:"address"`
	Place    string `json:"location"`
	ZipCode  string `json:"zip_code"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Homepage string `json:"website"`

	// Logo can be null, a string, or an asset object — handled via AssetURL.
	Logo interface{} `json:"logo"`

	CompanyDescription string `json:"intro"`
	Slogan             string `json:"slogan"`
	Subtitle           string `json:"subtitle"`
	Quote              string `json:"quote_text"`
	QuoteAuthor        string `json:"quote_author"`
	H1                 string `json:"headline"`

	// Media — Statamic asset fields, can be null, string, or object.
	Media       interface{} `json:"image_header"`
	MediaDetail interface{} `json:"image_preview"`

	Active                     bool `json:"published"`
	HasVisits                  bool `json:"winery_visits"`
	HasOvernights              bool `json:"overnight_stay"`
	HasBioWine                 bool `json:"organic_wine"`
	HasAccomodation            bool `json:"catering"`
	HasOnlineShop              bool `json:"online_shop"`
	HasDeliveryService         bool `json:"delivery_service"`
	HasDirectSales             bool `json:"direct_sales"`
	IsVinumHotel               bool `json:"vinum_hotel"`
	IsWineStories              bool `json:"wine_stories"`
	IsWineSummit               bool `json:"wine_summit"`
	IsSparklingWineAssociation bool `json:"sparkling_wine_association"`
	IsWinery                   bool `json:"winery"`
	IsWineryAssociation        bool `json:"winery_association"`
	IsSkyAlpsPartner           bool `json:"skyalps"`

	OnlineShopURL      *string `json:"url_online_shop"`
	DeliveryServiceURL *string `json:"url_delivery_service"`

	OpeningTimesWineShop   *string `json:"opening_hours_wine_sales"`
	OpeningTimesGuides     *string `json:"opening_hours_cellar_tours"`
	OpeningTimesGastronomy *string `json:"opening_hours_restaurant"`
	CompanyHoliday         *string `json:"holiday"`

	DescriptionSparklingWineProducer *string `json:"sparkling_wine_producer_text"`
	H1SparklingWineProducer          *string `json:"sparkling_wine_producer_headline"`
	H2SparklingWineProducer          *string `json:"sparkling_wine_producer_subheadline"`

	SocialsInstagram *string `json:"instagram"`
	SocialsFacebook  *string `json:"facebook"`
	SocialsLinkedIn  *string `json:"linkedin"`
	SocialsPinterest *string `json:"pinterest"`
	SocialsTikTok    *string `json:"tiktok"`
	SocialsYouTube   *string `json:"youtube"`
	SocialsTwitter   *string `json:"twitter"`

	// Importers — new API can return null, a string, an object, or an array.
	Importers *ImportersWrapper `json:"importers,omitempty"`
}

// ImportersWrapper handles every shape the new API returns for "importers":
// null, plain string (ignored), single object, or array of objects.
type ImportersWrapper struct {
	items []Importer
}

func (iw *ImportersWrapper) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		// plain string — not structured importer data, ignore
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
// be null, a plain string, or an object containing a "url" key.
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

// PtrString safely dereferences a *string, returning "" for nil.
func PtrString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

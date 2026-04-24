// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dto

import "encoding/json"

// RawData represents the wine company data as received from the api-crawler.
type RawData struct {
	De *LangData `json:"de,omitempty"`
	It *LangData `json:"it,omitempty"`
	En *LangData `json:"en,omitempty"`
	Ru *LangData `json:"ru,omitempty"`
	Jp *LangData `json:"jp,omitempty"`
	Us *LangData `json:"us,omitempty"`
}

// LangData wraps the companies object for a single language.
type LangData struct {
	Companies CompaniesWrapper `json:"companies"`
}

// CompaniesWrapper holds the item list. Item is json.RawMessage so we can
// handle both a single object and an array (XML-to-JSON quirk).
type CompaniesWrapper struct {
	RawItem json.RawMessage `json:"item"`
}

// Items decodes the raw item field into a slice, handling both object and array.
func (c *CompaniesWrapper) Items() []WineCompany {
	if c.RawItem == nil || string(c.RawItem) == "null" {
		return nil
	}
	var arr []WineCompany
	if err := json.Unmarshal(c.RawItem, &arr); err == nil {
		return arr
	}
	var single WineCompany
	if err := json.Unmarshal(c.RawItem, &single); err == nil {
		return []WineCompany{single}
	}
	return nil
}

// WineCompany maps to the XML fields from the SüdtirolWein API.
type WineCompany struct {
	ID        string `json:"id"`
	Latitude  string `json:"latidude"` // upstream typo preserved
	Longitude string `json:"longitude"`

	Title    string `json:"title"`
	FarmName string `json:"farmname"`
	Address  string `json:"address"`
	Place    string `json:"place"`
	ZipCode  string `json:"zipcode"`
	Region   string `json:"region"`
	Phone    string `json:"phone"`
	Email    string `json:"email"`
	Homepage string `json:"homepage"`
	Logo     string `json:"logo"`

	CompanyDescription string `json:"companydescription"`
	Slogan             string `json:"slogan"`
	Subtitle           string `json:"subtitle"`
	Quote              string `json:"quote"`
	QuoteAuthor        string `json:"quoteauthor"`
	H1                 string `json:"h1"`
	H2                 string `json:"h2"`

	Media                string `json:"media"`
	MediaDetail          string `json:"mediadetail"`
	ImageMetaTitle       string `json:"imagemetatitle"`
	ImageMetaDescription string `json:"imagemetadescription"`
	ImageMetaAlt         string `json:"imagemetaalt"`

	Active                     string `json:"active"`
	HasVisits                  string `json:"hasvisits"`
	HasOvernights              string `json:"hasovernights"`
	HasBioWine                 string `json:"hasbiowine"`
	HasAccomodation            string `json:"hasaccomodation"`
	HasOnlineShop              string `json:"hasonlineshop"`
	HasDeliveryService         string `json:"hasdeliveryservice"`
	HasDirectSales             string `json:"hasdirectsales"`
	HasSale                    string `json:"hassale"`
	IsVinumHotel               string `json:"isvinumhotel"`
	IsAnteprima                string `json:"isanteprima"`
	IsWineStories              string `json:"iswinestories"`
	IsWineSummit               string `json:"iswinesummit"`
	IsSparklingWineAssociation string `json:"issparklingwineassociation"`
	IsWinery                   string `json:"iswinery"`
	IsWineryAssociation        string `json:"iswineryassociation"`
	IsSkyAlpsPartner           string `json:"isskyalpspartner"`

	OnlineShopURL      string `json:"onlineshopurl"`
	DeliveryServiceURL string `json:"deliveryserviceurl"`

	OpeningTimesWineShop   string `json:"openingtimeswineshop"`
	OpeningTimesGuides     string `json:"openingtimesguides"`
	OpeningTimesGastronomy string `json:"openingtimesgastronomie"`
	CompanyHoliday         string `json:"companyholiday"`

	DescriptionSparklingWineProducer string `json:"descriptionsparklingwineproducer"`
	H1SparklingWineProducer          string `json:"h1sparklingwineproducer"`
	H2SparklingWineProducer          string `json:"h2sparklingwineproducer"`
	ImageSparklingWineProducer       string `json:"imagesparklingwineproducer"`

	Wines   string `json:"wines"`
	WineIDs string `json:"wineids"`
	Sort    string `json:"sort"`

	SocialsInstagram string `json:"socialsinstagram"`
	SocialsFacebook  string `json:"socialsfacebook"`
	SocialsLinkedIn  string `json:"socialslinkedIn"`
	SocialsPinterest string `json:"socialspinterest"`
	SocialsTikTok    string `json:"socialstiktok"`
	SocialsYouTube   string `json:"socialsyoutube"`
	SocialsTwitter   string `json:"socialstwitter"`

	Importers *ImportersWrapper `json:"importers,omitempty"`
}

type ImportersWrapper struct {
	RawImporter json.RawMessage `json:"importer"`
}

func (iw *ImportersWrapper) Importers() []Importer {
	if iw == nil || iw.RawImporter == nil || string(iw.RawImporter) == "null" {
		return nil
	}
	var arr []Importer
	if err := json.Unmarshal(iw.RawImporter, &arr); err == nil {
		return arr
	}
	var single Importer
	if err := json.Unmarshal(iw.RawImporter, &single); err == nil {
		return []Importer{single}
	}
	return nil
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

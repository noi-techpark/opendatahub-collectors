// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package odhContentModel

type discoverswissId struct {
	Id string `json:"id"`
}

type Accommodation struct {
	Source    string `default:"discoverswiss"`
	Active    bool   `default:"true"`
	Shortname string

	Mapping struct {
		DiscoverSwiss discoverswissId `json:"discoverswiss"`
	} `json:"Mapping"`

	AccoDetail struct {
		LanguageDe AccoDetailLanguage `json:"de"`
		LanguageEn AccoDetailLanguage `json:"en"`
		LanguageIt AccoDetailLanguage `json:"it"`
	} `json:"AccoDetail"`

	GpsInfo []struct {
		Gpstype               string  `json:"Gpstype"`
		Latitude              float64 `json:"Latitude"`
		Longitude             float64 `json:"Longitude"`
		Altitude              float64 `json:"Altitude"`
		AltitudeUnitofMeasure string  `json:"AltitudeUnitofMeasure"`
	} `json:"GpsInfo"`

	PublishedOn []string `json:"PublishedOn"`

	AccoTypeId string `json:"AccoTypeId"`

	AccoCategoryId string `json:"AccoCategoryId"`

	AccoOverview struct {
		TotalRooms   *int   `json:"TotalRooms"`
		SingleRooms  *int   `json:"SingleRooms"`
		DoubleRooms  *int   `json:"DoubleRooms"`
		TripleRooms  *int   `json:"TripleRooms"`
		CheckInFrom  string `json:"CheckInFrom"`
		CheckInTo    string `json:"CheckInTo"`
		CheckOutFrom string `json:"CheckOutFrom"`
		CheckOutTo   string `json:"CheckOutTo"`
		MaxPersons   int    `json:"MaxPersons"`
	} `json:"AccoOverview"`

	HasLanguage []string `json:"HasLanguage"`

	LicenseInfo struct {
		Author        string `json:"Author"`
		License       string `json:"License"`
		ClosedData    bool   `json:"ClosedData"`
		LicenseHolder string `json:"LicenseHolder"`
	} `json:"LicenseInfo"`

	LocationInfo struct {
		RegionInfo       Location `json:"RegionInfo"`
		MunicipalityInfo Location `json:"MunicipalityInfo"`
	} `json:"LocationInfo"`

	ImageGallery []ImageGalleryItem `json:"ImageGallery"`
}

type ImageGalleryItem struct {
	ImageUrl    string      `json:"ImageUrl"`              // From ContentUrl
	CopyRight   string      `json:"CopyRight"`             // From CopyrightNotice
	ImageDesc   LanguageMap `json:"ImageDesc"`             // From Name
	ImageName   *string     `json:"ImageName,omitempty"`   // From Identifier
	ImageSource *string     `json:"ImageSource,omitempty"` // From DataGovernance.Source.Name
}

type LanguageMap struct {
	DE string `json:"de,omitempty"`
	EN string `json:"en,omitempty"`
	IT string `json:"it,omitempty"`
	FR string `json:"fr,omitempty"`
}

type AccoDetailLanguage struct {
	Fax         string `json:"Fax"`
	Name        string `json:"Name"`
	Street      string `json:"Street"`
	Zip         string `json:"Zip"`
	City        string `json:"City"`
	CountryCode string `json:"CountryCode"`
	Email       string `json:"Email"`
	Phone       string `json:"Phone"`
}

type Location struct {
	Id   string `json:"Id"`
	Name Name   `json:"Name"`
}

type Name struct {
	De string `json:"de"`
	En string `json:"en"`
	It string `json:"it"`
	Fr string `json:"fr"`
}

type DiscoverSwissResponse struct {
	Count         int               `json:"count"`
	HasNextPage   bool              `json:"hasNextPage"`
	NextPageToken string            `json:"nextPageToken"`
	Data          []LodgingBusiness `json:"data"`
}
type LodgingBusiness struct {
	Name string `json:"name"`

	Address struct {
		AddressCountry  string `json:"addressCountry"`
		AddressLocality string `json:"addressLocality"`
		AddressRegion   string `json:"addressRegion"`
		PostalCode      string `json:"postalCode"`
		StreetAddress   string `json:"streetAddress"`
		Email           string `json:"email"`
		Telephone       string `json:"telephone"`
	} `json:"address"`

	FaxNumber string `json:"faxNumber"`

	Geo struct {
		Elevation float64 `json:"elevation"`
		Latitude  float64 `json:"latitude"`
		Longitude float64 `json:"longitude"`
	} `json:"geo"`

	NumberOfRooms []struct {
		PropertyID string `json:"propertyId"`
		Value      string `json:"value"`
	} `json:"numberOfRooms"`

	StarRating StarRating `json:"starRating"`

	NumberOfBeds int `json:"numberOfBeds"`

	Identifier string `json:"identifier"`

	CheckinTime      string `json:"checkinTime"`
	CheckinTimeTo    string `json:"checkinTimeTo"`
	CheckoutTimeFrom string `json:"checkoutTimeFrom"`
	CheckoutTime     string `json:"checkoutTime"`

	License string `json:"license"`

	Photo []Photo `json:"photo"`

	AdditionalType string `json:"additionalType"`

	DataGovernance DataGovernance `json:"dataGovernance"`
}

type Photo struct {
	ContentUrl      string               `json:"contentUrl"`      // Maps to ImageUrl
	CopyrightNotice string               `json:"copyrightNotice"` // Maps to CopyRight
	DataGovernance  DataGovernanceImages `json:"dataGovernance"`  // For extracting ImageSource
	Identifier      string               `json:"identifier"`      // Could map to ImageName
	Name            string               `json:"name"`            // Could map to ImageDesc
}

type DataGovernanceImages struct {
	Source Source `json:"source"`
}

type Source struct {
	Name string `json:"name"` // Maps to ImageSource
}

type DataGovernance struct {
	Provider Provider `json:"provider"`
}

type Provider struct {
	Link []Link `json:"link"`
}

type Link struct {
	Url string `json:"url"`
}

type StarRating struct {
	RatingValue float64 `json:"ratingValue"`
}

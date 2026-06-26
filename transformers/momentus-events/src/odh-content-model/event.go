package odhmodel

type EventLinked struct {
	Active                bool                              `json:"Active"`
	AdditionalProperties  map[string]any                    `json:"AdditionalProperties"`
	Altitude              float64                           `json:"Altitude,omitempty"`
	AltitudeUnitofMeasure string                            `json:"AltitudeUnitofMeasure,omitempty"`
	ClassificationRID     string                            `json:"ClassificationRID,omitempty"`
	ContactInfos          map[string]ContactInfos           `json:"ContactInfos,omitempty"`
	DateBegin             string                            `json:"DateBegin,omitempty"`
	DateBeginUTC          int64                             `json:"DateBeginUTC,omitempty"`
	DateEnd               string                            `json:"DateEnd,omitempty"`
	DateEndUTC            int64                             `json:"DateEndUTC,omitempty"`
	Detail                map[string]Detail                 `json:"Detail,omitempty"`
	DistanceInfo          map[string]any                    `json:"DistanceInfo"`
	DistrictId            string                            `json:"DistrictId,omitempty"`
	DistrictIds           []string                          `json:"DistrictIds"`
	Districts             []string                          `json:"Districts"`
	Documents             []map[string]any                  `json:"Documents"`
	EventAdditionalInfos  map[string]any                    `json:"EventAdditionalInfos"`
	EventBooking          map[string]any                    `json:"EventBooking"`
	EventDate             []EventDate                       `json:"EventDate,omitempty"`
	EventDateCounter      int                               `json:"EventDateCounter,omitempty"`
	EventDatesBegin       []string                          `json:"EventDatesBegin"`
	EventDatesEnd         []string                          `json:"EventDatesEnd"`
	EventLanguages        []string                          `json:"EventLanguages"`
	EventPrice            map[string]any                    `json:"EventPrice"`
	EventProperty         map[string]any                    `json:"EventProperty"`
	EventPublisher        map[string]any                    `json:"EventPublisher"`
	EventUrls             []EventUrl                        `json:"EventUrls,omitempty"`
	EventVariants         []map[string]any                  `json:"EventVariants"`
	FirstImport           string                            `json:"FirstImport,omitempty"`
	GpsInfo               []GpsInfo                         `json:"GpsInfo"`
	GpsPoints             map[string]any                    `json:"GpsPoints"`
	Gpstype               string                            `json:"Gpstype,omitempty"`
	HasLanguage           []string                          `json:"HasLanguage"`
	Id                    string                            `json:"Id"`
	ImageGallery          []ImageGalleryItem                `json:"ImageGallery"`
	LastChange            string                            `json:"LastChange,omitempty"`
	Latitude              float64                           `json:"Latitude,omitempty"`
	LicenseInfo           map[string]any                    `json:"LicenseInfo"`
	LocationInfo          map[string]any                    `json:"LocationInfo"`
	Longitude             float64                           `json:"Longitude,omitempty"`
	Mapping               map[string]map[string]string      `json:"Mapping,omitempty"`
	OdhActive             bool                              `json:"OdhActive,omitempty"`
	ODHTags               []map[string]any                  `json:"ODHTags"`
	OrganizerInfos        map[string]ContactInfos           `json:"OrganizerInfos,omitempty"`
	OrgRID                string                            `json:"OrgRID,omitempty"`
	PublishedOn           []string                          `json:"PublishedOn"`
	RelatedContent        []map[string]any                  `json:"RelatedContent"`
	Shortname             string                            `json:"Shortname,omitempty"`
	SignOn                string                            `json:"SignOn,omitempty"`
	SmgActive             bool                              `json:"SmgActive,omitempty"`
	SmgTags               []string                          `json:"SmgTags"`
	Source                string                            `json:"Source,omitempty"`
	TagIds                []string                          `json:"TagIds"`
	Ticket                string                            `json:"Ticket,omitempty"`
	TopicRIDs             []string                          `json:"TopicRIDs"`
	Topics                []string                          `json:"Topics"`
	VenueIds              []string                          `json:"VenueIds"`
	VenueLink             []map[string]any                  `json:"VenueLink"`
	VideoItems            []map[string]any                  `json:"VideoItems"`
}

type Detail struct {
	BaseText  string `json:"BaseText,omitempty"`
	Language  string `json:"Language,omitempty"`
	Title     string `json:"Title,omitempty"`
	SubHeader string `json:"SubHeader,omitempty"`
}

type EventDate struct {
	Active              bool     `json:"Active"`
	Begin               string   `json:"Begin,omitempty"`
	End                 string   `json:"End,omitempty"`
	From                string   `json:"From,omitempty"`
	To                  string   `json:"To,omitempty"`
	VenueRoomDetailsIds []string `json:"VenueRoomDetailsIds,omitempty"`
}

type EventUrl struct {
	Active bool              `json:"Active"`
	Type   string            `json:"Type,omitempty"`
	Url    map[string]string `json:"Url,omitempty"`
}

type ContactInfos struct {
	Address     string `json:"Address,omitempty"`
	City        string `json:"City,omitempty"`
	CompanyName string `json:"CompanyName,omitempty"`
	CountryName string `json:"CountryName,omitempty"`
	Email       string `json:"Email,omitempty"`
	Givenname   string `json:"Givenname,omitempty"`
	Language    string `json:"Language,omitempty"`
	Phonenumber string `json:"Phonenumber,omitempty"`
	Surname     string `json:"Surname,omitempty"`
	ZipCode     string `json:"ZipCode,omitempty"`
}

type GpsInfo struct {
	Gpstype               string  `json:"Gpstype"`
	Latitude              float64 `json:"Latitude"`
	Longitude             float64 `json:"Longitude"`
	Altitude              float64 `json:"Altitude"`
	AltitudeUnitofMeasure string  `json:"AltitudeUnitofMeasure,omitempty"`
}

type ImageGalleryItem struct {
	ImageName   string `json:"ImageName"`
	ImageUrl    string `json:"ImageUrl"`
	Width       int    `json:"Width"`
	Height      int    `json:"Height"`
	ImageSource string `json:"ImageSource"`
	ImageTitle  map[string]string `json:"ImageTitle"`
	ImageDesc   map[string]string `json:"ImageDesc"`
}

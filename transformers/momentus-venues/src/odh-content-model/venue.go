package odhcontentmodel

type VenueV2 struct {
	Id           string                `json:"Id"`
	RoomDetails  []VenueRoomDetailsV2  `json:"RoomDetails"`
	Mapping      map[string]map[string]string `json:"Mapping,omitempty"`
}

type VenueRoomDetailsV2 struct {
	Id                  string                           `json:"Id,omitempty"`
	Shortname           string                           `json:"Shortname,omitempty"`
	Detail              map[string]DetailGeneric         `json:"Detail,omitempty"`
	Active              bool                             `json:"Active"`
	VenueRoomProperties *VenueRoomProperties             `json:"VenueRoomProperties,omitempty"`
	MaxCapacity         *int                             `json:"MaxCapacity,omitempty"`
	Mapping             map[string]map[string]string     `json:"Mapping,omitempty"`
}

type DetailGeneric struct {
	Title    string `json:"Title"`
	Language string `json:"Language"`
}

type VenueRoomProperties struct {
	SquareMeters *float64 `json:"SquareMeters,omitempty"`
}

// Momentus models we receive from the Crawler
type MomentusRoom struct {
	Id                 string   `json:"id"`
	Name               string   `json:"name"`
	Group              string   `json:"group"`
	IsActive           bool     `json:"isActive"`
	SquareFootage      *float64 `json:"squareFootage"`
	MaxCapacity        *int     `json:"maxCapacity"`
	IsComboRoom        bool     `json:"isComboRoom"`
	SubRoomIds         []string `json:"subRoomIds"`
	ConflictingRoomIds []string `json:"conflictingRoomIds"`
	ItemCode           string   `json:"itemCode"`
}

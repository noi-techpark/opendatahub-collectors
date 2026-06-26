package main

// MomentusEvent represents the raw event from Momentus
type MomentusEvent struct {
	Id            string                `json:"id"`
	Name          string                `json:"name"`
	IsActive      bool                  `json:"isActive"`
	IsCanceled    bool                  `json:"isCanceled"`
	Start         string                `json:"start"`
	End           string                `json:"end"`
	Description   string                `json:"description"`
	Website       string                `json:"website"`
	AccountName   string                `json:"accountName"`
	EventTypeId   string                `json:"eventTypeId"`
	EventTypeName string                `json:"eventTypeName"`
	ExternalIds   []MomentusExternalId  `json:"externalIds"`
	ContactRoles  []MomentusContactRole `json:"contactRoles"`
	BookedSpaces  []MomentusBookedSpace `json:"bookedSpaces"`
}

type MomentusExternalId struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type MomentusContactRole struct {
	Name              string `json:"name"`
	AccountName       string `json:"accountName"`
	Email             string `json:"email"`
	Phone             string `json:"phone"`
	Address1          string `json:"address1"`
	AddressCity       string `json:"addressCity"`
	AddressPostalCode string `json:"addressPostalCode"`
	AddressCountry    string `json:"addressCountry"`
}

type MomentusBookedSpace struct {
	Id             string `json:"id"`
	UsageType      string `json:"usageType"`
	BookedSpaceId  string `json:"bookedSpaceId"`
	RoomId         string `json:"roomId"`
	StartDate      string `json:"startDate"`
	EndDate        string `json:"endDate"`
	StartTime      string `json:"startTime"`
	EndTime        string `json:"endTime"`
	SpaceUsageName string `json:"spaceUsageName"`
}

// MomentusFunction represents a multilingual function title from Momentus
type MomentusFunction struct {
	FunctionTypeName string `json:"functionTypeName"`
	Name             string `json:"name"`
}

// MomentusEventMessage is the wrapper received from the crawler
type MomentusEventMessage struct {
	Event        MomentusEvent         `json:"event"`
	Functions    []MomentusFunction    `json:"functions"`
	BookedSpaces []MomentusBookedSpace `json:"bookedSpaces"`
	Venue        ODHVenue              `json:"venue"`
}

// ODHVenue represents the simplified ODH Venue object
type ODHVenue struct {
	Id          string           `json:"Id"`
	Mapping     ODHVenueMapping  `json:"Mapping"`
	RoomDetails []ODHRoomDetails `json:"RoomDetails"`
}

type ODHVenueMapping struct {
	Tag map[string]string `json:"tag"`
}

type ODHRoomDetails struct {
	Id      string                     `json:"Id"`
	Mapping map[string]map[string]string `json:"Mapping"`
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: MPL-2.0

package main

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/google/uuid"
	dto "opendatahub.com/tr-traffic-event-prov-bz/dto"
	odhContentModel "opendatahub.com/tr-traffic-event-prov-bz/odh-content-model"
)

const (
	// Date/Time Layouts based on the example data
	DateTimeLayout = "2006-01-02T15:04:05"
	DateLayout     = "2006-01-02"
	PROV_BZ_WKT    = "POLYGON ((11.47689681459508 46.3639314013008, 11.557246492130922 46.35094542406965, 11.619359830085568 46.42521512447995, 11.624761688265473 46.471092949453, 11.715831629694499 46.5138472237901, 11.790427262789175 46.514006888773835, 11.811184716862723 46.53276429012237, 11.851350817996217 46.51783639374748, 11.966147470771404 46.54467792156675, 11.998418896176528 46.532855717993584, 12.06479009613956 46.62320174368202, 12.068269659362674 46.67507128575588, 12.19445995882491 46.604908993707504, 12.339704826308855 46.63137789446692, 12.385163313207313 46.62278170863068, 12.44290355407753 46.68812639131431, 12.37791833062784 46.72193052488212, 12.35116980752465 46.77706935919713, 12.30866336671443 46.78481899917115, 12.282430622956197 46.81499165133236, 12.306176436202904 46.83394090697565, 12.26649343517445 46.88714148389743, 12.215033121002348 46.874191038754375, 12.16820533358177 46.93788913589081, 12.131524775170526 46.9641165484173, 12.120988263371181 47.00665357758659, 12.1481506967679 47.024367654475085, 12.204790459886226 47.0278888156892, 12.225591385120214 47.08270624241812, 12.18668072282199 47.09178374646217, 12.020061028533314 47.04676017826602, 11.915327526073641 47.0325484877238, 11.836210497951654 46.99289620496199, 11.78178266658257 46.9920590539057, 11.747168164872113 46.96890272279262, 11.711231915465198 46.993023559218415, 11.664104053923312 46.99262761051312, 11.627210453739467 47.01257735093896, 11.538076938508825 46.98410808654417, 11.479858518248927 47.01099664274638, 11.442090680655232 46.97649994464387, 11.400908519190617 46.96524476417305, 11.358286501771762 46.990361692319404, 11.188908731929839 46.97015750831145, 11.139551429332078 46.927628964493096, 11.115012732548065 46.93100537834624, 11.101546106138908 46.88985971016322, 11.071439677737038 46.85179853802967, 11.083538350277792 46.82286632344014, 11.039798522774964 46.805084235259294, 11.021671831787362 46.76591196023026, 10.943972906206744 46.77513765747408, 10.882208080088317 46.76319332993614, 10.78878439384028 46.79470057937926, 10.763203143203857 46.823486004225025, 10.671446110003957 46.87069620220594, 10.550722648396805 46.84989772432367, 10.480096678376224 46.85871171023735, 10.448274566047061 46.80137095497578, 10.418273791549003 46.71781026084223, 10.387417994112502 46.68728024197745, 10.410754576344385 46.6351400570656, 10.445953812902335 46.64110366641241, 10.48916424288808 46.615017689434175, 10.45794042607491 46.51058961253598, 10.484530696550374 46.493600815227836, 10.551960791889437 46.49145665988791, 10.621836982320522 46.447960643498355, 10.684643595663356 46.45147086601842, 10.764977293405861 46.485957197067485, 10.800380535227276 46.44296614835808, 10.861324055865436 46.43613133397988, 10.911694331460223 46.44374201010712, 10.963782526010165 46.48207134200334, 10.988512298908145 46.48355185509846, 11.041239306833052 46.44723618463331, 11.074690576270728 46.45482334150326, 11.049005955067274 46.507041789795494, 11.129678620462217 46.48162936049757, 11.208304419503278 46.49584339002066, 11.220355842602778 46.462679463917915, 11.205031444901696 46.426818870995646, 11.214187254298217 46.398463136477574, 11.191564945796683 46.359794164116146, 11.203562204232139 46.34237104010292, 11.16287818198736 46.29117069347117, 11.174638171751425 46.23266729683924, 11.206429184384845 46.219772403066244, 11.248945235682529 46.23281736286569, 11.330973132354641 46.29386363976065, 11.358780819600133 46.265667625144474, 11.40425138703762 46.32497403020492, 11.454471643923158 46.334726578876385, 11.47689681459508 46.3639314013008))"
	BZ_LATITUTE    = 46.4981125
	BZ_LONGITUTE   = 11.3547801
)

var (
	ErrWithoutGeometry = errors.New("announcement without geometry")
)

// Helper functions (required for clean mapping)

func StringPtr(s string) *string {
	return &s
}

func IfNotEmpty(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func IntToStringPtr(i int) *string {
	s := strconv.Itoa(i)
	return &s
}

func Float64Ptr(f float64) *float64 {
	return &f
}

func BoolPtr(f bool) *bool {
	return &f
}

// GenericTrafficEvent holds the mapped (generic) type and subtype IDs.
type GenericTrafficEvent struct {
	TypeID    string
	SubtypeID string
}

// ProviderEvent represents the raw data coming from the external API.
type ProviderEvent struct {
	Type    string
	Subtype string
}

func mapEventToGenericType(event ProviderEvent) GenericTrafficEvent {
	// Mapped event structure initialized to empty strings
	mappedEvent := GenericTrafficEvent{}

	// --- 1. Subtype Mapping (High Priority) ---
	// We use a map for common, distinct subtype values.
	subtypeMap := map[string]string{
		"BAUSTELLE":                       "traffic-event:road-work",
		"UNFALL":                          "traffic-event:accident",
		"VERANSTALTUNG":                   "traffic-event:event",
		"TIERE AUF FAHRBAHN":              "traffic-event:animal-on-road",
		"VIEHABTRIEB":                     "traffic-event:animal-on-road",
		"RADARKONTROLLE":                  "traffic-event:speed-camera", // Maps radar subtype to its own *type*
		"\u00d6FFENTLICHE VERKEHRSMITTEL": "traffic-event:public-transport",
		"STREIK":                          "traffic-event:public-transport",
		"EISENBAHN":                       "traffic-event:restriction",
		"AMPELREGELUNG":                   "traffic-event:restriction",
		"SPRENGSATZ ":                     "traffic-event:restriction",
		"BESCHRÄNKUNG":                    "traffic-event:restriction",
		"FAHRVERBOT":                      "traffic-event:prohibition",
		"LKW FAHRVERBOT":                  "traffic-event:prohibition",
		"VORSICHT":                        "traffic-event:caution",
		"STEINSCHLAG":                     "traffic-event:caution",
		"NEBELBÄNKE":                      "traffic-event:weather-related",
		"SCHNEEFALL":                      "traffic-event:weather-related",
		"KETTENPFLICHT":                   "traffic-event:road-condition",
		"WINTERAUSRÜSTUNG":                "traffic-event:road-condition",
		"FREI BEFAHRBAR":                  "traffic-event:road-condition",
		"STAU":                            "traffic-event:congestion",
		"Kolonnenverkehr":                 "traffic-event:congestion",
		"SPERRE":                          "traffic-event:closure",
		"RADWEG_SPERRE":                   "traffic-event:closure",
		"WINTERSPERRE":                    "traffic-event:closure",
		"SICHERHEITSGRÜNDE":               "traffic-event:closure",
		"REINIGUNGSARBEITEN":              "traffic-event:maintenance",
		"\u00d6LSPUR":                     "traffic-event:maintenance",
	}

	if genericSubtype, ok := subtypeMap[event.Subtype]; ok {
		// Note: The speed camera subtype is intentionally mapped to a *type* ID
		if genericSubtype != "traffic-event:speed-camera" {
			mappedEvent.SubtypeID = genericSubtype
		}
	} else if event.Subtype == "" || event.Subtype == "VORÜBERGEHEND " {
		// Handle empty/miscellaneous subtypes; often these rely entirely on the Type.
		mappedEvent.SubtypeID = ""
	} else {
		// Fallback for any unmapped subtype to a general restriction
		mappedEvent.SubtypeID = "traffic-event:restriction"
	}

	// --- 2. Type Mapping ---
	switch event.Type {
	case "BEHIND":
		mappedEvent.TypeID = "traffic-event:hindrance"
	case "AKTUELLES":
		// If the type is 'Aktuelles' and the subtype is speed camera, prioritize the speed camera type.
		if event.Subtype == "RADARKONTROLLE" {
			mappedEvent.TypeID = "traffic-event:speed-camera"
		} else {
			mappedEvent.TypeID = "traffic-event:current"
		}
	case "SONDER":
		mappedEvent.TypeID = "traffic-event:special"
	case "PÄSSE":
		mappedEvent.TypeID = "traffic-event:mountain-pass"
	case "RADAR":
		// The provider has a 'RADAR' type, but the 'AKTUELLES' subtype is more common (314 vs 302).
		// We use the common generic ID for both.
		mappedEvent.TypeID = "traffic-event:speed-camera"
	default:
		// Fallback for any unknown provider type
		mappedEvent.TypeID = "traffic-event:hindrance"
	}

	return mappedEvent
}

var MessageNamespace = uuid.MustParse("d5697669-e0d5-4521-995a-c5c83f90117a")

func generateDeterministicUUID(messageID int) uuid.UUID {
	// Convert the integer to a string to use as the name
	name := fmt.Sprintf("%d", messageID)
	// Use UUIDv5 (SHA-1) for a deterministic, non-random UUID
	return uuid.NewSHA1(MessageNamespace, []byte(name))
}

func generateID(raw dto.TrafficEvent) string {
	return fmt.Sprintf("%s:%s", ID_TEMPLATE, generateDeterministicUUID(raw.MessageID).String())
}

func parseAndConvertToUTC(layout, rawDateTime string) (time.Time, error) {
	localTime, err := time.ParseInLocation(layout, rawDateTime, location)
	if err != nil {
		return time.Time{}, fmt.Errorf("error parsing time: %w", err)
	}

	return localTime.In(time.UTC), nil
}

// MapTrafficEventToAnnouncement converts the raw TrafficEvent to the Announcement struct.
func MapTrafficEventToAnnouncement(tags Tags, raw dto.TrafficEvent, id string) (odhContentModel.Announcement, error) {
	announcement := odhContentModel.Announcement{
		Generic: odhContentModel.Generic{
			Active: true,
			Source: StringPtr(SOURCE),
			LicenseInfo: &odhContentModel.LicenseInfo{
				ClosedData: false,
				License:    StringPtr("CC0"),
			},
			Geo: map[string]odhContentModel.GpsInfo{},
		},
	}

	// ID and Metadata
	messageIDStr := fmt.Sprintf("%d", raw.MessageID)
	announcement.ID = StringPtr(id)
	announcement.Mapping.ProviderProvinceBz.Id = messageIDStr

	// StartTime and EndTime
	// For start time we check if the publishing time is the same day of start time, if so
	// we use publishing which has more granularity, otherwise we have to fallback to startTime
	publishTime, err := parseAndConvertToUTC(DateTimeLayout, raw.PublishDateTime)
	if err != nil {
		return odhContentModel.Announcement{}, fmt.Errorf("failed to parse PublishDateTime: %w", err)
	}

	startTime, err := parseAndConvertToUTC(DateLayout, raw.BeginDate)
	if err != nil {
		return odhContentModel.Announcement{}, fmt.Errorf("failed to parse BeginDate: %w", err)
	}

	if publishTime.YearDay() == startTime.YearDay() {
		announcement.StartTime = &publishTime
	} else {
		announcement.StartTime = &startTime
	}

	// try get planned endtime
	if raw.EndDate != "" {
		endTime, err := time.Parse(DateLayout, raw.EndDate)
		if err != nil {
		} else {
			announcement.EndTime = &endTime
		}
	}

	// Geo Info (Position)
	hasPosition := false
	if raw.X != nil && raw.Y != nil {
		// 1. Attempt type assertion to float64 for X
		xFloat, xIsFloat := raw.X.(float64)

		// 2. Attempt type assertion to float64 for Y
		yFloat, yIsFloat := raw.Y.(float64)

		if xIsFloat && yIsFloat {
			// 4. Safely check for non-zero values using the asserted float values
			if xFloat != 0 && yFloat != 0 {
				announcement.Geo["position"] = odhContentModel.GpsInfo{
					Longitude: Float64Ptr(xFloat),
					Latitude:  Float64Ptr(yFloat),
					Default:   true,
					Geometry:  StringPtr(fmt.Sprintf("POINT (%f %f)", xFloat, yFloat)),
				}
				hasPosition = true
			}
		}
	}

	if !hasPosition {
		announcement.Geo["area"] = odhContentModel.GpsInfo{
			Default:  true,
			Geometry: StringPtr(PROV_BZ_WKT),
		}
		announcement.Geo["position"] = odhContentModel.GpsInfo{
			Latitude:  Float64Ptr(BZ_LATITUTE),
			Longitude: Float64Ptr(BZ_LONGITUTE),
			Default:   false,
			Geometry:  StringPtr(fmt.Sprintf("POINT (%f %f)", BZ_LATITUTE, BZ_LONGITUTE)),
		}
	}

	// Tags (Type and Subtype)
	mappedTags := mapEventToGenericType(ProviderEvent{raw.TycodeValue, raw.SubTycodeValue})

	// Rule: type and subtype should be handled via tags
	announcement.TagIds = []string{
		"announcement:traffic-event",
		mappedTags.TypeID,
		mappedTags.SubtypeID,
	}

	typeTag := tags.FindById(mappedTags.TypeID)
	subtypeTag := tags.FindById(mappedTags.SubtypeID)

	// Details (Descriptions)
	if nil != subtypeTag {
		announcement.Shortname = StringPtr(fmt.Sprintf("%s %s", typeTag.NameEn, subtypeTag.NameEn))
		announcement.Detail = map[string]*odhContentModel.DetailGeneric{
			"de": {
				Title:    StringPtr(fmt.Sprintf("%s %s", typeTag.NameDe, subtypeTag.NameDe)),
				BaseText: IfNotEmpty(raw.PlaceDe),
			},
			"it": {
				Title:    StringPtr(fmt.Sprintf("%s %s", typeTag.NameIt, subtypeTag.NameIt)),
				BaseText: IfNotEmpty(raw.PlaceIt),
			},
		}
	} else {
		announcement.Shortname = StringPtr(typeTag.NameEn)
		announcement.Detail = map[string]*odhContentModel.DetailGeneric{
			"de": {
				Title:    StringPtr(typeTag.NameDe),
				BaseText: IfNotEmpty(raw.PlaceDe),
			},
			"it": {
				Title:    StringPtr(typeTag.NameIt),
				BaseText: IfNotEmpty(raw.PlaceIt),
			},
		}
	}
	announcement.HasLanguage = []string{"it", "de"}

	// AdditionalProperties
	// announcement.AdditionalProperties.RoadIncidentProperties = odhContentModel.RoadIncidentProperties{
	// 	RoadsInvolved: []odhContentModel.RoadInvolved{
	// 		{
	// 			Code: StringPtr(raw.MessageStreetNr),
	// 		},
	// 	},
	// }

	return announcement, nil
}

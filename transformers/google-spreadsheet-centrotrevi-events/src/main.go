// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	odhmodel "opendatahub.com/google-spreadsheet-centrotrevi/odh-content-model"
)

var nowFunc = time.Now

var env struct {
	tr.Env
	OdhCoreUrl               string `envconfig:"ODH_CORE_URL" required:"true"`
	OdhCoreTokenUrl          string `envconfig:"ODH_CORE_TOKEN_URL"`
	OdhCoreTokenClientId     string `envconfig:"ODH_CORE_TOKEN_CLIENT_ID"`
	OdhCoreTokenClientSecret string `envconfig:"ODH_CORE_TOKEN_CLIENT_SECRET"`
}

type Spreadsheet struct {
	SpreadsheetID string  `json:"spreadsheetId"`
	Sheets        []Sheet `json:"sheets"`
}

type Sheet struct {
	Properties SheetProperties `json:"properties"`
	Data       []GridData      `json:"data"`
}

type SheetProperties struct {
	Title string `json:"title"`
	Index int    `json:"index"`
}

type GridData struct {
	RowData []RowData `json:"rowData"`
}

type RowData struct {
	Values []CellData `json:"values"`
}

type CellData struct {
	FormattedValue string `json:"formattedValue"`
}

type placeData struct {
	ID        string
	Names     map[string]string
	Addresses map[string]string
	Cities    map[string]string
	Email     string
	Phone     string
	ZipCode   string
	Province  string
	Lat       float64
	Lon       float64
}

type roomData struct {
	ID        string
	Names     map[string]string
	Addresses map[string]string
	Cities    map[string]string
	ZipCode   string
	MaxSeats  int
	PlaceID   string
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Google Spreadsheet CentroTrevi/Drin Transformer...")
	defer tel.FlushOnPanic()

	contentClient, err := clib.NewContentClient(clib.Config{
		BaseURL:      env.OdhCoreUrl,
		TokenURL:     env.OdhCoreTokenUrl,
		ClientID:     env.OdhCoreTokenClientId,
		ClientSecret: env.OdhCoreTokenClientSecret,
		DisableOAuth: env.OdhCoreTokenUrl == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create content client")

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		if r.Rawdata == "" {
			slog.Debug("Empty payload, skipping")
			return nil
		}

		var spreadsheet Spreadsheet
		if err := json.Unmarshal([]byte(r.Rawdata), &spreadsheet); err != nil {
			slog.Error("Failed to unmarshal spreadsheet JSON", "err", err)
			return err
		}

		return processSpreadsheet(ctx, contentClient, spreadsheet)
	})

	if err != nil {
		slog.Error("Error while listening to queue", "err", err)
		os.Exit(1)
	}
}

func sheetToSlice(sheet Sheet) [][]string {
	var rows [][]string
	for _, gridData := range sheet.Data {
		for _, rowData := range gridData.RowData {
			var row []string
			isEmpty := true
			for _, cellData := range rowData.Values {
				val := cellData.FormattedValue
				if val != "" {
					isEmpty = false
				}
				row = append(row, val)
			}
			if !isEmpty {
				rows = append(rows, row)
			}
		}
	}
	return rows
}

func getHeaderMap(row []string) map[string]int {
	m := make(map[string]int)
	for i, cell := range row {
		clean := strings.ToLower(strings.TrimSpace(cell))
		if clean != "" {
			m[clean] = i
		}
	}
	return m
}

func getValue(row []string, headerMap map[string]int, keys ...string) string {
	for _, key := range keys {
		if idx, ok := headerMap[strings.ToLower(key)]; ok && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
	}
	return ""
}

func normalizeID(val string) string {
	clean := strings.ToLower(val)
	clean = strings.ReplaceAll(clean, " ", "-")
	return clean
}

func processSpreadsheet(ctx context.Context, client clib.ContentAPI, spreadsheet Spreadsheet) error {
	slog.Info("Processing spreadsheet...", "spreadsheetID", spreadsheet.SpreadsheetID)

	drinCache, err := clib.LoadExisting(ctx, client, clib.LoadConfig[odhmodel.EventLinked]{
		EntityType:  "Event",
		QueryParams: map[string]string{"source": "drin"},
		IDFunc: func(p odhmodel.EventLinked) string {
			return p.Id
		},
	})
	if err != nil {
		slog.Error("Failed to load existing drin events", "err", err)
		return err
	}

	treviCache, err := clib.LoadExisting(ctx, client, clib.LoadConfig[odhmodel.EventLinked]{
		EntityType:  "Event",
		QueryParams: map[string]string{"source": "trevilab"},
		IDFunc: func(p odhmodel.EventLinked) string {
			return p.Id
		},
	})
	if err != nil {
		slog.Error("Failed to load existing trevilab events", "err", err)
		return err
	}

	eventCache := clib.NewCache[odhmodel.EventLinked]()
	for id, entry := range drinCache.Entries() {
		eventCache.Set(id, entry.Entity, entry.Hash)
	}
	for id, entry := range treviCache.Entries() {
		eventCache.Set(id, entry.Entity, entry.Hash)
	}

	activeCount, inactiveCount := 0, 0
	for _, e := range eventCache.Entries() {
		if e.Entity.Active {
			activeCount++
		} else {
			inactiveCount++
		}
	}
	slog.Info("Loaded existing Events", "count", len(eventCache.Entries()), "active", activeCount, "inactive", inactiveCount)

	var eventsSheet, placesSheet, roomsSheet *Sheet

	for i := range spreadsheet.Sheets {
		title := strings.ToLower(spreadsheet.Sheets[i].Properties.Title)
		switch title {
		case "events":
			eventsSheet = &spreadsheet.Sheets[i]
		case "places":
			placesSheet = &spreadsheet.Sheets[i]
		case "rooms":
			roomsSheet = &spreadsheet.Sheets[i]
		}
	}

	// 1. Parse Places
	places := make(map[string]*placeData)    // place ID (lowercased) → data
	venueIDs := make(map[string]string)      // place ID (lowercased) → venue URN

	if placesSheet != nil {
		rows := sheetToSlice(*placesSheet)
		if len(rows) > 1 {
			headers := getHeaderMap(rows[0])
			for _, row := range rows[1:] {
				id := getValue(row, headers, "id", "name", "place-id")
				if id == "" {
					continue
				}

				lat, _ := strconv.ParseFloat(getValue(row, headers, "latitude", "lat"), 64)
				lon, _ := strconv.ParseFloat(getValue(row, headers, "longitude", "lon", "lng"), 64)

				pd := &placeData{
					ID: id,
					Names: map[string]string{
						"it": firstNonEmpty(getValue(row, headers, "it:name"), id),
						"de": firstNonEmpty(getValue(row, headers, "de:name"), id),
						"en": firstNonEmpty(getValue(row, headers, "en:name"), id),
					},
					Addresses: map[string]string{
						"it": getValue(row, headers, "it:address", "address"),
						"de": getValue(row, headers, "de:address"),
						"en": getValue(row, headers, "en:address"),
					},
					Cities: map[string]string{
						"it": getValue(row, headers, "it:city", "city"),
						"de": getValue(row, headers, "de:city"),
						"en": getValue(row, headers, "en:city"),
					},
					Email:    getValue(row, headers, "email"),
					Phone:    getValue(row, headers, "phone", "phonenumber"),
					ZipCode:  getValue(row, headers, "zipcode", "zip"),
					Province: getValue(row, headers, "province"),
					Lat:      lat,
					Lon:      lon,
				}

				key := strings.ToLower(id)
				places[key] = pd

				venueID := "urn:venue:centrotrevi-drin:" + normalizeID(id)
				venueIDs[key] = venueID
			}
		}
	}

	// 2. Parse Rooms
	rooms := make(map[string]*roomData) // room IT name (lowercased) → data

	if roomsSheet != nil {
		rows := sheetToSlice(*roomsSheet)
		if len(rows) > 1 {
			headers := getHeaderMap(rows[0])
			for _, row := range rows[1:] {
				itName := getValue(row, headers, "it:name", "name", "id")
				placeID := getValue(row, headers, "place", "place-id", "venue")
				if itName == "" {
					continue
				}

				maxSeats := 0
				if v, err := strconv.Atoi(getValue(row, headers, "max_seats", "max_capacity", "capacity", "max-capacity")); err == nil {
					maxSeats = v
				}

				rd := &roomData{
					ID:      getValue(row, headers, "id"),
					PlaceID: placeID,
					Names: map[string]string{
						"it": itName,
						"de": firstNonEmpty(getValue(row, headers, "de:name"), itName),
						"en": firstNonEmpty(getValue(row, headers, "en:name"), itName),
					},
					Addresses: map[string]string{
						"it": getValue(row, headers, "it:address", "address"),
						"de": getValue(row, headers, "de:address"),
						"en": getValue(row, headers, "en:address"),
					},
					Cities: map[string]string{
						"it": getValue(row, headers, "it:city", "city"),
						"de": getValue(row, headers, "de:city"),
						"en": getValue(row, headers, "en:city"),
					},
					ZipCode:  getValue(row, headers, "zipcode", "zip"),
					MaxSeats: maxSeats,
				}

				rooms[strings.ToLower(itName)] = rd
			}
		}
	}



	// 4. Process Events
	spreadsheetEventIDs := make(map[string]bool)

	if eventsSheet != nil {
		rows := sheetToSlice(*eventsSheet)
		if len(rows) > 1 {
			headers := getHeaderMap(rows[0])
			for _, row := range rows[1:] {
				eventID := getValue(row, headers, "event-id", "id")
				titleIT := getValue(row, headers, "it:title", "it:name", "title")
				titleDE := getValue(row, headers, "de:title", "de:name")
				titleEN := getValue(row, headers, "en:title", "en:name")

				if titleIT == "" && titleDE == "" && titleEN == "" {
					continue
				}

				beginDate := getValue(row, headers, "begin_date", "start_date")
				beginTime := getValue(row, headers, "begin_time", "start_time")
				endDate := getValue(row, headers, "end_date")
				endTime := getValue(row, headers, "end_time")
				placeRef := getValue(row, headers, "place")
				roomRef := getValue(row, headers, "room")

				if eventID == "" {
					eventID = fmt.Sprintf("%s-%s-%s-%s-%s-%s",
						strings.ReplaceAll(beginDate, "/", ""),
						strings.ReplaceAll(beginTime, ":", ""),
						strings.ReplaceAll(endDate, "/", ""),
						strings.ReplaceAll(endTime, ":", ""),
						normalizeID(roomRef),
						normalizeID(titleIT),
					)
				}

				originalID := eventID
				eventID = strings.ToUpper(eventID)
				spreadsheetEventIDs[eventID] = true

				var event odhmodel.EventLinked
				if cachedEntry, ok := eventCache.Get(eventID); ok {
					event = cachedEntry.Entity
				} else {
					event = odhmodel.EventLinked{
						Id:          eventID,
						FirstImport: nowFunc().Format(time.RFC3339),
					}
				}

				event.Active = true
				event.Shortname = firstNonEmpty(titleIT, titleDE, titleEN)

				source := "trevilab"
				orgRID := "TreviLab"
				if strings.Contains(strings.ToLower(placeRef), "drin") {
					source = "drin"
					orgRID = "DRIN"
				}
				event.Source = source
				event.OrgRID = orgRID
				event.PublishedOn = []string{"centro-trevi." + source}

				// Detail
				event.Detail = make(map[string]odhmodel.Detail)
				descIT := getValue(row, headers, "it:decription", "it:description", "description")
				descDE := getValue(row, headers, "de:decription", "de:description")
				descEN := getValue(row, headers, "en:decription", "en:description")

				if titleIT != "" {
					event.Detail["it"] = odhmodel.Detail{Language: "it", Title: titleIT, BaseText: descIT}
				}
				if titleDE != "" {
					event.Detail["de"] = odhmodel.Detail{Language: "de", Title: titleDE, BaseText: descDE}
				}
				if titleEN != "" {
					event.Detail["en"] = odhmodel.Detail{Language: "en", Title: titleEN, BaseText: descEN}
				}

				// Dates
				if beginTime == "" {
					beginTime = "00:00"
				}
				if endTime == "" {
					endTime = "23:59"
				}

				fromISO := formatDateISO(beginDate)
				toISO := formatDateISO(endDate)

				ticket := getValue(row, headers, "ticket")
				ticketRequired := strings.EqualFold(ticket, "yes") || strings.EqualFold(ticket, "true")
				maxPersons := 0
				if v, err := strconv.Atoi(getValue(row, headers, "number_of_seats")); err == nil {
					maxPersons = v
				}

				event.EventDate = []odhmodel.EventDate{
					{
						Active:     true,
						From:       fromISO + "T00:00:00",
						To:         toISO + "T00:00:00",
						Begin:      beginTime + ":00",
						End:        endTime + ":00",
						Ticket:     ticketRequired,
						MaxPersons: maxPersons,
					},
				}
				event.DateBegin = fromISO + "T" + beginTime + ":00"
				event.DateEnd = toISO + "T" + endTime + ":00"

				// Venue linking
				placeKey := strings.ToLower(placeRef)
				if vid, ok := venueIDs[placeKey]; ok {
					event.VenueIds = []string{vid}
				}

				// Look up place and room data for rich metadata
				pd := places[placeKey]
				rd := rooms[strings.ToLower(roomRef)]

				// ContactInfos — room name + address (room's own address wins, falls back to its place)
				if rd != nil {
					roomURN := "urn:room:centrotrevi-drin:" + normalizeID(rd.Names["it"])
					event.EventDate[0].VenueRoomDetailsIds = []string{roomURN}

					pdAddr, pdCity, pdZip := map[string]string{}, map[string]string{}, ""
					if pd != nil {
						pdAddr, pdCity, pdZip = pd.Addresses, pd.Cities, pd.ZipCode
					}

					event.ContactInfos = map[string]odhmodel.ContactInfos{
						"it": {Language: "it", CompanyName: rd.Names["it"], Address: firstNonEmpty(rd.Addresses["it"], pdAddr["it"]), City: firstNonEmpty(rd.Cities["it"], pdCity["it"]), ZipCode: firstNonEmpty(rd.ZipCode, pdZip), CountryCode: "IT"},
						"de": {Language: "de", CompanyName: rd.Names["de"], Address: firstNonEmpty(rd.Addresses["de"], pdAddr["de"]), City: firstNonEmpty(rd.Cities["de"], pdCity["de"]), ZipCode: firstNonEmpty(rd.ZipCode, pdZip), CountryCode: "IT"},
						"en": {Language: "en", CompanyName: rd.Names["en"], Address: firstNonEmpty(rd.Addresses["en"], pdAddr["en"]), City: firstNonEmpty(rd.Cities["en"], pdCity["en"]), ZipCode: firstNonEmpty(rd.ZipCode, pdZip), CountryCode: "IT"},
					}
				}

				// OrganizerInfos — place name, email, phone
				if pd != nil {
					event.OrganizerInfos = map[string]odhmodel.ContactInfos{
						"it": {Language: "it", CompanyName: pd.Names["it"], Email: pd.Email, Phonenumber: pd.Phone, Address: pd.Addresses["it"], City: pd.Cities["it"], ZipCode: pd.ZipCode, CountryCode: "IT"},
						"de": {Language: "de", CompanyName: pd.Names["de"], Email: pd.Email, Phonenumber: pd.Phone, Address: pd.Addresses["de"], City: pd.Cities["de"], ZipCode: pd.ZipCode, CountryCode: "IT"},
						"en": {Language: "en", CompanyName: pd.Names["en"], Email: pd.Email, Phonenumber: pd.Phone, Address: pd.Addresses["en"], City: pd.Cities["en"], ZipCode: pd.ZipCode, CountryCode: "IT"},
					}
				}

				// EventAdditionalInfos — room multilingual name as Location, ticket registration link
				registrationLink := getValue(row, headers, "link_to_ticket_info")
				if rd != nil || registrationLink != "" {
					event.EventAdditionalInfos = map[string]map[string]string{}
					for _, lang := range []string{"it", "de", "en"} {
						info := map[string]string{"Language": lang}
						if rd != nil {
							info["Location"] = rd.Names[lang]
						}
						if registrationLink != "" {
							info["Registration"] = registrationLink
						}
						event.EventAdditionalInfos[lang] = info
					}
				}

				// GpsInfo from place with fallbacks
				var lat, lon float64
				if pd != nil {
					lat, lon = pd.Lat, pd.Lon
				}
				if lat == 0 && lon == 0 {
					if source == "trevilab" {
						lat, lon = 46.49581, 11.352324
					} else if source == "drin" {
						lat, lon = 46.4975, 11.3555
					}
				}
				if lat != 0 || lon != 0 {
					event.GpsInfo = []odhmodel.GpsInfo{
						{Gpstype: "position", Latitude: lat, Longitude: lon},
					}
				}

				// Topics + TagIds
				eventTypeKey := getValue(row, headers, "event_type_key")
				topicRID, topicInfo := getTopicByEventType(eventTypeKey)
				event.Topics = []map[string]any{
					{"TopicRID": topicRID, "TopicInfo": topicInfo},
				}
				event.TopicRIDs = []string{topicRID}
				event.TagIds = []string{topicRID}

				// EventProperty
				event.EventProperty = map[string]any{
					"TicketRequired":       ticketRequired,
					"RegistrationRequired": false,
					"EventOrganizerId":     orgRID,
				}

				// EventBooking
				price := getValue(row, headers, "price")
				if ticketRequired {
					event.EventBooking = map[string]any{
						"TicketRequired": true,
						"Price":          price,
					}
				}

				// Mapping (preserves original case event ID)
				event.Mapping = map[string]map[string]string{
					"culture": {"id": originalID},
				}

				event.LicenseInfo = map[string]any{
					"License": "CC0", "Author": "", "ClosedData": false, "LicenseHolder": "unknown",
				}

				hash, changed, hashErr := eventCache.HasChanged(eventID, event)
				if hashErr != nil {
					slog.Error("Failed to hash event", "err", hashErr, "id", event.Id)
					continue
				}

				if !changed {
					continue
				}

				err := client.Put(ctx, "Event", event.Id, &event)
				if err != nil {
					slog.Debug("Put event failed, trying Post", "err", err, "id", event.Id)
					err = client.Post(ctx, "Event", map[string]string{"generateid": "false"}, &event)
					if err != nil {
						slog.Error("Failed to save event", "err", err, "id", event.Id)
					} else {
						slog.Info("Saved event (Post)", "id", event.Id)
						eventCache.Set(eventID, event, hash)
					}
				} else {
					slog.Info("Saved event (Put)", "id", event.Id)
					eventCache.Set(eventID, event, hash)
				}
			}
		}
	}

	// 5. Deactivate orphaned events
	cacheIDs := make([]string, 0, len(eventCache.Entries()))
	for id := range eventCache.Entries() {
		cacheIDs = append(cacheIDs, id)
	}

	slog.Warn("Deactivation diagnostics", "seenCount", len(spreadsheetEventIDs), "cacheCount", len(cacheIDs))
	for _, id := range cacheIDs {
		if _, ok := spreadsheetEventIDs[id]; ok {
			continue
		}
		
		entry, _ := eventCache.Get(id)
		if !entry.Entity.Active {
			continue
		}

		slog.Info("Deactivating orphaned event", "id", id, "source", entry.Entity.Source)
		apiEvent := entry.Entity
		apiEvent.Active = false
		if err := client.Put(ctx, "Event", apiEvent.Id, &apiEvent); err != nil {
			slog.Error("Failed to deactivate event", "err", err, "id", apiEvent.Id)
		}
	}

	return nil
}

func formatDateISO(dateStr string) string {
	parts := strings.Split(dateStr, "/")
	if len(parts) == 3 {
		day := parts[0]
		month := parts[1]
		year := parts[2]
		if len(day) == 1 {
			day = "0" + day
		}
		if len(month) == 1 {
			month = "0" + month
		}
		return fmt.Sprintf("%s-%s-%s", year, month, day)
	}
	return dateStr
}

// getTopicByEventType maps the "Event_type_key" spreadsheet column to its fixed
// Content API TopicRID and German TopicInfo label. Ported from the legacy C#
// importer's GetTopicRid switch.
// TODO: replace this hardcoded switch with the "Event_topicid_mapping" column
// once it's added to the spreadsheet, looked up by Event_type_key.
func getTopicByEventType(eventTypeIT string) (topicRID, topicInfo string) {
	switch eventTypeIT {
	case "Convegni/conferenze":
		return "0D25868CC23242D6AC97AEB2973CB3D6", "Tagungen/Vorträge"
	case "Sport":
		return "162C0067811B477DA725D2F5F2D98398", "Sport"
	case "Enogastronomia/prodotti":
		return "252200A028C8449D9A6205369A6D0D36", "Gastronomie/Typische Produkte"
	case "Artigianato/tradizioni":
		return "33BDC54BD39946F4852B3394B00610AE", "Handwerk/Brauchtum"
	case "Fiere/mercati":
		return "4C4961D9FC5B48EEB73067BEB9D4402A", "Messen/Märkte"
	case "Teatro/cinema":
		return "6884FE362C88434B9F49725E3328112B", "Theater/Vorführungen"
	case "Corsi/lezioni":
		return "767F6F43FC394CE9A3C8A9725C6FF134", "Kurse/Bildung"
	case "Musica/danza":
		return "7E048074BA004EC58E29E330A9AA476B", "Musik/Tanz"
	case "Sagre/feste":
		return "9C3449EE278C4D94AA5A7C286729DEA0", "Volksfeste/Festivals"
	case "Gite/escursioni":
		return "ACE8B613F2074A7BB59C0B1DD40A43CD", "Wanderungen/Ausflüge"
	case "Visite guidate":
		return "B5467FEFE5C74FA5AD32B83793A76165", "Führungen/Besichtigungen"
	case "Mostre/arte":
		return "C72CE969B98947FABC99CBC7B033F28E", "Ausstellungen/Kunst"
	case "Famiglia":
		return "D98B49DF24C342D09A8161836435CF86", "Familie"
	default:
		return "C72CE969B98947FABC99CBC7B033F28E", "Ausstellungen/Kunst"
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

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
	ID       string
	Names    map[string]string
	MaxSeats int
	PlaceID  string
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
				var existingEvent odhmodel.EventLinked
				if err := client.Get(ctx, "Event/"+eventID, nil, &existingEvent); err == nil {
					event = existingEvent
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

				event.EventDate = []odhmodel.EventDate{
					{
						From:  fromISO + "T00:00:00",
						To:    toISO + "T00:00:00",
						Begin: beginTime + ":00",
						End:   endTime + ":00",
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

				// ContactInfos — room name + room/place address
				if rd != nil && pd != nil {
					event.ContactInfos = map[string]odhmodel.ContactInfos{
						"it": {Language: "it", CompanyName: rd.Names["it"], Address: pd.Addresses["it"], City: pd.Cities["it"], ZipCode: pd.ZipCode, CountryCode: "IT"},
						"de": {Language: "de", CompanyName: rd.Names["de"], Address: pd.Addresses["de"], City: pd.Cities["de"], ZipCode: pd.ZipCode, CountryCode: "IT"},
						"en": {Language: "en", CompanyName: rd.Names["en"], Address: pd.Addresses["en"], City: pd.Cities["en"], ZipCode: pd.ZipCode, CountryCode: "IT"},
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

				// EventAdditionalInfos — room multilingual name as Location
				if rd != nil {
					event.EventAdditionalInfos = map[string]map[string]string{
						"it": {"Language": "it", "Location": rd.Names["it"]},
						"de": {"Language": "de", "Location": rd.Names["de"]},
						"en": {"Language": "en", "Location": rd.Names["en"]},
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

				// Topics
				eventTypeKey := getValue(row, headers, "event_type_key", "category", "type")
				eventTypeIT := getValue(row, headers, "it:event_type")
				if eventTypeKey != "" {
					topic := map[string]any{"TopicRID": eventTypeKey}
					if eventTypeIT != "" {
						topic["TopicInfo"] = eventTypeIT
					}
					event.Topics = []map[string]any{topic}
				}

				// EventProperty
				ticket := getValue(row, headers, "ticket")
				ticketRequired := strings.EqualFold(ticket, "yes") || strings.EqualFold(ticket, "true")
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

				err := client.Put(ctx, "Event", event.Id, &event)
				if err != nil {
					slog.Debug("Put event failed, trying Post", "err", err, "id", event.Id)
					err = client.Post(ctx, "Event", nil, &event)
					if err != nil {
						slog.Error("Failed to save event", "err", err, "id", event.Id)
					}
				} else {
					slog.Info("Saved event", "id", event.Id)
				}
			}
		}
	}

	// 5. Deactivate orphaned events
	for _, source := range []string{"drin", "trevilab"} {
		var page struct {
			Items []odhmodel.EventLinked `json:"Items"`
		}
		err := client.Get(ctx, "Event", map[string]string{"source": source, "active": "true"}, &page)
		if err != nil {
			slog.Warn("Failed to fetch events for deactivation", "source", source, "err", err)
			continue
		}
		for _, apiEvent := range page.Items {
			if !spreadsheetEventIDs[strings.ToUpper(apiEvent.Id)] {
				slog.Info("Deactivating orphaned event", "id", apiEvent.Id, "source", source)
				apiEvent.Active = false
				if err := client.Put(ctx, "Event", apiEvent.Id, &apiEvent); err != nil {
					slog.Error("Failed to deactivate event", "err", err, "id", apiEvent.Id)
				}
			}
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

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

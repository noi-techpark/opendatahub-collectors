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
	"sort"
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

	var placesSheet, roomsSheet *Sheet

	for i := range spreadsheet.Sheets {
		title := strings.ToLower(spreadsheet.Sheets[i].Properties.Title)
		switch title {
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

	// 3. Build and save Venues with RoomDetails
	placeKeys := make([]string, 0, len(places))
	for k := range places {
		placeKeys = append(placeKeys, k)
	}
	sort.Strings(placeKeys)

	for _, placeKey := range placeKeys {
		pd := places[placeKey]
		venueID := venueIDs[placeKey]

		var venue odhmodel.VenueV2
		var existingVenue map[string]any
		if err := client.Get(ctx, "Venue/"+venueID, nil, &existingVenue); err == nil {
			venueBytes, _ := json.Marshal(existingVenue)
			_ = json.Unmarshal(venueBytes, &venue)
		} else {
			active := true
			venue = odhmodel.VenueV2{Id: venueID, Active: &active}
		}

		venue.Shortname = pd.Names["it"]
		venue.Source = "trevilab"
		if strings.Contains(strings.ToLower(pd.ID), "drin") {
			venue.Source = "drin"
		}
		venue.Detail = map[string]any{
			"it": map[string]any{"Language": "it", "Title": pd.Names["it"]},
			"de": map[string]any{"Language": "de", "Title": pd.Names["de"]},
			"en": map[string]any{"Language": "en", "Title": pd.Names["en"]},
		}
		venue.LocationInfo = map[string]any{"Latitude": pd.Lat, "Longitude": pd.Lon}
		venue.ContactInfos = map[string]any{
			"it": map[string]any{"Address": pd.Addresses["it"], "City": pd.Cities["it"], "Email": pd.Email, "Phonenumber": pd.Phone, "ZipCode": pd.ZipCode, "Language": "it"},
			"de": map[string]any{"Address": pd.Addresses["de"], "City": pd.Cities["de"], "Email": pd.Email, "Phonenumber": pd.Phone, "ZipCode": pd.ZipCode, "Language": "de"},
			"en": map[string]any{"Address": pd.Addresses["en"], "City": pd.Cities["en"], "Email": pd.Email, "Phonenumber": pd.Phone, "ZipCode": pd.ZipCode, "Language": "en"},
		}
		venue.GpsInfo = []map[string]any{
			{"Gpstype": "position", "Latitude": pd.Lat, "Longitude": pd.Lon},
		}

		// Attach rooms belonging to this place
		roomKeys := make([]string, 0, len(rooms))
		for k := range rooms {
			roomKeys = append(roomKeys, k)
		}
		sort.Strings(roomKeys)

		for _, roomKey := range roomKeys {
			rd := rooms[roomKey]
			if !strings.EqualFold(rd.PlaceID, pd.ID) {
				continue
			}

			roomURN := "urn:room:centrotrevi-drin:" + normalizeID(rd.Names["it"])

			foundIdx := -1
			for idx, r := range venue.RoomDetails {
				if r.Id == roomURN {
					foundIdx = idx
					break
				}
			}

			var room odhmodel.VenueRoomDetailsV2
			if foundIdx != -1 {
				room = venue.RoomDetails[foundIdx]
			} else {
				room = odhmodel.VenueRoomDetailsV2{Id: roomURN, Active: true}
			}

			room.Shortname = rd.Names["it"]
			room.Detail = map[string]odhmodel.DetailGeneric{
				"it": {Language: "it", Title: rd.Names["it"]},
				"de": {Language: "de", Title: rd.Names["de"]},
				"en": {Language: "en", Title: rd.Names["en"]},
			}
			if rd.MaxSeats > 0 {
				room.MaxCapacity = &rd.MaxSeats
			}

			if foundIdx != -1 {
				venue.RoomDetails[foundIdx] = room
			} else {
				venue.RoomDetails = append(venue.RoomDetails, room)
			}
		}

		var venueMap map[string]any
		venueBytes, _ := json.Marshal(venue)
		_ = json.Unmarshal(venueBytes, &venueMap)

		err := client.Put(ctx, "Venue", venue.Id, &venueMap)
		if err != nil {
			slog.Debug("Put venue failed, trying Post", "err", err, "id", venue.Id)
			err = client.Post(ctx, "Venue", map[string]string{"generateid": "false"}, &venueMap)
			if err != nil {
				slog.Error("Failed to save venue", "err", err.Error(), "id", venue.Id)
			}
		} else {
			slog.Info("Saved venue", "id", venue.Id)
		}
	}

	// 4. Events are handled by the events transformer

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

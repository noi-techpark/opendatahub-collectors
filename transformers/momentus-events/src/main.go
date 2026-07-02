// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	odhmodel "opendatahub.com/momentus-events/odh-content-model"
)

var env struct {
	tr.Env
	MomentusClientID     string `envconfig:"MOMENTUS_CLIENT_ID"`
	MomentusClientSecret string `envconfig:"MOMENTUS_CLIENT_SECRET"`
	ODHVenueAPI          string `envconfig:"ODH_VENUE_API" default:"https://tourism.api.opendatahub.testingmachine.eu/v1"`
	ODHVenueNoiID        string `envconfig:"ODH_VENUE_NOI_ID" default:"urn:venue:noi:6b3f0a14-3c5b-5d09-81f3-3ebe5b7885ea"`
	ODHVenueEuracID      string `envconfig:"ODH_VENUE_EURAC_ID" default:"urn:venue:eurac:df155f71-5cea-5a29-9ebc-213fad6ac1eb"`
	OdhCoreUrl                 string `envconfig:"ODH_CORE_URL"`
	OdhCoreTokenUrl            string `envconfig:"ODH_CORE_TOKEN_URL"`
	OdhCoreTokenClientId       string `envconfig:"ODH_CORE_TOKEN_CLIENT_ID"`
	OdhCoreTokenClientSecret   string `envconfig:"ODH_CORE_TOKEN_CLIENT_SECRET"`
}

type Transformer struct {
	momentusClient *MomentusClient
	odhClient      *ODHClient
	contentClient  clib.ContentAPI
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Momentus Events Transformer...")

	defer tel.FlushOnPanic()

	contentClient, err := clib.NewContentClient(clib.Config{
		BaseURL:      env.OdhCoreUrl,
		TokenURL:     env.OdhCoreTokenUrl,
		ClientID:     env.OdhCoreTokenClientId,
		ClientSecret: env.OdhCoreTokenClientSecret,
		DisableOAuth: env.OdhCoreTokenUrl == "",
	})
	ms.FailOnError(context.Background(), err, "failed to create content client")

	t := &Transformer{
		momentusClient: NewMomentusClient(env.MomentusClientID, env.MomentusClientSecret),
		odhClient:      NewODHClient(env.ODHVenueAPI, ""),
		contentClient:  contentClient,
	}

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		if r.Rawdata == "[]" {
			slog.Debug("Received empty array payload (end of stream), skipping")
			return nil
		}
		
		// Attempt to unmarshal as an array first (which is what the crawler currently sends)
		var events []MomentusEvent
		if err := json.Unmarshal([]byte(r.Rawdata), &events); err == nil {
			var firstErr error
			for _, event := range events {
				err := processEvent(ctx, t, event)
				if err != nil && firstErr == nil {
					firstErr = err
				}
			}
			return firstErr
		}

		// Fallback to unmarshal as a single event
		var event MomentusEvent
		if err := json.Unmarshal([]byte(r.Rawdata), &event); err != nil {
			slog.Error("Failed to unmarshal raw event string", "err", err, "rawdata", r.Rawdata)
			return err
		}
		
		return processEvent(ctx, t, event)
	})

	if err != nil {
		slog.Error("error while listening to queue", "err", err)
		os.Exit(1)
	}
}

func processEvent(ctx context.Context, t *Transformer, event MomentusEvent) error {
	slog.Info("RECEIVED RAW EVENT IN TRANSFORMER", "id", event.Id)
	if event.Id == "" {
		slog.Warn("Received event without ID, skipping")
		return nil
	}

	// We fetch additional data
	functions, err := t.momentusClient.GetFunctions(event.Id)
	if err != nil {
		slog.Error("Failed to fetch functions", "eventID", event.Id, "err", err)
		return err
	}

	bookedSpaces, err := t.momentusClient.GetBookedSpaces(event.Id)
	if err != nil {
		slog.Error("Failed to fetch booked spaces", "eventID", event.Id, "err", err)
		return err
	}

	venueEurac, _ := t.odhClient.GetVenue(env.ODHVenueEuracID)
	venueNoi, _ := t.odhClient.GetVenue(env.ODHVenueNoiID)

	venue := venueNoi
	if venue == nil {
		venue = venueEurac
	}

	// We pass nil for the base event here, but a real transformer with database access
	// would load the existing event from ODH and pass it here to preserve manually edited fields.
	eventLinked := ParseMomentusEvent(event, functions, bookedSpaces, venue, nil, true)
	if eventLinked == nil {
		slog.Info("Event skipped by parser (no languages)", "eventID", event.Id)
		return nil
	}

	err = t.contentClient.Put(ctx, "Event", eventLinked.Id, eventLinked)
	if err != nil {
		slog.Debug("Put failed, attempting Post as fallback", "err", err, "eventID", event.Id)
		err = t.contentClient.Post(ctx, "Event", nil, eventLinked)
		if err != nil {
			slog.Error("Failed to push Event to ODH Core API (both Put and Post failed)", "err", err, "eventID", event.Id)
			return err
		}
	}

	slog.Info("Successfully processed event and pushed to Core", "eventID", event.Id)
	return nil
}


// ----------------------------------------------------------------------------
// PARSER LOGIC
// ----------------------------------------------------------------------------

func ParseMomentusEvent(mevent MomentusEvent, functions []MomentusFunction, bookedSpaces []MomentusBookedSpace, venue *ODHVenue, base *odhmodel.EventLinked, optimizedays bool) *odhmodel.EventLinked {
	var eventLinked *odhmodel.EventLinked
	if base != nil {
		copy := *base
		eventLinked = &copy
	} else {
		eventLinked = new(odhmodel.EventLinked)
		eventLinked.FirstImport = time.Now().Format(time.RFC3339)
	}

	// Preserve these fields from the existing record (as per legacy parser logic)
	oldImageGallery := eventLinked.ImageGallery
	oldTagIds := eventLinked.TagIds
	oldDocuments := eventLinked.Documents
	oldVideoItems := eventLinked.VideoItems

	eventLinked.Id = "urn:event:momentus:" + mevent.Id
	eventLinked.Shortname = mevent.Name
	eventLinked.Source = "momentus"

	eventLinked.Active = mevent.IsActive && !mevent.IsCanceled
	eventLinked.LastChange = time.Now().Format(time.RFC3339)

	// Restore preserved fields
	eventLinked.ImageGallery = oldImageGallery
	eventLinked.TagIds = oldTagIds
	eventLinked.Documents = oldDocuments
	eventLinked.VideoItems = oldVideoItems

	if mevent.Start != "" {
		eventLinked.DateBegin = mevent.Start
	}
	if mevent.End != "" {
		eventLinked.DateEnd = mevent.End
	}

	details := buildDetailFromFunctions(functions, mevent.Description, mevent.Name)
	if len(details) > 0 {
		eventLinked.Detail = details
	} else {
		return nil
	}

	if venue != nil && venue.Id != "" {
		eventLinked.VenueIds = []string{venue.Id}
	}

	eventLinked.EventDate = buildEventDates(mevent, venue, bookedSpaces)

	if optimizedays {
		refineRootDatesFromEventDates(eventLinked)
	}

	if len(mevent.ContactRoles) > 0 {
		firstRole := mevent.ContactRoles[0]
		contact := buildContactInfo(firstRole)
		eventLinked.GpsInfo = []odhmodel.GpsInfo{{}}
		eventLinked.ImageGallery = []odhmodel.ImageGalleryItem{{}}
		eventLinked.ContactInfos = map[string]odhmodel.ContactInfos{
			"en": contact,
		}
		if mevent.AccountName != "" {
			organizer := buildOrganizerInfo(firstRole, mevent.AccountName)
			eventLinked.OrganizerInfos = map[string]odhmodel.ContactInfos{
				"en": organizer,
			}
		}
	}

	momentusMapping := map[string]string{
		"id":            mevent.Id,
		"eventTypeId":   mevent.EventTypeId,
		"eventTypeName": mevent.EventTypeName,
	}
	for _, e := range mevent.ExternalIds {
		if e.Key != "" {
			momentusMapping[e.Key] = e.Value
		}
	}
	eventLinked.Mapping = map[string]map[string]string{
		"momentus": momentusMapping,
	}

	if mevent.Website != "" {
		eventLinked.EventUrls = []odhmodel.EventUrl{
			{
				Url:    map[string]string{"en": mevent.Website},
				Type:   "default",
				Active: true,
			},
		}
	}

	var venueEventLocation string
	if venue != nil && venue.Mapping.Tag != nil {
		venueEventLocation = venue.Mapping.Tag["eventlocation"]
	}

	eventLinked.PublishedOn = determinePublishedOn(mevent, bookedSpaces, venueEventLocation)

	var tagIds []string
	if venueEventLocation != "" {
		tagIds = append(tagIds, venueEventLocation)
	}

	if orgInfos, ok := eventLinked.OrganizerInfos["en"]; ok {
		if strings.HasPrefix(orgInfos.CompanyName, "NOI - ") {
			tagIds = assignTechnologyFields(orgInfos.CompanyName, tagIds)
		}
	}

	if len(tagIds) > 0 {
		eventLinked.TagIds = tagIds
	}

	if !eventLinked.Active {
		eventLinked.PublishedOn = []string{}
	}

	return eventLinked
}

func buildDetailFromFunctions(functions []MomentusFunction, description string, eventName string) map[string]odhmodel.Detail {
	details := make(map[string]odhmodel.Detail)

	for _, fn := range functions {
		fnType := strings.TrimSpace(fn.FunctionTypeName)
		name := fn.Name
		lang := ""
		isSub := false

		if fnType == "EN Title" {
			lang = "en"
		} else if fnType == "DE Title" {
			lang = "de"
		} else if fnType == "IT Title" {
			lang = "it"
		} else if fnType == "EN SUBtitle" {
			lang = "en"
			isSub = true
		} else if fnType == "DE SUBtitle" {
			lang = "de"
			isSub = true
		} else if fnType == "IT SUBtitle" {
			lang = "it"
			isSub = true
		}

		if lang != "" {
			d := details[lang]
			d.Language = lang
			if isSub {
				d.SubHeader = name
			} else {
				d.Title = name
			}
			details[lang] = d
		}
	}

	if len(details) == 0 && eventName != "" {
		for _, lang := range []string{"en", "de", "it"} {
			details[lang] = odhmodel.Detail{
				Language: lang,
				Title:    eventName,
			}
		}
	}

	if description != "" {
		for lang, d := range details {
			if d.BaseText == "" {
				d.BaseText = description
				details[lang] = d
			}
		}
	}

	return details
}

func determinePublishedOn(mevent MomentusEvent, bookedSpaces []MomentusBookedSpace, venueEventLocation string) []string {
	eventSpaceIds := make(map[string]bool)

	for _, space := range mevent.BookedSpaces {
		if strings.EqualFold(space.UsageType, "event") {
			if space.BookedSpaceId != "" {
				eventSpaceIds[space.BookedSpaceId] = true
			}
		}
	}

	if len(eventSpaceIds) == 0 {
		return []string{}
	}

	var spaceUsageNames []string
	for _, b := range bookedSpaces {
		if eventSpaceIds[b.Id] {
			usage := strings.ToUpper(strings.TrimSpace(b.SpaceUsageName))
			if usage != "" {
				spaceUsageNames = append(spaceUsageNames, usage)
			}
		}
	}

	if len(spaceUsageNames) == 0 {
		return []string{}
	}

	allPrivate := true
	for _, u := range spaceUsageNames {
		if !strings.Contains(u, "PRIVATE") {
			allPrivate = false
			break
		}
	}
	if allPrivate {
		return []string{}
	}

	effectiveType := ""
	for _, u := range spaceUsageNames {
		if strings.Contains(u, "PUBLIC") {
			effectiveType = "PUBLIC"
			break
		}
	}
	if effectiveType == "" {
		for _, u := range spaceUsageNames {
			if strings.Contains(u, "VIDEOWALL") {
				effectiveType = "VIDEOWALL"
				break
			}
		}
	}
	if effectiveType == "" {
		for _, u := range spaceUsageNames {
			if strings.Contains(u, "ROOM") {
				effectiveType = "ROOM"
				break
			}
		}
	}

	if effectiveType == "" {
		return []string{}
	}

	isEurac := strings.EqualFold(venueEventLocation, "ec")
	isNoi := strings.EqualFold(venueEventLocation, "noi")
	var publishers []string

	if effectiveType == "PUBLIC" {
		if isEurac {
			publishers = append(publishers, "eurac-videowall", "eurac-seminarroom")
		}
		if isNoi {
			publishers = append(publishers, "noi-totem", "today.noi.bz.it")
		}
	} else if effectiveType == "VIDEOWALL" {
		if isEurac {
			publishers = append(publishers, "eurac-videowall")
		}
		if isNoi {
			publishers = append(publishers, "today.noi.bz.it")
		}
	} else if effectiveType == "ROOM" {
		if isEurac {
			publishers = append(publishers, "eurac-seminarroom")
		}
		if isNoi {
			publishers = append(publishers, "noi-totem")
		}
	}

	return publishers
}

func buildEventDates(mevent MomentusEvent, venue *ODHVenue, bookedSpaces []MomentusBookedSpace) []odhmodel.EventDate {
	var eventDates []odhmodel.EventDate

	for _, space := range mevent.BookedSpaces {
		if !strings.EqualFold(space.UsageType, "event") || space.StartDate == "" {
			continue
		}

		var extSpace *MomentusBookedSpace
		for i, b := range bookedSpaces {
			if b.Id == space.BookedSpaceId {
				extSpace = &bookedSpaces[i]
				break
			}
		}

		usageName := ""
		if extSpace != nil {
			usageName = extSpace.SpaceUsageName
		}
		isPrivate := strings.Contains(strings.ToUpper(usageName), "PRIVATE")

		ed := odhmodel.EventDate{
			From:   space.StartDate,
			To:     space.EndDate,
			Active: !isPrivate,
		}

		if space.StartTime != "" {
			ed.Begin = space.StartTime
		}
		if space.EndTime != "" {
			ed.End = space.EndTime
		}

		if space.RoomId != "" && venue != nil {
			for _, r := range venue.RoomDetails {
				if mm, ok := r.Mapping["momentus"]; ok {
					if mm["id"] == space.RoomId {
						ed.VenueRoomDetailsIds = []string{r.Id}
						break
					}
				}
			}
		}

		eventDates = append(eventDates, ed)
	}

	return eventDates
}

func refineRootDatesFromEventDates(eventLinked *odhmodel.EventLinked) {
	if len(eventLinked.EventDate) == 0 {
		return
	}

	var firstFrom string
	var lastTo string

	for _, d := range eventLinked.EventDate {
		if d.Active {
			if d.From != "" && (firstFrom == "" || d.From < firstFrom) {
				firstFrom = d.From
			}
			if d.To != "" && (lastTo == "" || d.To > lastTo) {
				lastTo = d.To
			}
		}
	}

	if firstFrom != "" {
		eventLinked.DateBegin = firstFrom
	}
	if lastTo != "" {
		eventLinked.DateEnd = lastTo
	}
}

func buildContactInfo(contact MomentusContactRole) odhmodel.ContactInfos {
	var givenname, surname string
	if contact.Name != "" {
		parts := strings.SplitN(strings.TrimSpace(contact.Name), " ", 2)
		givenname = parts[0]
		if len(parts) > 1 {
			surname = parts[1]
		}
	}

	return odhmodel.ContactInfos{
		Language:    "en",
		Givenname:   givenname,
		Surname:     surname,
		CompanyName: contact.AccountName,
		Email:       contact.Email,
		Phonenumber: contact.Phone,
		Address:     contact.Address1,
		City:        contact.AddressCity,
		ZipCode:     contact.AddressPostalCode,
		CountryName: contact.AddressCountry,
	}
}

func buildOrganizerInfo(contact MomentusContactRole, accountName string) odhmodel.ContactInfos {
	info := buildContactInfo(contact)
	info.CompanyName = accountName
	return info
}

func cloneContactInfo(info odhmodel.ContactInfos, lang string) odhmodel.ContactInfos {
	info.Language = lang
	return info
}

func assignTechnologyFields(companyName string, techFields []string) []string {
	cName := strings.ToLower(companyName)
	checkAndAdd := func(check, assign string) {
		if strings.Contains(cName, check) {
			found := false
			for _, tf := range techFields {
				if tf == assign {
					found = true
					break
				}
			}
			if !found {
				techFields = append(techFields, assign)
			}
		}
	}

	checkAndAdd("digital", "digital")
	checkAndAdd("alpine", "alpine")
	checkAndAdd("automotive", "automotiveautomation")
	checkAndAdd("food", "food")
	checkAndAdd("green", "green")

	return techFields
}

// ----------------------------------------------------------------------------
// CLIENT LOGIC


type ODHClient struct {
	BaseURL    string
	Token      string
	httpClient *http.Client
	venuesCache map[string]*ODHVenue
	mu         sync.Mutex
}

func NewODHClient(baseURL, token string) *ODHClient {
	return &ODHClient{
		BaseURL:     baseURL,
		Token:       token,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		venuesCache: make(map[string]*ODHVenue),
	}
}

func (c *ODHClient) GetVenue(venueID string) (*ODHVenue, error) {
	if venueID == "" {
		return nil, nil
	}

	c.mu.Lock()
	if v, ok := c.venuesCache[venueID]; ok {
		c.mu.Unlock()
		return v, nil
	}
	c.mu.Unlock()

	req, err := http.NewRequest("GET", c.BaseURL+"/Venue/"+venueID, nil)
	if err != nil {
		return nil, err
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch venue %s: %d", venueID, resp.StatusCode)
	}

	var venue ODHVenue
	if err := json.NewDecoder(resp.Body).Decode(&venue); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.venuesCache[venueID] = &venue
	c.mu.Unlock()

	return &venue, nil
}

// ----------------------------------------------------------------------------
// CLIENT LOGIC
// ----------------------------------------------------------------------------

type MomentusClient struct {
	ClientID     string
	ClientSecret string
	BaseURL      string
	AuthURL      string
	Token        string
	TokenExpiry  time.Time
	mu           sync.Mutex
	httpClient   *http.Client
}

func NewMomentusClient(clientID, clientSecret string) *MomentusClient {
	return &MomentusClient{
		ClientID:     clientID,
		ClientSecret: clientSecret,
		BaseURL:      "https://api.eu-venueops.com/v1",
		AuthURL:      "https://auth-api.eu-venueops.com/token",
		httpClient:   &http.Client{Timeout: 30 * time.Second},
	}
}

func (c *MomentusClient) getToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Token != "" && time.Now().Before(c.TokenExpiry) {
		return c.Token, nil
	}

	payload := map[string]string{
		"clientId":     c.ClientID,
		"clientSecret": c.ClientSecret,
	}
	body, _ := json.Marshal(payload)

	req, err := http.NewRequest("POST", c.AuthURL, bytes.NewBuffer(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth failed with status: %d", resp.StatusCode)
	}

	var res map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}

	tokenStr, ok := res["accessToken"].(string)
	if !ok {
		return "", fmt.Errorf("no accessToken in response: %v", res)
	}
	c.Token = tokenStr
	c.TokenExpiry = time.Now().Add(50 * time.Minute)

	return c.Token, nil
}

func (c *MomentusClient) getAuthRequest(method, endpoint string) (*http.Request, error) {
	token, err := c.getToken()
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequest(method, c.BaseURL+endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	return req, nil
}

func (c *MomentusClient) GetFunctions(eventID string) ([]MomentusFunction, error) {
	req, err := c.getAuthRequest("GET", "/functions/event/"+eventID)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []MomentusFunction{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get functions: %d", resp.StatusCode)
	}

	var functions []MomentusFunction
	if err := json.NewDecoder(resp.Body).Decode(&functions); err != nil {
		return nil, err
	}
	return functions, nil
}

func (c *MomentusClient) GetBookedSpaces(eventID string) ([]MomentusBookedSpace, error) {
	req, err := c.getAuthRequest("GET", "/booked-spaces/"+eventID)
	if err != nil {
		return nil, err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return []MomentusBookedSpace{}, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get booked spaces: %d", resp.StatusCode)
	}

	var spaces []MomentusBookedSpace
	if err := json.NewDecoder(resp.Body).Decode(&spaces); err != nil {
		return nil, err
	}
	return spaces, nil
}

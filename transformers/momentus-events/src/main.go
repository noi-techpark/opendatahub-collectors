// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/json"
	"log/slog"
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
	VenueMapping             string `envconfig:"VENUE_MAPPING" default:"{\"NOI TECHPARK\":\"urn:venue:noi:6b3f0a14-3c5b-5d09-81f3-3ebe5b7885ea\",\"EURAC RESEARCH HQ\":\"urn:venue:eurac:df155f71-5cea-5a29-9ebc-213fad6ac1eb\"}"`
	OdhCoreUrl               string `envconfig:"ODH_CORE_URL"`
	OdhCoreTokenUrl          string `envconfig:"ODH_CORE_TOKEN_URL"`
	OdhCoreTokenClientId     string `envconfig:"ODH_CORE_TOKEN_CLIENT_ID"`
	OdhCoreTokenClientSecret string `envconfig:"ODH_CORE_TOKEN_CLIENT_SECRET"`
	OdhCoreReferer           string `envconfig:"ODH_CORE_REFERER"`
}

type Transformer struct {
	contentClient clib.ContentAPI
	venuesCache   map[string]*ODHVenue
	mu            sync.Mutex
	venueMapping  map[string]string
}

func (t *Transformer) getVenues(ctx context.Context) []*ODHVenue {
	t.mu.Lock()
	defer t.mu.Unlock()

	var venues []*ODHVenue
	for _, venueID := range t.venueMapping {
		if v, ok := t.venuesCache[venueID]; ok {
			venues = append(venues, v)
			continue
		}
		
		var venue ODHVenue
		err := t.contentClient.Get(ctx, "Venue/"+venueID, nil, &venue)
		ms.FailOnError(ctx, err, "Fatal error: Configured Venue ID not found in ODH Content API", "venueID", venueID)
		
		t.venuesCache[venueID] = &venue
		venues = append(venues, &venue)
	}
	return venues
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
	}, clib.WithReferer(env.OdhCoreReferer))
	ms.FailOnError(context.Background(), err, "failed to create content client")

	var venueMap map[string]string
	if err := json.Unmarshal([]byte(env.VenueMapping), &venueMap); err != nil {
		ms.FailOnError(context.Background(), err, "failed to parse VENUE_MAPPING")
	}

	t := &Transformer{
		contentClient: contentClient,
		venuesCache:   make(map[string]*ODHVenue),
		venueMapping:  venueMap,
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

	venues := t.getVenues(ctx)
	
	// Determine the correct venue by looking at the event's booked spaces' room IDs
	var matchedVenue *ODHVenue
	for _, space := range event.BookedSpaces {
		if space.RoomId != "" {
			for _, v := range venues {
				for _, r := range v.RoomDetails {
					if mm, ok := r.Mapping["momentus"]; ok {
						if mm["id"] == space.RoomId {
							matchedVenue = v
							break
						}
					}
				}
				if matchedVenue != nil {
					break
				}
			}
		}
		if matchedVenue != nil {
			break
		}
	}

	if matchedVenue == nil {
		slog.Warn("Could not determine venue for event from room IDs, skipping venue linking", "eventID", event.Id)
	}

	// Fetch existing event from ODH to preserve manual fields
	eventLinkedID := "urn:event:momentus:" + event.Id
	var baseEvent *odhmodel.EventLinked
	var existingEvent odhmodel.EventLinked
	if err := t.contentClient.Get(ctx, "Event/" + eventLinkedID, nil, &existingEvent); err == nil {
		baseEvent = &existingEvent
	}

	eventLinked := ParseMomentusEvent(event, matchedVenue, baseEvent, true)
	if eventLinked == nil {
		slog.Info("Event skipped by parser (no languages)", "eventID", event.Id)
		return nil
	}

	err := t.contentClient.Put(ctx, "Event", eventLinked.Id, eventLinked)
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

func ParseMomentusEvent(mevent MomentusEvent, venue *ODHVenue, base *odhmodel.EventLinked, optimizedays bool) *odhmodel.EventLinked {
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

	details := buildDetailFromFunctions(mevent.Functions, mevent.Description, mevent.Name, base)
	if len(details) > 0 {
		eventLinked.Detail = details
	} else {
		return nil
	}

	if venue != nil && venue.Id != "" {
		eventLinked.VenueIds = []string{venue.Id}
	}

	eventLinked.EventDate = buildEventDates(mevent, venue, mevent.BookedSpacesDetails)

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

	eventLinked.PublishedOn = determinePublishedOn(mevent, mevent.BookedSpacesDetails, venueEventLocation)

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

func buildDetailFromFunctions(functions []MomentusFunction, description string, eventName string, base *odhmodel.EventLinked) map[string]odhmodel.Detail {
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

	// Restore existing BaseTexts from the base event if they are not empty
	if base != nil && base.Detail != nil {
		for lang, baseDetail := range base.Detail {
			if baseDetail.BaseText != "" {
				d, ok := details[lang]
				if ok {
					d.BaseText = baseDetail.BaseText
					details[lang] = d
				} else {
					details[lang] = odhmodel.Detail{
						Language: lang,
						BaseText: baseDetail.BaseText,
					}
				}
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

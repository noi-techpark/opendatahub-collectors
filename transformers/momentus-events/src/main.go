// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"sort"
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

func (t *Transformer) resetVenues() {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.venuesCache = make(map[string]*ODHVenue)
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
	err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(func(ctx context.Context, rawMsg *rdb.Raw[json.RawMessage]) error {
		// Reload the venues on every message, so rooms added by tr-momentus-venues
		// in the meantime are matched (and get eventlocation / PublishedOn).
		t.resetVenues()

		eventCache, err := clib.LoadExisting(ctx, t.contentClient, clib.LoadConfig[odhmodel.EventLinked]{
			EntityType:  "Event",
			QueryParams: map[string]string{"source": "momentus"},
			IDFunc: func(e odhmodel.EventLinked) string {
				return e.Id
			},
		})
		if err != nil {
			slog.Error("Failed to load existing events cache", "err", err)
			return err
		}

		// Attempt to unmarshal as a raw array first, flattening any nested arrays
		var rawArray []json.RawMessage
		processedIDs := make(map[string]bool)

		unmarshalErr := json.Unmarshal([]byte(rawMsg.Rawdata), &rawArray)
		if unmarshalErr != nil {
			var wrapper struct {
				Rooms []json.RawMessage `json:"rooms"`
			}
			if errWrap := json.Unmarshal([]byte(rawMsg.Rawdata), &wrapper); errWrap == nil && wrapper.Rooms != nil {
				rawArray = wrapper.Rooms
				unmarshalErr = nil
			}
		}

		if unmarshalErr == nil {
			var events []MomentusEvent
			hasMalformed := false
			for _, raw := range rawArray {
				var e MomentusEvent
				if err := json.Unmarshal(raw, &e); err == nil && e.Id != "" {
					events = append(events, e)
				} else {
					var nested []MomentusEvent
					if err := json.Unmarshal(raw, &nested); err == nil {
						events = append(events, nested...)
					} else {
						slog.Error("Failed to parse element in snapshot array", "err", err)
						hasMalformed = true
					}
				}
			}

			if hasMalformed {
				slog.Error("Snapshot contains malformed elements; rejecting to avoid incorrect deactivations")
				return nil // Reject permanently since payload is fundamentally malformed
			}

			slog.Info("Flattened events", "count", len(events))

			if len(events) == 0 {
				// An empty snapshot is far more likely a crawler/Momentus API problem than
				// "no events at all"; deactivating everything on it would be wrong.
				slog.Warn("Received empty events payload, skipping processing and deactivation")
				return nil
			}

			var firstErr error
			for _, event := range events {
				if event.Id != "" {
					processedIDs["urn:event:momentus:"+event.Id] = true
				}
				err := processEvent(ctx, t, event, eventCache)
				if err != nil && firstErr == nil {
					firstErr = err
				}
			}

			if firstErr == nil {
				if err := deactivateMissingEvents(ctx, t, eventCache, processedIDs); err != nil {
					return err
				}
				return nil
			}
			return firstErr
		} else {
			slog.Debug("Failed to unmarshal as raw array (falling back to single object)", "err", unmarshalErr)
		}

		// Fallback to unmarshal as a single event
		var event MomentusEvent
		if err := json.Unmarshal([]byte(rawMsg.Rawdata), &event); err != nil {
			slog.Error("Failed to unmarshal raw event string (likely wrong payload type)", "err", err)
			return nil
		}

		if event.Id != "" {
			processedIDs["urn:event:momentus:"+event.Id] = true
		}
		err = processEvent(ctx, t, event, eventCache)
		return err
	}))

	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func deactivateMissingEvents(ctx context.Context, t *Transformer, eventCache *clib.Cache[odhmodel.EventLinked], processedIDs map[string]bool) error {
	slog.Info("Running deactivation loop for missing events")
	var firstErr error
	now := time.Now()
	for id, entry := range eventCache.Entries() {
		if !processedIDs[id] && entry.Entity.Active {
			if endedBeforeCrawlWindow(entry.Entity, now) {
				// Missing because it has taken place, not because it was deleted or cancelled.
				continue
			}
			slog.Info("Deactivating event no longer present in payload", "eventID", id)

			eventLinked := entry.Entity
			eventLinked.Active = false
			eventLinked.PublishedOn = []string{}
			eventLinked.LastChange = time.Now().Format(time.RFC3339)

			err := t.contentClient.Put(ctx, "Event", eventLinked.Id, &eventLinked)
			if err != nil {
				slog.Debug("Put failed during deactivation, attempting Post as fallback", "err", err, "eventID", eventLinked.Id)
				err = t.contentClient.Post(ctx, "Event", map[string]string{"generateid": "false"}, &eventLinked)
				if err != nil {
					slog.Error("Failed to deactivate Event in ODH Core API", "err", err, "eventID", eventLinked.Id)
					if firstErr == nil {
						firstErr = err
					}
				} else {
					slog.Info("Successfully deactivated event (Post)", "eventID", eventLinked.Id)
				}
			} else {
				slog.Info("Successfully deactivated event (Put)", "eventID", eventLinked.Id)
			}
		}
	}
	return firstErr
}

// endedBeforeCrawlWindow reports whether the event ended before the first day the
// crawler fetches (momentus-events.silky.yaml: start = now -1d). Such events are no
// longer in the payload because they are past, so they must not be deactivated.
// Events without a parsable DateEnd are treated as not ended.
func endedBeforeCrawlWindow(e odhmodel.EventLinked, now time.Time) bool {
	if len(e.DateEnd) < 10 {
		return false
	}
	end, err := time.Parse("2006-01-02", e.DateEnd[:10])
	if err != nil {
		return false
	}
	windowStart, _ := time.Parse("2006-01-02", now.UTC().AddDate(0, 0, -1).Format("2006-01-02"))
	return end.Before(windowStart)
}

func processEvent(ctx context.Context, t *Transformer, event MomentusEvent, eventCache *clib.Cache[odhmodel.EventLinked]) error {
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
	if cachedEntry, ok := eventCache.Get(eventLinkedID); ok {
		baseEvent = &cachedEntry.Entity
	}

	eventLinked := ParseMomentusEvent(event, matchedVenue, baseEvent)
	if eventLinked == nil {
		slog.Info("Event skipped by parser (no languages)", "eventID", event.Id)
		return nil
	}

	hash, changed, hashErr := eventCache.HasChanged(eventLinked.Id, *eventLinked)
	if hashErr != nil {
		slog.Error("Failed to hash event", "err", hashErr, "eventID", event.Id)
		return hashErr
	}

	if !changed {
		return nil
	}

	err := t.contentClient.Put(ctx, "Event", eventLinked.Id, eventLinked)
	if err != nil {
		slog.Debug("Put failed, attempting Post as fallback", "err", err, "eventID", event.Id)
		err = t.contentClient.Post(ctx, "Event", map[string]string{"generateid": "false"}, eventLinked)
		if err != nil {
			slog.Error("Failed to push Event to ODH Core API (both Put and Post failed)", "err", err, "eventID", event.Id)
			return err
		} else {
			slog.Info("Successfully processed event and pushed to Core (Post)", "eventID", event.Id)
			eventCache.Set(eventLinked.Id, *eventLinked, hash)
		}
	} else {
		slog.Info("Successfully processed event and pushed to Core (Put)", "eventID", event.Id)
		eventCache.Set(eventLinked.Id, *eventLinked, hash)
	}

	return nil
}

// ----------------------------------------------------------------------------
// PARSER LOGIC
// ----------------------------------------------------------------------------

// DateBegin/DateEnd are not set: the API calculates them from the active EventDates and ignores sent values.
func ParseMomentusEvent(mevent MomentusEvent, venue *ODHVenue, base *odhmodel.EventLinked) *odhmodel.EventLinked {
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
	// The API trims the name; keep it identical so the change detection hash matches.
	eventLinked.Shortname = strings.TrimSpace(mevent.Name)
	eventLinked.Source = "momentus"

	eventLinked.Active = mevent.IsActive && !mevent.IsCanceled
	eventLinked.LastChange = time.Now().Format(time.RFC3339)

	// HasLanguage is calculated by the API (order de, en, it); a copied list would keep its old order
	eventLinked.HasLanguage = nil

	// Restore preserved fields
	eventLinked.ImageGallery = oldImageGallery
	eventLinked.TagIds = oldTagIds
	eventLinked.Documents = oldDocuments
	eventLinked.VideoItems = oldVideoItems

	details := buildDetailFromFunctions(mevent.Functions, mevent.Description, mevent.Name, base)
	if len(details) > 0 {
		eventLinked.Detail = details
	} else {
		return nil
	}

	if venue != nil && venue.Id != "" {
		eventLinked.VenueIds = []string{venue.Id}
	}

	var venueEventLocation string
	if venue != nil && venue.Mapping.Tag != nil {
		venueEventLocation = venue.Mapping.Tag["eventlocation"]
	}

	eventLinked.EventDate = buildEventDates(mevent, venue, mevent.BookedSpacesDetails, venueEventLocation)
	eventLinked.EventDate = buildDetailFromFunctionsForEventDates(mevent.Functions, eventLinked.EventDate)
	// After the SUBtitle matching, which compares the raw Momentus date/time values.
	normalizeEventDates(eventLinked.EventDate)

	if len(mevent.ContactRoles) > 0 {
		firstRole := mevent.ContactRoles[0]
		contact := buildContactInfo(firstRole)
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
		"start":         dateOnly(mevent.Start),
		"end":           dateOnly(mevent.End),
		"starttime":     mevent.StartTime,
		"endtime":       mevent.EndTime,
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
		// The API returns TagIds sorted; keep the same order so the hash matches.
		sort.Strings(tagIds)
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
		// Only event-wide functions without a specific room set the root title/subtitle
		if !fn.IsEventWide || fn.RoomId != "" {
			continue
		}

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

	if description != "" {
		for lang, d := range details {
			if d.BaseText == "" {
				d.BaseText = description
				details[lang] = d
			}
		}
	}

	// Languages, Titles and SubHeaders always come from Momentus, so removed ones are not kept.
	// Only an existing BaseText is restored, for languages Momentus still delivers without a description.
	if base != nil && base.Detail != nil {
		for lang, baseDetail := range base.Detail {
			d, ok := details[lang]
			if !ok {
				continue
			}

			if baseDetail.BaseText != "" && d.BaseText == "" {
				d.BaseText = baseDetail.BaseText
				details[lang] = d
			}
		}
	}

	return details
}

// buildDetailFromFunctionsForEventDates assigns SUBtitle functions to the Detail
// of the matching EventDate, found by date, time and room. As in the legacy
// importer, isEventWide is not checked: event-wide SUBtitles bound to a room
// are skipped for the root Detail and land here.
func buildDetailFromFunctionsForEventDates(functions []MomentusFunction, eventDates []odhmodel.EventDate) []odhmodel.EventDate {
	for _, fn := range functions {
		fnType := strings.TrimSpace(fn.FunctionTypeName)
		lang := ""
		if fnType == "EN SUBtitle" {
			lang = "en"
		} else if fnType == "DE SUBtitle" {
			lang = "de"
		} else if fnType == "IT SUBtitle" {
			lang = "it"
		}
		if lang == "" {
			continue
		}

		if fn.StartDate == "" {
			continue
		}
		endDate := fn.EndDate
		if endDate == "" {
			endDate = fn.StartDate
		}

		for i := range eventDates {
			ed := &eventDates[i]
			if ed.From != fn.StartDate || ed.To != endDate || ed.Begin != fn.StartTime || ed.End != fn.EndTime {
				continue
			}

			// A function without room matches the date in any room
			if fn.RoomId != "" {
				roomId := ""
				if mm, ok := ed.Mapping["momentus"]; ok {
					roomId = mm["roomId"]
				}
				if roomId != fn.RoomId {
					continue
				}
			}

			if ed.Detail == nil {
				ed.Detail = make(map[string]odhmodel.Detail)
			}
			d := ed.Detail[lang]
			d.Language = lang
			d.Title = fn.Name
			ed.Detail[lang] = d
			break
		}
	}

	return eventDates
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

// determineEventDatePublishedOn applies the same publisher rules as
// determinePublishedOn, but for the usage name of a single event date's
// booked space rather than the aggregated usage names of the whole event.
func determineEventDatePublishedOn(usageName string, venueEventLocation string) []string {
	usage := strings.ToUpper(strings.TrimSpace(usageName))
	if usage == "" || strings.Contains(usage, "PRIVATE") {
		return []string{}
	}

	effectiveType := ""
	if strings.Contains(usage, "PUBLIC") {
		effectiveType = "PUBLIC"
	} else if strings.Contains(usage, "VIDEOWALL") {
		effectiveType = "VIDEOWALL"
	} else if strings.Contains(usage, "ROOM") {
		effectiveType = "ROOM"
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

func buildEventDates(mevent MomentusEvent, venue *ODHVenue, bookedSpaces []MomentusBookedSpace, venueEventLocation string) []odhmodel.EventDate {
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

		momentusMapping := map[string]string{}
		if usageName != "" {
			momentusMapping["spaceUsageName"] = usageName
		}
		if space.RoomId != "" {
			momentusMapping["roomId"] = space.RoomId
		}
		if len(momentusMapping) > 0 {
			ed.Mapping = map[string]map[string]string{"momentus": momentusMapping}
		}

		ed.PublishedOn = determineEventDatePublishedOn(usageName, venueEventLocation)

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

// normalizeEventDates converts the Momentus date ("2006-01-02") and time ("15:04")
// values to the format the API stores and returns ("2006-01-02T00:00:00",
// "15:04:05"). Otherwise the record loaded from the API never hashes equal to the
// parsed one and every event is updated on every run. Values in any other format
// are left unchanged.
func normalizeEventDates(eventDates []odhmodel.EventDate) {
	for i := range eventDates {
		ed := &eventDates[i]
		ed.From = normalizeLayout(ed.From, "2006-01-02", "2006-01-02T15:04:05")
		ed.To = normalizeLayout(ed.To, "2006-01-02", "2006-01-02T15:04:05")
		ed.Begin = normalizeLayout(ed.Begin, "15:04", "15:04:05")
		ed.End = normalizeLayout(ed.End, "15:04", "15:04:05")
	}
}

func normalizeLayout(value, from, to string) string {
	parsed, err := time.Parse(from, value)
	if err != nil {
		return value
	}
	return parsed.Format(to)
}

// dateOnly cuts a Momentus date or datetime value to "2006-01-02", as the legacy importer does.
func dateOnly(value string) string {
	if len(value) > 10 {
		return value[:10]
	}
	return value
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

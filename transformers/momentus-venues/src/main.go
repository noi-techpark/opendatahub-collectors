package main

import (
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
	odhmodel "opendatahub.com/momentus-venues/odh-content-model"
)

var env struct {
	tr.Env
	ODHVenueAPI     string `envconfig:"ODH_VENUE_API" default:"https://tourism.api.opendatahub.testingmachine.eu/v1"`
	ODHVenueNoiID   string `envconfig:"ODH_VENUE_NOI_ID" default:"urn:venue:noi:6b3f0a14-3c5b-5d09-81f3-3ebe5b7885ea"`
	ODHVenueEuracID string `envconfig:"ODH_VENUE_EURAC_ID" default:"urn:venue:eurac:df155f71-5cea-5a29-9ebc-213fad6ac1eb"`
	OdhCoreUrl                 string `envconfig:"ODH_CORE_URL"`
	OdhCoreTokenUrl            string `envconfig:"ODH_CORE_TOKEN_URL"`
	OdhCoreTokenClientId       string `envconfig:"ODH_CORE_TOKEN_CLIENT_ID"`
	OdhCoreTokenClientSecret   string `envconfig:"ODH_CORE_TOKEN_CLIENT_SECRET"`
}

type Transformer struct {
	odhClient     *ODHClient
	contentClient clib.ContentAPI
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting Momentus Venues Transformer...")

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
		odhClient:     NewODHClient(env.ODHVenueAPI, ""),
		contentClient: contentClient,
	}

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		if r.Rawdata == "[]" {
			slog.Debug("Received empty array payload (end of stream), skipping")
			return nil
		}
		var room odhmodel.MomentusRoom
		if err := json.Unmarshal([]byte(r.Rawdata), &room); err != nil {
			slog.Error("Failed to unmarshal raw venue string", "err", err, "rawdata", r.Rawdata)
			return err
		}
		if room.Id == "" {
			slog.Warn("Received room without ID, skipping")
			return nil
		}

		// Determine Venue ID based on group
		var venueID string
		groupUpper := strings.ToUpper(room.Group)
		if strings.Contains(groupUpper, "NOI TECHPARK") {
			venueID = env.ODHVenueNoiID
		} else if strings.Contains(groupUpper, "EURAC") {
			venueID = env.ODHVenueEuracID
		} else {
			slog.Warn("Unknown room group, cannot match to venue", "roomID", room.Id, "group", room.Group)
			return nil
		}

		venue, err := t.odhClient.GetVenue(venueID)
		if err != nil {
			slog.Error("Failed to fetch venue from ODH", "venueID", venueID, "err", err)
			return err
		}
		if venue == nil {
			slog.Error("Venue not found in ODH", "venueID", venueID)
			return nil
		}

		venueLinked := ParseMomentusVenue(room, venue)

		err = t.contentClient.Put(ctx, "Venue", venueLinked.Id, venueLinked)
		if err != nil {
			slog.Debug("Put failed, attempting Post as fallback", "err", err, "venueID", venueLinked.Id)
			err = t.contentClient.Post(ctx, "Venue", nil, venueLinked)
			if err != nil {
				slog.Error("Failed to push Venue to ODH Core API (both Put and Post failed)", "err", err, "venueID", venueLinked.Id)
				return err
			}
		}

		slog.Info("Successfully processed room and pushed to Core", "roomID", room.Id, "venueID", venueLinked.Id)
		return nil
	})

	if err != nil {
		slog.Error("error while listening to queue", "err", err)
		os.Exit(1)
	}
}

// ----------------------------------------------------------------------------
// PARSER LOGIC
// ----------------------------------------------------------------------------

func ParseMomentusVenue(momentusRoom odhmodel.MomentusRoom, baseVenue *odhmodel.VenueV2) *odhmodel.VenueV2 {
	// We modify the venue in place/copy it
	venue := new(odhmodel.VenueV2)
	if baseVenue != nil {
		*venue = *baseVenue
	}
	if venue.RoomDetails == nil {
		venue.RoomDetails = []odhmodel.VenueRoomDetailsV2{}
	}

	// Normalize room name for matching
	shortnameToMatch := strings.ReplaceAll(momentusRoom.Name, "NOI - ", "")
	shortnameToMatch = strings.ReplaceAll(shortnameToMatch, "EURAC - ", "")

	var matchedRoom *odhmodel.VenueRoomDetailsV2
	var matchedIdx int = -1

	// Check if venue Room has all infos
	for i, r := range venue.RoomDetails {
		if r.Shortname == shortnameToMatch {
			matchedRoom = &venue.RoomDetails[i]
			matchedIdx = i
			break
		}
	}

	if matchedRoom == nil {
		matchedRoom = &odhmodel.VenueRoomDetailsV2{
			Detail: make(map[string]odhmodel.DetailGeneric),
		}
		matchedRoom.Detail["en"] = odhmodel.DetailGeneric{
			Language: "en",
			Title:    shortnameToMatch,
		}
	}

	matchedRoom.Active = momentusRoom.IsActive

	if momentusRoom.SquareFootage != nil {
		if matchedRoom.VenueRoomProperties == nil {
			matchedRoom.VenueRoomProperties = &odhmodel.VenueRoomProperties{}
		}
		matchedRoom.VenueRoomProperties.SquareMeters = momentusRoom.SquareFootage
	}

	if momentusRoom.MaxCapacity != nil {
		matchedRoom.MaxCapacity = momentusRoom.MaxCapacity
	}

	if matchedRoom.Mapping == nil {
		matchedRoom.Mapping = make(map[string]map[string]string)
	}

	momentusMapping := map[string]string{
		"name":               momentusRoom.Name,
		"isComboRoom":        fmt.Sprintf("%t", momentusRoom.IsComboRoom),
		"id":                 momentusRoom.Id,
		"group":              momentusRoom.Group,
		"subRoomIds":         strings.Join(momentusRoom.SubRoomIds, ","),
		"conflictingRoomIds": strings.Join(momentusRoom.ConflictingRoomIds, ","),
	}

	if momentusRoom.ItemCode != "" {
		momentusMapping["itemCode"] = momentusRoom.ItemCode
	}

	matchedRoom.Mapping["momentus"] = momentusMapping

	if matchedIdx == -1 {
		// New room, add to list
		// In C#, there's GenerateRoomDetailIds() which generates a new ID.
		// For our transformer, we should probably generate a UUID if Id is empty, but we'll leave it empty to let the API handle it if we want, or generate one.
		matchedRoom.Id = "urn:room:" + momentusRoom.Id // Use Momentus ID as temporary/permanent ID
		venue.RoomDetails = append(venue.RoomDetails, *matchedRoom)
	} else {
		// Update existing
		venue.RoomDetails[matchedIdx] = *matchedRoom
	}

	return venue
}

// ----------------------------------------------------------------------------
// CLIENT LOGIC
// ----------------------------------------------------------------------------

type ODHClient struct {
	BaseURL     string
	Token       string
	httpClient  *http.Client
	venuesCache map[string]*odhmodel.VenueV2
	mu          sync.Mutex
}

func NewODHClient(baseURL, token string) *ODHClient {
	return &ODHClient{
		BaseURL:     baseURL,
		Token:       token,
		httpClient:  &http.Client{Timeout: 30 * time.Second},
		venuesCache: make(map[string]*odhmodel.VenueV2),
	}
}

func (c *ODHClient) GetVenue(venueID string) (*odhmodel.VenueV2, error) {
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

	var venue odhmodel.VenueV2
	if err := json.NewDecoder(resp.Body).Decode(&venue); err != nil {
		return nil, err
	}

	c.mu.Lock()
	c.venuesCache[venueID] = &venue
	c.mu.Unlock()

	return &venue, nil
}

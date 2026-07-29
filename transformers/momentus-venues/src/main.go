// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	odhmodel "opendatahub.com/momentus-venues/odh-content-model"
)

var env struct {
	tr.Env
	VenueMapping             string `envconfig:"VENUE_MAPPING" default:"{\"NOI TECHPARK\":\"urn:venue:noi:6b3f0a14-3c5b-5d09-81f3-3ebe5b7885ea\",\"EURAC RESEARCH HQ\":\"urn:venue:eurac:df155f71-5cea-5a29-9ebc-213fad6ac1eb\"}"`
	OdhCoreUrl               string `envconfig:"ODH_CORE_URL"`
	OdhCoreTokenUrl          string `envconfig:"ODH_CORE_TOKEN_URL"`
	OdhCoreTokenClientId     string `envconfig:"ODH_CORE_TOKEN_CLIENT_ID"`
	OdhCoreTokenClientSecret string `envconfig:"ODH_CORE_TOKEN_CLIENT_SECRET"`
}

type Transformer struct {
	contentClient clib.ContentAPI
	venueMapping  map[string]string
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

	var venueMap map[string]string
	if err := json.Unmarshal([]byte(env.VenueMapping), &venueMap); err != nil {
		ms.FailOnError(context.Background(), err, "failed to parse VENUE_MAPPING")
	}

	t := &Transformer{
		contentClient: contentClient,
		venueMapping:  venueMap,
	}

	listener := tr.NewTr[string](context.Background(), env.Env)
	err = listener.Start(context.Background(), func(ctx context.Context, r *rdb.Raw[string]) error {
		if r.Rawdata == "[]" {
			slog.Debug("Received empty array payload (end of stream), skipping")
			return nil
		}
		var rooms []odhmodel.MomentusRoom
		if err := json.Unmarshal([]byte(r.Rawdata), &rooms); err != nil {
			slog.Error("Failed to unmarshal raw venue string", "err", err, "rawdata", r.Rawdata)
			return err
		}
		
		// Group rooms by venue
		roomsByVenue := make(map[string][]odhmodel.MomentusRoom)

		for _, room := range rooms {
			if room.Id == "" {
				slog.Warn("Received room without ID, skipping")
				continue
			}

			// Determine Venue ID based on group mapping
			var venueID string
			groupUpper := strings.ToUpper(room.Group)
			
			for mapKey, mapVal := range t.venueMapping {
				if strings.Contains(groupUpper, mapKey) {
					venueID = mapVal
					break
				}
			}

			if venueID == "" {
				slog.Warn("Unknown room group, cannot match to venue", "roomID", room.Id, "group", room.Group)
				continue
			}

			roomsByVenue[venueID] = append(roomsByVenue[venueID], room)
		}

		for venueID, groupedRooms := range roomsByVenue {
			var venueMap map[string]interface{}
			err := t.contentClient.Get(ctx, "Venue/"+venueID, nil, &venueMap)
			ms.FailOnError(ctx, err, "Fatal error: Configured Venue ID not found in ODH Content API", "venueID", venueID)

			// 1. Marshal back to JSON to unmarshal into VenueV2
			venueBytes, _ := json.Marshal(venueMap)
			var venueLinked odhmodel.VenueV2
			json.Unmarshal(venueBytes, &venueLinked)

			// 2. Process Rooms
			for _, room := range groupedRooms {
				venueLinkedPtr := ParseMomentusVenue(room, &venueLinked)
				venueLinked = *venueLinkedPtr
			}

			// 3. Overwrite ONLY RoomDetails in the original map
			venueMap["RoomDetails"] = venueLinked.RoomDetails

			// 4. Send map to ODH API
			err = t.contentClient.Put(ctx, "Venue", venueLinked.Id, &venueMap)
			if err != nil {
				slog.Debug("Put failed, attempting Post as fallback", "err", err, "venueID", venueLinked.Id)
				err = t.contentClient.Post(ctx, "Venue", map[string]string{"generateid": "false"}, &venueMap)
				if err != nil {
					slog.Error("Failed to push Venue to ODH Core API (both Put and Post failed)", "err", err, "venueID", venueLinked.Id)
					continue
				}
			}

			slog.Info("Successfully processed grouped rooms and pushed to Core", "venueID", venueLinked.Id, "roomCount", len(groupedRooms))
		}
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
		matchedRoom.Id = "urn:room:" + momentusRoom.Id // Use Momentus ID as temporary/permanent ID
		venue.RoomDetails = append(venue.RoomDetails, *matchedRoom)
	} else {
		// Update existing
		venue.RoomDetails[matchedIdx] = *matchedRoom
	}

	return venue
}

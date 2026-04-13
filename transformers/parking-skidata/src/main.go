// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
)

const (
	SOURCE      = "skidata"
	ID_TEMPLATE = "urn:stations:skidata"
)

const (
	stationTypeParent = "ParkingFacility"
	stationType       = "ParkingStation"

	shortStay   = "short_stay"
	subscribers = "subscribers"

	dataTypeFreeShort     = "free_" + shortStay
	dataTypeFreeSubs      = "free_" + subscribers
	dataTypeFreeTotal     = "free"
	dataTypeOccupiedShort = "occupied_" + shortStay
	dataTypeOccupiedSubs  = "occupied_" + subscribers
	dataTypeOccupiedTotal = "occupied"
)

// categoryDataTypes maps countingCategoryId → (free dataType, occupied dataType).
// Hardcoded from Skidata cc.json convention:
//
//	1 = SostaBreve  (short stay)
//	2 = Abbonati    (subscribers)
//	3 = Totale      (total)
var categoryDataTypes = map[int]struct {
	free     string
	occupied string
}{
	1: {dataTypeFreeShort, dataTypeOccupiedShort},
	2: {dataTypeFreeSubs, dataTypeOccupiedSubs},
	3: {dataTypeFreeTotal, dataTypeOccupiedTotal},
}

var env tr.Env
var stations Stations

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting parking-skidata transformer...")

	b := bdplib.FromEnv(bdplib.BdpEnv{
		BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
		BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
		BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
		BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
		BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),
		BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
		BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
	})
	defer tel.FlushOnPanic()

	stations = ReadStations("../resources/stations.csv")

	slog.Info("Syncing data types on startup")
	err := syncDataTypes(b)
	ms.FailOnError(context.Background(), err, "failed to sync types")

	slog.Info("Starting transformer listener...")

	listener := tr.NewTr[ParkingEvent](context.Background(), env)
	err = listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[ParkingEvent] {
	return func(ctx context.Context, payload *rdb.Raw[ParkingEvent]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[ParkingEvent]) error {
	event := payload.Rawdata
	ts := payload.Timestamp.UnixMilli()

	slog.Info("Processing parking event",
		"facilityNr", event.Carpark.FacilityNr,
		"carparkId", event.Carpark.Id,
		"category", event.CountingCategoryId,
		"level", event.Level, "capacity", event.Capacity)

	parentProviderID := strconv.Itoa(event.Carpark.FacilityNr)
	childProviderID := fmt.Sprintf("%d_%d", event.Carpark.FacilityNr, event.Carpark.Id)

	parentData := stations.GetStationByID(parentProviderID)
	if parentData == nil {
		return fmt.Errorf("no parent station metadata for facility %q", parentProviderID)
	}
	childData := stations.GetStationByID(childProviderID)
	if childData == nil {
		return fmt.Errorf("no station metadata for %q", childProviderID)
	}

	parentID := clib.GenerateID(ID_TEMPLATE, parentProviderID)
	childID := clib.GenerateID(ID_TEMPLATE, childProviderID)

	parent := bdplib.CreateStation(
		parentID, parentData.Name,
		stationTypeParent, parentData.Lat, parentData.Lon, bdp.GetOrigin())
	parent.MetaData = parentData.ToMetadata()
	parent.MetaData["provider_id"] = parentProviderID

	child := bdplib.CreateStation(
		childID, childData.Name,
		stationType, childData.Lat, childData.Lon, bdp.GetOrigin())
	child.ParentStation = parent.Id
	child.MetaData = childData.ToMetadata()
	child.MetaData["provider_id"] = childProviderID

	dtypes, ok := categoryDataTypes[event.CountingCategoryId]
	if !ok {
		return fmt.Errorf("unknown countingCategoryId %d", event.CountingCategoryId)
	}

	free := event.Capacity - event.Level
	occupied := event.Level

	dataMap := bdp.CreateDataMap()
	dataMap.AddRecord(child.Id, dtypes.free, bdplib.CreateRecord(ts, free, 600))
	dataMap.AddRecord(child.Id, dtypes.occupied, bdplib.CreateRecord(ts, occupied, 600))

	if err := bdp.SyncStations(stationTypeParent, []bdplib.Station{parent}, true, false); err != nil {
		return fmt.Errorf("failed to sync parent station: %w", err)
	}
	if err := bdp.SyncStations(stationType, []bdplib.Station{child}, true, false); err != nil {
		return fmt.Errorf("failed to sync child station: %w", err)
	}
	if err := bdp.PushData(stationType, dataMap); err != nil {
		return fmt.Errorf("failed to push data: %w", err)
	}
	return nil
}

func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{
		bdplib.CreateDataType(dataTypeFreeShort, "", "Amount of free 'short stay' parking slots", "Instantaneous"),
		bdplib.CreateDataType(dataTypeFreeSubs, "", "Amount of free 'subscribed' parking slots", "Instantaneous"),
		bdplib.CreateDataType(dataTypeFreeTotal, "", "Amount of free parking slots", "Instantaneous"),
		bdplib.CreateDataType(dataTypeOccupiedShort, "", "Amount of occupied 'short stay' parking slots", "Instantaneous"),
		bdplib.CreateDataType(dataTypeOccupiedSubs, "", "Amount of occupied 'subscribed' parking slots", "Instantaneous"),
		bdplib.CreateDataType(dataTypeOccupiedTotal, "", "Amount of occupied parking slots", "Instantaneous"),
	}
	return bdp.SyncDataTypes(dataTypes)
}

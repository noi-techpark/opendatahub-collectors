// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/relvacode/iso8601"
)

// Global Configuration
const period = 60                  // Period is 60 seconds (1 minute), as configured in the Java app
const stationtype = "ParkingSpace" // Using a sensible type name based on the data
const STATION_DATATYPE_FREE = "free"
const STATION_DATATYPE_OCCUPIED = "occupied"

// Global environment variable for the microservice
var env tr.Env

// --- Data Structures for Unmarshalling ---

// MqttParkingPayload represents the JSON object inside the rawdata.Payload string.
// This structure is derived from OnStreetParkingDataMqqtConnector.java's parseMessage logic.
type MqttParkingPayload struct {
	Type string `json:"type"`
	Data struct {
		GUID       string       `json:"guid"`
		Name       string       `json:"name"`
		State      string       `json:"state"` // "free" or "occupied"
		LastChange iso8601.Time `json:"last_change"`
		Position   struct {
			Latitude  float64 `json:"latitude"`
			Longitude float64 `json:"longitude"`
		} `json:"position"`
	} `json:"data"`
}

type RawDocument struct {
	MsgId   int    `json:"MsgId"`
	Topic   string `json:"Topic"`
	Payload string `json:"Payload"` // This holds the MqttParkingPayload as a JSON string
}

// SetupDataTypes returns the list of required data types.
// This replicates the logic in OnStreetParkingSensorService.setupDataType.
func SetupDataTypes() []bdplib.DataType {
	return []bdplib.DataType{
		bdplib.CreateDataType(STATION_DATATYPE_FREE, "", "Amount of free parking slots", "Instantaneous"),
		bdplib.CreateDataType(STATION_DATATYPE_OCCUPIED, "", "Amount of occupied parking slots", "Instantaneous"),
	}
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[RawDocument] {
	return func(ctx context.Context, payload *rdb.Raw[RawDocument]) error {
		return Transform(ctx, bdp, payload)
	}
}

func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[RawDocument]) error {
	var mqttPayload MqttParkingPayload
	if err := json.Unmarshal([]byte(payload.Rawdata.Payload), &mqttPayload); err != nil {
		return fmt.Errorf("error parsing MQTT payload JSON: %w", err)
	}

	// Perform the transformation
	// Extract necessary fields
	guid := mqttPayload.Data.GUID
	name := mqttPayload.Data.Name
	state := mqttPayload.Data.State
	lastChangeMillis := mqttPayload.Data.LastChange.UnixMilli()

	// 2. Create the Station DTO
	station := bdplib.CreateStation(
		guid,
		name,
		stationtype,
		mqttPayload.Data.Position.Latitude,
		mqttPayload.Data.Position.Longitude,
		bdp.GetOrigin(),
	)

	enhancement := StationProto.GetStationByGUID(guid)
	if enhancement != nil {
		station.MetaData = map[string]interface{}{
			"group":        enhancement.Group,
			"municipality": enhancement.Municipality,
		}
	} else {
		slog.Warn("guid without enhancement data", "guid", guid)
	}

	// 3. Determine the 'free' and 'occupied' values based on state (Replicating value logic in OnStreetParkingSensorService.applyParkingData)
	var freeValue *float64
	var occupiedValue *float64

	switch state {
	case "free":
		// Set Free=1, Occupied=0
		freeVal := float64(1)
		occupiedVal := float64(0)
		freeValue = &freeVal
		occupiedValue = &occupiedVal
	case "occupied":
		// Set Free=0, Occupied=1
		freeVal := float64(0)
		occupiedVal := float64(1)
		freeValue = &freeVal
		occupiedValue = &occupiedVal
	default:
		// Handle unexpected state (mirroring robust error handling)
		return fmt.Errorf("unknown parking state: %s for guid: %s", state, guid)
	}

	// 4. Create the DataMap and add records (Replicating DataMapDto and SimpleRecordDto creation)
	dm := bdp.CreateDataMap()

	// Add 'free' record: dataMap.addRecord(guid, STATION_DATATYPE_FREE, new SimpleRecordDto(..., freeValue, period))
	dm.AddRecord(guid, STATION_DATATYPE_FREE, bdplib.CreateRecord(lastChangeMillis, freeValue, period))

	// Add 'occupied' record: dataMap.addRecord(guid, STATION_DATATYPE_OCCUPIED, new SimpleRecordDto(..., occupiedValue, period))
	dm.AddRecord(guid, STATION_DATATYPE_OCCUPIED, bdplib.CreateRecord(lastChangeMillis, occupiedValue, period))

	// 3. Station Synchronization (Replicating jsonPusher.syncStations in OnStreetParkingSensorService.applyParkingData)
	if err := bdp.SyncStations(stationtype, []bdplib.Station{station}, true, true); err != nil {
		return fmt.Errorf("error syncing station %s: %w", station.Id, err)
	}

	// 4. Data Push (Replicating jsonPusher.pushData in OnStreetParkingSensorService.applyParkingData)
	if err := bdp.PushData(stationtype, dm); err != nil {
		return fmt.Errorf("error pushing data: %w", err)
	}

	slog.Info("Processed", "strationcode", station.Id)
	return nil
}

// --- Main Application Entrypoint ---

var StationProto Stations = nil

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)

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

	StationProto = ReadStations("resources/stations.csv")

	// 1. Sync Data Types
	dts := SetupDataTypes()
	ms.FailOnError(ctx, b.SyncDataTypes(dts), "error pushing datatypes")
	slog.Info("Successfully synced datatypes")

	// 2. Start the Transformer Listener
	listener := tr.NewTr[RawDocument](context.Background(), env)

	err := listener.Start(context.Background(), TransformWithBdp(b))
	ms.FailOnError(context.Background(), err, "error while listening to queue")
}

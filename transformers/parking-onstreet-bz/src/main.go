// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/gocarina/gocsv"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
)

var cfg struct {
	tr.Env
}

type MqttRaw struct {
	MsgId   uint16
	Topic   string
	Payload string
}

type Payload struct {
	DeviceInfo struct {
		DevEui string
	}
	Time string
	Data string
}

var dtOccupied = bdplib.CreateDataType("occupied", "", "occupied", "Instantaneous")

const ParkingSensor = "ParkingSensor"

func main() {
	envconfig.MustProcess("", &cfg)
	ms.InitLog(cfg.LOG_LEVEL)

	b := bdplib.FromEnv()

	// DATATYPES
	err := b.SyncDataTypes(ParkingSensor, []bdplib.DataType{dtOccupied})
	ms.FailOnError(err, "error syncing datatypes")

	// STATIONS
	stations := []bdplib.Station{}
	for _, s := range readStations() {
		stations = append(stations, bdplib.Station{
			Id:          s.Stationcode,
			Name:        s.Description,
			Latitude:    s.Latitude,
			Longitude:   s.Longitude,
			Origin:      b.Origin,
			StationType: ParkingSensor,
			MetaData: map[string]any{
				"id2":          s.Id2,
				"group":        s.Group,
				"municipality": s.Municipality,
			},
		})
	}
	ms.FailOnError(b.SyncStations(ParkingSensor, stations, true, true), "failed syncing stations")

	// LISTEN FOR MEASUREMENTS
	tr.ListenFromEnv(cfg.Env, func(r *dto.Raw[MqttRaw]) error {
		var p Payload
		if err := json.Unmarshal([]byte(r.Rawdata.Payload), &p); err != nil {
			return fmt.Errorf("unexpected payload format: %w", err)
		}

		s, err := decodeStatus(p.Data)
		if err != nil {
			return err
		}

		// s <= 0 means unknown packet format, which we ignore
		if s > 0 {
			m := b.CreateDataMap()

			t, err := parseTime(p.Time)
			if err != nil {
				return err
			}

			occupied := 1
			// 1 means it's free
			if s == 1 {
				occupied = 2
			}

			m.AddRecord(p.DeviceInfo.DevEui, dtOccupied.Name, bdplib.CreateRecord(t.UnixMilli(), occupied, 1))

			if err := b.PushData(ParkingSensor, m); err != nil {
				return fmt.Errorf("could not push data to bdp: %w", err)
			}
		} else {
			slog.Warn("Unknown packet format. ignoring...", "data", p.Data)
		}

		return nil
	})
}

type Station struct {
	Stationcode  string
	Id2          string
	Group        string
	Longitude    float64
	Latitude     float64
	Description  string
	Municipality string
}

func readStations() []Station {
	f, err := os.Open("stations.csv")
	ms.FailOnError(err, "failed reading stations csv")
	defer f.Close()

	stations := []Station{}

	ms.FailOnError(gocsv.Unmarshal(f, &stations), "failed unmarshalling csv")
	return stations
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339, s)
}

func isMeasurePacket(b []byte) bool {
	return b[0] == 0x80 && b[6] == 0xF4
}
func isKeepalivePacket(b []byte) bool {
	return b[0] == 0x80 && b[6] == 0xF3
}

func decodeStatus(s string) (int, error) {
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		return 0, err
	}

	// refer to documentation for packet byte mapping.
	// This is the actual 1 byte long status field. It's 1 when free, and 2 when busy
	if isMeasurePacket(b) {
		return int(b[13]), nil
	} else if isKeepalivePacket(b) {
		return int(b[8]), nil
	} else {
		return -1, nil
	}
}

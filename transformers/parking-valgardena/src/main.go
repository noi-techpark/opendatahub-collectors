// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	"github.com/noi-techpark/go-timeseries-writer-client/bdplib"
)

const Station = "ParkingStation"
const Period = 120
const Origin = "GARDENA"
const DataType = "occupied"

var env struct {
	tr.Env

	MQ_META_QUEUE    string
	MQ_META_EXCHANGE string
	MQ_META_KEY      string
	MQ_META_CLIENT   string
}

type payloadMetaData struct {
	Uid      string `json:"id"`
	NameDE   string `json:"name_DE"`
	NameIT   string `json:"name_IT"`
	Lat      string `json:"latitude"`
	Long     string `json:"longitude"`
	Capacity int    `json:"capacity"`
}

type payloadMetaArray []payloadMetaData

type payloadData struct {
	Uid       string `json:"id"`
	Time      string `json:"timestamp"`
	Occupancy int    `json:"occupancy"`
}

func main() {
	envconfig.MustProcess("", &env)
	ms.InitLog(env.Env.LOG_LEVEL)

	b := bdplib.FromEnv()
	if b == nil {
		slog.Error("Failed to initialize BDP client")
		os.Exit(1)
	}

	rabbit, err := mq.Connect(env.Env.MQ_URI, env.Env.MQ_CLIENT)
	ms.FailOnError(err, "failed connecting to rabbitmq")
	defer rabbit.Close()

	dataMQ, err := rabbit.Consume(env.Env.MQ_EXCHANGE, env.Env.MQ_QUEUE, env.Env.MQ_KEY)
	ms.FailOnError(err, "failed creating data queue")

	go tr.HandleQueue(dataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		parkingData := b.CreateDataMap()
		payload, err := unmarshalRawdata[payloadData](r.Rawdata)
		if err != nil {
			slog.Error("cannot unmarshall raw data", "err", err)
			return err
		}
		parkingid := stationId(payload.Uid, Origin)
		parkingData.AddRecord(parkingid, DataType, bdplib.CreateRecord(r.Timestamp.UnixMilli(), payload.Occupancy, Period))
		if err := b.PushData(Station, parkingData); err != nil {
			slog.Error("error pushing parking occupancy data:", "err", err)
			return err
		}
		slog.Info("Updated parking station occupancy")
		return nil
	})

	metaDataMQ, err := rabbit.Consume(env.MQ_META_EXCHANGE, env.MQ_META_QUEUE, env.MQ_META_KEY)
	ms.FailOnError(err, "failed creating data queue")

	go tr.HandleQueue(metaDataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		payloadArray, err := unmarshalRawdata[payloadMetaArray](r.Rawdata)
		if err != nil {
			slog.Error("cannot unmarshall raw data", "err", err)
		}
		var stations []bdplib.Station
		for _, payload := range *payloadArray {
			parkingid := stationId(payload.Uid, b.Origin)
			lat, err := strconv.ParseFloat(payload.Lat, 64)
			if err != nil {
				slog.Error("cannot parse latitude", "err", err)
				continue
			}
			lon, err := strconv.ParseFloat(payload.Long, 64)
			if err != nil {
				slog.Error("cannot parse longitude", "err", err)
				continue
			}
			s := bdplib.CreateStation(parkingid, payload.NameIT, Station, lat, lon, Origin)

			MetaData := make(map[string]interface{})
			MetaData["name_DE"] = payload.NameDE
			MetaData["capacity"] = payload.Capacity

			s.MetaData = MetaData
			stations = append(stations, s)
		}

		if err := b.SyncStations(Station, stations, true, false); err != nil {
			slog.Error("Error syncing stations", "err", err)
		}

		slog.Info("Updated parking station occupancy")
		return nil
	})

	select {}
}

func stationId(id string, origin string) string {
	return fmt.Sprintf("%s:%s", origin, id)
}

func unmarshalRawdata[T any](s string) (*T, error) {
	var p T
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
	}
	return &p, nil
}

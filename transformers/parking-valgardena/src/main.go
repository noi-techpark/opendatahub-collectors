// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	//"fmt"

	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	//"strconv"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	//"go.starlark.net/lib/time"
)

const Station = "ParkingStation"
const Period = 120
const Origin = "GARDENA"
const DataTypeO = "occupied"

var env struct {
	tr.Env

	MQ_META_QUEUE    string
	MQ_META_EXCHANGE string
	MQ_META_KEY      string
	MQ_META_CLIENT   string

	//MQ_CONSUMER string
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
	failOnError(err, "failed connecting to rabbitmq")
	defer rabbit.Close()

	fmt.Println("ARRIVES HERE?")
	// rabbit.OnClose(func(err *amqp091.Error) {
	// 	slog.Error("rabbitmq connection closed unexpectedly")
	// 	panic(err)
	// })

	dataMQ, err := rabbit.Consume(env.Env.MQ_EXCHANGE, env.Env.MQ_QUEUE, env.Env.MQ_KEY)
	failOnError(err, "failed creating data queue")

	go tr.HandleQueue(dataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		fmt.Println("DATA FLOWING")
		fmt.Println(r.Rawdata)
		parkingData := b.CreateDataMap()
		payload, err := unmarshalRawdata[payloadData](r.Rawdata)
		if err != nil {
			slog.Error("cannot unmarshall raw data", "err", err)
			return err
		}
		parkingid := stationId(payload.Uid, Origin)
		parkingData.AddRecord(parkingid, DataTypeO, bdplib.CreateRecord(r.Timestamp.UnixMilli(), payload.Occupancy, Period))
		if err := b.PushData(Station, parkingData); err != nil {
			return fmt.Errorf("error pushing parking occupancy data: %w", err)
		}
		slog.Info("Updated parking station occupancy")
		return nil

	})

	metaDataMQ, err := rabbit.Consume(env.MQ_META_EXCHANGE, env.MQ_META_QUEUE, env.MQ_META_KEY)
	failOnError(err, "failed creating data queue")

	go tr.HandleQueue(metaDataMQ, env.Env.MONGO_URI, func(r *dto.Raw[string]) error {
		fmt.Println("META DATA FLOWING")
		fmt.Println(r.Rawdata)
		//parkingMetaData := b.CreateDataMap()
		payloadArray, _ := unmarshalRawdata[payloadMetaArray](r.Rawdata)

		for _, payload := range *payloadArray {
			parkingid := stationId(payload.Uid, b.Origin)
			lat, _ := strconv.ParseFloat(payload.Lat, 64)
			lon, _ := strconv.ParseFloat(payload.Long, 64)
			s := bdplib.CreateStation(parkingid, payload.NameIT, Station, lat, lon, Origin)

			MetaData := make(map[string]interface{})
			MetaData["name_DE"] = payload.NameDE
			MetaData["capacity"] = payload.Capacity

			s.MetaData = MetaData
			if err := b.SyncStations(Station, []bdplib.Station{s}, false, false); err != nil {
				slog.Error("Error syncing stations", "err", err)
			}

		}
		slog.Info("Updated parking station occupancy")
		return nil
	})

	select {}
}

func failOnError(err error, msg string) {
	if err != nil {
		slog.Error(msg, "err", err)
		panic(err)
	}
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

// if err := b.PushData(Vehicle, dm); err != nil {
// 	slog.Error("Error pushing data to bdp", "err", err, "msg", msgBody)
// 	msgReject(&msg)
// }

// failOnError(msg.Ack(false), "Could not ACK elaborated msg")
// log.Fatal("Message channel closed!")

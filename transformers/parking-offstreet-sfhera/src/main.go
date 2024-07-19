// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"log/slog"
	"os"
	"strconv"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// read logger level from env and uses INFO as default
func initLogging() {
	logLevel := os.Getenv("LOG_LEVEL")

	level := new(slog.LevelVar)
	level.UnmarshalText([]byte(logLevel))

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("Start logger with level: " + logLevel)
}
func failOnError(err error, msg string) {
	if err != nil {
		slog.Error(msg, "err", err)
		panic(err)
	}
}

const ParkingStation = "ParkingStation"

func main() {
	initLogging()

	conn, err := amqp091.Dial(os.Getenv("MQ_LISTEN_URI"))
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclare(os.Getenv("MQ_LISTEN_QUEUE"), true, false, false, false, nil)
	failOnError(err, "Failed to declare a queue")
	err = ch.QueueBind(q.Name, os.Getenv("MQ_LISTEN_KEY"), os.Getenv("MQ_LISTEN_EXCHANGE"), false, nil)
	failOnError(err, "Failed binding queue to exchange")
	mq, err := ch.Consume(q.Name, os.Getenv("MQ_LISTEN_CONSUMER"), false, false, false, false, nil)
	failOnError(err, "Failed to register a consumer")

	go func() {
		b := bdplib.FromEnv()

		dtFree := bdplib.CreateDataType("free", "", "free", "Instantaneous")
		dtOccupied := bdplib.CreateDataType("occupied", "", "occupied", "Instantaneous")
		dtEnter := bdplib.CreateDataType("entering-vehicles-today", "", "Number of vehicles that entered since start of day", "Instantaneous")
		dtExit := bdplib.CreateDataType("exiting-vehicles-today", "", "Number of vehicles that exited since start of day", "Instantaneous")

		ds := []bdplib.DataType{dtFree, dtOccupied, dtEnter, dtExit}
		failOnError(b.SyncDataTypes(ParkingStation, ds), "Error pushing datatypes")

		for msg := range mq {
			log.Printf("Received a message: %s", msg.Body)

			msgBody := incoming{}
			if err := json.Unmarshal(msg.Body, &msgBody); err != nil {
				slog.Error("Error unmarshalling mq message", "err", err)
				msgReject(&msg)
				continue
			}

			rawFrame, err := getRawFrame(msgBody)
			if err != nil {
				slog.Error("Cannot get mongo raw data", "err", err, "msg", msgBody)
				msgReject(&msg)
				continue
			}

			raw, err := unmarshalRaw(rawFrame.Rawdata)
			if err != nil {
				slog.Error("Unable to unmarshal raw payload", "err", err, "msg", msgBody, "raw", raw)
				msgReject(&msg)
				continue
			}

			lat, _ := strconv.ParseFloat(raw.Lat, 64)
			lon, _ := strconv.ParseFloat(raw.Long, 64)

			sname := fmt.Sprintf("parking-bz:%s:%s", raw.Uid, raw.Floor)
			s := bdplib.CreateStation(sname, raw.Park, ParkingStation, lat, lon, b.Origin)

			tot, _ := strconv.Atoi(raw.Tot)
			floor, _ := strconv.Atoi(raw.Floor)
			s.MetaData = map[string]any{
				"floor":        floor,
				"capacity":     tot,
				"municipality": "Bolzano - Bozen",
			}
			if err := b.SyncStations(ParkingStation, []bdplib.Station{s}, true, false); err != nil {
				slog.Error("Error syncing stations", "err", err, "msg", msgBody)
				msgReject(&msg)
				continue
			}

			dm := b.CreateDataMap()
			dm.AddRecord(s.Id, dtFree.Name, bdplib.CreateRecord(rawFrame.Timestamp.UnixMilli(), raw.Lots, 300))
			dm.AddRecord(s.Id, dtOccupied.Name, bdplib.CreateRecord(rawFrame.Timestamp.UnixMilli(), tot-raw.Lots, 300))
			in, _ := strconv.Atoi(raw.In)
			dm.AddRecord(s.Id, dtEnter.Name, bdplib.CreateRecord(rawFrame.Timestamp.UnixMilli(), in, 300))
			out, _ := strconv.Atoi(raw.Out)
			dm.AddRecord(s.Id, dtExit.Name, bdplib.CreateRecord(rawFrame.Timestamp.UnixMilli(), out, 300))

			if err := b.PushData(ParkingStation, dm); err != nil {
				slog.Error("Error pushing data to bdp", "err", err, "msg", msgBody)
				msgReject(&msg)
				continue
			}

			failOnError(msg.Ack(false), "Could not ACK elaborated msg")
		}
		log.Fatal("Message channel closed!")
	}()

	<-make(chan int) //wait forever
}

func getRawFrame(m incoming) (*raw, error) {
	raw, err := getMongo(m)
	if err != nil {
		return nil, fmt.Errorf("error getting raw from mongo: %w", err)
	}

	slog.Debug("Dumping raw data", "dto", raw)
	return raw, nil
}

func msgReject(d *amqp091.Delivery) {
	if err := d.Reject(false); err != nil {
		slog.Error("error rejecting already errored message", "err", err)
		panic(err)
	}
}

type payload struct {
	Uid      string
	Park     string
	Lat      string
	Long     string
	In       string
	Out      string
	Floor    string
	Lots     int
	Tot      string
	Reserved string
}

func unmarshalRaw(s string) (payload, error) {
	var p payload
	decoded, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		slog.Error("Debug failed base64", "string", s)
		return p, fmt.Errorf("error decoding raw from base64: %w", err)
	}
	if err := json.Unmarshal(decoded, &p); err != nil {
		return p, fmt.Errorf("error unmarshalling payload json: %w", err)
	}

	return p, nil
}

type raw struct {
	Provider  string
	Timestamp time.Time
	Rawdata   string
	ID        string
}
type incoming struct {
	Id         string
	Db         string
	Collection string
}

func getMongo(m incoming) (*raw, error) {
	c, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(os.Getenv("MONGO_URI")))
	if err != nil {
		return nil, err
	}
	defer c.Disconnect(context.TODO())
	id, err := primitive.ObjectIDFromHex(m.Id)
	if err != nil {
		return nil, err
	}
	r := &raw{}
	if err := c.Database(m.Db).Collection(m.Collection).FindOne(context.TODO(), bson.M{"_id": id}).Decode(r); err != nil {
		return nil, err
	}
	return r, nil
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"log"
	"log/slog"
	"os"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"opendatahub.com/tr-parking-offstreet-sfhera/bdplib"
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

func dataTypes() []bdplib.DataType {
	ds := []bdplib.DataType{
		bdplib.CreateDataType("free", "", "free", "Instantaneous"),
		bdplib.CreateDataType("entering-vehicles", "", "Number of vehicles that entered the parking station", "Instananteous"),
		bdplib.CreateDataType("exiting-vehicles", "", "Number of vehicles that exited the parking station", "Instananteous"),
	}
	return ds
}

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
	msgs, err := ch.Consume(q.Name, os.Getenv("MQ_LISTEN_CONSUMER"), false, false, false, false, nil)
	failOnError(err, "Failed to register a consumer")

	go func() {
		b := bdplib.FromEnv()
		failOnError(b.SyncDataTypes(ParkingStation, dataTypes()), "Error pushing datatypes")

		for d := range msgs {
			log.Printf("Received a message: %s", d.Body)
			// Get raw data from mongo
			m := incoming{}
			if err := json.Unmarshal(d.Body, &m); err != nil {
				slog.Error("Error unmarshalling mq message", "err", err)
				msgReject(&d)
			}
			raw, err := getMongo(m)
			if err != nil {
				slog.Error("Error getting raw from mongo", "err", err)
				msgReject(&d)
			}

			slog.Debug("Dumping raw data", "dto", raw)

			decoded, err := base64.StdEncoding.DecodeString(raw.Rawdata)
			if err != nil {
				slog.Error("Error decoding raw payload from base64", "err", err)
				msgReject(&d)
			}
			var payload payload
			if err := json.Unmarshal(decoded, &payload); err != nil {
				slog.Error("Error unmarshalling payload to json dto", "err", err)
				msgReject(&d)
			}

			slog.Debug("Decoded payload", "payload", payload)

			// push bdp
			failOnError(d.Nack(false, true), "Could not ACK elaborated msg")

		}
		log.Fatal("Message channel closed!")
	}()

	<-make(chan int) //wait forever
}

func msgReject(d *amqp091.Delivery) {
	if err := d.Reject(false); err != nil {
		slog.Error("Error rejecting already errored message", "err", err)
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

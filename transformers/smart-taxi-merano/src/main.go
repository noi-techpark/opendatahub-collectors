// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	//"fmt"
	"context"
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

func failOnError(err error, msg string) {
	if err != nil {
		log.Fatalf("%s: %s", msg, err)
	}
}

const Vehicle = "Vehicle"
const Period = 60
const Origin = "smart-taxi-merano"

func main() {
	// Read environment variables
	mqURI := os.Getenv("MQ_LISTEN_URI")
	conn, err := amqp091.Dial(mqURI)
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	mqQueue := os.Getenv("MQ_LISTEN_QUEUE")

	q, err := ch.QueueDeclare(
		mqQueue, // name
		true,    // durable
		false,   // delete when unused
		false,   // exclusive
		false,   // no-wait
		nil,     // arguments
	)

	failOnError(err, "Failed to declare a queue")

	mqKey := os.Getenv("MQ_LISTEN_KEY")
	mqExchange := os.Getenv("MQ_LISTEN_EXCHANGE")
	if mqExchange == "" {
		log.Fatal("MQ_LISTEN_EXCHANGE environment variable is not set")
	}
	err = ch.QueueBind(
		q.Name,     // queue name
		mqKey,      // routing key
		mqExchange, // exchange
		false,
		nil)
	failOnError(err, "Failed to bind queue to exchange")

	mqConsumer := os.Getenv("MQ_LISTEN_CONSUMER")

	msgs, err := ch.Consume(
		q.Name,     // queue
		mqConsumer, // consumer
		true,       // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)

	failOnError(err, "Failed to register a consumer")

	//from here on it might be better to run everything with go fun() (meaning within a go routine)
	go func() {
		b := bdplib.FromEnv()
		dtState := bdplib.CreateDataType("state", "", "description", "Instantaneous")
		dtPosition := bdplib.CreateDataType("position", "", "description", "Instantaneous")
		ds := []bdplib.DataType{dtState, dtPosition}
		failOnError(b.SyncDataTypes(Vehicle, ds), "Error pushing datatypes")
		log.Println("Waiting for messages. To exit press CTRL+C")

		for msg := range msgs {
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

			log.Printf("Received a message: %s", rawFrame.Rawdata)
			rawArray, err := unmarshalRaw(rawFrame.Rawdata)
			if err != nil {
				slog.Error("Unable to unmarshal raw payload", "err", err, "msg", msgBody, "raw", rawArray)
				msgReject(&msg)
				continue
			}

			for _, raw := range rawArray {
				lat, _ := strconv.ParseFloat(raw.Lat, 64)
				lon, _ := strconv.ParseFloat(raw.Long, 64)
				sname := fmt.Sprintf("vehicle:%s", raw.Uid)
				s := bdplib.CreateStation(sname, raw.Nickname, Vehicle, lat, lon, Origin)

				if err := b.SyncStations(Vehicle, []bdplib.Station{s}, false, false); err != nil {
					slog.Error("Error syncing stations", "err", err, "msg", msgBody)
					msgReject(&msg)
					continue
				}

				latLongMap := map[string]string{
					"lat": raw.Lat,
					"lon": raw.Long,
				}

				dm := b.CreateDataMap()
				dm.AddRecord(s.Id, dtState.Name, bdplib.CreateRecord(rawFrame.Timestamp.UnixMilli(), raw.State, Period))
				dm.AddRecord(s.Id, dtPosition.Name, bdplib.CreateRecord(rawFrame.Timestamp.UnixMilli(), latLongMap, Period))

				if err := b.PushData(Vehicle, dm); err != nil {
					slog.Error("Error pushing data to bdp", "err", err, "msg", msgBody)
					msgReject(&msg)
					continue
				}
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
	Uid      string `json:"IdUtente"`
	Nickname string `json:"Nickname"`
	State    string `json:"Stato"`
	Lat      string `json:"Latitudine"`
	Long     string `json:"Longitudine"`
	Time     string `json:"OraComunicazione"`
}

type payloadArray []payload

func unmarshalRaw(s string) (payloadArray, error) {
	var p payloadArray
	if err := json.Unmarshal([]byte(s), &p); err != nil {
		return nil, fmt.Errorf("error unmarshalling payload json: %w", err)
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

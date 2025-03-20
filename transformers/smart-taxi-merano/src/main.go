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

	"github.com/noi-techpark/go-timeseries-writer-client/bdplib"
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

func contains(whitelist []int, value int) bool {
	for _, item := range whitelist {
		if item == value {
			return true
		}
	}
	return false
}

var Whitelist = []int{2343, 2344, 2345, 2350, 2764}

const Vehicle = "ON_DEMAND_VEHICLE"
const Period = 60
const Origin = "smart-taxi-merano"

func mapStatus(status string) string {
	m := map[string]string{
		"1": "FREE",
		"2": "OCCUPIED",
		"3": "AVAILABLE",
	}
	val, ok := m[status]
	if ok {
		return val
	}
	return "undefined status"
}

func initLogging() {
	logLevel := os.Getenv("LOG_LEVEL")

	level := new(slog.LevelVar)
	level.UnmarshalText([]byte(logLevel))

	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))

	slog.Info("Start logger with level: " + logLevel)
}

func main() {
	initLogging()
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
		false,      // auto-ack
		false,      // exclusive
		false,      // no-local
		false,      // no-wait
		nil,        // args
	)

	failOnError(err, "Failed to register a consumer")

	b := bdplib.FromEnv()
	dtState := bdplib.CreateDataType("state", "", "state", "Instantaneous")
	dtPosition := bdplib.CreateDataType("position", "", "position", "Instantaneous")
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

		dm := b.CreateDataMap()
		for _, raw := range rawArray {
			num, _ := strconv.Atoi(raw.Uid)
			if contains(Whitelist, num) {
				fmt.Println("INSERTING RAW_ID", raw.Uid)
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
				state := mapStatus(raw.State)
				parsedTime, err := time.Parse("02/01/2006 15:04:05", raw.Time)
				if err != nil {
					slog.Error("Error parsing time", "err", err, "raw_time", raw.Time)
					// Handle the error appropriately
				}

				//substituted raw.state with an int version
				dm.AddRecord(s.Id, dtState.Name, bdplib.CreateRecord(parsedTime.UnixMilli(), state, Period))
				dm.AddRecord(s.Id, dtPosition.Name, bdplib.CreateRecord(parsedTime.UnixMilli(), latLongMap, Period))
			}
		}

		if err := b.PushData(Vehicle, dm); err != nil {
			slog.Error("Error pushing data to bdp", "err", err, "msg", msgBody)
			msgReject(&msg)
		}

		failOnError(msg.Ack(false), "Could not ACK elaborated msg")
	}

	log.Fatal("Message channel closed!")

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

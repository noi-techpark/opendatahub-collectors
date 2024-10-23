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
	"strconv"
	"time"

	"github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type raw struct {
	Provider  string
	Timestamp time.Time
	Rawdata   payload
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

func listen(handler func(*raw) error) {
	conn, err := amqp091.Dial(os.Getenv("MQ_LISTEN_URI"))
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	prefetch := 10
	if s, found := os.LookupEnv("MQ_LISTEN_QOS_PREFETCH_COUNT"); found {
		prefetch, err = strconv.Atoi(s)
		failOnError(err, fmt.Sprintf("Invalid prefetch setting: %s", s))
	}
	ch.Qos(prefetch, 0, true)

	q, err := ch.QueueDeclare(os.Getenv("MQ_LISTEN_QUEUE"), true, false, false, false, nil)
	failOnError(err, "Failed to declare a queue")
	err = ch.QueueBind(q.Name, os.Getenv("MQ_LISTEN_KEY"), os.Getenv("MQ_LISTEN_EXCHANGE"), false, nil)
	failOnError(err, "Failed binding queue to exchange")
	mq, err := ch.Consume(q.Name, os.Getenv("MQ_LISTEN_CONSUMER"), false, false, false, false, nil)
	failOnError(err, "Failed to register a consumer")

	for msg := range mq {
		slog.Debug("Received a message", "body", msg.Body)

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

		err = handler(rawFrame)
		if err != nil {
			slog.Error("Error during handling of message", "err", err)
			msgReject(&msg)
			continue
		}

		failOnError(msg.Ack(false), "Could not ACK elaborated msg")
	}
}

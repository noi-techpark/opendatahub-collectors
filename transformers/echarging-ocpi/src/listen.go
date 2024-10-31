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
	"time"

	"github.com/rabbitmq/amqp091-go"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type raw[Rawtype any] struct {
	Provider  string
	Timestamp time.Time
	Rawdata   Rawtype
}
type incoming struct {
	Id         string
	Db         string
	Collection string
}

func getMongo[Rawtype any](m incoming) (*raw[Rawtype], error) {
	c, err := mongo.Connect(context.TODO(), options.Client().ApplyURI(os.Getenv("MONGO_URI")))
	if err != nil {
		return nil, err
	}
	defer c.Disconnect(context.TODO())
	id, err := primitive.ObjectIDFromHex(m.Id)
	if err != nil {
		return nil, err
	}
	r := &raw[Rawtype]{}
	if err := c.Database(m.Db).Collection(m.Collection).FindOne(context.TODO(), bson.M{"_id": id}).Decode(r); err != nil {
		return nil, err
	}
	return r, nil
}

func getRawFrame[Rawtype any](m incoming) (*raw[Rawtype], error) {
	raw, err := getMongo[Rawtype](m)
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

// Default Listen function for typical transformer with one queue
func Listen[Rawtype any](handler func(*raw[Rawtype]) error) {
	r, err := RabbitConnect(os.Getenv("MQ_LISTEN_URI"))
	if err != nil {
		panic(err)
	}
	mq, err := r.Consume(
		os.Getenv("MQ_LISTEN_EXCHANGE"),
		os.Getenv("MQ_LISTEN_QUEUE"),
		os.Getenv("MQ_LISTEN_KEY"),
		os.Getenv("MQ_LISTEN_CONSUMER"),
	)
	if err != nil {
		panic(err)
	}
	HandleRawQueue(mq, handler)
}

func HandleRawQueue[Rawtype any](mq <-chan amqp091.Delivery, handler func(*raw[Rawtype]) error) {
	for msg := range mq {
		slog.Debug("Received a message", "body", msg.Body)

		msgBody := incoming{}
		if err := json.Unmarshal(msg.Body, &msgBody); err != nil {
			slog.Error("Error unmarshalling mq message", "err", err)
			msgReject(&msg)
			continue
		}

		rawFrame, err := getRawFrame[Rawtype](msgBody)
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

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log"
	"log/slog"
	"os"

	"github.com/rabbitmq/amqp091-go"
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

func main() {
	initLogging()
	conn, err := amqp091.Dial(os.Getenv("MQ_URI"))
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclare(
		os.Getenv("MQ_QUEUE"), // name
		true,                  // durable
		false,                 // delete when unused
		false,                 // exclusive
		false,                 // no-wait
		nil,                   // arguments
	)
	failOnError(err, "Failed to declare a queue")

	err = ch.QueueBind(
		q.Name,
		os.Getenv("MQ_KEY"),
		os.Getenv("MQ_EXCHANGE"),
		false, //nowait
		nil)   //args

	failOnError(err, "Failed binding queue to exchange")

	msgs, err := ch.Consume(
		q.Name,                   // queue
		os.Getenv("MQ_CONSUMER"), // consumer
		true,                     // auto-ack
		false,                    // exclusive
		false,                    // no-local
		false,                    // no-wait
		nil,                      // args
	)
	failOnError(err, "Failed to register a consumer")

	var forever chan struct{}

	// push bdp provenance
	// push bdp datatype

	go func() {
		for d := range msgs {
			log.Printf("Received a message: %s", d.Body)
			// Get raw data from mongo
			// decode base64
			// push bdp
		}
	}()

	<-forever
}

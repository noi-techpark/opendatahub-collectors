package main

import (
	"context"
	"log/slog"
	"os"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

/*
Heavily based on
https://github.com/rabbitmq/rabbitmq-tutorials/blob/64526d042d75d08bacb3fe91a811c29a016e017b/go/send.go
under Apache-2.0 license
*/

func failOnError(err error, msg string) {
	if err != nil {
		slog.Error(msg, "err", err)
		panic(err)
	}
}

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	// get env variables
	rUrl := os.Getenv("RABBITMQ_URI")
	rCName := os.Getenv("RABBITMQ_CLIENTNAME")
	fName := os.Getenv("FILEPATH")
	provider := os.Getenv("PROVIDER")

	conn, err := amqp.DialConfig(rUrl, amqp.Config{
		Properties: amqp.Table{"connection_name": rCName},
	})
	failOnError(err, "Failed to connect to RabbitMQ")
	defer conn.Close()

	ch, err := conn.Channel()
	failOnError(err, "Failed to open a channel")
	defer ch.Close()

	q, err := ch.QueueDeclarePassive(
		"ingress-q", // name
		true,        // durable
		false,       // delete when unused
		false,       // exclusive
		false,       // no-wait
		nil,         // arguments
	)
	failOnError(err, "Failed to declare a queue")
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	body, err := os.ReadFile(fName)
	failOnError(err, "Failed to read file "+fName)

	err = ch.PublishWithContext(ctx,
		"ingress", // exchange
		q.Name,    // routing key
		false,     // mandatory
		false,     // immediate
		amqp.Publishing{
			ContentType: "text/plain",
			Body:        []byte(body),
			Headers:     amqp.Table{"provider": provider},
		})
	failOnError(err, "Failed to publish a message")
	slog.Info(" [x] Sent file content. Job done")
}

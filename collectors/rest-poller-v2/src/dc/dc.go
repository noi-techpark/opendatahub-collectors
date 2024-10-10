// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package dc

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"time"

	"github.com/kelseyhightower/envconfig"
	amqp "github.com/rabbitmq/amqp091-go"
)

func FailOnError(err error, msg string) {
	if err != nil {
		slog.Error(msg, "err", err)
		panic(err)
	}
}

func InitLog(lv string) {
	level := &slog.LevelVar{}
	level.UnmarshalText([]byte(lv))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}

type Env struct {
	RABBITMQ_URI        string
	RABBITMQ_Exchange   string `default:"ingress"`
	RABBITMQ_Clientname string

	Provider string
	LogLevel string `default:"INFO"`
}

func LoadEnv(e interface{}) {
	envconfig.MustProcess("", e)
}

type MqMsg struct {
	Provider  string    `json:"provider"`
	Timestamp time.Time `json:"timestamp"`
	Rawdata   any       `json:"rawdata"`
	Meta      any       `json:"metadata"`
}

func PubFromEnv(e Env) chan<- MqMsg {
	return Pub(e.RABBITMQ_URI, e.RABBITMQ_Exchange, e.RABBITMQ_Clientname, e.Provider)
}

func Pub(uri string, exchange string, client string, provider string) chan<- MqMsg {
	conn, err := amqp.DialConfig(uri, amqp.Config{
		Properties: amqp.Table{"connection_name": client},
	})
	FailOnError(err, "Failed to connect to RabbitMQ")

	ch, err := conn.Channel()
	FailOnError(err, "Failed to open a channel")

	rabbitChan := make(chan MqMsg)

	go func() {
		for msg := range rabbitChan {
			payload, err := json.Marshal(msg)
			if err != nil {
				slog.Error("Error marshalling message to json", "err", err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			err = ch.PublishWithContext(ctx,
				exchange, // exchange
				provider, // routing key
				false,    // mandatory
				false,    // immediate
				amqp.Publishing{
					ContentType: "application/json",
					Body:        payload,
					Headers:     amqp.Table{"provider": provider},
				})
			FailOnError(err, "Failed to publish a message")
			cancel()
		}
	}()
	return rabbitChan //UwU
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"encoding/json"
	"log/slog"
	"time"

	"github.com/ThreeDotsLabs/watermill"
	"github.com/ThreeDotsLabs/watermill-amqp/v2/pkg/amqp"
	"github.com/ThreeDotsLabs/watermill/message"
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dc"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/rabbitmq/amqp091-go"
)

type Rawdata struct {
	MsgId   uint16
	Topic   string
	Payload string
}

var cfg struct {
	dc.Env
	MQTT_user     string
	MQTT_pass     string
	MQTT_uri      string
	MQTT_clientid string
	MQTT_topic    string
}

func main() {
	envconfig.MustProcess("", &cfg)
	ms.InitLog(cfg.LOG_LEVEL)

	slog.Info("Started with config", "cfg", cfg)

	rabbit := NewRabbitPublisher(cfg.MQ_URI, cfg.MQ_EXCHANGE, cfg.MQ_CLIENT, cfg.PROVIDER)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTT_uri)
	opts.SetClientID(cfg.MQTT_clientid)
	opts.SetUsername(cfg.MQTT_user)
	opts.SetPassword(cfg.MQTT_pass)
	opts.SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		c.Subscribe(cfg.MQTT_topic, 1, func(c mqtt.Client, m mqtt.Message) {
			// We assume the payload is a string (json probably)
			slog.Debug("got MQTT message", "id", m.MessageID(), "topic", m.Topic(), "payload", string(m.Payload()))
			msg := dto.RawAny{
				Provider:  cfg.PROVIDER,
				Timestamp: time.Now(),
				Rawdata: Rawdata{
					MsgId:   m.MessageID(),
					Topic:   m.Topic(),
					Payload: string(m.Payload()),
				},
			}
			rabbit <- msg
		})
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	select {}
}

func NewRabbitPublisher(uri string, exchange string, client string, routingkey string) chan<- dto.RawAny {
	pubConfig := amqp.NewDurablePubSubConfig(uri, nil)
	pubConfig.Connection.AmqpConfig = &amqp091.Config{}
	pubConfig.Connection.AmqpConfig.Properties = amqp091.Table{}
	pubConfig.Connection.AmqpConfig.Properties.SetClientConnectionName(client)
	pubConfig.Exchange.GenerateName = amqp.GenerateQueueNameConstant(exchange)

	pub, err := amqp.NewPublisher(pubConfig, watermill.NewSlogLogger(slog.Default()))
	if err != nil {
		panic(err)
	}

	ch := make(chan dto.RawAny)

	go func() {
		for msg := range ch {
			payload, err := json.Marshal(msg)
			if err != nil {
				slog.Error("can't marshal msg", "err", err, "msg", msg)
				panic(err)
			}
			pub.Publish(routingkey, message.NewMessage(watermill.NewUUID(), payload))
		}
	}()

	return ch
}

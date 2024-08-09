// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"log/slog"
	"os"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/kelseyhightower/envconfig"
)

var cfg struct {
	RABBITMQ_URI        string
	RABBITMQ_Exchange   string
	RABBITMQ_ClientNAME string

	MQTT_user     string
	MQTT_pass     string
	MQTT_uri      string
	MQTT_clientid string
	MQTT_topic    string

	LogLevel string `default:"INFO"`
}

func initLog() {
	level := &slog.LevelVar{}
	level.UnmarshalText([]byte(cfg.LogLevel))
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: level,
	})))
}

func main() {
	envconfig.MustProcess("APP", &cfg)
	initLog()

	slog.Info("Started with config", "cfg", cfg)

	opts := mqtt.NewClientOptions()
	opts.AddBroker(cfg.MQTT_uri)
	opts.SetClientID(cfg.MQTT_clientid)
	opts.SetUsername(cfg.MQTT_user)
	opts.SetPassword(cfg.MQTT_pass)
	opts.SetAutoReconnect(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		c.Subscribe(cfg.MQTT_topic, 1, func(c mqtt.Client, m mqtt.Message) {
			slog.Info("got message", "msg", m)
		})
	})

	client := mqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		panic(token.Error())
	}

	select {}
}

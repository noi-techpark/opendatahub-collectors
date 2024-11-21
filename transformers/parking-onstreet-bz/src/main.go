// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: CC0-1.0

package main

import (
	"os"

	"github.com/gocarina/gocsv"
	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
)

var cfg struct {
	tr.Env
}

type MqttRaw struct {
	MsgId   uint16
	Topic   string
	Payload string
}

func main() {
	envconfig.MustProcess("", &cfg)
	ms.InitLog(cfg.LOG_LEVEL)

	// b := bdplib.FromEnv()
	tr.ListenFromEnv(cfg.Env, func(r *dto.Raw[MqttRaw]) error {

		return nil
	})
}

type Station struct {
	DevEUI      string
	Id          string
	Group       string
	Longitude   float64
	Latitude    float64
	Description string
}

func readStations() []Station {
	f, err := os.Open("stations.csv")
	ms.FailOnError(err, "failed reading stations csv")
	defer f.Close()

	stations := []Station{}

	ms.FailOnError(gocsv.Unmarshal(f, &stations), "failed unmarshalling csv")
	return stations
}

// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"fmt"
	"log/slog"
	"strconv"
	"sync"

	"github.com/kelseyhightower/envconfig"
	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-opendatahub-ingest/dto"
	"github.com/noi-techpark/go-opendatahub-ingest/mq"
	"github.com/noi-techpark/go-opendatahub-ingest/ms"
	"github.com/noi-techpark/go-opendatahub-ingest/tr"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/rabbitmq/amqp091-go"
)

const stationTypeLocation = "EChargingStation"
const stationTypePlug = "EChargingPlug"
const period = 1

var dtNumberAvailable = bdplib.DataType{
	Name:        "number-available",
	Description: "number of available vehicles / charging points",
	Rtype:       "Instantaneous",
}
var dtPlugStatus = bdplib.DataType{
	Name:        "echarging-plug-status-ocpi",
	Description: "Current state of echarging plug according to OCPI standard",
	Rtype:       "Instantaneous",
}

func syncDataTypes(b *bdplib.Bdp) {
	ms.FailOnError(b.SyncDataTypes(stationTypeLocation, []bdplib.DataType{dtNumberAvailable}), "could not sync data types. aborting...")
	ms.FailOnError(b.SyncDataTypes(stationTypePlug, []bdplib.DataType{dtPlugStatus}), "could not sync data types. aborting...")
}

var cfg struct {
	ms.Env

	MQ_URI      string
	MQ_CONSUMER string
	MQ_EXCHANGE string
	MONGO_URI   string

	// for data incoming from echarging-ocpi pushes
	MQ_PUSH_QUEUE string
	MQ_PUSH_KEY   string

	// for data coming from rest-poller
	MQ_POLL_QUEUE string
	MQ_POLL_KEY   string

	NINJA_URL string
}

type EVSERaw struct {
	Params struct {
		Country_code string
		Evse_uid     string
		Location_id  string
		Party_id     string
	}
	Body OCPIEvse
}

func setupNinja() {
	odhts.C.BaseUrl = cfg.NINJA_URL
	odhts.C.Referer = cfg.MQ_CONSUMER
}

var locDataMu = sync.Mutex{}

func main() {
	envconfig.MustProcess("", &cfg)
	ms.InitLog(cfg.LOG_LEVEL)

	b := bdplib.FromEnv()
	setupNinja()

	syncDataTypes(b)

	rabbit, err := mq.Connect(cfg.MQ_URI, cfg.MQ_CONSUMER)
	ms.FailOnError(err, "failed connecting to rabbitmq")
	defer rabbit.Close()

	rabbit.OnClose(func(err *amqp091.Error) {
		slog.Error("rabbitmq connection closed unexpectedly")
		panic(err)
	})

	pushMQ, err := rabbit.Consume(cfg.MQ_EXCHANGE, cfg.MQ_PUSH_QUEUE, cfg.MQ_PUSH_KEY)
	ms.FailOnError(err, "failed creating push queue")

	// Handle push updates, coming via OCPI endpoint
	go tr.HandleQueue(pushMQ, cfg.MONGO_URI, func(r *dto.Raw[EVSERaw]) error {
		plugData := b.CreateDataMap()

		plugid := stationId(r.Rawdata.Params.Evse_uid, b.Origin)
		plugData.AddRecord(plugid, dtPlugStatus.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), r.Rawdata.Body.Status, period))
		if err := b.PushData(stationTypePlug, plugData); err != nil {
			return fmt.Errorf("error pushing plug data: %w", err)
		}
		slog.Info("Updated plug state", "plugid", plugid)

		// Update parent station "number available data type"
		go func() {
			// Mutex this to avoid race conditions with ourselves
			locDataMu.Lock()
			defer locDataMu.Unlock()

			req := odhts.DefaultRequest()
			req.StationTypes = append(req.StationTypes, stationTypePlug)
			req.Repr = odhts.FlatNode
			req.DataTypes = append(req.DataTypes, dtPlugStatus.Name)
			// count available plugs under same parent
			req.Where = fmt.Sprintf("sactive.eq.true,pcode.eq.\"%s\",mvalue.eq.AVAILABLE", r.Rawdata.Params.Location_id)
			req.Select = "scode"

			res := odhts.Response[[]struct{ Mvalue string }]{}

			if err := odhts.Latest(req, &res); err != nil {
				slog.Error("failed requesting sibling plug states", "err", err)
				return
			}

			numAvailable := len(res.Data)
			recs := b.CreateDataMap()
			locationId := stationId(r.Rawdata.Params.Location_id, b.Origin)
			recs.AddRecord(locationId, dtNumberAvailable.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), numAvailable, period))
			if err := b.PushData(stationTypePlug, plugData); err != nil {
				slog.Error("error pushing location data", "err", err)
			}
			slog.Info("Updated location state", "locationid", locationId)
		}()

		return nil
	})

	pullMQ, err := rabbit.Consume(cfg.MQ_EXCHANGE, cfg.MQ_POLL_QUEUE, cfg.MQ_POLL_KEY)
	ms.FailOnError(err, "failed creating poll queue")

	// Handle full station details, coming a few times a day via REST poller
	go tr.HandleQueue(pullMQ, cfg.MONGO_URI, func(r *dto.Raw[[]OCPILocations]) error {
		stations := []bdplib.Station{}
		locationData := b.CreateDataMap()
		plugs := []bdplib.Station{}
		plugData := b.CreateDataMap()

		for _, loc := range r.Rawdata {
			lat, _ := strconv.ParseFloat(loc.Coordinates.Latitude, 64)
			lon, _ := strconv.ParseFloat(loc.Coordinates.Longitude, 64)
			station := bdplib.CreateStation(
				stationId(loc.ID, b.Origin),
				loc.Name,
				stationTypeLocation,
				lat,
				lon,
				b.Origin)

			station.MetaData = map[string]any{
				"country_code":  loc.CountryCode,
				"party_id":      loc.PartyID,
				"address":       loc.Address,
				"city":          loc.City,
				"postal_code":   loc.PostalCode,
				"time_zone":     loc.TimeZone,
				"opening_times": loc.OpeningTimes,
			}
			if len(loc.Directions) > 0 {
				station.MetaData["directions"] = loc.Directions
			}

			stations = append(stations, station)

			numAvailable := 0

			for _, evse := range loc.Evses {
				plug := bdplib.CreateStation(
					stationId(evse.UID, b.Origin),
					evse.EvseID,
					stationTypePlug,
					station.Latitude,
					station.Longitude,
					b.Origin)

				plug.ParentStation = station.Id

				plug.MetaData = map[string]any{}

				if len(evse.Capabilities) > 0 {
					plug.MetaData["capabilities"] = evse.Capabilities
				}
				if len(evse.Capabilities) > 0 {
					plug.MetaData["connectors"] = evse.Connectors
				}

				plugs = append(plugs, plug)
				if evse.Status == "AVAILABLE" {
					numAvailable++
				}
				plugData.AddRecord(plug.Id, dtPlugStatus.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), evse.Status, period))
			}

			locationData.AddRecord(station.Id, dtNumberAvailable.Name, bdplib.CreateRecord(r.Timestamp.UnixMilli(), numAvailable, period))
		}

		// TODO: figure out some way to sync the total set of stations, and identify inactive ones
		// e.g. all that have not been updated for a month
		if err := b.SyncStations(stationTypeLocation, stations, true, true); err != nil {
			return fmt.Errorf("error syncing %s: %w", stationTypeLocation, err)
		}
		if err := b.SyncStations(stationTypePlug, plugs, true, true); err != nil {
			return fmt.Errorf("error syncing %s: %w", stationTypePlug, err)
		}
		if err := b.PushData(stationTypeLocation, locationData); err != nil {
			return fmt.Errorf("error pushing location data: %w", err)
		}
		if err := b.PushData(stationTypePlug, plugData); err != nil {
			return fmt.Errorf("error pushing plug data: %w", err)
		}

		// push all
		return nil
	})

	select {}
}

func stationId(id string, origin string) string {
	return fmt.Sprintf("%s:%s", origin, id)
}

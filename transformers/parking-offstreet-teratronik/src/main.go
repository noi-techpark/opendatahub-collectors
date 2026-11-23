// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"fmt"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const ParkingStation = "ParkingStation"
const Period = 300

// The provider describes a single car park, so there is one station and its
// code is fixed.
const stationCode = "parking-sta-marlengo"
const municipality = "Marling - Marlengo"

var env struct {
	tr.Env
	bdplib.BdpEnv
}

var (
	dtFree     = bdplib.CreateDataType("free", "", "free", "Instantaneous")
	dtOccupied = bdplib.CreateDataType("occupied", "", "occupied", "Instantaneous")
)

type payload struct {
	Id           string
	Name_IT      string
	Name_DE      string
	Latitude     float64
	Longitude    float64
	Capacity     int
	Measurements []struct {
		Timestamp    string
		Availability int
	}
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	ms.InitWithEnv(ctx, "", &enrichEnv)
	ms.InitWithEnv(ctx, "", &discoveryEnv)
	logger.Get(ctx).Info("Starting parking-offstreet-teratronik transformer...")

	defer tel.FlushOnPanic()

	b := bdplib.FromEnv(env.BdpEnv)

	ms.FailOnError(ctx, syncDataTypes(b), "failed to sync data types")

	ms.FailOnError(ctx, startEnrichment(ctx, env.Env), "failed to start the enrichment table")
	defer stopEnrichment()
	ms.FailOnError(ctx, startDiscovery(ctx, env.Env), "failed to start the discovery reporter")

	// rest-push stores the provider's JSON body base64-encoded as a string.
	listener := tr.NewTr[string](ctx, env.Env)
	err := listener.Start(ctx, tr.RawBase64JsonMiddleware(TransformWithBdp(b)))
	ms.FailOnError(ctx, err, "error while listening to queue")
}

func syncDataTypes(b bdplib.Bdp) error {
	return b.SyncDataTypes([]bdplib.DataType{dtFree, dtOccupied})
}

func TransformWithBdp(b bdplib.Bdp) tr.Handler[payload] {
	return func(ctx context.Context, r *rdb.Raw[payload]) error {
		return Transform(ctx, b, r)
	}
}

// buildStation keeps the provider's Italian name as the station name and both
// names under their capitalised keys, which differ from the lowercase ones the
// enrichment rules write.
func buildStation(origin string, p payload) bdplib.Station {
	s := bdplib.CreateStation(stationCode, p.Name_IT, ParkingStation, p.Latitude, p.Longitude, origin)
	s.MetaData = map[string]any{
		"name_DE":      p.Name_DE,
		"name_IT":      p.Name_IT,
		"capacity":     p.Capacity,
		"municipality": municipality,
	}
	return s
}

// Transform maps one poll of the car park. The measurement carries the raw
// record's timestamp, never the one inside the provider's measurement.
func Transform(ctx context.Context, b bdplib.Bdp, r *rdb.Raw[payload]) error {
	log := logger.Get(ctx)
	p := r.Rawdata

	if len(p.Measurements) == 0 {
		return errors.New("payload has no measurements")
	}

	s := buildStation(b.GetOrigin(), p)

	entry := discoveryEntry(s, map[string]any{
		"capacity":    p.Capacity,
		"name_it":     p.Name_IT,
		"name_de":     p.Name_DE,
		"provider_id": p.Id,
	})
	defer reportDiscovery(ctx)

	if hasRecord(stationCode) {
		entry.Status = discovery.StatusPublished
	} else {
		entry.Reason = "no-record"
		if enrichEnv.REFTABLE_GATE {
			log.Warn("withholding station without enrichment record", "scode", stationCode)
			entry.Status = discovery.StatusWithheld
			observe(stationCode, entry)
			return nil
		}
		entry.Status = discovery.StatusPublished
		log.Debug("publishing station without enrichment record", "scode", stationCode)
	}
	observe(stationCode, entry)

	enrichStation(stationCode, &s)
	if err := b.SyncStations(ParkingStation, []bdplib.Station{s}, true, false); err != nil {
		return fmt.Errorf("failed to sync stations: %w", err)
	}

	free := p.Measurements[0].Availability
	ts := r.Timestamp.UnixMilli()

	dm := b.CreateDataMap()
	dm.AddRecord(stationCode, dtFree.Name, bdplib.CreateRecord(ts, free, Period))
	dm.AddRecord(stationCode, dtOccupied.Name, bdplib.CreateRecord(ts, p.Capacity-free, Period))

	if err := b.PushData(ParkingStation, dm); err != nil {
		return fmt.Errorf("failed to push data: %w", err)
	}
	return nil
}

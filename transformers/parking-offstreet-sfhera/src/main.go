// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"strconv"

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

const municipality = "Bolzano - Bozen"

var env struct {
	tr.Env
	bdplib.BdpEnv
}

var (
	dtFree     = bdplib.CreateDataType("free", "", "free", "Instantaneous")
	dtOccupied = bdplib.CreateDataType("occupied", "", "occupied", "Instantaneous")
	dtEnter    = bdplib.CreateDataType("entering-vehicles-today", "", "Number of vehicles that entered since start of day", "Instantaneous")
	dtExit     = bdplib.CreateDataType("exiting-vehicles-today", "", "Number of vehicles that exited since start of day", "Instantaneous")
)

// payload is one parking level as the provider pushes it: every number is a
// string except Lots.
type payload struct {
	Uid      string
	Park     string
	Lat      string
	Long     string
	In       string
	Out      string
	Floor    string
	Lots     int
	Tot      string
	Reserved string
}

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	ms.InitWithEnv(ctx, "", &enrichEnv)
	ms.InitWithEnv(ctx, "", &discoveryEnv)
	logger.Get(ctx).Info("Starting parking-offstreet-sfhera transformer...")

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
	return b.SyncDataTypes([]bdplib.DataType{dtFree, dtOccupied, dtEnter, dtExit})
}

func TransformWithBdp(b bdplib.Bdp) tr.Handler[payload] {
	return func(ctx context.Context, r *rdb.Raw[payload]) error {
		return Transform(ctx, b, r)
	}
}

// scode is the station code, and the key of the station's enrichment and
// discovery records.
func scode(p payload) string {
	return fmt.Sprintf("parking-bz:%s:%s", p.Uid, p.Floor)
}

// Unparsable numbers become zero, as they always have: a level with a garbled
// field is still published.
func buildStation(origin string, p payload) (bdplib.Station, int) {
	lat, _ := strconv.ParseFloat(p.Lat, 64)
	lon, _ := strconv.ParseFloat(p.Long, 64)
	s := bdplib.CreateStation(scode(p), p.Park, ParkingStation, lat, lon, origin)

	tot, _ := strconv.Atoi(p.Tot)
	floor, _ := strconv.Atoi(p.Floor)
	s.MetaData = map[string]any{
		"floor":        floor,
		"capacity":     tot,
		"municipality": municipality,
	}
	return s, tot
}

// Transform maps one parking level, which is one station per message.
func Transform(ctx context.Context, b bdplib.Bdp, r *rdb.Raw[payload]) error {
	log := logger.Get(ctx)
	p := r.Rawdata

	s, tot := buildStation(b.GetOrigin(), p)
	code := s.Id

	entry := discoveryEntry(s, map[string]any{
		"capacity": s.MetaData["capacity"],
		"floor":    s.MetaData["floor"],
		"uid":      p.Uid,
	})
	defer reportDiscovery(ctx)

	if hasRecord(code) {
		entry.Status = discovery.StatusPublished
	} else {
		entry.Reason = "no-record"
		if enrichEnv.REFTABLE_GATE {
			log.Warn("withholding station without enrichment record", "scode", code)
			entry.Status = discovery.StatusWithheld
			observe(code, entry)
			return nil
		}
		entry.Status = discovery.StatusPublished
		log.Debug("publishing station without enrichment record", "scode", code)
	}
	observe(code, entry)

	enrichStation(code, &s)
	if err := b.SyncStations(ParkingStation, []bdplib.Station{s}, true, false); err != nil {
		return fmt.Errorf("failed to sync stations: %w", err)
	}

	in, _ := strconv.Atoi(p.In)
	out, _ := strconv.Atoi(p.Out)
	ts := r.Timestamp.UnixMilli()

	dm := b.CreateDataMap()
	dm.AddRecord(code, dtFree.Name, bdplib.CreateRecord(ts, p.Lots, Period))
	dm.AddRecord(code, dtOccupied.Name, bdplib.CreateRecord(ts, tot-p.Lots, Period))
	dm.AddRecord(code, dtEnter.Name, bdplib.CreateRecord(ts, in, Period))
	dm.AddRecord(code, dtExit.Name, bdplib.CreateRecord(ts, out, Period))

	if err := b.PushData(ParkingStation, dm); err != nil {
		return fmt.Errorf("failed to push data: %w", err)
	}
	return nil
}

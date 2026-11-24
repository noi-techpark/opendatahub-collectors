// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/merge"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
)

// Station descriptions edited in the parking control tower, published through
// rest-push and read from the raw data lake. Records are keyed by the
// provider's own station id, not by the station code built from it.
var enrichEnv struct {
	REFTABLE_DB         string `default:""`
	REFTABLE_COLLECTION string `default:""`
	REFTABLE_KEY        string `default:"key"`
	REFTABLE_MQ_QUEUE   string `default:""`
	REFTABLE_MQ_KEY     string `default:""`
	REFTABLE_PAGE_LIMIT int    `default:"0"`

	// REFTABLE_GATE withholds a station that has no record. Off, it is
	// published as the provider describes it.
	REFTABLE_GATE bool `default:"false"`
}

var discoveryEnv struct {
	DISCOVERY_PROVIDER   string `default:""`
	DISCOVERY_GONE_AFTER int    `default:"0"`
	RAW_WRITER_URL       string `default:""`
}

// stationRules relocates a parking.v1 record into the shape of a BDP station.
// No rule reaches capacity or any other field the transformation owns.
var stationRules = merge.MustCompile(merge.Map{
	Target: "bdp.Station",
	Schema: "parking.v1",
	Rules: []merge.Rule{
		{Src: "$.name", Dst: "$.name"},
		{Src: "$.gps.lat", Dst: "$.latitude"},
		{Src: "$.gps.lon", Dst: "$.longitude"},

		{Src: "$.names.de", Dst: "$.metaData.name_de"},
		{Src: "$.names.it", Dst: "$.metaData.name_it"},
		{Src: "$.names.en", Dst: "$.metaData.name_en"},
		{Src: "$.names.lad", Dst: "$.metaData.name_lad"},
		{Src: "$.standard_name", Dst: "$.metaData.standard_name"},

		// The record owns the netex subtree outright: the provider supplies none
		// of it.
		{Src: "$.netex", Dst: "$.metaData.netex_parking", Policy: merge.Replace},
	},
})

var (
	refTable      reftable.Lookup[map[string]any]
	enrichmentSet *reftable.Set

	// discoverer reports every station seen; nil until startDiscovery ran.
	discoverer discovery.Observer
)

func hasRecord(key string) bool {
	if refTable == nil {
		return false
	}
	_, ok := refTable.Get(key)
	return ok
}

// enrichStation overlays the record stored under key onto s and reports
// whether one exists. A record that fails to merge leaves s unenriched.
func enrichStation(key string, s *bdplib.Station) bool {
	if refTable == nil {
		return false
	}
	record, ok := refTable.Get(key)
	if !ok {
		return false
	}
	overlay(key, record, s)
	return true
}

func overlay(key string, record map[string]any, s *bdplib.Station) {
	if s.MetaData == nil {
		s.MetaData = map[string]any{}
	}
	name := s.Name
	if err := merge.Into(s, func(doc map[string]any) error {
		return stationRules.Apply(doc, record)
	}); err != nil {
		slog.Error("failed merging enrichment; publishing the station unenriched", "key", key, "err", err)
	}
	// A record that clears its name must not blank the station: the writer
	// rejects a station without one.
	if s.Name == "" {
		s.Name = name
	}
}

// startEnrichment bootstraps the table and subscribes to changes. It refuses to
// run unconfigured and fails closed: without the table every station would be
// published without its description, or withheld.
func startEnrichment(ctx context.Context, conn tr.Env) error {
	if enrichEnv.REFTABLE_DB == "" {
		return errors.New("REFTABLE_DB is not set")
	}
	tbl := reftable.New[map[string]any](
		rdb.NewRDBridge(rdb.Env{RAW_DATA_BRIDGE_ENDPOINT: conn.RAW_DATA_BRIDGE_ENDPOINT}),
		reftable.Config{
			Name:       "parking-enrichment",
			DB:         enrichEnv.REFTABLE_DB,
			Collection: enrichEnv.REFTABLE_COLLECTION,
			Key:        enrichEnv.REFTABLE_KEY,
			PageLimit:  enrichEnv.REFTABLE_PAGE_LIMIT,
			Schema:     stationRules.Schema(),
			// Without the gate an empty table publishes everything unchanged, so the
			// first record need not exist before the transformer may start.
			AllowMissing: !enrichEnv.REFTABLE_GATE,

			// rest-push stores the control tower's body base64-encoded.
			Decode: reftable.DecodeBase64,

			MQ_URI:      conn.MQ_URI,
			MQ_CLIENT:   conn.MQ_CLIENT + "-enrichment",
			MQ_EXCHANGE: conn.MQ_EXCHANGE,
			MQ_QUEUE:    enrichEnv.REFTABLE_MQ_QUEUE,
			MQ_KEY:      enrichEnv.REFTABLE_MQ_KEY,
		})
	enrichmentSet = reftable.NewSet(tbl)
	if err := enrichmentSet.Start(ctx); err != nil {
		return err
	}
	refTable = tbl
	slog.Info("enrichment ready", "entries", tbl.Len(), "gate", enrichEnv.REFTABLE_GATE)
	return nil
}

func stopEnrichment() {
	if enrichmentSet != nil {
		_ = enrichmentSet.Close()
	}
}

// startDiscovery starts the reporter of the discovery table. It refuses to run
// unconfigured: a transformer that withholds or gates stations silently is what
// the table exists to prevent.
func startDiscovery(ctx context.Context, conn tr.Env) error {
	if discoveryEnv.DISCOVERY_PROVIDER == "" {
		return errors.New("DISCOVERY_PROVIDER is not set")
	}
	if discoveryEnv.RAW_WRITER_URL == "" {
		return errors.New("RAW_WRITER_URL is not set")
	}
	r := discovery.New(
		rdb.NewRDBridge(rdb.Env{RAW_DATA_BRIDGE_ENDPOINT: conn.RAW_DATA_BRIDGE_ENDPOINT}),
		discovery.Config{
			Name:         "parking-discovery",
			Provider:     discoveryEnv.DISCOVERY_PROVIDER,
			RawWriterURL: discoveryEnv.RAW_WRITER_URL,
			GoneAfter:    discoveryEnv.DISCOVERY_GONE_AFTER,
		})
	if err := r.Start(ctx); err != nil {
		return err
	}
	discoverer = r
	return nil
}

func observe(key string, e discovery.Entry) {
	if discoverer != nil {
		discoverer.Observe(key, e)
	}
}

// reportDiscovery closes one payload. A failed flush stays pending in the
// reporter and must not fail the message.
func reportDiscovery(ctx context.Context, fullSet bool) {
	if discoverer == nil {
		return
	}
	if fullSet {
		discoverer.Complete()
	}
	if err := discoverer.Flush(ctx); err != nil {
		slog.Error("failed flushing discovery table", "err", err)
	}
}

// seed is the reference record that reproduces s: only keys whose rule's
// destination already carries the value today, so adopting it changes nothing.
func seed(s bdplib.Station) map[string]any {
	out := map[string]any{
		"gps": map[string]any{"lat": s.Latitude, "lon": s.Longitude},
	}
	if s.Name != "" {
		out["name"] = s.Name
	}
	return out
}

// discoveryEntry describes the station as the transformation built it, before
// any overlay, so the operator sees the provider's view.
func discoveryEntry(s bdplib.Station, parent string, data map[string]any) discovery.Entry {
	lat, lon := s.Latitude, s.Longitude
	return discovery.Entry{
		Name:   s.Name,
		Parent: parent,
		Lat:    &lat,
		Lon:    &lon,
		Data:   data,
		Seed:   seed(s),
	}
}

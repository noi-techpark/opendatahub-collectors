// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/merge"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
)

// Station descriptions edited in the parking control tower, published through
// rest-push and read from the raw data lake. Records are keyed by provider id:
// the counting area's id, which survives the provider renaming the area when
// its capacity changes, and the Standort for a facility.
var enrichEnv struct {
	REFTABLE_DB         string `default:""`
	REFTABLE_COLLECTION string `default:""`
	REFTABLE_KEY        string `default:"key"`
	REFTABLE_MQ_QUEUE   string `default:""`
	REFTABLE_MQ_KEY     string `default:""`
	REFTABLE_PAGE_LIMIT int    `default:"0"`
}

// stationRules relocates a parking.v1 record into the shape of a BDP station.
// No rule reaches capacity or any other field the transformation owns.
var stationRules = merge.MustCompile(merge.Map{
	Target: "bdp.Station",
	Schema: "parking.v1",
	Rules: []merge.Rule{
		{Src: "$.standard_name", Dst: "$.name"},
		{Src: "$.gps.lat", Dst: "$.latitude"},
		{Src: "$.gps.lon", Dst: "$.longitude"},

		{Src: "$.names.de", Dst: "$.metaData.name_de"},
		{Src: "$.names.it", Dst: "$.metaData.name_it"},
		{Src: "$.names.en", Dst: "$.metaData.name_en"},
		{Src: "$.standard_name", Dst: "$.metaData.standard_name"},

		// The record owns the netex subtree outright: the provider supplies none
		// of it.
		{Src: "$.netex", Dst: "$.metaData.netex_parking", Policy: merge.Replace},
	},
})

var (
	refTable      reftable.Lookup[map[string]any]
	enrichmentSet *reftable.Set
)

func hasRecord(code string) bool {
	if refTable == nil {
		return false
	}
	_, ok := refTable.Get(code)
	return ok
}

// enrichStation overlays the record stored under code onto s and reports
// whether one exists. A record that fails to merge leaves s unenriched.
func enrichStation(code string, s *bdplib.Station) bool {
	if refTable == nil {
		return false
	}
	record, ok := refTable.Get(code)
	if !ok {
		return false
	}
	if s.MetaData == nil {
		s.MetaData = map[string]any{}
	}
	name := s.Name
	if err := merge.Into(s, func(doc map[string]any) error {
		return stationRules.Apply(doc, record)
	}); err != nil {
		slog.Error("failed merging enrichment; publishing the station unenriched", "code", code, "err", err)
	}
	// A record that clears its name must not blank the station: the writer
	// rejects a station without one.
	if s.Name == "" {
		s.Name = name
	}
	return true
}

// startEnrichment bootstraps the table and subscribes to changes. It refuses to
// run unconfigured and fails closed: without the table every station would be
// published without its description.
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
	slog.Info("enrichment ready", "entries", tbl.Len())
	return nil
}

func stopEnrichment() {
	if enrichmentSet != nil {
		_ = enrichmentSet.Close()
	}
}

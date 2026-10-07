// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/merge"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

// Operator-authored enrichment: names, coordinates, netex attributes.
//
// This is the ONLY merge input. Whatever merge mechanism this transformer grows
// — hand-written today, a DSL later — composes the provider-derived station
// with records from here and nothing else. Provider-derived reference data
// (see category_source.go) is deliberately kept out of this set, because
// merging it would let the two authorities overwrite each other: an operator
// must not be able to edit a capacity the provider states, and the provider
// must not overwrite a name an operator chose.
var enrichEnv struct {
	REFTABLE_DB         string `default:""`
	REFTABLE_COLLECTION string `default:""`
	REFTABLE_KEY        string `default:"key"`
	REFTABLE_MQ_QUEUE   string `default:""`
	REFTABLE_MQ_KEY     string `default:""`

	// PageLimit is settable so a test can force the bootstrap to page. Left at
	// the default, a small table loads in one request and the paging path is
	// never exercised — which is exactly how a broken cursor once shipped.
	REFTABLE_PAGE_LIMIT int `default:"0"`
}

// stationRules relocates an enrichment record into the shape of a BDP station.
//
// Declared here, in Go, next to the table it applies to: it is program
// structure, and MustCompile turns a mistake in it into a failure to start
// rather than a field that quietly stops arriving.
//
// The record is written in the parking vocabulary and shares no shape with
// bdplib.Station — `names.it` reaches two different destinations, `netex` is a
// subtree that lands nested under another name, `gps` splits into two scalars.
// Relocating that by hand in every transformer is the duplication this removes.
//
// What is not here is as important: no rule writes provider_id, facility_id,
// carpark_id or capacity. Those are the transformation's, and a record cannot
// reach them because no path leads there.
var stationRules = merge.MustCompile(merge.Map{
	Target: "bdp.Station",
	Schema: "parking.v1",
	Rules: []merge.Rule{
		{Src: "$.names.it", Dst: "$.name"},
		{Src: "$.gps.lat", Dst: "$.latitude"},
		{Src: "$.gps.lon", Dst: "$.longitude"},

		{Src: "$.names.de", Dst: "$.metaData.name_de"},
		{Src: "$.names.it", Dst: "$.metaData.name_it"},
		{Src: "$.names.en", Dst: "$.metaData.name_en"},
		{Src: "$.standard_name", Dst: "$.metaData.standard_name"},

		{Src: "$.municipality", Dst: "$.metaData.municipality"},

		// The record owns the netex subtree outright. Skidata supplies none of
		// it, so anything the transformation might put there is wrong rather
		// than merely older — and a merge would leave it in place.
		{Src: "$.netex", Dst: "$.metaData.netex_parking", Policy: merge.Replace},
	},
})

var (
	// refTable holds enrichment records keyed by provider id ("0404467" for a
	// facility, "0404467_0" for a carpark).
	//
	// The record stays a decoded document rather than a Go struct. A struct
	// would have to be kept in step with whatever the publisher sends, and a
	// field whose type drifted would fail to unmarshal and take the whole
	// record with it. The rules address paths, so a record carrying more than
	// the rules read is simply carrying more.
	refTable reftable.Lookup[map[string]any]

	// enrichmentSet owns the lifecycle of the merge inputs. It is a separate
	// set from the provider flow on purpose — see the package comment above.
	enrichmentSet *reftable.Set
)

// enrichStation overlays an enrichment record onto a station the transformer
// has already built, looked up by provider id.
//
// A station with no record in the table is left exactly as the transformation
// produced it.
func enrichStation(providerID string, s *bdplib.Station) {
	if refTable == nil {
		return
	}
	record, ok := refTable.Get(providerID)
	if !ok || len(record) == 0 {
		return
	}

	if err := merge.Into(s, func(doc map[string]any) error {
		return stationRules.Apply(doc, record)
	}); err != nil {
		slog.Error("failed merging enrichment; publishing the station unenriched",
			"station", providerID, "err", err)
	}
}

// startEnrichment brings the merge inputs up.
//
// Bootstrap is fail-closed: starting with an empty table would push
// provider-only metadata over every station an operator has since edited.
func startEnrichment(ctx context.Context, b bdplib.Bdp) error {
	log := logger.Get(ctx)
	if enrichEnv.REFTABLE_DB == "" {
		log.Info("REFTABLE_DB unset; stations will carry no operator enrichment")
		return nil
	}

	tbl := reftable.New[map[string]any](
		rdb.NewRDBridge(rdb.Env{RAW_DATA_BRIDGE_ENDPOINT: env.RAW_DATA_BRIDGE_ENDPOINT}),
		reftable.Config{
			Name:       "parking-enrichment",
			DB:         enrichEnv.REFTABLE_DB,
			Collection: enrichEnv.REFTABLE_COLLECTION,
			Key:        enrichEnv.REFTABLE_KEY,
			PageLimit:  enrichEnv.REFTABLE_PAGE_LIMIT,

			// Declared once, on the rule table. A record stamped with another
			// version is refused before it reaches a rule: the table was
			// written against this shape and nothing else vouches for it.
			Schema: stationRules.Schema(),

			// Enrichment is published through the generic rest-push collector,
			// which sends the body as []byte and so marshals it to base64, and
			// through the legacy writer, which stores what it is given. The
			// counting categories take the other path — rest-push-skidata to
			// raw-writer-2 — where the body is kept verbatim as a string and
			// the default DecodeText is correct. Two live paths, two
			// encodings; this is the deployment fact that decides which.
			Decode: reftable.DecodeBase64,

			MQ_URI:      env.MQ_URI,
			MQ_CLIENT:   env.MQ_CLIENT + "-enrichment",
			MQ_EXCHANGE: env.MQ_EXCHANGE,
			MQ_QUEUE:    enrichEnv.REFTABLE_MQ_QUEUE,
			MQ_KEY:      enrichEnv.REFTABLE_MQ_KEY,

			// An operator edit has to reach BDP without waiting for a restart.
			// Only the description changed, so the provider-derived half of each
			// station still stands and just needs re-merging.
			OnChange: func(ctx context.Context, applied int) {
				logger.Get(ctx).Info("enrichment changed, re-syncing stations", "applied", applied)
				resyncAll(ctx, b)
			},
		})

	enrichmentSet = reftable.NewSet(tbl)
	if err := enrichmentSet.Start(ctx); err != nil {
		return err
	}
	refTable = tbl
	log.Info("enrichment ready", "entries", refTable.Len())
	return nil
}

func stopEnrichment() {
	if enrichmentSet != nil {
		_ = enrichmentSet.Close()
	}
}

// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

// Counting categories are a second *collection flow*, not enrichment.
//
// They come from the provider, through the same collector, into the same raw
// data lake. What they describe — the facility→carpark topology and the total
// capacity — is provider fact, and it feeds the provider-derived half of a
// station. It is never merged with operator input and must never appear in the
// enrichment set: whatever merge mechanism this transformer grows composes
// stations with operator records only.
//
// The reftable machinery is reused because the mechanics happen to be
// identical — bootstrap the compacted view, subscribe for changes, sweep to
// stay correct. That is a shared implementation, not a shared role.
var catEnv struct {
	CATEGORIES_DB         string `default:""`
	CATEGORIES_COLLECTION string `default:"counting-categories"`
	CATEGORIES_KEY        string `default:"facility"`
	CATEGORIES_MQ_QUEUE   string `default:""`
	CATEGORIES_MQ_KEY     string `default:""`

	// PageLimit is settable so a test can force the bootstrap to page. Left at
	// the default, a small table loads in one request and the paging path is
	// never exercised — which is exactly how a broken cursor once shipped.
	CATEGORIES_PAGE_LIMIT int `default:"0"`
}

var (
	// catTable holds the provider's counting categories keyed by facility id
	// ("0404467"). Typed as the read interface so tests can inject a fixture.
	catTable reftable.Lookup[FacilityCategories]

	categorySet *reftable.Set
)

// facilityCategories returns a facility's category list, or nil.
func facilityCategories(facilityID string) FacilityCategories {
	if catTable == nil {
		return nil
	}
	c, ok := catTable.Get(facilityID)
	if !ok {
		return nil
	}
	return c
}

// allFacilities returns every facility the flow describes.
func allFacilities() map[string]FacilityCategories {
	if catTable == nil {
		return nil
	}
	return catTable.All()
}

// startCategories brings the provider reference flow up.
//
// Bootstrap is fail-closed: without a topology the transformer cannot tell how
// many carparks a facility has, and would publish facility totals with no idea
// what is missing from them.
func startCategories(ctx context.Context, b bdplib.Bdp) error {
	log := logger.Get(ctx)
	if catEnv.CATEGORIES_DB == "" {
		log.Info("CATEGORIES_DB unset; no topology and no capacity will be published")
		return nil
	}

	tbl := reftable.New[FacilityCategories](
		rdb.NewRDBridge(rdb.Env{RAW_DATA_BRIDGE_ENDPOINT: env.RAW_DATA_BRIDGE_ENDPOINT}),
		reftable.Config{
			Name:       "counting-categories",
			DB:         catEnv.CATEGORIES_DB,
			Collection: catEnv.CATEGORIES_COLLECTION,
			Key:        catEnv.CATEGORIES_KEY,
			PageLimit:  catEnv.CATEGORIES_PAGE_LIMIT,

			MQ_URI:      env.MQ_URI,
			MQ_CLIENT:   env.MQ_CLIENT + "-categories",
			MQ_EXCHANGE: env.MQ_EXCHANGE,
			MQ_QUEUE:    catEnv.CATEGORIES_MQ_QUEUE,
			MQ_KEY:      catEnv.CATEGORIES_MQ_KEY,

			// A new facility, a new carpark or a corrected total capacity all
			// arrive here. Unlike an enrichment edit this invalidates the
			// provider-derived half of the station, so the bases are rebuilt
			// before re-merging.
			OnChange: func(ctx context.Context, applied int) {
				logger.Get(ctx).Info("counting categories changed, rebuilding stations", "applied", applied)
				rebuildBases(ctx)
				resyncAll(ctx, b)
			},
		})

	categorySet = reftable.NewSet(tbl)
	if err := categorySet.Start(ctx); err != nil {
		return err
	}
	catTable = tbl
	log.Info("counting categories ready", "facilities", catTable.Len())
	return nil
}

func stopCategories() {
	if categorySet != nil {
		_ = categorySet.Close()
	}
}

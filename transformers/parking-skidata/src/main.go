// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	ms "github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	tel "github.com/noi-techpark/opendatahub-go-sdk/tel"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

const (
	ID_TEMPLATE = "urn:parking:skidata"

	stationTypeParent = "ParkingFacility"
	stationType       = "ParkingStation"

	measurementPeriod = 600
)

var env struct {
	tr.Env

	// Time-series API used to hydrate the in-memory cache at startup.
	// The same OAuth client_id/secret is used for both BDP writes and
	// timeseries reads (mirrors the pattern in people-flow-systems-me).
	TS_API_BASE_URL  string `default:""`
	TS_API_TOKEN_URL string `default:""`
	TS_API_REFERER   string `default:"tr-parking-skidata"`
}

var cache *Cache

func main() {
	ctx := context.Background()
	ms.InitWithEnv(ctx, "", &env)
	ms.InitWithEnv(ctx, "", &enrichEnv)
	ms.InitWithEnv(ctx, "", &catEnv)
	log := logger.Get(ctx)
	log.Info("Starting parking-skidata transformer...")

	b := bdplib.FromEnv(bdplib.BdpEnv{
		BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
		BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
		BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
		BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
		BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),
		BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
		BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
	})
	bdpForRebuild = b
	defer tel.FlushOnPanic()

	// Two independent sources, started separately because their authority is
	// separate: the provider states the topology and the capacity, the operator
	// states the description, and only the latter is ever merged into a station.
	// Both are fail-closed — an empty enrichment table would overwrite operator
	// edits, and an empty category flow would publish facility totals without
	// knowing how many carparks are missing from them.
	ms.FailOnError(ctx, startCategories(ctx, b), "failed to start the counting-categories flow")
	defer stopCategories()

	ms.FailOnError(ctx, startEnrichment(ctx, b), "failed to start the enrichment table")
	defer stopEnrichment()

	log.Info("Syncing data types on startup", "datatypes", allDataTypeNames())
	ms.FailOnError(ctx, syncDataTypes(b), "failed to sync types")

	cache = NewCache()

	// Hydration is not an optimisation. The provider reports one carpark at a
	// time, and in production a carpark can stay silent for many hours — the
	// worst observed gap was over a day. Facility totals are refused until every
	// carpark of that facility has a value, so without hydration a facility
	// publishes nothing at all until the last of its carparks has spoken.
	//
	// The topology that makes this checkable comes from the counting categories.
	if env.TS_API_BASE_URL != "" {
		ts := odhts.NewCustomClient(env.TS_API_BASE_URL, env.TS_API_TOKEN_URL, env.TS_API_REFERER)
		ts.UseAuth(os.Getenv("ODH_CLIENT_ID"), os.Getenv("ODH_CLIENT_SECRET"))

		// One read answers two questions: which stations exist, and how to map a
		// URN back to a provider id. Both used to come from the counting
		// categories, which cover twelve facilities of the fleet -- so a carpark
		// without them was neither seeded nor hydrated.
		rows, sErr := fetchStations(ts, os.Getenv("BDP_ORIGIN"))
		if sErr != nil {
			log.Warn("Station listing failed; enrichment reaches a station only once it reports", "err", sErr)
		} else {
			seedRegistry(ctx, b, rows)
			// Reconcile once, here, rather than leaving it to whatever happens
			// first. The change-detection cache is empty at startup, so the
			// next reconciliation would treat every seeded station as changed
			// -- and if that were an enrichment edit, one operator's change to
			// one car park would sync the whole fleet. Paying it at boot keeps
			// every later sync proportional to what actually changed.
			syncChanged(ctx, b, nil)
		}

		index := buildURNIndex()
		for _, r := range rows {
			index[clib.GenerateID(ID_TEMPLATE, r.ProviderID)] = r.ProviderID
		}
		if hErr := hydrateCache(cache, ts, os.Getenv("BDP_ORIGIN"),
			allDataTypeNames(), index); hErr != nil {
			// Best-effort: the cache self-corrects as carparks report. Until it
			// does, facility figures are withheld rather than published wrong.
			log.Warn("Cache hydration failed; facility totals stay unpublished until every carpark reports", "err", hErr)
		}
	} else {
		log.Info("TS_API_BASE_URL unset; skipping station seeding and cache hydration")
	}

	log.Info("Starting transformer listener...")
	// raw-writer-2 stores a JSON body verbatim as a string, so the payload is
	// unwrapped once before it reaches the handler.
	listener := tr.NewTr[string](ctx, env.Env)
	err := listener.Start(ctx, tr.RawString2JsonMiddleware(TransformWithBdp(b)))
	ms.FailOnError(ctx, err, "error while listening to queue")
}

// buildURNIndex maps each known station's URN back to its provider id. The
// cache is keyed by provider id, but BDP returns URNs, so hydration needs the
// inverse of clib.GenerateID — which is not invertible, hence the index.
func buildURNIndex() map[string]string {
	out := map[string]string{}
	if catTable == nil {
		return out
	}
	for facilityID, cats := range allFacilities() {
		out[clib.GenerateID(ID_TEMPLATE, facilityID)] = facilityID
		for _, carparkID := range cats.CarparkIDs() {
			id := carparkProviderID(facilityID, carparkID)
			out[clib.GenerateID(ID_TEMPLATE, id)] = id
		}
	}
	return out
}

// clampInt constrains v to [0, hi]. If hi is negative (e.g. a garbage
// capacity), the upper bound collapses to 0.
func clampInt(v, hi int) int {
	if hi < 0 {
		hi = 0
	}
	if v < 0 {
		return 0
	}
	if v > hi {
		return hi
	}
	return v
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[ParkingEvent] {
	return func(ctx context.Context, payload *rdb.Raw[ParkingEvent]) error {
		return Transform(ctx, bdp, payload)
	}
}

// Transform handles a single Skidata push event.
//
//  1. Registers the stations the event implies, and syncs them if the merge of
//     provider data and enrichment changed.
//  2. Publishes the event's own category on the carpark, if it is a category we
//     publish at all.
//  3. Publishes the facility aggregate, but only when every carpark of that
//     facility has a value.
//
// An event describes one carpark and one counting category. It never describes
// a facility, which is why step 3 needs the cache and the topology.
func Transform(ctx context.Context, bdp bdplib.Bdp, payload *rdb.Raw[ParkingEvent]) error {
	event := payload.Rawdata
	ts := payload.Timestamp.UnixMilli()
	log := logger.Get(ctx)

	facilityID, carparkID := providerIDs(event)

	// A malformed event carries no facility. Registering it would create a
	// station named after a zero id and publish it as real.
	if event.Carpark.FacilityNr <= 0 {
		log.Warn("dropping event with no facility number",
			"carparkId", event.Carpark.Id, "category", event.CountingCategoryId)
		return nil
	}

	log.Info("Processing parking event",
		"facilityNr", event.Carpark.FacilityNr,
		"carparkId", carparkID,
		"category", event.CountingCategoryId,
		"level", event.Level, "capacity", event.Capacity)

	childProviderID := carparkProviderID(facilityID, carparkID)

	// Register the stations this event implies and sync whichever the merge
	// changed. Nothing is dropped for being unenriched: a bare station is
	// published so its measurements have somewhere to land.
	observe(ctx, bdp, event, facilityID, carparkID)

	suffix, published := suffixFor(event.CountingCategoryId)
	if !published {
		// Per-floor occupancy and site-specific categories end here. They were
		// already being discarded further down; doing it in one place makes it
		// visible instead of silent.
		log.Debug("category is not published",
			"category", event.CountingCategoryId, "name", event.Name)
		return nil
	}

	free, freeKnown := freeFromEvent(ctx, event)
	occupied := event.Level
	if occupied < 0 {
		occupied = 0
	}

	parentURN := clib.GenerateID(ID_TEMPLATE, facilityID)
	childURN := clib.GenerateID(ID_TEMPLATE, childProviderID)

	// 1. Cache and publish the carpark's own figures.
	carparkData := bdp.CreateDataMap()
	cache.Set(childProviderID, occupiedType(suffix), occupied, ts)
	carparkData.AddRecord(childURN, occupiedType(suffix), bdplib.CreateRecord(ts, occupied, measurementPeriod))

	if freeKnown {
		cache.Set(childProviderID, freeType(suffix), free, ts)
		carparkData.AddRecord(childURN, freeType(suffix), bdplib.CreateRecord(ts, free, measurementPeriod))
	}

	if err := bdp.PushData(stationType, carparkData); err != nil {
		return fmt.Errorf("failed to push carpark data: %w", err)
	}

	// 2. Facility aggregate. Withheld entirely unless every carpark of the
	//    facility has a value for the datatype, because a sum over the carparks
	//    that happen to have reported is not a total.
	cats := facilityCategories(facilityID)
	if len(cats) == 0 {
		log.Warn("no counting categories for facility; facility totals not published",
			"facility", facilityID)
		return nil
	}
	siblings := make([]string, 0, len(cats.CarparkIDs()))
	for _, id := range cats.CarparkIDs() {
		siblings = append(siblings, carparkProviderID(facilityID, id))
	}

	facilityData := bdp.CreateDataMap()
	pushed := 0
	for _, dt := range []string{occupiedType(suffix), freeType(suffix)} {
		v, complete := cache.FacilityTotal(siblings, dt)
		if !complete {
			continue
		}
		facilityData.AddRecord(parentURN, dt, bdplib.CreateRecord(ts, v, measurementPeriod))
		pushed++
	}
	if pushed == 0 {
		log.Debug("facility total incomplete, nothing published",
			"facility", facilityID, "carparks", len(siblings))
		return nil
	}

	if err := bdp.PushData(stationTypeParent, facilityData); err != nil {
		return fmt.Errorf("failed to push facility data: %w", err)
	}
	return nil
}

// freeFromEvent computes free slots from the event's own figures.
//
// The event's capacity is authoritative for its own category and nothing else —
// for some carparks the per-category capacity is a live quota that moves every
// few minutes, which is exactly why it is used here and stored nowhere. When it
// is the sentinel there is no capacity to subtract from, so free is not
// computable and is withheld; occupied is a real count and is still published.
//
// For the total category this is the same number the station advertises as its
// capacity (see observedCapacity), so free can never exceed it.
func freeFromEvent(ctx context.Context, event ParkingEvent) (int, bool) {
	capacity := normalizeCapacity(event.Capacity)
	if capacity == capacityUnknown {
		logger.Get(ctx).Debug("capacity unusable, free not published",
			"facilityNr", event.Carpark.FacilityNr, "carparkId", event.Carpark.Id,
			"category", event.CountingCategoryId, "capacity", event.Capacity)
		return 0, false
	}

	free := capacity - event.Level
	if free < 0 || free > capacity {
		logger.Get(ctx).Warn("Clamping out-of-range parking values into [0, capacity]",
			"facilityNr", event.Carpark.FacilityNr, "carparkId", event.Carpark.Id,
			"category", event.CountingCategoryId,
			"level", event.Level, "capacity", capacity, "raw_free", free)
		free = clampInt(free, capacity)
	}
	return free, true
}

// syncDataTypes registers the datatypes this transformer publishes.
//
// The set is fixed rather than derived from what the provider sends. Deriving
// it meant a name the provider invented could mint a datatype, which is how
// per-floor labels in three languages ended up being generated (and then
// silently dropped) for a quarter of all traffic.
func syncDataTypes(bdp bdplib.Bdp) error {
	dataTypes := []bdplib.DataType{}
	for _, suffix := range publishedSuffixes {
		label := "parking slots"
		if suffix != "" {
			label = "'" + strings.ReplaceAll(suffix, "_", " ") + "' parking slots"
		}
		dataTypes = append(dataTypes,
			bdplib.CreateDataType(freeType(suffix), "", "Amount of free "+label, "Instantaneous"),
			bdplib.CreateDataType(occupiedType(suffix), "", "Amount of occupied "+label, "Instantaneous"),
		)
	}
	return bdp.SyncDataTypes(dataTypes)
}

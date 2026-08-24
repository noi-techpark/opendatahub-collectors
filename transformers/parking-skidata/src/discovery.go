// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

// Stations are discovered from the data, not declared in advance.
//
// A station has two halves. The provider-derived half is built from the event
// and the counting categories — identity, parentage, capacity. The operator
// half comes from the enrichment table — names, coordinates, netex. They are
// merged on every change and pushed only when the result actually differs from
// what was last sent.
//
// Capacity belongs to the provider half. It is not enrichment and must not be
// editable from the backoffice: the provider states it, the transformer caches
// it after bootload, and it changes only when the provider says so.

// entry is the provider-derived half of a station, kept so it can be rebuilt
// when the categories change and re-merged when the enrichment changes.
type entry struct {
	base bdplib.Station
	typ  string

	// what the base was built from, so it can be rebuilt without the event
	facilityID string
	carparkID  int // -1 for a facility station
	name       string
}

var (
	registry = map[string]entry{}
	regMu    sync.Mutex

	// observedCapacity is the total capacity the provider last stated in a
	// category-3 event, per carpark provider id.
	//
	// It exists so that the capacity published as metadata and the capacity the
	// free count is computed from are the same number. Taking metadata from the
	// counting categories while computing free from the event let a station
	// advertise 245 slots and report 395 free — which is the defect this whole
	// change exists to remove. The event wins because it is the live figure;
	// the categories supply it before any event has arrived, which is what
	// hydration and the facility sum need at boot.
	observedCapacity = map[string]int{}

	// syncedStations remembers the last station pushed for each provider id, so
	// an unchanged merge costs nothing. Commit only happens after the sync
	// succeeds — a failed push must not be remembered as done.
	syncedStations = tr.NewCache[bdplib.Station]()
)

// baseFacility builds the provider-derived facility station.
func baseFacility(bdp bdplib.Bdp, facilityID, name string) bdplib.Station {
	s := bdplib.CreateStation(
		clib.GenerateID(ID_TEMPLATE, facilityID),
		stationName(name, facilityID),
		stationTypeParent, 0, 0, bdp.GetOrigin())

	s.MetaData = map[string]any{
		"provider_id": facilityID,
		// Sum of the carpark totals, or capacityUnknown when none is known.
		"capacity": facilityCapacityLocked(facilityID),
	}
	return s
}

// carparkCapacityLocked returns the capacity to publish for one carpark: what
// the provider last stated in an event, falling back to the counting
// categories. Callers must hold regMu.
func carparkCapacityLocked(facilityID string, carparkID int) int {
	if c, ok := observedCapacity[carparkProviderID(facilityID, carparkID)]; ok {
		return c
	}
	return facilityCategories(facilityID).TotalCapacity(carparkID)
}

// facilityCapacityLocked sums the known carpark capacities over the facility's
// topology. Carparks whose capacity is unknown contribute nothing rather than
// zero, and a facility where none is known is itself unknown. Callers must hold
// regMu.
func facilityCapacityLocked(facilityID string) int {
	cats := facilityCategories(facilityID)
	if len(cats) == 0 {
		return capacityUnknown
	}
	sum, known := 0, false
	for _, id := range cats.CarparkIDs() {
		if c := carparkCapacityLocked(facilityID, id); c != capacityUnknown {
			sum += c
			known = true
		}
	}
	if !known {
		return capacityUnknown
	}
	return sum
}

// baseCarpark builds the provider-derived carpark station.
func baseCarpark(bdp bdplib.Bdp, facilityID string, carparkID int, name string) bdplib.Station {
	providerID := carparkProviderID(facilityID, carparkID)

	s := bdplib.CreateStation(
		clib.GenerateID(ID_TEMPLATE, providerID),
		stationName(name, providerID),
		stationType, 0, 0, bdp.GetOrigin())
	s.ParentStation = clib.GenerateID(ID_TEMPLATE, facilityID)

	s.MetaData = map[string]any{
		"provider_id": providerID,
		"facility_id": facilityID,
		"carpark_id":  carparkID,
		// The category-3 total only. Per-category capacities are a live quota
		// for some carparks, so they are measurements, not metadata.
		"capacity": carparkCapacityLocked(facilityID, carparkID),
	}
	return s
}

// stationName falls back to the provider id when the provider sends no name.
// The writer rejects a station with an empty name outright, and dropping it
// would lose the measurements that came with it.
func stationName(name, providerID string) string {
	if name != "" {
		return name
	}
	return providerID
}

// merged applies the current enrichment to a base station.
func merged(providerID string, base bdplib.Station) bdplib.Station {
	s := base
	// Copy the metadata: enrichment writes into the map, and the base is kept
	// for re-merging later.
	s.MetaData = make(map[string]any, len(base.MetaData))
	for k, v := range base.MetaData {
		s.MetaData[k] = v
	}
	enrichStation(providerID, &s)
	return s
}

// observe registers the stations an event implies and syncs whichever of them
// the merge has changed. Called on every event; after the first one for a
// carpark it is a hash comparison and nothing else.
func observe(ctx context.Context, bdp bdplib.Bdp, ev ParkingEvent, facilityID string, carparkID int) {
	childID := carparkProviderID(facilityID, carparkID)

	regMu.Lock()
	// A category-3 event restates the carpark's total capacity. Record it
	// before building the bases so the station advertises the same number the
	// free count is derived from.
	if ev.CountingCategoryId == catTotal {
		if c := normalizeCapacity(ev.Capacity); c != capacityUnknown {
			observedCapacity[childID] = c
		} else {
			delete(observedCapacity, childID)
		}
	}
	registry[facilityID] = entry{
		base:       baseFacility(bdp, facilityID, ev.Carpark.Name),
		typ:        stationTypeParent,
		facilityID: facilityID,
		carparkID:  -1,
		name:       ev.Carpark.Name,
	}
	registry[childID] = entry{
		base:       baseCarpark(bdp, facilityID, carparkID, ev.Carpark.Name),
		typ:        stationType,
		facilityID: facilityID,
		carparkID:  carparkID,
		name:       ev.Carpark.Name,
	}
	regMu.Unlock()

	syncChanged(ctx, bdp, []string{facilityID, childID})
}

// rebuildBases regenerates the provider-derived half of every known station.
//
// Called when the counting categories change: a new carpark, a retired one or a
// corrected capacity all invalidate what was built from the previous topology.
func rebuildBases(ctx context.Context) {
	regMu.Lock()
	defer regMu.Unlock()
	for id, e := range registry {
		if e.carparkID < 0 {
			e.base = baseFacility(bdpForRebuild, e.facilityID, e.name)
		} else {
			e.base = baseCarpark(bdpForRebuild, e.facilityID, e.carparkID, e.name)
		}
		registry[id] = e
	}
	logger.Get(ctx).Info("rebuilt station bases from counting categories", "stations", len(registry))
}

// bdpForRebuild is the client the change callbacks rebuild with. The callbacks
// are registered before any event is handled and only ever run with the single
// client main built, so this is set once at startup rather than threaded
// through the reference table's callback signature.
var bdpForRebuild bdplib.Bdp

// syncChanged re-merges the given stations (or all known ones when ids is nil)
// and pushes only those whose result differs from what was last synced.
func syncChanged(ctx context.Context, bdp bdplib.Bdp, ids []string) {
	log := logger.Get(ctx)

	regMu.Lock()
	if ids == nil {
		ids = make([]string, 0, len(registry))
		for id := range registry {
			ids = append(ids, id)
		}
	}
	type pending struct {
		id      string
		typ     string
		station bdplib.Station
	}
	var changed []pending
	for _, id := range ids {
		e, ok := registry[id]
		if !ok {
			continue
		}
		s := merged(id, e.base)
		isChanged, err := syncedStations.Changed(id, s)
		if err != nil {
			log.Error("failed hashing station for change detection", "id", id, "err", err)
			continue
		}
		if isChanged {
			changed = append(changed, pending{id, e.typ, s})
		}
	}
	regMu.Unlock()

	if len(changed) == 0 {
		return
	}

	// Group by station type: SyncStations takes one type per call.
	byType := map[string][]bdplib.Station{}
	for _, p := range changed {
		byType[p.typ] = append(byType[p.typ], p.station)
	}
	for typ, list := range byType {
		// onlyActivate: this transformer never sees the full station set, so it
		// must never be read as "everything else is gone".
		if err := bdp.SyncStations(typ, list, true, true); err != nil {
			log.Error("failed syncing stations", "station_type", typ, "count", len(list), "err", err)
			return
		}
	}

	// Only now is it safe to remember them: a failed push must stay pending.
	for _, p := range changed {
		if err := syncedStations.Commit(p.id, p.station); err != nil {
			log.Error("failed caching synced station", "id", p.id, "err", err)
		}
	}
	log.Info("synced changed stations", "count", len(changed))
}

// resyncAll re-evaluates every known station after a reference table changed.
func resyncAll(ctx context.Context, bdp bdplib.Bdp) {
	regMu.Lock()
	known := len(registry)
	regMu.Unlock()
	if known == 0 {
		logger.Get(ctx).Info("reference data changed, but no station has been observed yet")
		return
	}
	syncChanged(ctx, bdp, nil)
}

// providerIDs derives the facility id and carpark id an event refers to.
func providerIDs(ev ParkingEvent) (facilityID string, carparkID int) {
	return fmt.Sprintf("%07d", ev.Carpark.FacilityNr), ev.Carpark.Id
}

// carparkProviderID is the station identity for one carpark.
func carparkProviderID(facilityID string, carparkID int) string {
	return fmt.Sprintf("%s_%d", facilityID, carparkID)
}

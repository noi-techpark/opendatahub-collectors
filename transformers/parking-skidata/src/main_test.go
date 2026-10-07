// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/noi-techpark/go-bdp-client/bdplib"
	"github.com/noi-techpark/go-bdp-client/bdpmock"
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable"
	tr "github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
	"github.com/noi-techpark/opendatahub-go-sdk/testsuite"
	"github.com/stretchr/testify/require"
)

// loadTestFixtures wires the transformer the way main() does, minus the
// ingestion stack: both reference tables are injected as fixtures instead of
// being bootstrapped from the raw data lake.
//
// Stations are deliberately not preloaded — there is no list any more. Each
// test discovers them from the events it feeds in.
func loadTestFixtures(t *testing.T) bdplib.Bdp {
	t.Helper()
	cache = NewCache()

	// Discovery state is package-level. Reset it so one test cannot leak into
	// the next through the registry or the synced-station cache — a station
	// left cached would make the next test's sync look like a no-op.
	regMu.Lock()
	registry = map[string]entry{}
	observedCapacity = map[string]int{}
	regMu.Unlock()
	syncedStations = tr.NewCache[bdplib.Station]()

	refTable = loadJSONFixture[map[string]any](t, "testdata/enrichment.json")
	catTable = loadJSONFixture[FacilityCategories](t, "testdata/counting_categories.json")
	t.Cleanup(func() { refTable = nil; catTable = nil })

	b := bdpmock.MockFromEnv(bdplib.BdpEnv{})
	bdpForRebuild = b
	return b
}

// loadJSONFixture reads a `{"<key>": {…}}` document — the same shape a
// reference table holds at runtime.
func loadJSONFixture[T any](t *testing.T, path string) reftable.Lookup[T] {
	t.Helper()
	raw, err := os.ReadFile(path)
	require.Nil(t, err, path)
	tbl, err := reftable.StaticFromJSON[T](raw)
	require.Nil(t, err, path)
	return tbl
}

func event(facilityNr, carparkID, categoryID, level, capacity int, name string) ParkingEvent {
	return ParkingEvent{
		Name:               name,
		Level:              level,
		Capacity:           capacity,
		CountingCategoryId: categoryID,
		Carpark:            Carpark{Name: "Demo", FacilityNr: facilityNr, Id: carparkID},
	}
}

func feed(t *testing.T, b bdplib.Bdp, ev ParkingEvent) {
	t.Helper()
	raw := rdb.Raw[ParkingEvent]{Rawdata: ev, Timestamp: time.Unix(0, 0).UTC()}
	require.Nil(t, Transform(context.TODO(), b, &raw))
}

// records returns every (datatype -> value) pushed for a station type.
func records(t *testing.T, b bdplib.Bdp, stype string) map[string]int {
	t.Helper()
	out := map[string]int{}
	for _, dm := range b.(*bdpmock.BdpMock).Requests().SyncedData[stype] {
		for _, station := range dm.Branch {
			for datatype, leaf := range station.Branch {
				for _, rec := range leaf.Data {
					if v, ok := toInt(rec.Value); ok {
						out[datatype] = v
					}
				}
			}
		}
	}
	return out
}

func toInt(v any) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	case json.Number:
		i, err := n.Int64()
		return int(i), err == nil
	}
	return 0, false
}

// metaInt reads a numeric metadata value regardless of how it was produced.
// A station the merge touched carries json.Number in its untyped metadata, so
// the published literal stays exact; one it did not carries a plain int. Both
// serialize identically.
func metaInt(t *testing.T, s bdplib.Station, key string) int {
	t.Helper()
	v, ok := toInt(s.MetaData[key])
	require.True(t, ok, "metadata %q is not numeric: %#v", key, s.MetaData[key])
	return v
}

func syncedStations_(t *testing.T, b bdplib.Bdp) []bdplib.Station {
	t.Helper()
	var out []bdplib.Station
	for _, calls := range b.(*bdpmock.BdpMock).Requests().SyncedStations {
		for _, c := range calls {
			out = append(out, c.Stations...)
		}
	}
	return out
}

// stationByProviderID returns the most recently synced copy of a station. The
// last one matters: a re-sync appends, so taking the first would report the
// state before the change under test.
func stationByProviderID(t *testing.T, b bdplib.Bdp, providerID string) (bdplib.Station, bool) {
	t.Helper()
	want := clib.GenerateID(ID_TEMPLATE, providerID)
	var found bdplib.Station
	ok := false
	for _, s := range syncedStations_(t, b) {
		if s.Id == want {
			found, ok = s, true
		}
	}
	return found, ok
}

// ---------------------------------------------------------------- datatypes

// The published set is fixed. Deriving it from what the provider sends is what
// let per-floor labels in three languages mint datatypes that were then
// silently dropped for a quarter of all traffic.
func TestPublishedDataTypesAreExactlySix(t *testing.T) {
	got := allDataTypeNames()
	want := []string{
		"free", "free_short_stay", "free_subscribers",
		"occupied", "occupied_short_stay", "occupied_subscribers",
	}
	require.Equal(t, want, got)
}

func TestSuffixForOnlyPublishesTotalShortStayAndSubscribers(t *testing.T) {
	cases := map[int]struct {
		suffix    string
		published bool
	}{
		catTotal:       {"", true},
		catShortStay:   {"short_stay", true},
		catSubscribers: {"subscribers", true},
		4:              {"", false}, // Autobus / Meusburger / QRCode
		5:              {"", false}, // Camper / Nobis Privat
		6:              {"", false}, // Nobis Abo
		0:              {"", false}, // what a countingAreaId message decodes to
	}
	for id, want := range cases {
		suffix, published := suffixFor(id)
		require.Equal(t, want.published, published, "category %d", id)
		if published {
			require.Equal(t, want.suffix, suffix, "category %d", id)
		}
	}
}

// A per-floor message arrives with countingAreaId, which this DTO does not
// decode, so it lands as category 0. It must produce nothing at all.
func TestFloorMessageProducesNothing(t *testing.T) {
	b := loadTestFixtures(t)

	var ev ParkingEvent
	require.Nil(t, json.Unmarshal([]byte(`{
	  "name":"Ebene -2","level":23,"capacity":49,"countingAreaId":3,
	  "carpark":{"name":"RIBOPARKING","facilityNr":406009,"id":0}
	}`), &ev))
	require.Equal(t, 0, ev.CountingCategoryId, "countingAreaId must not populate the category")

	feed(t, b, ev)

	require.Empty(t, records(t, b, stationType), "a floor message published measurements")
	require.Empty(t, records(t, b, stationTypeParent))
}

func TestNonPublishedCategoryProducesNoMeasurements(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(602581, 0, 4, 10, 50, "Autobus"))
	require.Empty(t, records(t, b, stationType), "category 4 published measurements")
}

// A dropped category must still register the station: the carpark exists even
// if this particular measurement is not published.
func TestNonPublishedCategoryStillRegistersTheStation(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(602581, 0, 4, 10, 50, "Autobus"))
	_, ok := stationByProviderID(t, b, "0602581_0")
	require.True(t, ok, "the carpark station was not synced")
}

// ---------------------------------------------------------------- capacity

func TestCarparkCapacityIsTheTotalOnly(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(607242, 0, catTotal, 70, 245, "Totale"))

	s, ok := stationByProviderID(t, b, "0607242_0")
	require.True(t, ok)

	require.Equal(t, 245, metaInt(t, s, "capacity"), "carpark capacity must be the category-3 total")
	for _, gone := range []string{
		"capacity_short_stay", "capacity_subscribers",
		"free_limit", "free_limit_short_stay",
		"occupancy_limit", "occupancy_limit_subscribers",
	} {
		require.NotContains(t, s.MetaData, gone,
			"%s is a live quota or a signage threshold and must not be metadata", gone)
	}
}

// The defect that started all of this: a station advertising 125 slots while
// reporting 154 free. The capacity published as metadata and the capacity the
// free count is computed from have to be the same number.
func TestFreeNeverExceedsTheAdvertisedCapacity(t *testing.T) {
	b := loadTestFixtures(t)

	// The categories say 245 for this carpark; the event says 490. Whichever
	// wins, the two published figures must agree with each other.
	feed(t, b, event(600015, 0, catTotal, 95, 490, "Totale"))

	s, ok := stationByProviderID(t, b, "0600015_0")
	require.True(t, ok)
	capacity := metaInt(t, s, "capacity")

	free := records(t, b, stationType)["free"]
	require.Equal(t, 490, capacity, "the live event states the total capacity")
	require.LessOrEqual(t, free, capacity,
		"published free (%d) exceeds the advertised capacity (%d)", free, capacity)
	require.Equal(t, 395, free)
}

// Before any event arrives there is nothing live to go on, so the counting
// categories supply the capacity — which is what the facility sum and
// hydration need at boot.
func TestCapacityFallsBackToTheCountingCategories(t *testing.T) {
	b := loadTestFixtures(t)

	// A non-total category never restates the carpark total.
	feed(t, b, event(600015, 0, catShortStay, 4, 104, "SostaBreve"))

	s, ok := stationByProviderID(t, b, "0600015_0")
	require.True(t, ok)
	require.Equal(t, 245, metaInt(t, s, "capacity"),
		"with no category-3 event yet, the counting categories supply the total")
}

func TestFacilityCapacityIsTheSumOfItsCarparks(t *testing.T) {
	cats := FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catTotal, Capacity: 200},
		{CarparkId: 1, CountingCategoryId: catTotal, Capacity: 50},
		{CarparkId: 0, CountingCategoryId: catShortStay, Capacity: 150},
	}
	require.Equal(t, 250, cats.FacilityCapacity())
	require.Equal(t, []int{0, 1}, cats.CarparkIDs())
}

// The sentinel is the bug that put "19998 free spaces" on the public API.
func TestSentinelCapacityBecomesUnknown(t *testing.T) {
	require.Equal(t, capacityUnknown, normalizeCapacity(sentinelCapacity))
	require.Equal(t, capacityUnknown, normalizeCapacity(10000))
	require.Equal(t, capacityUnknown, normalizeCapacity(-5))
	require.Equal(t, 245, normalizeCapacity(245))
	require.Equal(t, 0, normalizeCapacity(0))
}

// Only carparks with a known capacity contribute; a facility with none is
// itself unknown rather than zero.
func TestUnknownCapacityDoesNotContributeToTheFacility(t *testing.T) {
	mixed := FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catTotal, Capacity: 222},
		{CarparkId: 1, CountingCategoryId: catTotal, Capacity: sentinelCapacity},
	}
	require.Equal(t, 222, mixed.FacilityCapacity(), "the sentinel carpark must contribute nothing")
	require.Equal(t, capacityUnknown, mixed.TotalCapacity(1))

	allUnknown := FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catTotal, Capacity: sentinelCapacity},
		{CarparkId: 1, CountingCategoryId: catTotal, Capacity: sentinelCapacity},
	}
	require.Equal(t, capacityUnknown, allUnknown.FacilityCapacity())

	noTotalRow := FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catShortStay, Capacity: 100},
	}
	require.Equal(t, capacityUnknown, noTotalRow.TotalCapacity(0))
}

// ------------------------------------------------------------- measurements

// occupied is a real count and is published; free is not computable without a
// capacity and is withheld rather than invented.
func TestSentinelEventPublishesOccupiedButNotFree(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(602581, 0, catTotal, 137, sentinelCapacity, "Totale"))

	got := records(t, b, stationType)
	require.Equal(t, 137, got["occupied"], "occupied is a real count and must survive")
	require.NotContains(t, got, "free", "free was published from a sentinel capacity")
}

func TestFreeIsCapacityMinusLevel(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(607242, 0, catShortStay, 5, 160, "SostaBreve"))

	got := records(t, b, stationType)
	require.Equal(t, 155, got["free_short_stay"])
	require.Equal(t, 5, got["occupied_short_stay"])
}

func TestOutOfRangeValuesAreClamped(t *testing.T) {
	b := loadTestFixtures(t)
	// level above capacity would otherwise yield a negative free
	feed(t, b, event(607242, 0, catTotal, 300, 245, "Totale"))

	got := records(t, b, stationType)
	require.Equal(t, 0, got["free"])
	require.Equal(t, 300, got["occupied"], "occupied is reported as measured")
}

// ---------------------------------------------------------------- facility

// The provider reports one carpark at a time and a carpark can stay silent for
// most of a day. A sum over only the carparks heard from is not a total, so it
// must not be published at all.
func TestFacilityTotalIsWithheldUntilEveryCarparkReports(t *testing.T) {
	b := loadTestFixtures(t)

	// 0601393 has three carparks in the fixture.
	cats := facilityCategories("0601393")
	require.Len(t, cats.CarparkIDs(), 3, "fixture changed; this test needs a multi-carpark facility")

	feed(t, b, event(601393, 0, catTotal, 10, 150, "Totale"))
	require.Empty(t, records(t, b, stationTypeParent),
		"a facility total was published from one carpark out of three")

	feed(t, b, event(601393, 1, catTotal, 20, 140, "Totale"))
	require.Empty(t, records(t, b, stationTypeParent),
		"a facility total was published from two carparks out of three")

	feed(t, b, event(601393, 2, catTotal, 5, 70, "Totale"))
	got := records(t, b, stationTypeParent)
	require.Equal(t, 35, got["occupied"], "10 + 20 + 5")
	require.Equal(t, 325, got["free"], "140 + 120 + 65")
}

func TestFacilityTotalNeedsEveryCarparkInTheCache(t *testing.T) {
	c := NewCache()
	siblings := []string{"0601393_0", "0601393_1", "0601393_2"}

	c.Set("0601393_0", "free", 100, 0)
	_, ok := c.FacilityTotal(siblings, "free")
	require.False(t, ok, "an incomplete sum was reported as complete")

	c.Set("0601393_1", "free", 50, 0)
	_, ok = c.FacilityTotal(siblings, "free")
	require.False(t, ok)

	c.Set("0601393_2", "free", 25, 0)
	v, ok := c.FacilityTotal(siblings, "free")
	require.True(t, ok)
	require.Equal(t, 175, v)

	_, ok = c.FacilityTotal(nil, "free")
	require.False(t, ok, "a facility with no known carparks has no total")
}

// A facility with an unknown carpark capacity still aggregates measurements —
// capacity being unknown does not stop the counts from being real. But the
// sentinel carpark publishes no free, so the facility free stays incomplete.
func TestFacilityFreeStaysIncompleteWhenACarparkHasNoCapacity(t *testing.T) {
	b := loadTestFixtures(t)

	cats := facilityCategories("0600858")
	require.Len(t, cats.CarparkIDs(), 2, "fixture changed; this test needs a two-carpark facility")

	feed(t, b, event(600858, 0, catTotal, 30, 222, "Totale"))
	feed(t, b, event(600858, 1, catTotal, 12, sentinelCapacity, "Totale"))

	got := records(t, b, stationTypeParent)
	require.Equal(t, 42, got["occupied"], "occupied is known for both carparks")
	require.NotContains(t, got, "free", "free was summed across a carpark that has none")
}

// ---------------------------------------------------------------- discovery

func TestUnenrichedStationIsStillSynced(t *testing.T) {
	b := loadTestFixtures(t)
	// 0600015 (demo) is in the category fixture but not in the enrichment one.
	feed(t, b, event(600015, 0, catTotal, 95, 245, "Totale"))

	s, ok := stationByProviderID(t, b, "0600015_0")
	require.True(t, ok, "an unenriched station was not published")
	require.NotEmpty(t, s.Name, "the writer rejects a station with no name")
}

func TestUnchangedStationIsNotResynced(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(607242, 0, catTotal, 70, 245, "Totale"))
	first := len(syncedStations_(t, b))
	require.NotZero(t, first)

	feed(t, b, event(607242, 0, catTotal, 71, 245, "Totale"))
	require.Equal(t, first, len(syncedStations_(t, b)),
		"a station was re-synced although only its measurement changed")
}

func TestEnrichmentChangeResyncsTheStation(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(404467, 0, catTotal, 10, 601, "Totale"))
	before := len(syncedStations_(t, b))

	static := refTable.(*reftable.Static[map[string]any])
	rec, _ := static.Get("0404467_0")
	rec["names"].(map[string]any)["it"] = "Parcheggio Rinominato"
	static.Set("0404467_0", rec)

	resyncAll(context.TODO(), b)

	require.Greater(t, len(syncedStations_(t, b)), before, "an enrichment edit did not reach BDP")
	s, ok := stationByProviderID(t, b, "0404467_0")
	require.True(t, ok)
	require.Equal(t, "Parcheggio Rinominato", s.MetaData["name_it"])
}

// Capacity comes from the provider, not from an operator. When the categories
// change, the provider-derived half of the station has to be rebuilt — not just
// re-merged with the enrichment.
func TestCategoryChangeRebuildsCapacityAndResyncs(t *testing.T) {
	b := loadTestFixtures(t)
	// A non-total event registers the station without stating a total, so the
	// categories are what the capacity comes from.
	feed(t, b, event(607242, 0, catShortStay, 5, 160, "SostaBreve"))

	s, ok := stationByProviderID(t, b, "0607242_0")
	require.True(t, ok)
	require.Equal(t, 245, metaInt(t, s, "capacity"))

	static := catTable.(*reftable.Static[FacilityCategories])
	static.Set("0607242", FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catTotal, Name: "Totale", Capacity: 300},
	})

	rebuildBases(context.TODO())
	resyncAll(context.TODO(), b)

	s, ok = stationByProviderID(t, b, "0607242_0")
	require.True(t, ok)
	require.Equal(t, 300, metaInt(t, s, "capacity"), "a provider capacity change did not reach BDP")
}

// Once the provider has stated a total in an event, a category table that
// disagrees does not override it. The event is the live figure and it is what
// free is computed from, so letting a refresh move the metadata away from it
// would recreate free > capacity.
func TestLiveCapacityIsNotOverriddenByTheCategoryTable(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(607242, 0, catTotal, 70, 245, "Totale"))

	static := catTable.(*reftable.Static[FacilityCategories])
	static.Set("0607242", FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catTotal, Name: "Totale", Capacity: 300},
	})
	rebuildBases(context.TODO())
	resyncAll(context.TODO(), b)

	s, ok := stationByProviderID(t, b, "0607242_0")
	require.True(t, ok)
	require.Equal(t, 245, metaInt(t, s, "capacity"),
		"a stale category table overrode the capacity the provider just reported")
}

// A new carpark appearing in the topology changes the facility, even though no
// event has been seen for it yet.
func TestNewCarparkInTopologyWidensTheFacility(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, event(607242, 0, catTotal, 70, 245, "Totale"))

	f, ok := stationByProviderID(t, b, "0607242")
	require.True(t, ok)
	require.Equal(t, 245, metaInt(t, f, "capacity"))

	static := catTable.(*reftable.Static[FacilityCategories])
	static.Set("0607242", FacilityCategories{
		{CarparkId: 0, CountingCategoryId: catTotal, Name: "Totale", Capacity: 245},
		{CarparkId: 1, CountingCategoryId: catTotal, Name: "Totale", Capacity: 60},
	})
	rebuildBases(context.TODO())
	resyncAll(context.TODO(), b)

	f, ok = stationByProviderID(t, b, "0607242")
	require.True(t, ok)
	require.Equal(t, 305, metaInt(t, f, "capacity"), "the new carpark did not widen the facility")
}

// A malformed message carries no facility. The old CSV station list filtered it
// out; discovery has to refuse it explicitly or it publishes a station called
// "0000000".
func TestEventWithoutAFacilityIsDropped(t *testing.T) {
	b := loadTestFixtures(t)
	feed(t, b, ParkingEvent{Name: "", Level: 0, Capacity: 0, Carpark: Carpark{}})

	require.Empty(t, syncedStations_(t, b), "a malformed event created a station")
	require.Empty(t, records(t, b, stationType))
}

// ---------------------------------------------------------------- topology

func TestCategoriesCarryTheTopology(t *testing.T) {
	b := loadTestFixtures(t)
	_ = b

	cats := facilityCategories("0601393")
	require.Equal(t, []int{0, 1, 2}, cats.CarparkIDs())

	require.Nil(t, facilityCategories("0000000"), "an unknown facility must not invent a topology")
}

func TestURNIndexCoversFacilitiesAndCarparks(t *testing.T) {
	b := loadTestFixtures(t)
	_ = b

	idx := buildURNIndex()
	require.Equal(t, "0601393", idx[clib.GenerateID(ID_TEMPLATE, "0601393")])
	require.Equal(t, "0601393_2", idx[clib.GenerateID(ID_TEMPLATE, "0601393_2")])
}

// ---------------------------------------------------------------- snapshot

func TestTransform_Snapshot(t *testing.T) {
	b := loadTestFixtures(t)

	var in ParkingEvent
	require.Nil(t, testsuite.LoadInputData(&in, "testdata/in1.json"))

	timestamp, err := time.Parse("2006-01-02", "2025-01-01")
	require.Nil(t, err)
	raw := rdb.Raw[ParkingEvent]{Rawdata: in, Timestamp: timestamp}

	require.Nil(t, syncDataTypes(b))
	require.Nil(t, Transform(context.TODO(), b, &raw))

	req := b.(*bdpmock.BdpMock).Requests()

	var out bdpmock.BdpMockCalls
	if err := testsuite.LoadOutput(&out, "testdata/out1.json"); err != nil {
		t.Logf("No snapshot found, generating testdata/out1.json")
		if werr := testsuite.WriteOutput(req, "testdata/out1.json"); werr != nil {
			t.Fatalf("failed to write snapshot: %v", werr)
		}
		t.Log("Snapshot generated. Re-run the test to validate.")
		return
	}
	bdpmock.CompareBdpMockCalls(t, out, req)
}

// The registry used to fill only from incoming events, so an operator's
// enrichment reached a car park that had been quiet since the last restart only
// when it next reported — with nothing anywhere to say so.
func TestSeedingMakesAQuietStationEnrichable(t *testing.T) {
	b := loadTestFixtures(t)

	seedRegistry(t.Context(), b, []stationRow{
		{ProviderID: "0600015", FacilityID: "0600015", CarparkID: -1, Name: "Parcheggio Demo"},
		{ProviderID: "0600015_0", FacilityID: "0600015", CarparkID: 0, Name: "Parcheggio Demo 1"},
	})

	regMu.Lock()
	_, facility := registry["0600015"]
	_, carpark := registry["0600015_0"]
	regMu.Unlock()
	if !facility || !carpark {
		t.Fatal("seeding did not register the pair; enrichment would be inert until they report")
	}

	// No event has ever been handled, and the station still syncs.
	syncChanged(t.Context(), b, []string{"0600015_0"})
	if len(syncedStations_(t, b)) == 0 {
		t.Error("a seeded station did not reach BDP without an event")
	}
}

// The facility took the name of whichever carpark reported last, flipping
// between them and syncing a station each time. Nothing the provider sends
// names a facility.
func TestAFacilityIsNotRenamedByItsCarparks(t *testing.T) {
	b := loadTestFixtures(t)

	regMu.Lock()
	registerLocked(b, "0600015", -1, "First Carpark")
	registerLocked(b, "0600015", 0, "First Carpark")
	registerLocked(b, "0600015", 1, "Second Carpark")
	// A later event from the second carpark, as observe would apply it.
	registerLocked(b, "0600015", -1, "Second Carpark")
	got := registry["0600015"].name
	child := registry["0600015_1"].name
	regMu.Unlock()

	if got != "First Carpark" {
		t.Errorf("facility name = %q; a carpark event renamed it", got)
	}
	if child != "Second Carpark" {
		t.Errorf("carpark name = %q; a carpark still names itself", child)
	}
}

// Seeding fills the registry while the change-detection cache starts empty, so
// the first reconciliation after startup treats every station as changed. Left
// to chance that first reconciliation is an operator's enrichment edit, and one
// car park's change syncs the whole fleet.
func TestBootReconciliationLeavesLaterSyncsProportional(t *testing.T) {
	b := loadTestFixtures(t)
	rows := []stationRow{
		{ProviderID: "0600015", FacilityID: "0600015", CarparkID: -1, Name: "Demo"},
		{ProviderID: "0600015_0", FacilityID: "0600015", CarparkID: 0, Name: "Demo 1"},
		{ProviderID: "0600015_1", FacilityID: "0600015", CarparkID: 1, Name: "Demo 2"},
	}
	seedRegistry(t.Context(), b, rows)
	syncChanged(t.Context(), b, nil) // what main does at boot
	afterBoot := len(syncedStations_(t, b))
	if afterBoot == 0 {
		t.Fatal("the boot reconciliation pushed nothing")
	}

	// An enrichment edit now touches only what it changed.
	before := len(syncedStations_(t, b))
	resyncAll(t.Context(), b)
	if got := len(syncedStations_(t, b)); got != before {
		t.Errorf("a no-op resync pushed %d more stations; the cache did not take", got-before)
	}
}

<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Reference tables

A transformer consumes exactly one data queue. When the transformation also needs slowly-changing
lookup data, such as station descriptions, code tables, per-provider configuration or a provider's
category list, that data is a **reference table**
(`github.com/noi-techpark/opendatahub-go-sdk/ingest/reftable`). Don't bake it into the image as a
CSV, and don't read it from a spreadsheet at runtime.

```
any publisher (rest-push, a collector, a backoffice)
   → raw data lake  db <db>, collection <collection>, one document per change,
                    key in meta.<Key>, optional meta.op (upsert|delete) and meta.schema
   → router notification on `routed`, routing key <db>.<collection>
transformer: reftable reconciles the newest document per key into an in-memory map,
             re-reads it on a timer, and applies notifications as they arrive
             → table.Get(key) inside Transform
```

- **Correctness comes from the reconcile.** The table re-reads the whole compacted collection at
  startup and every `Sweep` (default 5 minutes). The queue subscription only lowers latency, and
  running without `MQ_*` is a supported, reconcile-only configuration.
- **The table stays small:** tens to hundreds of rows, read in one page. It is configuration, not a
  second record source. If it grows to the size of the data, it's a data stream and belongs on its
  own pipeline.
- **A stale table keeps serving** what it last had. Staleness is state: `Table.Health()` and
  `Set.Healthy()` report it, and that is what a readiness or liveness probe should read.

Exemplars:
- `transformers/parking-skidata/src/category_source.go`: provider-derived data, decoded into a
  typed struct.
- `transformers/parking-offstreet-famas/src/enrichment.go`: operator-maintained station
  descriptions, overlaid with merge rules.
- `transformers/parking-skidata/src/enrichment_source.go`: two tables in one transformer, each with
  `OnChange`.

## Publishing

Any producer that reaches the raw data lake can publish a table. The router routes what it stores
in `<db>/<collection>` under the key `<db>.<collection>`. Two things are the publisher's job:

- **The key**, stored inside the document's `meta` sub-document. It is set through an
  `X-OpenDataHub-<Key>` header, e.g. `X-OpenDataHub-Key: 103`; `Config.Key` names that field.
  Without it the documents cannot be grouped per key.
- **Optional control fields**, also in `meta`:
  - `op`: `upsert` (the default) or `delete`, which removes the key;
  - `schema`: a version id, checked against `Config.Schema`.

How the body is stored decides `Decode`:
- `DecodeText`, the default, for writers that keep a textual body as a string (raw-writer-2);
- `DecodeBase64` for the generic rest-push, which stores the body base64-encoded;
- `DecodeJSON` when the payload is stored as an object.

A wrong decoder fails every record, so check one stored document before choosing.

## Wiring

One env block per table, prefixed so several tables can coexist:

```go
var refEnv struct {
    REFTABLE_DB         string `default:""`
    REFTABLE_COLLECTION string `default:""`
    REFTABLE_KEY        string `default:"key"`
    REFTABLE_MQ_QUEUE   string `default:""`
    REFTABLE_MQ_KEY     string `default:""` // "<db>.<collection>"
    REFTABLE_PAGE_LIMIT int    `default:"0"`
}

var table reftable.Lookup[map[string]any] // the read interface, so tests can inject a fixture

tbl := reftable.New[map[string]any](
    rdb.NewRDBridge(rdb.Env{RAW_DATA_BRIDGE_ENDPOINT: env.RAW_DATA_BRIDGE_ENDPOINT}),
    reftable.Config{
        Name:       "my-table",
        DB:         refEnv.REFTABLE_DB,
        Collection: refEnv.REFTABLE_COLLECTION,
        Key:        refEnv.REFTABLE_KEY,
        PageLimit:  refEnv.REFTABLE_PAGE_LIMIT,
        Schema:     "my.v1",               // optional
        Decode:     reftable.DecodeBase64, // match the publisher
        MQ_URI:      env.MQ_URI,
        MQ_CLIENT:   env.MQ_CLIENT + "-my-table",
        MQ_EXCHANGE: env.MQ_EXCHANGE,
        MQ_QUEUE:    refEnv.REFTABLE_MQ_QUEUE,
        MQ_KEY:      refEnv.REFTABLE_MQ_KEY,
        OnChange: func(ctx context.Context, applied int) { /* optional: react to an edit */ },
    })
set := reftable.NewSet(tbl) // several tables share one Set
ms.FailOnError(ctx, set.Start(ctx), "failed to start the reference table")
defer set.Close()
table = tbl
slog.Info("reference table ready", "entries", tbl.Len())
```

- **The record type:**
  - Use a Go struct when you own the shape and want it validated, e.g. the categories table.
  - Use `map[string]any` when the publisher's shape may grow, and address it by paths (see Merge
    rules). Then a field whose type drifts doesn't take the whole record down.
- **Fail closed when the table is mandatory.** Refuse to start when its env is unset, and let
  `Start` errors kill the pod. Starting on an empty table silently changes the output.
- **`MQ_KEY` must be `<db>.<collection>`.** A wrong key binds a queue that never receives anything,
  and nothing reports it: the table just falls back to the sweep.
- **Every deployment reading a table needs its own `MQ_QUEUE`.** Two deployments sharing one queue
  compete for the notifications, and each misses some.
- **`OnChange`** runs after a refresh that applied at least one record, never during the bootstrap.
  Keep it short. Use it when a change must reach the output before the next data message, e.g. to
  re-sync stations that are only synced rarely.
- **Schema:** when the publisher stamps a version, set `Config.Schema` to it. Records stamped with
  another version are refused before they reach the transformation. A new, incompatible shape is a
  new version, released together with the transformer that reads it. Adding a field the
  transformer ignores needs no new version.

Helm `env:` (test and prod values):

```yaml
REFTABLE_DB: famas
REFTABLE_COLLECTION: parking-enrichment
REFTABLE_KEY: key
REFTABLE_MQ_QUEUE: tr-parking-offstreet-famas-enrichment
REFTABLE_MQ_KEY: famas.parking-enrichment
```

## Merge rules

When the table describes the transformer's output, e.g. station names, coordinates or attributes,
overlay the record with declared rules from `.../ingest/merge` instead of copying fields by hand:

```go
var stationRules = merge.MustCompile(merge.Map{
    Target: "bdp.Station",
    Schema: "parking.v1",            // pass stationRules.Schema() to Config.Schema
    Rules: []merge.Rule{
        {Src: "$.names.it", Dst: "$.metaData.name_it"},
        {Src: "$.gps.lat", Dst: "$.latitude"},
        {Src: "$.netex", Dst: "$.metaData.netex_parking", Policy: merge.Replace},
    },
})

err := merge.Into(&station, func(doc map[string]any) error { return stationRules.Apply(doc, record) })
```

- `MustCompile` runs at package init, so a malformed path is a failed start, not a field that
  quietly stops arriving.
- **Absent source → rule skipped.** A record without a field simply doesn't set it, so adding a
  field to the publisher needs no republish of existing records.
- **JSON `null` deletes** the destination, and an empty string is written as an empty value.
- **Policies:** the default is RFC 7386 merge, where objects merge recursively. `merge.Replace`
  lets the record own a subtree outright.
- **Write no rule for fields the transformation owns,** such as provider-stated values, ids and
  parent links, so a record has no path to them.
- **Keep provider data and operator data in separate tables.** Never merge the two into one
  document: each would overwrite the other's authority.
- **On a merge error,** log it and publish the item without the overlay; don't drop the batch.

## Missing keys

Decide explicitly, per transformer, what an item without a record does, and make it a switch
(`REFTABLE_GATE`) rather than code:
- **gate off:** publish the item as the transformation built it.
- **gate on:** withhold it until a record exists. Use this when the record is the only source of
  something mandatory.

Whatever the gate decides, the item must not vanish silently: report it to a discovery table
(below). A withheld item reaches no silo, so nothing downstream can notice it otherwise.

`Config.AllowMissing` lets the table start before its collection exists. Set it to `!gate`: with
the gate off an empty table changes nothing, so the first record must not be a prerequisite for
starting; with the gate on a missing collection is far more often a misspelt name and must stop
the process.

## Discovery table

`.../ingest/discovery` writes the entities the transformer saw, and what it did with each, into
`<db>/<collection>` (next to the reference table, e.g. `famas/parking-discovery` beside
`famas/parking-enrichment`), one normalised document per key, only when it changed. Anyone reads
it with `reftable.New[discovery.Record]` (`Schema: discovery.Schema`, `DecodeText`), without
knowing the provider format.

```go
rep := discovery.New(bridge, discovery.Config{
    Provider: "famas/parking-discovery", RawWriterURL: env.RAW_WRITER_URL, GoneAfter: 3})
ms.FailOnError(ctx, rep.Start(ctx), "failed to start the discovery reporter")
observer = rep // package-level discovery.Observer; tests inject discovery.NewRecorder()

// per item, inside Transform:
observer.Observe(key, discovery.Entry{Status: discovery.StatusWithheld, Reason: "no-record",
    Name: providerName, Data: map[string]any{"capacity": capacity}, Seed: seed})
// once per message; a failed flush is logged, never fails the message:
if err := observer.Flush(ctx); err != nil { log.Error("discovery flush failed", "err", err) }
```

- **Key** = exactly the key the reference-table lookup uses.
- **Name/Lat/Lon/Parent/Data** = the provider's view before the overlay; nothing that changes every
  poll (no measurements), or every poll rewrites the document.
- **Complete()** only when the payload is the provider's full set (then `GoneAfter` consecutive
  misses mark a key `gone`); never when several deployments share the table or the source is
  event-driven.
- **Seed**: the record that, overlaid with your rules, reproduces what you publish without one,
  built only from values the provider actually states (never 0,0 placeholders or a name that is
  just the code). Test it: the item built without a record equals the item built with its Seed.

## Rollout

Records first, transformer second:
1. Publish the table in the target environment (or, with the gate off, rely on `AllowMissing`).
2. Confirm that the lake has the expected number of distinct `meta.<Key>` values, with the right
   `meta.schema`.
3. Deploy the transformer, and check that it logs the table as ready with the expected count.

Moving an existing pipeline to a gate must neither discontinue what is published nor release what
is filtered: deploy with the gate at today's behaviour, create records from the Seeds of the
`published` discovery entries (never in bulk for `withheld` ones), verify, then turn the gate on.

## Testing

Inject a fixture through the `Lookup` interface with `reftable.StaticFromJSON` (key → record):

```go
func loadFixtures(t *testing.T) {
    t.Helper()
    raw, err := os.ReadFile("testdata/enrichment.json")
    require.NoError(t, err)
    table, err = reftable.StaticFromJSON[map[string]any](raw)
    require.NoError(t, err)
    t.Cleanup(func() { table = nil })
}
```

- When moving data from a baked-in CSV into a table, build the fixture from the records you will
  publish and keep the golden output unchanged. That proves the move changes nothing.
- Cover a missing key, a record lacking a field, and a change reaching the output
  (`reftable.Static.Set`, then whatever `OnChange` calls).
- To exercise paging, set the page limit below the fixture size: a small table never pages
  otherwise.
- **Validation stack:** before starting the transformer, publish the records into
  `<db>/<collection>` with the same key header and encoding the real publisher uses.

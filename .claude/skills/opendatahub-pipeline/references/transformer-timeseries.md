<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Transformers → timeseries silo (BDP)

Target: the timeseries writer (`bdp-core`) via `github.com/noi-techpark/go-bdp-client/bdplib`. Model: **stations** (optionally hierarchical via parent stations) + **data types** (metrics) + **records** (timestamp in Unix **milliseconds**, value, period) + provenance.

## Listener bootstrap

```go
var env struct {
    tr.Env // MQ_URI, MQ_EXCHANGE (default "routed"), MQ_CLIENT, MQ_QUEUE, MQ_KEY, RAW_DATA_BRIDGE_ENDPOINT
}

func main() {
    ctx := context.Background()
    ms.InitWithEnv(ctx, "", &env)
    defer tel.FlushOnPanic()

    b := bdplib.FromEnv(bdplib.BdpEnv{
        BDP_BASE_URL:           os.Getenv("BDP_BASE_URL"),
        BDP_PROVENANCE_VERSION: os.Getenv("BDP_PROVENANCE_VERSION"),
        BDP_PROVENANCE_NAME:    os.Getenv("BDP_PROVENANCE_NAME"),
        BDP_ORIGIN:             os.Getenv("BDP_ORIGIN"),
        BDP_TOKEN_URL:          os.Getenv("ODH_TOKEN_URL"),   // cluster secret oauth-collector binds the ODH_* names
        BDP_CLIENT_ID:          os.Getenv("ODH_CLIENT_ID"),
        BDP_CLIENT_SECRET:      os.Getenv("ODH_CLIENT_SECRET"),
    })

    ms.FailOnError(ctx, syncDataTypes(b), "failed to sync types")   // always at startup

    listener := tr.NewTr[MyDTO](ctx, env.Env)
    err := listener.Start(ctx, TransformWithBdp(b))
    ms.FailOnError(ctx, err, "error while listening to queue")
}

func TransformWithBdp(bdp bdplib.Bdp) tr.Handler[MyDTO] {
    return func(ctx context.Context, payload *rdb.Raw[MyDTO]) error {
        return Transform(ctx, bdp, payload)
    }
}
```

Naming note: go-bdp-client v1.4+ natively reads `BDP_TOKEN_URL`/`BDP_CLIENT_ID`/`BDP_CLIENT_SECRET`; this repo's deployments bind the cluster secret to `ODH_TOKEN_URL`/`ODH_CLIENT_ID`/`ODH_CLIENT_SECRET` and map them explicitly as above. Follow the repo pattern.

**Payload typing rule** — depends on how the collector stored `rawdata`:

- Raw stored as a JSON **object** → `tr.NewTr[MyDTO]` and handle `*rdb.Raw[MyDTO]` directly (e.g. parking-offstreet-skidata).
- Raw stored as a JSON **string** (double-encoded, typical for rest-poller/api-crawler string bodies) →
  `tr.NewTr[string]` + `tr.RawString2JsonMiddleware[MyDTO](Transform)` (e.g. bike-boxes). A `RawBase64JsonMiddleware` exists for binary payloads.

The SDK does the rest: the AMQP message is only a `Notification{Urn}`; the payload is fetched from the Raw Data Bridge; handler error → Nack **without requeue**; success → Ack. Per-message spans and `raw_data_urn` logger correlation are automatic — use `logger.Get(ctx)` for logs inside `Transform`.

## Station sync: two variants

**Default — stations derived from every payload** (exemplar: `transformers/bike-boxes/src/main.go`). Use whenever the payload itself carries the station information. Each message rebuilds the station list and syncs it together with the measurements:

```go
err := bdp.SyncStations(StationType, stations, true, false) // syncState=true, onlyActivate=false:
                                                            // stations absent from this list get deactivated
```

**Variant — station set fully known at startup** (exemplar: `transformers/parking-offstreet-skidata/src/main.go`; note it uses an older go-bdp-client API surface — follow the `FromEnv(bdplib.BdpEnv{...})` bootstrap above for new code). Only possible when the complete station master data is known upfront (e.g. shipped as CSVs in `resources/`). Sync stations **once at startup**, keep the hot path measurements-only:

```go
err := bdp.SyncStations(stationType, stations, true, true)  // onlyActivate=true: activate/update listed
                                                            // stations, never deactivate others
```

Situational techniques some stateful transformers add — reach for them only when the pipeline needs them: an in-memory latest-value cache hydrated at startup from the timeseries API (`github.com/noi-techpark/go-timeseries-client/odhts`, reusing the same OAuth client) so aggregates survive restarts, and a `RESOURCES_OVERLAY` env that merges `*.test.csv` overlay rows in lower environments. **Normally stations do not need overlays** — do not add them by default. If you ship master-data CSVs, add referential-integrity tests for them (see testing reference).

Hierarchy: create parents and children as separate station types, link with `station.ParentStation = parent.Id` (and `ParentStationType`). Station metadata lives in `station.MetaData map[string]any` — only stations carry metadata, never measurements.

## Data types and measurements

```go
// startup
bdp.SyncDataTypes([]bdplib.DataType{
    bdplib.CreateDataType("free", "count", "Free parking spots", "Instantaneous"),
    ...
})

// per message
const period = 600 // seconds; pick per dataset and keep constant
dm := bdp.CreateDataMap()
dm.AddRecord(stationCode, "free", bdplib.CreateRecord(payload.Timestamp.UnixMilli(), value, period))
if err := bdp.PushData(StationType, dm); err != nil {
    return fmt.Errorf("failed to push data: %w", err)
}
```

Record values are `interface{}` — numbers and strings both work. Provenance is handled inside bdplib: it lazily registers `{Lineage: BDP_ORIGIN, DataCollector: BDP_PROVENANCE_NAME, DataCollectorVersion: BDP_PROVENANCE_VERSION}` and stamps every DataMap; CI sets `BDP_PROVENANCE_VERSION` to the git SHA at deploy.

Station IDs: **always** derive the BDP station code with `clib.GenerateID("urn:<domain>:<source>", providerKey)` (UUIDv5 — deterministic, idempotent, globally unique). This is universal — do **not** use the raw provider id as the scode, even when it looks stable and unique (e.g. a provider UUID). Compute the code once per station and reuse it for both the station and all its records; keep the raw provider id in `station.MetaData` (e.g. `meta["provider_id"] = providerKey`) for traceability. Older transformers (`bike-boxes`, `parking-offstreet-skidata`) use raw ids and predate this rule — don't copy that part.

## Error strategy

- Startup (`SyncDataTypes`, initial `SyncStations`): `ms.FailOnError` — panic and let the pod restart.
- Hot path (`Transform`): return wrapped errors (`fmt.Errorf("...: %w", err)`) — the SDK Nacks without requeue. Do **not** call `ms.FailOnError` per message.
- Bad provider values: clamp or skip with a `Warn` log rather than failing the whole batch; skip events for unknown stations.

## Environment variables

`.env.example` template:

```
LOG_LEVEL=DEBUG
MQ_URI=amqp://guest:guest@localhost:5672
MQ_EXCHANGE=routed
MQ_QUEUE=<tuple-dotted>            # e.g. skidata.parking-stations
MQ_KEY=<tuple-dotted>
MQ_CLIENT=tr-<name>
RAW_DATA_BRIDGE_ENDPOINT=http://localhost:2000/

BDP_BASE_URL=http://localhost:8081
BDP_PROVENANCE_VERSION=0.1.0
BDP_PROVENANCE_NAME=tr-<name>
BDP_ORIGIN=<origin>

ODH_TOKEN_URL=
ODH_CLIENT_ID=
ODH_CLIENT_SECRET=

SERVICE_NAME=tr-<name>
TELEMETRY_TRACE_GRPC_ENDPOINT=
```

In-cluster values (helm `env:`): `BDP_BASE_URL: http://bdp-core.core.svc.cluster.local`, `RAW_DATA_BRIDGE_ENDPOINT: http://raw-data-bridge.core.svc.cluster.local:2000`, `TELEMETRY_TRACE_GRPC_ENDPOINT: tempo-distributor-discovery.monitoring.svc.cluster.local:4317`; auth via `envSecretRef` → secret `oauth-collector` (keys `tokenUri`/`clientId`/`clientSecret`) and `MQ_URI` → secret `rabbitmq-svcbind` key `uri`. See the deployment reference.

## bdplib quick interface

```go
type Bdp interface {
    SyncDataTypes(dataTypes []DataType) error
    SyncStations(stationType string, stations []Station, syncState bool, onlyActivate bool) error
    PushData(stationType string, dataMap DataMap) error
    CreateDataMap() DataMap
    GetOrigin() string
    SyncStationStates(stationType string, origin *string, stations []string, onlyActivation bool) error
}
// constructors: FromEnv, CreateStation(id, name, type, lat, lon, origin),
//               CreateDataType(name, unit, description, rtype), CreateRecord(tsMillis, value, period)
```

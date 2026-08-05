<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Transformers → content silo (Content API)

Target: the Open Data Hub Content API (tourism API host) via `github.com/noi-techpark/opendatahub-go-sdk/clib`. Use for structured entities (announcements, POIs, events…). In-repo exemplar: `transformers/traffic-event-prov-bz/src/` (predates clib — it hand-rolls the content client and hash cache, but follows exactly these patterns: URN IDs, tag sync, change-detection hashes, `PutMultiple`). The clib-based reference implementations named by the official docs are `transformers/traffic-event-a22` / `traffic-event-a22-opendata` (upstream on GitHub if not present on the current branch). For new code use clib, not a hand-rolled client.

## Bootstrap

```go
const (
    SOURCE      = "a22"                      // entity Source field + query filter
    ID_TEMPLATE = "urn:announcements:a22"    // prefix for deterministic IDs
)

var env struct {
    tr.Env
    ODH_CORE_URL                 string // https://tourism.api.opendatahub.com/v1 (prod)
    ODH_CORE_TOKEN_URL           string // Keycloak token endpoint; empty → OAuth disabled (read-only/local)
    ODH_CORE_TOKEN_CLIENT_ID     string
    ODH_CORE_TOKEN_CLIENT_SECRET string
}

func main() {
    ms.InitWithEnv(context.Background(), "", &env)
    defer tel.FlushOnPanic()

    contentClient, err := clib.NewContentClient(clib.Config{
        BaseURL:      env.ODH_CORE_URL,
        TokenURL:     env.ODH_CORE_TOKEN_URL,
        ClientID:     env.ODH_CORE_TOKEN_CLIENT_ID,
        ClientSecret: env.ODH_CORE_TOKEN_CLIENT_SECRET,
        DisableOAuth: env.ODH_CORE_TOKEN_URL == "",
    })
    ms.FailOnError(context.Background(), err, "failed to create client")

    // change-detection cache, rebuilt from existing entities at startup
    annCache, err := clib.LoadExisting(context.Background(), contentClient, clib.LoadConfig[Announcement]{
        EntityType:  "Announcement",
        QueryParams: map[string]string{"active": "true", "source": SOURCE,
            "rawfilter": "isnotnull(Mapping.ProviderA22.Id)"},
        IDFunc:      func(a Announcement) string { return *a.ID },
    })
    ms.FailOnError(context.Background(), err, "failed to load announcements")

    tags, err := clib.ReadTagDefs("../resources/tags.json")
    ms.FailOnError(context.Background(), err, "failed to read tags")
    err = clib.SyncTags(context.Background(), contentClient, tags, clib.SyncTagsConfig{Source: "announcement"})
    ms.FailOnError(context.Background(), err, "failed to sync tags") // idempotent: swallows ErrAlreadyExists

    listener := tr.NewTr[string](context.Background(), env.Env)
    err = listener.Start(context.Background(), tr.RawString2JsonMiddleware(Transform))
    ms.FailOnError(context.Background(), err, "error while listening to queue")
}
```

## Idempotent upserts

- **Deterministic IDs**: `clib.GenerateID(ID_TEMPLATE, providerKey)` = `"{prefix}:{uuidv5(input)}"`. Same provider event → same Content ID → re-processing upserts instead of duplicating. Choose the input carefully: a stable provider ID if one exists, otherwise a JSON of the identity-defining fields.
- **Change detection**: hash cache. `cache.HasChanged(id, entity)` hashes the candidate; push only new/changed entities. Struct tags control hashing on the target model: `hash:"ignore"` for volatile/server-managed fields (`Id`, `_Meta`, `FirstImport`, `LastChange`, sync times), `hash:"set"` for order-insensitive slices (`TagIds`, `HasLanguage`). After a successful push, `cache.Set(id, entity, hash)`.
- **Lifecycle**: entities present in cache but absent from the current batch get `EndTime = sourceTime`, are pushed once, then removed from the cache (see the a22 exemplars for the startup post-filter that drops already-ended entries).
- **Bulk write**: collect changed entities into a slice and call one `contentClient.PutMultiple(ctx, "Announcement", list)`; skip the call entirely when the list is empty.

## Target model shape

Mirror the Content API C# model: embed a `Generic` struct (`ID`, `_Meta`, `LicenseInfo`, `Shortname`, `Active`, `HasLanguage`, `Source`, `TagIds`, `Geo map[string]clib.GpsInfo`) plus entity-specific fields. Provider-specific raw fields go under `Mapping.Provider<X>` with **explicit typing** (a `map[string]any` does not hash properly). Multilingual text in `Detail map[string]*clib.DetailGeneric` keyed `it`/`de`/`en` (+ `lld`), and set `HasLanguage` accordingly. Geometry as WKT strings in `GpsInfo.Geometry` (`POINT (lon lat)` / `LINESTRING (...)`).

Tags are declarative JSON in `resources/tags.json` (id + `name-it/de/en` + types), loaded with `clib.ReadTagDefs` and synced at startup with `clib.SyncTags`.

## Error handling

Per-entity mapping errors: log a warning and `continue` — one bad event must not fail the batch. Only the final `PutMultiple` failure is returned (→ Nack, no requeue). Content client errors: `401/403` → `clib.ErrUnauthorized`; `errors.Is(err, clib.ErrAlreadyExists)` is used for idempotent creates; other non-2xx → `*clib.APIError`.

## clib API surface

```go
type ContentAPI interface {
    Get(ctx, apiPath string, queryParams map[string]string, responseStruct interface{}) error
    Post(ctx, apiPath string, queryParams map[string]string, payload interface{}) error
    Put(ctx, apiPath string, id string, payload interface{}) error
    PutMultiple(ctx, apiPath string, payload interface{}) error // bulk upsert (not on all endpoints)
}
```

## Environment / deployment specifics

- Test host: `https://api.tourism.testingmachine.eu/v1`; prod: `https://tourism.api.opendatahub.com/v1`. Token URL: Keycloak `.../realms/noi/protocol/openid-connect/token` on the matching auth host.
- Content transformers get their OAuth client via GitHub secrets injected into `envSecret` at deploy (a dedicated dataprocessor client) — not the `oauth-collector` cluster secret used by BDP transformers. `MQ_URI` still comes from `envSecretRef` → `rabbitmq-svcbind`.
- Static resources (`resources/tags.json`, lookup data) are copied into the image next to the binary; the code reads them via `../resources/...` relative paths (binary runs with cwd `src/` in dev, `/app` with `/resources` in the image — follow the exemplar Dockerfile).

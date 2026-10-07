<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Data collectors

Two architectural families exist in `collectors/`:

1. **go-sdk `dc` collectors** (pull/poll on a cron): `rest-poller`, `multi-rest-poller`, `s3-poller`, `api-crawler`. Fully generic — configured purely via env vars (and optionally a config file); onboarding a new dataset needs **no code**.
2. **Bespoke inbound push servers**: `rest-push` (generic, hosted), `rest-push-skidata`, `echarging-ocpi`, `sftp-server`, `mqtt-client`. These publish to RabbitMQ directly.

## The go-sdk `dc` pattern

Base env struct every dc collector embeds (`opendatahub-go-sdk/ingest/dc`):

```go
type Env struct {
    PROVIDER    string            // "<part1>/<part2>", e.g. rest-poller/traffic-lights-merano
    MQ_URI      string            // amqp://...  (prod: secret rabbitmq-svcbind, key uri)
    MQ_EXCHANGE string `default:"ingress"`
    MQ_CLIENT   string            // e.g. dc-traffic-lights-merano
}
```

Envelope published (JSON, to fanout exchange `ingress`, empty routing key):

```go
rdb.RawAny{Provider: env.PROVIDER, Timestamp: time.Now(), Rawdata: raw} // raw: string, or []byte when RAW_BINARY
```

Canonical main loop (from `collectors/rest-poller/src/main.go` — copy this shape for custom collectors):

```go
var env struct {
    dc.Env
    CRON        string
    RAW_BINARY  bool
    HTTP_URL    string
    HTTP_METHOD string `default:"GET"`
}

func main() {
    ms.InitWithEnv(context.Background(), "", &env)   // logging + envconfig + telemetry + graceful shutdown
    defer tel.FlushOnPanic()

    collector := dc.NewDc[dc.EmptyData](context.Background(), env.Env)

    c := cron.New(cron.WithSeconds())                // cron has a SECONDS field: "0 0/10 * * * *"
    c.AddFunc(env.CRON, func() {
        collector.GetInputChannel() <- dc.NewInput[dc.EmptyData](context.Background(), nil)
    })
    go func() { c.Run() }()

    err := collector.Start(context.Background(), func(ctx context.Context, a dc.EmptyData) (*rdb.RawAny, error) {
        // fetch data; on error return (nil, err)
        return &rdb.RawAny{Provider: env.PROVIDER, Timestamp: time.Now(), Rawdata: raw}, nil
    })
    ms.FailOnError(context.Background(), err, err.Error())
}
```

Telemetry env (from go-sdk `tel`): `SERVICE_NAME`, `SERVICE_VERSION`, `TELEMETRY_TRACE_GRPC_ENDPOINT` (in-cluster: `tempo-distributor-discovery.monitoring.svc.cluster.local:4317`), `LOG_LEVEL`.

## rest-poller — poll one URL

Use for: "hit this URL every N minutes and forward the body".

| Env var | Meaning |
|---|---|
| `CRON` | schedule with seconds field, e.g. `0 0/10 * * * *` |
| `HTTP_URL` | endpoint to poll |
| `HTTP_METHOD` | default `GET` |
| `RAW_BINARY` | `true` → store body as bytes (base64), else string |
| `HTTP_HEADER_*` | any env var with this prefix is parsed as `Name: Value` request header, e.g. `HTTP_HEADER_CALLER="X-Caller-ID: NOI-Techpark"` |
| `PAGING_PARAM_TYPE`, `PAGING_SIZE`, `PAGING_LIMIT_NAME`, `PAGING_OFFSET_NAME` | optional paging |

Deployments are per-instance helm values files: `collectors/rest-poller/infrastructure/helm/<dataset>.yaml`.

## multi-rest-poller — DEPRECATED

**Deprecated — do not use for new pipelines.** Use `api-crawler` (below) for any tree-of-endpoints / multi-step crawl; it is strictly more capable (env-var secret expansion, jq transforms, `forEach`/`forValues`, streaming). Left in the repo only for the instances still bound to it.

(Historical: like rest-poller but walked multiple endpoints defined in a config file `HTTP_CONFIG_PATH`; validated `PROVIDER` is a two-part tuple.)

## s3-poller — poll one S3 object

Use for: provider drops/overwrites a file in an S3 bucket.

| Env var | Meaning |
|---|---|
| `CRON` | schedule with seconds |
| `AWS_REGION`, `AWS_S3_BUCKET_NAME`, `AWS_S3_FILE_NAME` | object to fetch |
| `AWS_ACCESS_KEY_ID`, `AWS_ACCESS_SECRET_KEY` | credentials (inject via secrets at deploy) |
| `RAW_BINARY` | bytes vs string |

## api-crawler — declarative multi-step crawl

Use for: pagination, per-item detail calls, auth flows, jq transforms — all described in a **go-silky YAML config**, no Go code. Engine: `github.com/noi-techpark/go-silky`.

| Env var | Meaning |
|---|---|
| `CRON` | schedule with seconds |
| `CONFIG_PATH` | path to the silky YAML, e.g. `/config/config.yaml` (mounted via chart `configMap`) |

Silky config essentials (examples in `collectors/api-crawler/infrastructure/crawler-config/*.silky.yaml`): top-level `rootContext`, `stream: true|false`, optional `auth` (`type: oauth`, `method: client_credentials`, `tokenUrl`), `headers`, and a `steps` tree of `request`, `forValues` (iterate literal list), `forEach` (iterate a path), `mergeOn` (jq merge of `$res`), `resultTransformer` (jq).

Two publish modes in `main.go` (uses the lower-level `collector.StartCollection` API): `stream: true` → one `RawAny` per top-level entity of an **array** root context (`rootContext: []`); otherwise (`stream: false`) one merged `RawAny` per crawl carrying `GetData()` (the whole root context). Either way the payload is published as a JSON **string** — the transformer consumes it with `tr.NewTr[string]` + `tr.RawString2JsonMiddleware[DTO]`. HTTP retries via `retryablehttp` with 4xx (except 429) treated as unrecoverable.

Secrets via env-var expansion (preferred over injecting into the config): go-silky runs `ExpandEnv` over the raw YAML at load time, so `${VAR}` / `${VAR:-default}` in any field (URL, header, body, auth) is filled from the **container env** at runtime. Put e.g. `Authorization: "key ${AFFLUENCES_API_KEY}"` in the config and inject the token via `envSecret.AFFLUENCES_API_KEY` — the committed config holds only the placeholder and the secret stays in the k8s Secret, never in the ConfigMap. (Some older api-crawler workflows instead `yq` the secret straight into the config with `.auth.clientId = "${{ secrets.X }}"`, which lands it in the ConfigMap — avoid that for new work.)

Config building blocks (go-silky): top-level `rootContext` (`[]` or `{}`), `stream`, global `headers`/`auth`; step types `request`, `forEach` (iterate a jq `path`), `forValues` (iterate a literal `values:` list, bind each with `as:` and reference via `{{ .name }}` in templates / `$ctx.name` in jq). Per-step `resultTransformer` (jq on the response) and merge rules `mergeOn` (into current ctx), `mergeWithParentOn`, `mergeWithContext: {name, rule}`, `noopMerge`; `$res` is the step result. To fan out over a fixed set of ids and emit each as its own root-array entity, `forValues` the ids, fetch per id, and append with `mergeWithContext: {name: root, rule: ". += [$res]"}` (nested detail requests merge into the entity first, then the parent appends). Validate/iterate a config quickly with the go-silky terminal IDE or a tiny `silky.NewApiCrawler(path)` + `Run` harness before wiring deployment.

At deploy time the workflow inlines the config into the values:
`yq -i '.configMap.files["config.yaml"] = load_str("infrastructure/crawler-config/<dataset>.silky.yaml")'`.

## rest-push — hosted push endpoint (no deployment per provider)

Already running: production `https://push.api.opendatahub.com`, test `https://push.api.dev.opendatahub.testingmachine.eu`. Providers POST to `/push/<provider>/<dataset>` with an OAuth2 client-credentials Bearer token; authorization per `<provider>/<dataset>` path is enforced with Keycloak **UMA** resources/policies (audience = the push client). Also serves `GET /health` and `GET /apispec`.

Onboarding a new pusher = create the Keycloak resource/policy/permission for the path and hand out `client_id`/`client_secret` + token URL. The URL path becomes the provider tuple; the body is stored as-is as `rawdata`.

Implementation notes (`collectors/rest-push/src/`): bespoke `echo` server + `gocloak` UMA middleware + raw `amqp091` publish to exchange `ingress` with routing key and `provider` header set to `<provider>/<dataset>`. It does **not** use the go-sdk `dc` package — don't copy it as a template for poll collectors.

## Other collectors (for awareness)

`mqtt-client` (MQTT topic subscriber; needs unique `MQTT_CLIENTID`), `sftp-server` (receives file uploads, forwards to RabbitMQ), `file-loader` (one-shot CI-triggered file load), `lorawan`, `bike-ecocounter`, `traffic-swiss`, plus legacy Java collectors (`google-spreadsheet`, `meteorology-eurac`). Shared local-dev compose pieces live in `collectors/lib/docker-compose/`.

---
name: opendatahub-pipeline
description: Develop, test and deploy Open Data Hub data ingestion pipelines (data collectors and transformers) in this monorepo. Use when creating or changing a collector, a transformer, a provider onboarding, helm values, pipeline GitHub workflows, or pipeline tests.
---

<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# Open Data Hub ingestion pipelines

An ingestion pipeline is composed of at least two microservices:

```
provider → [data collector] → RabbitMQ exchange `ingress` (fanout)
             → router persists raw payload in MongoDB (raw data lake)
             → notification on exchange `routed`
           [transformer] (queue bound via MQ_KEY) → fetches payload from Raw Data Bridge
             → maps and writes to ONE silo:
                 • timeseries (stations + measurements) → BDP writer (go-bdp-client)
                 • content (structured entities)        → Content API (go-sdk clib)
```

Non-negotiable rules:

- **Always use `github.com/noi-techpark/opendatahub-go-sdk`** (`ingest/dc`, `ingest/tr`, `ingest/ms`, `ingest/rdb`, `tel`, `clib`, `testsuite`) plus `github.com/noi-techpark/go-bdp-client` (`bdplib`, `bdpmock`) for timeseries. Never hand-roll AMQP handling, env config, logging, or telemetry.
- The RabbitMQ message a transformer receives is **only a notification (URN)** — the SDK fetches the actual payload from `RAW_DATA_BRIDGE_ENDPOINT`. Never read MongoDB directly in new code.
- Transformers must be **idempotent**: reprocessing the same raw event must not create duplicates.
- Write "Open Data Hub" in prose and `opendatahub` in identifiers — never the abbreviation "ODH" (existing env var names like `ODH_CORE_URL` stay verbatim).
- Every new file needs a REUSE/SPDX header (CI-enforced by `.github/workflows/reuse.yml`).

## Naming conventions

- **Provider tuple**: `PROVIDER=<part1>/<part2>` (e.g. `parking-offstreet/skidata`, `rest-poller/traffic-lights-merano`). The transformer's `MQ_KEY` is the same tuple dot-separated (e.g. `skidata.parking-stations` style keys must match what the collector publishes). Collector and transformer MUST agree on tuple and origin.
- Services: `dc-<name>` (collector) / `tr-<name>` (transformer). Images: `ghcr.io/noi-techpark/opendatahub-collectors/<dc|tr>-<name>`. Kubernetes namespace: `collector`.
- One GitHub workflow per deployed pipeline component: `.github/workflows/dc-<name>.yml` / `tr-<name>.yml` (paired, e.g. `dc-api-crawler-bike-boxes` ↔ `tr-bike-boxes`).

## Step 1 — choose the collector (reuse a generic one; new code is the last resort)

| Situation | Use | Onboarding effort |
|---|---|---|
| Provider can POST data to us | `collectors/rest-push` (hosted service) | Keycloak UMA resource/policy + credentials — no code, no deployment |
| Poll a single REST URL on a cron | `collectors/rest-poller` | helm values file + workflow only |
| Poll a tree of endpoints from one config / multi-step crawl (auth, pagination, jq transforms, foreach) | `collectors/api-crawler` (declarative go-silky YAML) | silky config + helm values + workflow |
| ~~Poll a tree of endpoints from one config~~ | ~~`collectors/multi-rest-poller`~~ **DEPRECATED — do not use for new pipelines; use `api-crawler`** | — |
| Provider drops files in S3 | `collectors/s3-poller` | helm values file + workflow only |
| Provider publishes to an MQTT broker | `collectors/mqtt-client` | helm values + workflow |
| Provider uploads via SFTP | `collectors/sftp-server` | helm values + workflow |
| None of the above fits | custom collector with go-sdk `ingest/dc` | copy `collectors/rest-poller` as template |

Onboarding a new dataset on a generic collector requires **no code**: add a per-instance helm values file under `collectors/<collector>/infrastructure/helm/` and a `dc-<collector>-<dataset>.yml` workflow. Details: [references/collectors.md](references/collectors.md).

## Step 2 — scaffold the transformer

Always scaffold with the wizard:

```bash
cd transformers/boilerplate && ./setup_go.sh
```

It asks for project name, the two provider-tuple parts, and origin (must match the collector), then generates `src/`, Dockerfile, docker-compose, helm values, `.env.example`, the `.github/workflows/tr-<project>.yml` workflow, and runs `go mod init` + `go get` for go-bdp-client and the go-sdk.

## Step 3 — pick the target silo and follow its reference

- **Timeseries** (stations + measurements, BDP): [references/transformer-timeseries.md](references/transformer-timeseries.md)
- **Content** (structured entities, Content API): [references/transformer-content.md](references/transformer-content.md)

## New-pipeline checklist (end to end)

1. Collector chosen via the table above; for generic collectors add values file + workflow, for `rest-push` request Keycloak UMA setup.
2. Transformer scaffolded with `setup_go.sh`; provider tuple and origin match the collector.
3. DTO + `Transform` implemented. Error strategy: startup failures → `ms.FailOnError` (panic, fail fast); per-item mapping errors → log and continue; write/batch errors → return wrapped error (message is Nacked, not requeued).
4. Snapshot tests written with `testsuite` + `bdpmock`/`clibmock` **before** wiring deployment: [references/testing.md](references/testing.md).
5. **End-to-end validation (mandatory)**: spin up the bundled stack (`assets/docker-compose.validation.yml`, profiles `timeseries`/`content` — no extra repositories needed, images pull from ghcr.io), run collector + transformer against it, and prove data flow, idempotency (replay the same raw event → no duplicates), and restart resilience (event published while the transformer is down is consumed after restart). Full procedure: [references/validation.md](references/validation.md).
6. Deployment files in place: multi-stage Dockerfile, `docker-compose.build.yml`, helm values (`values.test.yaml` + `values.prod.yaml`), workflow with paths filter → test → build → deploy-test (branch `main`) → deploy-prod (branch `prod`). Secrets only via `envSecret`/`envSecretRef`, never hardcoded: [references/deployment.md](references/deployment.md).
7. SPDX headers on every new file; `.env.example` checked in (`.env` is gitignored); `calls.http` for manual request testing where relevant.
8. After merge to `main`, verify on the testingmachine environment before promoting the `prod` branch.

## Exemplars (ground truth — prefer these over memory or external docs)

| Concern | Look at |
|---|---|
| Generic poll collector | `collectors/rest-poller/src/main.go` |
| S3 collector | `collectors/s3-poller/src/main.go` |
| Declarative crawler + configs | `collectors/api-crawler/src/main.go`, `collectors/api-crawler/infrastructure/crawler-config/*.silky.yaml` |
| Push collector + Keycloak UMA | `collectors/rest-push/src/` |
| Timeseries transformer (stations derived per payload — default) | `transformers/bike-boxes/src/` |
| Timeseries transformer (station set known at startup from CSV) | `transformers/parking-offstreet-skidata/src/` |
| Content transformer | `transformers/traffic-event-prov-bz/src/` (pre-clib client, same patterns); clib-based reference per docs: `transformers/traffic-event-a22-opendata` upstream |
| Deployment (chart, workflows) | `helm/generic-collector/`, `.github/workflows/tr-parking-offstreet-skidata.yml`, `.github/workflows/tr-bike-boxes.yml` |
| Local validation stack | `.claude/skills/opendatahub-pipeline/assets/docker-compose.validation.yml` |
| Local kind cluster recipe | `kind.sh` (repo root) |

Official documentation: <https://docs.opendatahub.com/data-ingestion/getting-started>, the "from scratch" guides, and the collector/transformer blueprint pages under `docs.opendatahub.com/data-ingestion/`.

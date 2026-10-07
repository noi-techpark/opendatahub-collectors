<!--
SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>

SPDX-License-Identifier: CC0-1.0
-->

# End-to-end pipeline validation (mandatory before calling a pipeline done)

Every new or changed pipeline must be validated locally against the real platform services: spin up the stack, run the collector + transformer, and prove data flow, **idempotency**, and **restart resilience**. The skill ships a self-contained compose — the `infrastructure-v2` and `opendatahub-content-api` repositories are NOT needed; all images come from ghcr.io.

## 1. Start the stack

```bash
cd .claude/skills/opendatahub-pipeline/assets
docker compose -f docker-compose.validation.yml --profile timeseries up -d   # BDP target
docker compose -f docker-compose.validation.yml --profile content up -d     # Content API target
# (profiles can be combined; core services start always)
```

| Service | URL | Notes |
|---|---|---|
| RabbitMQ | `amqp://guest:guest@localhost:5672`, UI <http://localhost:15672> | exchanges `ingress`/`routed` appear once writer/router connect |
| MongoDB (raw lake) | `mongodb://localhost:27017/?directConnection=true` | db = tuple part 1, collection = part 2 |
| Raw Data Bridge | `http://localhost:2000` | `GET /urns/<urn>` returns the raw envelope |
| BDP writer | `http://localhost:8081` | `BDP_BASE_URL` |
| Ninja timeseries API | `http://localhost:8082` | **no `/v2` prefix locally**: `GET /flat,node/<StationType>` |
| Content API | `http://localhost:8083/v1` | `ODH_CORE_URL`; seeded schema, empty announcements/tags |

Auth (internet required): the BDP writer and Content API validate JWTs against the public testingmachine Keycloak. For BDP use the public development client (`ODH_TOKEN_URL=https://auth.opendatahub.testingmachine.eu/auth/realms/noi/protocol/openid-connect/token`, `ODH_CLIENT_ID=odh-mobility-datacollector-development`, `ODH_CLIENT_SECRET=7bd46f8f-c296-416d-a13d-dc81e68d0830` — public dev credentials from the official docs). For Content API writes use the transformer's real client from its `.env` (needs Create/Update roles; unauthenticated PUT correctly returns 403).

## 2. Run the transformer against the stack

Export the transformer's `.env` and override the stack endpoints (pick a dedicated validation tuple so you don't collide with real queues):

```bash
cd transformers/<name>/src
set -a && source ../.env && set +a
export MQ_URI=amqp://guest:guest@localhost:5672 MQ_EXCHANGE=routed \
       MQ_QUEUE=validation.<name> MQ_KEY=validation.<name> MQ_CLIENT=tr-<name>-validation \
       RAW_DATA_BRIDGE_ENDPOINT=http://localhost:2000 \
       BDP_BASE_URL=http://localhost:8081 TELEMETRY_ENABLED=false
go run . 2>&1 | tee /tmp/transformer.log
# from a script/agent: detach with `setsid nohup ... &` — if the parent shell dies,
# the AMQP consumer dies with it while the process may linger consumer-less
```

Startup must show: token acquired, data types/tags synced against the LOCAL target, "consuming from AMQP channel".

## 3. Feed data: real collector, or simulate it

Preferably run the actual collector (`go run` with `.env` pointing at `amqp://guest:guest@localhost:5672`). To simulate one, publish the raw envelope the collector would produce (`rawdata` as a JSON **string** for the usual `RawString2JsonMiddleware` transformers):

```bash
python3 - <<'EOF'
import json, urllib.request, base64
raw = open("transformers/<name>/testdata/in.json").read()      # provider-shaped test input
env = json.dumps({"provider": "validation/<name>", "timestamp": "2026-01-01T12:00:00Z", "rawdata": raw})
body = json.dumps({"properties": {}, "routing_key": "", "payload": env, "payload_encoding": "string"}).encode()
req = urllib.request.Request("http://localhost:15672/api/exchanges/%2f/ingress/publish", data=body,
    headers={"content-type": "application/json",
             "authorization": "Basic " + base64.b64encode(b"guest:guest").decode()})
print(urllib.request.urlopen(req).read().decode())              # -> {"routed":true}
EOF
```

The provider tuple `validation/<name>` becomes routing key `validation.<name>` on `routed` — it must match the transformer's `MQ_KEY`.

## 4. Verify the flow (each hop)

```bash
# raw lake: document persisted (db/collection = provider tuple)
docker exec opendatahub-pipeline-validation-mongodb-1 mongosh --quiet validation \
  --eval 'db["<name>"].find().sort({$natural:-1}).limit(1)'
# bridge serves it (URN is in the routed notification: urn:raw:<part1>:<part2>:<id>)
curl -s http://localhost:2000/urns/<urn>
# timeseries silo: stations + latest measurements
curl -s "http://localhost:8082/flat,node/<StationType>" | jq '.data | length'
curl -s "http://localhost:8082/flat,node/<StationType>/<datatype>/latest?limit=5"
# content silo: entities present, write requests visible in the API access log
curl -s "http://localhost:8083/v1/<EntityType>" | jq '.TotalResults'
docker logs opendatahub-pipeline-validation-content-api-1 2>&1 | grep '"method":"PUT"' | tail
```

## 5. Idempotency test (mandatory)

Re-publish the **identical** envelope and assert nothing duplicates:

- **Timeseries**: station count unchanged; measurement history count unchanged (BDP skips records with timestamp ≤ latest):
  `curl -s "http://localhost:8082/flat,node/<Type>/<datatype>/<from>/<to>?limit=-1" | jq '.data | length'` before vs after.
- **Content**: entity `TotalResults` unchanged AND no new `PUT` in the API access log (the hash cache must skip unchanged entities).

## 6. Restart test (mandatory)

1. Kill the transformer.
2. Publish a **new** event (bump the envelope timestamp) while it is down — the durable queue must buffer it (`curl -su guest:guest http://localhost:15672/api/queues/%2f/<queue>` → `messages: 1`; management stats lag a few seconds).
3. Restart the transformer: it must re-sync data types/tags, consume the buffered message, and push exactly the new records — no loss, no duplicates (e.g. history count = stations × timestamps).

## 7. Pipeline-specific checks

Add whatever the design makes risky:
- **Stateful transformers** (in-memory caches, aggregations): verify values after a restart mid-stream — a cache rebuilt empty must not push wrong aggregates (hydrate from the timeseries API or tolerate the gap explicitly).
- **Station lifecycle**: with per-payload sync (`onlyActivate=false`), publish a payload missing a station and verify it gets deactivated, not deleted; with `onlyActivate=true`, verify absent stations stay active.
- **Content lifecycle**: publish a batch without a previously seen entity and verify it is closed (`EndTime`) exactly once.
- **Bad input**: feed a malformed/edge-case payload; the message must be Nacked (dead-lettered, not requeued in a loop) or the bad item skipped with a warning, per the transformer's error strategy.

## 8. Cleanup

```bash
docker compose -f docker-compose.validation.yml --profile timeseries --profile content down -v
```

## Known local quirks (hit during validation of this procedure)

- Local ninja serves `/flat,node/...` **without** the `/v2` prefix used by the public API.
- Current RabbitMQ refuses non-exclusive transient queues — declare probe queues with `"durable":true`.
- The Content API needs `ASPNETCORE_ApiConfig__Url` and a writable `ASPNETCORE_JsonConfig__Jsondir` or it crash-loops at startup (already set in the bundled compose).
- The seeded content database provides the schema and metadata but zero announcements/tags — your transformer's startup tag sync and writes populate them.

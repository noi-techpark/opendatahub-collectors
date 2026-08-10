#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# End-to-end validation of the bike-boxes-loewenbytes pipeline against the
# REAL Loewenbytes test API: runs the actual api-crawler collector (real
# HTTP calls, real OAuth2) and the actual transformer against a disposable
# local copy of the Open Data Hub platform (RabbitMQ, raw data lake, BDP
# writer, ninja timeseries API - see docker-compose.validation.yml).
#
# Usage:
#   cp .env.example .env   # then fill in LOEWENBYTES_CLIENT_ID/SECRET
#   ./validate.sh          # bring up the stack, run one crawl, verify data landed
#   ./validate.sh down     # tear down the local stack
#
# Requires: docker (with compose plugin), go, curl, jq. Internet access
# (real Loewenbytes API + public testingmachine Keycloak for BDP auth).

set -uo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$HERE/../../.." && pwd)"
TR_DIR="$REPO_ROOT/transformers/bike-boxes-loewenbytes"
DC_DIR="$REPO_ROOT/collectors/api-crawler"
STACK_COMPOSE="$REPO_ROOT/.claude/skills/opendatahub-pipeline/assets/docker-compose.validation.yml"
SILKY_CONFIG="$DC_DIR/infrastructure/crawler-config/bike-boxes-loewenbytes.silky.yaml"
LOGDIR="$HERE/.logs"
mkdir -p "$LOGDIR"

log()  { printf '\n\033[1;34m==> %s\033[0m\n' "$1"; }
ok()   { printf '\033[1;32m[OK]\033[0m %s\n' "$1"; }
warn() { printf '\033[1;33m[WARN]\033[0m %s\n' "$1"; }
fail() { printf '\033[1;31m[FAIL]\033[0m %s\n' "$1"; }

if [[ "${1:-}" == "down" ]]; then
  log "Tearing down the local validation stack"
  docker compose -f "$STACK_COMPOSE" --profile timeseries down -v
  exit 0
fi

for bin in docker go curl jq; do
  command -v "$bin" >/dev/null || { fail "missing required tool: $bin"; exit 1; }
done

if [[ ! -f "$HERE/.env" ]]; then
  fail "$HERE/.env not found."
  echo "  cp $HERE/.env.example $HERE/.env"
  echo "  then fill in LOEWENBYTES_CLIENT_ID / LOEWENBYTES_CLIENT_SECRET"
  exit 1
fi

# Defensive cleanup: kill any transformer binary left running by a previous
# (possibly interrupted) run of this script, so it doesn't sit as an extra
# competing AMQP consumer on the queue.
pkill -f "$LOGDIR/tr-bike-boxes-loewenbytes" 2>/dev/null || true
set -a; source "$HERE/.env"; set +a
if [[ -z "${LOEWENBYTES_CLIENT_ID:-}" || -z "${LOEWENBYTES_CLIENT_SECRET:-}" ]]; then
  fail "LOEWENBYTES_CLIENT_ID / LOEWENBYTES_CLIENT_SECRET are empty in $HERE/.env"
  exit 1
fi

log "Starting local platform stack (RabbitMQ, raw data lake, BDP writer, ninja)"
docker compose -f "$STACK_COMPOSE" --profile timeseries up -d

log "Waiting for ninja (timeseries read API) to become reachable"
for i in $(seq 1 60); do
  code=$(curl -s -o /dev/null -w '%{http_code}' "http://localhost:8082/flat,node/BikeParking" || true)
  [[ "$code" == "200" ]] && { ok "ninja is up"; break; }
  sleep 2
  [[ "$i" == 60 ]] && { fail "ninja never became ready, check: docker compose -f $STACK_COMPOSE logs bdp ninja"; exit 1; }
done

log "Building the transformer binary"
TR_BIN="$LOGDIR/tr-bike-boxes-loewenbytes"
( cd "$TR_DIR/src" && go build -o "$TR_BIN" . ) >"$LOGDIR/transformer-build.log" 2>&1
if [[ ! -x "$TR_BIN" ]]; then
  fail "transformer build failed, see $LOGDIR/transformer-build.log"
  cat "$LOGDIR/transformer-build.log"
  exit 1
fi

log "Starting the transformer (tr-bike-boxes-loewenbytes) against the local stack"
# `go run .` was tried first but doesn't reliably forward SIGTERM to its
# compiled child, leaving orphan processes behind that keep competing as
# extra AMQP consumers on later runs. Build once and exec the real binary
# instead, so kill below actually terminates it.
(
  cd "$TR_DIR/src"
  set -a
  source "$TR_DIR/.env.example"
  TELEMETRY_ENABLED=false
  export TELEMETRY_ENABLED
  set +a
  exec "$TR_BIN"
) >"$LOGDIR/transformer.log" 2>&1 &
TR_PID=$!
trap 'kill "$TR_PID" 2>/dev/null || true' EXIT

log "Waiting for the transformer to sync data types and start consuming"
for i in $(seq 1 20); do
  grep -qi "consum\|listening" "$LOGDIR/transformer.log" 2>/dev/null && { ok "transformer is consuming"; break; }
  sleep 1
  [[ "$i" == 20 ]] && warn "didn't see a 'consuming' log line yet, continuing anyway (see $LOGDIR/transformer.log)"
done

log "Building the api-crawler collector binary"
DC_BIN="$LOGDIR/dc-api-crawler"
( cd "$DC_DIR/src" && go build -o "$DC_BIN" . ) >"$LOGDIR/collector-build.log" 2>&1
if [[ ! -x "$DC_BIN" ]]; then
  fail "collector build failed, see $LOGDIR/collector-build.log"
  cat "$LOGDIR/collector-build.log"
  exit 1
fi

log "Running the REAL api-crawler collector once against the live Loewenbytes test API"
# We do NOT want to hammer the real provider API repeatedly. A repeating
# "every second" cron plus kill-on-detect is racy (the next tick can fire
# before the kill signal lands): schedule the crawl for exactly one
# specific second a few seconds from now instead, so the process
# structurally cannot fire twice.
FIRE_AT=$(date -d '+3 seconds' '+%S %M %H')
(
  export PROVIDER=api-crawler/bike-boxes-loewenbytes
  export MQ_URI=amqp://guest:guest@localhost:5672
  export MQ_EXCHANGE=ingress
  export MQ_CLIENT=dc-api-crawler-bike-boxes-loewenbytes-validation
  export CRON="$FIRE_AT * * *"
  export CONFIG_PATH="$SILKY_CONFIG"
  export LOEWENBYTES_CLIENT_ID LOEWENBYTES_CLIENT_SECRET
  export TELEMETRY_ENABLED=false
  export LOG_LEVEL=DEBUG
  export SERVICE_NAME=dc-api-crawler-bike-boxes-loewenbytes-validation
  exec "$DC_BIN"
) >"$LOGDIR/collector.log" 2>&1 &
DC_PID=$!

for i in $(seq 1 150); do
  if grep -qE "collection completed|failed to crawl|panic:" "$LOGDIR/collector.log" 2>/dev/null; then
    break
  fi
  kill -0 "$DC_PID" 2>/dev/null || break
  sleep 0.1
done
kill "$DC_PID" 2>/dev/null || true
wait "$DC_PID" 2>/dev/null || true

echo "--- collector.log (tail) ---"
tail -n 40 "$LOGDIR/collector.log"
echo "----------------------------"

if grep -qi "401\|unauthorized\|invalid_client\|failed to crawl" "$LOGDIR/collector.log"; then
  fail "collector run looks like it failed (auth or request error) - see $LOGDIR/collector.log above"
fi

log "Waiting for the message to flow: collector -> raw lake -> transformer -> BDP"
sleep 6

log "Querying the local ninja API for BikeParking stations"
STATIONS_JSON=$(curl -s "http://localhost:8082/flat,node/BikeParking")
COUNT=$(echo "$STATIONS_JSON" | jq '.data | length' 2>/dev/null || echo 0)

if [[ "$COUNT" -gt 0 ]]; then
  ok "$COUNT BikeParking station(s) synced"
  echo "$STATIONS_JSON" | jq '.data[0]'
  log "Latest 'free' measurements (all stations)"
  curl -s "http://localhost:8082/flat,node/BikeParking/free/latest?limit=5" | jq
else
  fail "no BikeParking stations found in ninja - check $LOGDIR/collector.log and $LOGDIR/transformer.log"
fi

log "Done"
echo "Transformer log:  $LOGDIR/transformer.log"
echo "Collector log:    $LOGDIR/collector.log"
echo "RabbitMQ UI:      http://localhost:15672 (guest/guest)"
echo "Ninja API:        http://localhost:8082/flat,node/BikeParking"
echo
echo "Re-run this script any time to trigger another crawl (idempotent: same"
echo "stations get updated, not duplicated). Tear the stack down with:"
echo "  $0 down"

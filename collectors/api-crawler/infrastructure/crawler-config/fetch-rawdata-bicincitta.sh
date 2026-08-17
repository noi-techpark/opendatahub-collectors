#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# Replays the exact request graph of bike-boxes.silky.yaml (bicincitta/weelo
# API) against the real API and writes the merged result to a local JSON
# file, shaped exactly like the raw data the api-crawler collector publishes
# (and that transformers/bike-boxes/src/dto.go::BikeBoxRawData decodes):
#
#   { "en": [...], "de": [...], "lld": [...], "it": [...] }
#
# where en/de/lld locations carry a `stations` array trimmed to
# {address, locationName, name, stationID} (translation pass), and it
# carries full station objects including nested `places` (data pass).
#
# Use this to compare against fetch-rawdata-loewenbytes.sh output and see
# whether the bike-boxes transformer could run unmodified against the
# loewenbytes API.
#
# Usage:
#   cp .env.example .env   # then fill in BICINCITTA_CLIENT_ID / BICINCITTA_CLIENT_SECRET
#   ./fetch-rawdata-bicincitta.sh
#
# Requires: curl, jq

set -euo pipefail
cd "$(dirname "$0")"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

: "${BICINCITTA_CLIENT_ID:?Set BICINCITTA_CLIENT_ID in .env}"
: "${BICINCITTA_CLIENT_SECRET:?Set BICINCITTA_CLIENT_SECRET in .env}"

BASE_URL="https://sta.api.weelo.it"
OUT_DIR="rawdata"
mkdir -p "$OUT_DIR"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="$OUT_DIR/bicincitta-${TS}.json"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "== Fetching OAuth token =="
TOKEN_RESPONSE=$(curl -sS -X POST "$BASE_URL/connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$BICINCITTA_CLIENT_ID" \
  --data-urlencode "client_secret=$BICINCITTA_CLIENT_SECRET")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token // empty')
if [[ -z "$ACCESS_TOKEN" ]]; then
  echo "Failed to obtain access token. Response:"
  echo "$TOKEN_RESPONSE"
  exit 1
fi
echo "Got token."

LOCATION_PARAM="locationId"
# shellcheck disable=SC1091
source "./lib-nested-fetch.sh"

echo
echo "== Translation pass (en, de, lld): locations -> stations (trimmed fields) =="
for lang in en de lld; do
  fetch_language "$lang" 0 "$WORK_DIR/result_${lang}.json"
done

echo
echo "== Data pass (it): locations -> stations (full) -> station places =="
fetch_language "it" 1 "$WORK_DIR/result_it.json"

jq -n \
  --slurpfile en "$WORK_DIR/result_en.json" \
  --slurpfile de "$WORK_DIR/result_de.json" \
  --slurpfile lld "$WORK_DIR/result_lld.json" \
  --slurpfile it "$WORK_DIR/result_it.json" \
  '{en: $en[0], de: $de[0], lld: $lld[0], it: $it[0]}' > "$OUT_FILE"

echo
echo "== Done =="
echo "Raw data written to: $OUT_FILE"
jq -r 'to_entries[] | "  \(.key): \(.value | length) location(s), \([.value[].stations[]] | length) station(s)"' "$OUT_FILE"

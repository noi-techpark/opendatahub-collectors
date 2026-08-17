#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# Fetches Loewenbytes raw data two ways and writes both to one local JSON
# file, { "flat": {...}, "nested": {...} }:
#
#   flat   - replays bike-boxes-loewenbytes.silky.yaml exactly: per
#            language, a single flat GET /resources/stations?languageID=X
#            call (no locationID filter). Shaped like the raw data the
#            api-crawler collector actually publishes for this provider
#            today: { "it": [...], "de": [...], "en": [...], "lld": [...] }
#            of full Station objects.
#
#   nested - replays bike-boxes.silky.yaml's (bicincitta's) request graph
#            instead: per language, GET /resources/locations, then per
#            location GET /resources/stations?languageID=X&locationID=<id>,
#            then per station GET /resources/station?languageID=X&stationID=<id>
#            merged in as .places. Same { "it": [...], ... } shape as flat,
#            but each entry is a Location{locationID, name, stations:[...]}
#            instead of a bare Station - directly comparable to
#            fetch-rawdata-bicincitta.sh's output.
#
# The two can disagree: /resources/locations groups by location and the
# embedded station stubs there are minimal (stationID, type only), so this
# also checks whether going through locations surfaces stations that the
# flat call misses (see test-loewenbytes-locations.sh for the earlier,
# count-only version of this check).
#
# Use this to compare against fetch-rawdata-bicincitta.sh output and see
# whether the bike-boxes transformer could run unmodified against this API.
#
# Usage:
#   cp .env.example .env   # then fill in LOEWENBYTES_CLIENT_ID / LOEWENBYTES_CLIENT_SECRET
#   ./fetch-rawdata-loewenbytes.sh
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

: "${LOEWENBYTES_CLIENT_ID:?Set LOEWENBYTES_CLIENT_ID in .env}"
: "${LOEWENBYTES_CLIENT_SECRET:?Set LOEWENBYTES_CLIENT_SECRET in .env}"

BASE_URL="https://test.app.loewenbytes.com/api/v1"
OUT_DIR="rawdata"
mkdir -p "$OUT_DIR"
TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUT_FILE="$OUT_DIR/loewenbytes-${TS}.json"

WORK_DIR="$(mktemp -d)"
trap 'rm -rf "$WORK_DIR"' EXIT

echo "== Fetching OAuth token =="
TOKEN_RESPONSE=$(curl -sS -X POST "$BASE_URL/connect/token" \
  -H "Content-Type: application/x-www-form-urlencoded" \
  --data-urlencode "grant_type=client_credentials" \
  --data-urlencode "client_id=$LOEWENBYTES_CLIENT_ID" \
  --data-urlencode "client_secret=$LOEWENBYTES_CLIENT_SECRET")

ACCESS_TOKEN=$(echo "$TOKEN_RESPONSE" | jq -r '.access_token // empty')
if [[ -z "$ACCESS_TOKEN" ]]; then
  echo "Failed to obtain access token. Response:"
  echo "$TOKEN_RESPONSE"
  exit 1
fi
echo "Got token."

LOCATION_PARAM="locationID"
# shellcheck disable=SC1091
source "./lib-nested-fetch.sh"

echo
echo "== Flat pass: GET /resources/stations per language (no locationID filter) =="
for lang in it de en lld; do
  stations_json=$(auth_get "$BASE_URL/resources/stations?languageID=${lang}")
  if ! echo "$stations_json" | jq -e 'type == "array"' >/dev/null 2>&1; then
    echo "  languageID=$lang: /resources/stations did not return an array:" >&2
    echo "$stations_json" >&2
    exit 1
  fi
  count=$(echo "$stations_json" | jq 'length')
  echo "  languageID=$lang: $count station(s)"
  echo "$stations_json" > "$WORK_DIR/flat_${lang}.json"
done

echo
echo "== Nested pass: /resources/locations -> per-location stations -> per-station places =="
echo "   (reuses lib-nested-fetch.sh's fetch_language, full=1 for every language since"
echo "   loewenbytes' /resources/stations always returns full Station objects)"
for lang in it de en lld; do
  fetch_language "$lang" 1 "$WORK_DIR/nested_${lang}.json"
done

jq -n \
  --slurpfile flat_it "$WORK_DIR/flat_it.json" \
  --slurpfile flat_de "$WORK_DIR/flat_de.json" \
  --slurpfile flat_en "$WORK_DIR/flat_en.json" \
  --slurpfile flat_lld "$WORK_DIR/flat_lld.json" \
  --slurpfile nested_it "$WORK_DIR/nested_it.json" \
  --slurpfile nested_de "$WORK_DIR/nested_de.json" \
  --slurpfile nested_en "$WORK_DIR/nested_en.json" \
  --slurpfile nested_lld "$WORK_DIR/nested_lld.json" \
  '{
     flat:   {it: $flat_it[0],   de: $flat_de[0],   en: $flat_en[0],   lld: $flat_lld[0]},
     nested: {it: $nested_it[0], de: $nested_de[0], en: $nested_en[0], lld: $nested_lld[0]}
   }' > "$OUT_FILE"

echo
echo "== Done =="
echo "Raw data written to: $OUT_FILE"
echo "Flat station counts:"
jq -r '.flat | to_entries[] | "  \(.key): \(.value | length) station(s)"' "$OUT_FILE"
echo "Nested counts (locations / stations-across-all-locations):"
jq -r '.nested | to_entries[] | "  \(.key): \(.value | length) location(s), \([.value[].stations[]] | length) station(s)"' "$OUT_FILE"

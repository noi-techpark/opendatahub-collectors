#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# Ad-hoc test: does GET /resources/stations (bicincitta/weelo API) return the
# full station list without a locationId filter, or is the per-location loop
# in bike-boxes.silky.yaml actually required?
#
# Compares:
#   A) flat call:   /resources/stations?languageID=it
#   B) nested loop: /resources/locations?languageID=it, then
#                   /resources/stations?languageID=it&locationId=<id> per location
#
# Usage:
#   cp .env.example .env   # then fill in BICINCITTA_CLIENT_ID / BICINCITTA_CLIENT_SECRET
#   ./test-bike-boxes-locations-loop.sh
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
OUT_DIR="$(mktemp -d)"
trap 'rm -rf "$OUT_DIR"' EXIT

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

auth_get() {
  curl -sS -H "Authorization: Bearer $ACCESS_TOKEN" -H "Accept: application/json" "$1"
}

echo
echo "== A) Flat call: GET /resources/stations?languageID=it (no locationId) =="
auth_get "$BASE_URL/resources/stations?languageID=it" > "$OUT_DIR/flat.json"

if ! jq -e 'type == "array"' "$OUT_DIR/flat.json" >/dev/null 2>&1; then
  echo "Flat call did not return a JSON array. Raw response:"
  cat "$OUT_DIR/flat.json"
  echo
  echo "(This alone would explain why the original author looped per location.)"
  FLAT_FAILED=1
else
  FLAT_FAILED=0
  FLAT_COUNT=$(jq 'length' "$OUT_DIR/flat.json")
  jq -r '.[].stationID' "$OUT_DIR/flat.json" | sort -n > "$OUT_DIR/flat_ids.txt"
  echo "Flat call returned $FLAT_COUNT stations."
fi

echo
echo "== B) Nested loop: /resources/locations?languageID=it, then per-location /resources/stations =="
auth_get "$BASE_URL/resources/locations?languageID=it" > "$OUT_DIR/locations.json"
LOCATION_COUNT=$(jq 'length' "$OUT_DIR/locations.json")
echo "Got $LOCATION_COUNT locations."

: > "$OUT_DIR/nested_ids.txt"
jq -r '.[].locationID' "$OUT_DIR/locations.json" | while read -r LOC_ID; do
  auth_get "$BASE_URL/resources/stations?languageID=it&locationId=$LOC_ID" \
    | jq -r '.[].stationID' >> "$OUT_DIR/nested_ids.txt"
done
sort -n -o "$OUT_DIR/nested_ids.txt" "$OUT_DIR/nested_ids.txt"
NESTED_COUNT=$(wc -l < "$OUT_DIR/nested_ids.txt" | tr -d ' ')
echo "Nested loop returned $NESTED_COUNT stations across $LOCATION_COUNT locations."

echo
echo "== Comparison =="
if [[ "$FLAT_FAILED" -eq 1 ]]; then
  echo "Flat call failed outright -> locations loop is required (API rejects/errors without locationId)."
else
  echo "Flat count:   $FLAT_COUNT"
  echo "Nested count: $NESTED_COUNT"
  if diff -q "$OUT_DIR/flat_ids.txt" "$OUT_DIR/nested_ids.txt" >/dev/null; then
    echo "MATCH: flat call returns the exact same station set as the nested loop."
    echo "-> the locations loop is very likely unnecessary; /resources/stations?languageID=it alone is enough."
  else
    echo "MISMATCH. Station IDs only in flat call (missing from nested loop):"
    comm -23 "$OUT_DIR/flat_ids.txt" "$OUT_DIR/nested_ids.txt" | sed 's/^/  /'
    echo "Station IDs only in nested loop (missing from flat call):"
    comm -13 "$OUT_DIR/flat_ids.txt" "$OUT_DIR/nested_ids.txt" | sed 's/^/  /'
    echo "-> the locations loop is (at least partially) required."
  fi
fi

echo
echo "Raw responses kept at: $OUT_DIR (will be deleted on exit; copy now if you want to keep them)"
read -r -p "Press enter to clean up and exit..." _ || true

#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# Ad-hoc test: does loewenbytes' /resources/locations actually group multiple
# stations per location (like bicincitta does), or is it effectively 1:1
# (one station per location) in the live test environment?
#
# Compares:
#   A) /resources/locations?languageID=it  -> locationID -> embedded station stubs
#   B) /resources/stations?languageID=it    -> flat list, grouped by locationID here
#
# Usage:
#   Add to .env: LOEWENBYTES_CLIENT_ID / LOEWENBYTES_CLIENT_SECRET
#   ./test-loewenbytes-locations.sh
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
OUT_DIR="$(mktemp -d)"
trap 'rm -rf "$OUT_DIR"' EXIT

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

auth_get() {
  curl -sS -H "Authorization: Bearer $ACCESS_TOKEN" -H "Accept: application/json" "$1"
}

echo
echo "== A) GET /resources/locations?languageID=it =="
auth_get "$BASE_URL/resources/locations?languageID=it" > "$OUT_DIR/locations.json"

if ! jq -e 'type == "array"' "$OUT_DIR/locations.json" >/dev/null 2>&1; then
  echo "locations call did not return a JSON array. Raw response:"
  cat "$OUT_DIR/locations.json"
  exit 1
fi

LOCATION_COUNT=$(jq 'length' "$OUT_DIR/locations.json")
echo "Got $LOCATION_COUNT locations."
echo
echo "Stations-per-location (from /resources/locations, embedded stubs):"
jq -r '.[] | "  \(.locationID)\t\(.name)\t\(.stations | length) station(s)"' "$OUT_DIR/locations.json"

MULTI_STATION_LOCATIONS=$(jq '[.[] | select((.stations | length) > 1)] | length' "$OUT_DIR/locations.json")
echo
echo "Locations with more than 1 station: $MULTI_STATION_LOCATIONS / $LOCATION_COUNT"

echo
echo "== B) GET /resources/stations?languageID=it (flat), grouped by locationID =="
auth_get "$BASE_URL/resources/stations?languageID=it" > "$OUT_DIR/stations.json"

if ! jq -e 'type == "array"' "$OUT_DIR/stations.json" >/dev/null 2>&1; then
  echo "stations call did not return a JSON array. Raw response:"
  cat "$OUT_DIR/stations.json"
  exit 1
fi

STATION_COUNT=$(jq 'length' "$OUT_DIR/stations.json")
echo "Got $STATION_COUNT stations total."
echo
echo "Stations-per-location (from /resources/stations, grouped by locationID field):"
jq -r 'group_by(.locationID) | .[] | "  \(.[0].locationID)\t\(.[0].locationName)\t\(length) station(s)"' "$OUT_DIR/stations.json"

echo
echo "== Comparison =="
jq -r '.[].locationID' "$OUT_DIR/locations.json" | sort > "$OUT_DIR/loc_ids_from_locations.txt"
jq -r '.[].locationID' "$OUT_DIR/stations.json" | sort -u > "$OUT_DIR/loc_ids_from_stations.txt"

if diff -q "$OUT_DIR/loc_ids_from_locations.txt" "$OUT_DIR/loc_ids_from_stations.txt" >/dev/null; then
  echo "Location ID sets MATCH between /resources/locations and /resources/stations."
else
  echo "Location ID sets DIFFER:"
  echo "  only in /resources/locations:"
  comm -23 "$OUT_DIR/loc_ids_from_locations.txt" "$OUT_DIR/loc_ids_from_stations.txt" | sed 's/^/    /'
  echo "  only in /resources/stations:"
  comm -13 "$OUT_DIR/loc_ids_from_locations.txt" "$OUT_DIR/loc_ids_from_stations.txt" | sed 's/^/    /'
fi

echo
if [[ "$MULTI_STATION_LOCATIONS" -gt 0 ]]; then
  echo "-> loewenbytes DOES model real 1:many Location->Station grouping (like bicincitta)."
else
  echo "-> in this test environment, loewenbytes locations are currently all 1:1 with stations"
  echo "   (no location groups more than one station) - can't confirm the grouping is used in practice yet."
fi

echo
echo "Raw responses kept at: $OUT_DIR (deleted on exit; copy now if you want to keep them)"
read -r -p "Press enter to clean up and exit..." _ || true

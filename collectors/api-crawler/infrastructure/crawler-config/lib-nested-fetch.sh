# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0
#
# Shared by fetch-rawdata-bicincitta.sh and fetch-rawdata-loewenbytes.sh:
# the locations -> per-location stations -> per-station places request
# graph used by bike-boxes.silky.yaml (bicincitta). Not runnable on its
# own - source it after setting BASE_URL, ACCESS_TOKEN, LOCATION_PARAM and
# WORK_DIR.

auth_get() {
  curl -sS -H "Authorization: Bearer $ACCESS_TOKEN" -H "Accept: application/json" "$1"
}

# fetch_language <languageID> <full: 0|1> <out_json_file>
#
# full=0 mirrors bicincitta's "Fetch translations for all locations"
# forValues step: per location, keep only
# {address, locationName, name, stationID}.
# full=1 mirrors bicincitta's "Get all locations" (it) branch: full station
# objects, each augmented with .places from GET /resources/station.
fetch_language() {
  local lang="$1" full="$2" out="$3"
  local locations_file="$WORK_DIR/locations_${lang}.json"
  local ndjson="$WORK_DIR/locations_${lang}.ndjson"
  : > "$ndjson"

  auth_get "$BASE_URL/resources/locations?languageID=${lang}" > "$locations_file"
  if ! jq -e 'type == "array"' "$locations_file" >/dev/null 2>&1; then
    echo "  languageID=$lang: /resources/locations did not return an array:" >&2
    cat "$locations_file" >&2
    exit 1
  fi
  local loc_count
  loc_count=$(jq 'length' "$locations_file")
  echo "  languageID=$lang: $loc_count location(s)"

  jq -c '.[]' "$locations_file" | while IFS= read -r LOC; do
    loc_id=$(echo "$LOC" | jq -r '.locationID')
    stations_json=$(auth_get "$BASE_URL/resources/stations?languageID=${lang}&${LOCATION_PARAM}=${loc_id}")

    if [[ "$full" == "0" ]]; then
      stations_json=$(echo "$stations_json" | jq -c '[.[] | {address, locationName, name, stationID}]')
    else
      station_ndjson="$WORK_DIR/stations_${lang}_${loc_id}.ndjson"
      : > "$station_ndjson"
      echo "$stations_json" | jq -c '.[]' | while IFS= read -r ST; do
        station_id=$(echo "$ST" | jq -r '.stationID')
        detail=$(auth_get "$BASE_URL/resources/station?languageID=${lang}&stationID=${station_id}")
        places=$(echo "$detail" | jq -c '.places // []')
        echo "$ST" | jq -c --argjson places "$places" '. + {places: $places}' >> "$station_ndjson"
      done
      stations_json=$(jq -s -c '.' "$station_ndjson")
    fi

    echo "$LOC" | jq -c --argjson stations "$stations_json" '. + {stations: $stations}' >> "$ndjson"
  done

  jq -s -c '.' "$ndjson" > "$out"
}

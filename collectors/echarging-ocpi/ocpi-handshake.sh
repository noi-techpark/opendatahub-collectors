#!/bin/bash

# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: AGPL-3.0-or-later

set -e

# prints the response body and HTTP status to stderr on failure
req() {
  local out code
  out=$(curl -sS -w '\n%{http_code}' "$@") || { echo "curl failed for: $*" >&2; exit 1; }
  code=${out##*$'\n'}
  out=${out%$'\n'*}
  if [[ "$code" != 2* ]]; then
    echo "HTTP $code: $out" >&2
    exit 1
  fi
  echo "$out"
}

require() {
  if [[ -z "$1" || "$1" == "null" ]]; then
    echo "Could not determine $2 from response:" >&2
    echo "$3" >&2
    exit 1
  fi
}

# WaveLAB returns URLs prefixed with "undefined" (unset base URL on their side), so we substitute our own
fix_url() {
  echo "${1/#undefined/$API_BASE}"
}

read -rp "Token A: " TOKEN_A
read -rp "Is Token A already base64 encoded? [y/N]: " A_IS_B64
read -rp "Versions endpoint URL: " VERSIONS_URL
read -rp "Our versions URL: " OUR_VERSIONS_URL
read -rp "OCPI version [2.2]: " OCPI_VERSION
OCPI_VERSION=${OCPI_VERSION:-2.2}
API_BASE=${VERSIONS_URL%/versions}

if [[ "$A_IS_B64" =~ ^[yY] ]]; then
  TOKEN_A_B64="$TOKEN_A"
else
  TOKEN_A_B64=$(echo -n "$TOKEN_A" | base64 -w0)
fi

VERSIONS_RESPONSE=$(req -H "Authorization: Token $TOKEN_A_B64" "$VERSIONS_URL")
VERSION_URL=$(echo "$VERSIONS_RESPONSE" | jq -r --arg v "$OCPI_VERSION" '.data[] | select(.version == $v) | .url')

require "$VERSION_URL" "version URL for $OCPI_VERSION" "$VERSIONS_RESPONSE"
VERSION_URL=$(fix_url "$VERSION_URL")
VERSION_DETAILS=$(req -H "Authorization: Token $TOKEN_A_B64" "$VERSION_URL")
CREDENTIALS_URL=$(echo "$VERSION_DETAILS" | jq -r '.data.endpoints[] | select(.identifier == "credentials") | .url')
LOCATIONS_URL=$(echo "$VERSION_DETAILS" | jq -r '.data.endpoints[] | select(.identifier == "locations") | .url')

require "$CREDENTIALS_URL" "credentials URL" "$VERSION_DETAILS"
require "$LOCATIONS_URL" "locations URL" "$VERSION_DETAILS"
CREDENTIALS_URL=$(fix_url "$CREDENTIALS_URL")
LOCATIONS_URL=$(fix_url "$LOCATIONS_URL")
echo "Credentials URL: $CREDENTIALS_URL"

read -rp "Token B (plain), leave empty to generate: " TOKEN_B
TOKEN_B=${TOKEN_B:-$(openssl rand -hex 20)}
TOKEN_B_B64=$(echo -n "$TOKEN_B" | base64 -w0)

echo
echo "Token B plain:  $TOKEN_B"
echo "Token B base64: $TOKEN_B_B64"
echo
echo "Set OCPI_TOKENS=$TOKEN_B_B64 in your app config, then make sure the service is running."
echo
read -rp "Press Enter to continue..."

CREDS_RESPONSE=$(req -X POST "$CREDENTIALS_URL" \
  -H "Authorization: Token $TOKEN_A_B64" \
  -H "Content-Type: application/json" \
  -d "{
    \"token\": \"$TOKEN_B\",
    \"url\": \"$OUR_VERSIONS_URL\",
    \"roles\": [{
      \"role\": \"EMSP\",
      \"party_id\": \"OpenDataHub\",
      \"country_code\": \"IT\",
      \"business_details\": {\"name\": \"Open Data Hub\"}
    }]
  }")

TOKEN_C=$(echo "$CREDS_RESPONSE" | jq -r '.data.token')
require "$TOKEN_C" "Token C" "$CREDS_RESPONSE"
TOKEN_C_B64=$(echo -n "$TOKEN_C" | base64 -w0)

echo
echo "Token C plain:  $TOKEN_C"
echo "Token C base64: $TOKEN_C_B64"
echo
echo "Set PULL_TOKEN=$TOKEN_C_B64 in your app config."
echo
echo "Locations URL: $LOCATIONS_URL"
echo "Set PULL_LOCATIONS_ENDPOINT=$LOCATIONS_URL in your app config."

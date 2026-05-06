#!/bin/bash

# SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: AGPL-3.0-or-later

set -e

read -rp "Token A (plain): " TOKEN_A
read -rp "Versions endpoint URL: " VERSIONS_URL
read -rp "Our versions URL: " OUR_VERSIONS_URL

TOKEN_A_B64=$(echo -n "$TOKEN_A" | base64)

VERSIONS_RESPONSE=$(curl -sf -H "Authorization: Token $TOKEN_A_B64" "$VERSIONS_URL")
VERSION_URL=$(echo "$VERSIONS_RESPONSE" | jq -r '.data[] | select(.version == "2.2") | .url')

VERSION_DETAILS=$(curl -sf -H "Authorization: Token $TOKEN_A_B64" "$VERSION_URL")
CREDENTIALS_URL=$(echo "$VERSION_DETAILS" | jq -r '.data.endpoints[] | select(.identifier == "credentials") | .url')
LOCATIONS_URL=$(echo "$VERSION_DETAILS" | jq -r '.data.endpoints[] | select(.identifier == "locations") | .url')

echo "Credentials URL: $CREDENTIALS_URL"

TOKEN_B=$(openssl rand -hex 20)
TOKEN_B_B64=$(echo -n "$TOKEN_B" | base64)

echo
echo "Token B plain:  $TOKEN_B"
echo "Token B base64: $TOKEN_B_B64"
echo
echo "Set OCPI_TOKENS=$TOKEN_B_B64 in your app config, then make sure the service is running."
echo
read -rp "Press Enter to continue..."

CREDS_RESPONSE=$(curl -sf -X POST "$CREDENTIALS_URL" \
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
TOKEN_C_B64=$(echo -n "$TOKEN_C" | base64)

echo
echo "Token C plain:  $TOKEN_C"
echo "Token C base64: $TOKEN_C_B64"
echo
echo "Set PULL_TOKEN=$TOKEN_C_B64 in your app config."
echo
echo "Locations URL: $LOCATIONS_URL"
echo "Set PULL_LOCATIONS_ENDPOINT=$LOCATIONS_URL in your app config."

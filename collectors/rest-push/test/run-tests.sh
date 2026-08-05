#!/bin/sh

# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"

docker compose -f "$SCRIPT_DIR/docker-compose.yml" down
docker compose -f "$SCRIPT_DIR/docker-compose.yml" up --build app
result=$?

docker compose -f "$SCRIPT_DIR/docker-compose.yml" down

exit $result
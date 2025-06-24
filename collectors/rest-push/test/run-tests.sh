#!/bin/sh

# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: CC0-1.0

docker compose down
docker compose up --build app
result=$?

docker compose down

exit $result
#!/bin/bash
# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: AGPL-3.0-or-later

# Reproducible one-time build of resources/a22_road.json from shapefiles.
# Uses a Docker container so no local Python/venv is needed.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_DIR="$(cd "$SCRIPT_DIR/.." && pwd)"

docker run --rm \
  -v "$PROJECT_DIR/resources:/data/resources" \
  -v "$SCRIPT_DIR:/data/scripts" \
  python:3.13-slim \
  bash -c '
    pip install --quiet geopandas pyproj &&
    python3 /data/scripts/build_road_json.py
  '

echo "Done: $(ls -lh "$PROJECT_DIR/resources/a22_road.json")"

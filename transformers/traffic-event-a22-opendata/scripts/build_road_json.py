#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
#
# SPDX-License-Identifier: AGPL-3.0-or-later

"""
Pre-processes A22 road axis and km marker shapefiles into a JSON file
suitable for embedding in the Go transformer.

Requires: geopandas, pyproj (install via: pip install geopandas pyproj)

Usage: python3 scripts/build_road_json.py
"""

import json
import os
from math import sqrt

import geopandas as gpd
from pyproj import Transformer
from shapely.geometry import LineString, Point

SCRIPT_DIR = os.path.dirname(os.path.abspath(__file__))
RESOURCES_DIR = os.path.join(SCRIPT_DIR, "..", "resources")
TRACK_DIR = os.path.join(RESOURCES_DIR, "track")
OUTPUT = os.path.join(RESOURCES_DIR, "a22_road.json")


def main():
    # 1. Read road axis segments
    asse = gpd.read_file(os.path.join(TRACK_DIR, "A22_asse.shp"))
    asse_sorted = asse.sort_values("DI").reset_index(drop=True)
    print(f"Read {len(asse_sorted)} road segments")

    # 2. Merge into continuous line, handling reversed segments
    continuous_coords = []
    for _, row in asse_sorted.iterrows():
        coords = list(row.geometry.coords)
        if continuous_coords:
            last = continuous_coords[-1]
            dist_normal = sqrt(
                (last[0] - coords[0][0]) ** 2 + (last[1] - coords[0][1]) ** 2
            )
            dist_reversed = sqrt(
                (last[0] - coords[-1][0]) ** 2 + (last[1] - coords[-1][1]) ** 2
            )
            if dist_reversed < dist_normal:
                coords = list(reversed(coords))
            # skip first point if very close to previous (avoids duplicates)
            continuous_coords.extend(coords[1:])
        else:
            continuous_coords.extend(coords)

    print(f"Merged into {len(continuous_coords)} points")

    # 3. Compute cumulative Euclidean distance (accurate in UTM)
    cum_dists = [0.0]
    for i in range(1, len(continuous_coords)):
        dx = continuous_coords[i][0] - continuous_coords[i - 1][0]
        dy = continuous_coords[i][1] - continuous_coords[i - 1][1]
        cum_dists.append(cum_dists[-1] + sqrt(dx * dx + dy * dy))

    total_length = cum_dists[-1]
    print(f"Total line length: {total_length:.0f} m ({total_length / 1000:.1f} km)")

    # 4. Convert UTM 32N -> WGS84
    transformer = Transformer.from_crs("EPSG:32632", "EPSG:4326", always_xy=True)
    wgs84_coords = []
    for c in continuous_coords:
        lon, lat = transformer.transform(c[0], c[1])
        wgs84_coords.append((lon, lat))

    # 5. Read km markers and project onto the merged line
    km_gdf = gpd.read_file(os.path.join(TRACK_DIR, "A22_km.shp"))
    km_gdf = km_gdf.sort_values("KM").reset_index(drop=True)
    assert len(km_gdf) == 314, f"Expected 314 km markers, got {len(km_gdf)}"

    merged_line = LineString(continuous_coords)
    km_dists = []
    for _, row in km_gdf.iterrows():
        projected_dist = merged_line.project(Point(row.geometry.x, row.geometry.y))
        km_dists.append(round(projected_dist, 1))

    # Validate monotonicity
    for i in range(1, len(km_dists)):
        assert km_dists[i] > km_dists[i - 1], (
            f"km_dists not monotonic at km {i}: {km_dists[i-1]} >= {km_dists[i]}"
        )

    print(f"Km markers: {len(km_dists)} (km 0 at {km_dists[0]:.0f}m, km 313 at {km_dists[-1]:.0f}m)")

    # 6. Resample at 1km intervals for reduced granularity
    STEP = 1000.0  # meters
    resampled_coords = []
    resampled_dists = []

    # Always include the first point
    resampled_coords.append(wgs84_coords[0])
    resampled_dists.append(cum_dists[0])

    next_dist = STEP
    for i in range(1, len(cum_dists)):
        while next_dist <= cum_dists[i]:
            # Interpolate between point i-1 and i
            t = (next_dist - cum_dists[i - 1]) / (cum_dists[i] - cum_dists[i - 1])
            lon = wgs84_coords[i - 1][0] + t * (wgs84_coords[i][0] - wgs84_coords[i - 1][0])
            lat = wgs84_coords[i - 1][1] + t * (wgs84_coords[i][1] - wgs84_coords[i - 1][1])
            resampled_coords.append((lon, lat))
            resampled_dists.append(next_dist)
            next_dist += STEP

    # Always include the last point
    resampled_coords.append(wgs84_coords[-1])
    resampled_dists.append(cum_dists[-1])

    print(f"Resampled: {len(resampled_coords)} points (every {STEP:.0f}m)")

    # 7. Build output
    points = []
    for i, (lon, lat) in enumerate(resampled_coords):
        points.append([round(lon, 7), round(lat, 7), round(resampled_dists[i], 1)])

    output = {"points": points, "km_dists": km_dists}

    with open(OUTPUT, "w") as f:
        json.dump(output, f, separators=(",", ":"))

    size_kb = os.path.getsize(OUTPUT) / 1024
    print(f"Written {OUTPUT} ({size_kb:.0f} KB)")


if __name__ == "__main__":
    main()

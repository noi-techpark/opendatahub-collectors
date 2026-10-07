// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"github.com/noi-techpark/opendatahub-go-sdk/clib"
)

const ID_PREFIX = "urn:parking:swarco"

// stationCode derives the BDP station code from the provider id as a
// deterministic UUIDv5 URN. The raw provider id stays available in metadata
// under provider_id.
func stationCode(providerID string) string {
	return clib.GenerateID(ID_PREFIX, providerID)
}

// position extracts a representative WGS84 coordinate from an SMI Location:
// the point itself, the polygon center (first vertex as fallback), or the
// first polyline vertex.
func position(loc *Location) (lat float64, lon float64, ok bool) {
	coord := func(p *Point) (float64, float64, bool) {
		if p == nil || p.Coordinates == nil {
			return 0, 0, false
		}
		return p.Coordinates.Latitude, p.Coordinates.Longitude, true
	}
	if loc == nil {
		return 0, 0, false
	}
	if lat, lon, ok := coord(loc.Point); ok {
		return lat, lon, true
	}
	if loc.Polygon != nil {
		if lat, lon, ok := coord(loc.Polygon.Center); ok {
			return lat, lon, true
		}
		if len(loc.Polygon.Vertices) > 0 {
			if lat, lon, ok := coord(&loc.Polygon.Vertices[0]); ok {
				return lat, lon, true
			}
		}
	}
	if loc.Polyline != nil && len(loc.Polyline.Vertices) > 0 {
		if lat, lon, ok := coord(&loc.Polyline.Vertices[0]); ok {
			return lat, lon, true
		}
	}
	return 0, 0, false
}

// ToMetadata passes every provider field that is not mapped to a first-class
// station attribute through to metadata, dropping empty values. The capacity
// list is flattened to capacity / capacity_<space type> keys.
func (s *StaticPOIData) ToMetadata() map[string]any {
	meta := map[string]any{}
	for k, v := range s.raw {
		switch k {
		case "location", "name", "objectID", "areas", "capacity":
			continue
		}
		if pruned, keep := pruneValue(v); keep {
			meta[k] = pruned
		}
	}
	meta["provider_id"] = s.ObjectID
	flattenCapacity(meta, s.Capacity)
	return meta
}

func areaMetadata(a StaticAreaData) map[string]any {
	meta := map[string]any{"provider_id": a.ObjectID}
	if a.GuID != nil && *a.GuID != "" {
		meta["guID"] = *a.GuID
	}
	if a.Capacity != nil {
		flattenCapacity(meta, []CapacityData{*a.Capacity})
	}
	return meta
}

func flattenCapacity(meta map[string]any, capacities []CapacityData) {
	for _, c := range capacities {
		slug, known := spaceTypeSlug[c.ParkingSpaceType]
		if !known {
			continue
		}
		key := "capacity"
		if slug != "" {
			key = "capacity_" + slug
		}
		meta[key] = c.Capacity
	}
}

// pruneValue recursively drops nulls, empty strings/slices/maps and the
// provider's literal "null" strings so metadata stays free of noise.
func pruneValue(v any) (any, bool) {
	switch t := v.(type) {
	case nil:
		return nil, false
	case string:
		if t == "" || t == "null" {
			return nil, false
		}
		return t, true
	case []any:
		out := []any{}
		for _, e := range t {
			if pe, keep := pruneValue(e); keep {
				out = append(out, pe)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	case map[string]any:
		out := map[string]any{}
		for k, e := range t {
			if pe, keep := pruneValue(e); keep {
				out[k] = pe
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	default:
		return v, true
	}
}

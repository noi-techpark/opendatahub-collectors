// SPDX-FileCopyrightText: 2025 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"

	"github.com/noi-techpark/go-timeseries-client/odhts"
	"github.com/noi-techpark/opendatahub-go-sdk/tel/logger"
)

// hydrateCache primes the in-memory cache with the latest measurement
// values currently stored in BDP for our origin. It looks up the
// ParkingStation rows whose datatype matches one of the names we care
// about (free, occupied, free_<cat>, occupied_<cat>) and converts the
// returned scode (a URN) back to its provider id via urnToProviderID.
//
// The function is best-effort: errors are logged and returned, but
// callers can choose to keep starting if hydration fails.
func hydrateCache(c *Cache, ts odhts.C, origin string, datatypes []string, urnToProviderID map[string]string) error {
	if origin == "" {
		return fmt.Errorf("BDP_ORIGIN is empty; refusing to hydrate without an origin filter")
	}
	if len(datatypes) == 0 {
		logger.Get(context.Background()).Info("No datatypes to hydrate; skipping")
		return nil
	}

	req := odhts.DefaultRequest()
	req.AddStationType(stationType)
	for _, dt := range datatypes {
		req.AddDataType(dt)
	}
	req.Origin = origin
	// /latest gives one row per (station, datatype) combination already.
	// The default limit is 200 which is too low for our ~22 carparks ×
	// ~16 datatypes; bump generously.
	req.Limit = 10000

	var resp odhts.Response[[]odhts.LatestDto]
	if err := odhts.Latest(ts, req, &resp); err != nil {
		return fmt.Errorf("query BDP latest: %w", err)
	}

	seeded, skipped := 0, 0
	for _, row := range resp.Data {
		providerID, ok := urnToProviderID[row.Scode]
		if !ok {
			skipped++
			continue
		}
		c.Set(providerID, row.Tname, row.MValue, row.MValidTime.UnixMilli())
		seeded++
	}

	logger.Get(context.Background()).Info("Hydrated cache from BDP",
		"origin", origin,
		"seeded", seeded,
		"skipped_unknown_scode", skipped,
		"datatypes", len(datatypes))
	return nil
}

// stationRow is one station as the timeseries reports it, reduced to what the
// registry needs.
type stationRow struct {
	ProviderID string
	FacilityID string
	CarparkID  int // -1 for a facility
	Name       string
}

// fetchStations lists the stations this transformer has already published.
//
// The durable answer to "what exists": every station ever synced under this
// origin, whether or not it is currently reporting and whether or not the
// provider still returns counting categories for it. Twelve of the fleet's
// facilities have categories; the demo facility has none, and it still has
// stations.
func fetchStations(ts odhts.C, origin string) ([]stationRow, error) {
	if origin == "" {
		return nil, fmt.Errorf("BDP_ORIGIN is empty; refusing to list stations without an origin filter")
	}

	req := odhts.DefaultRequest()
	req.AddStationType(stationType)
	req.AddStationType(stationTypeParent)
	req.Where = fmt.Sprintf("sorigin.eq.%q", origin)
	req.Select = "scode,sname,stype,smetadata"
	req.Limit = -1
	req.Shownull = true

	var res odhts.Response[[]struct {
		Scode     string         `json:"scode"`
		Sname     string         `json:"sname"`
		Stype     string         `json:"stype"`
		Smetadata map[string]any `json:"smetadata"`
	}]
	if err := odhts.StationType(ts, req, &res); err != nil {
		return nil, fmt.Errorf("listing stations: %w", err)
	}

	out := make([]stationRow, 0, len(res.Data))
	for _, r := range res.Data {
		m := r.Smetadata
		providerID, _ := m["provider_id"].(string)
		if providerID == "" {
			// Published by something that is not this transformer, or from
			// before provider_id was stamped. Nothing maps it back to a
			// facility, so it cannot be enriched or re-synced from here.
			continue
		}
		row := stationRow{ProviderID: providerID, Name: r.Sname, CarparkID: -1, FacilityID: providerID}
		if r.Stype == stationType {
			facility, _ := m["facility_id"].(string)
			carpark, ok := m["carpark_id"].(float64) // JSON numbers decode as float64
			if facility == "" || !ok {
				continue
			}
			row.FacilityID, row.CarparkID = facility, int(carpark)
		}
		out = append(out, row)
	}
	return out, nil
}

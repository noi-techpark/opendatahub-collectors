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

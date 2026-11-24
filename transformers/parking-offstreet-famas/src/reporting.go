// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"log/slog"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/discovery"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/tr"
)

// The discovery table lists every station the transformer saw and what it did
// with it, so the control tower can show what is missing a record.
var discoveryEnv struct {
	DISCOVERY_PROVIDER   string `default:""`
	DISCOVERY_GONE_AFTER int    `default:"0"`
	RAW_WRITER_URL       string `default:""`

	// REFTABLE_GATE withholds a station without an enrichment record. It
	// defaults to true so a deployment that forgets the variable fails closed.
	REFTABLE_GATE bool `default:"true"`
}

type noopObserver struct{}

func (noopObserver) Observe(string, discovery.Entry) {}
func (noopObserver) Flush(context.Context) error     { return nil }
func (noopObserver) Complete()                       {}

// observer is replaced by a discovery.Recorder in tests.
var observer discovery.Observer = noopObserver{}

func startDiscovery(ctx context.Context, conn tr.Env) error {
	if discoveryEnv.DISCOVERY_PROVIDER == "" {
		return errors.New("DISCOVERY_PROVIDER is not set")
	}
	if discoveryEnv.RAW_WRITER_URL == "" {
		return errors.New("RAW_WRITER_URL is not set")
	}
	r := discovery.New(
		rdb.NewRDBridge(rdb.Env{RAW_DATA_BRIDGE_ENDPOINT: conn.RAW_DATA_BRIDGE_ENDPOINT}),
		discovery.Config{
			Provider:     discoveryEnv.DISCOVERY_PROVIDER,
			RawWriterURL: discoveryEnv.RAW_WRITER_URL,
			GoneAfter:    discoveryEnv.DISCOVERY_GONE_AFTER,
		})
	if err := r.Start(ctx); err != nil {
		return err
	}
	observer = r
	return nil
}

// flushDiscovery never fails the message: a failed write stays pending and is
// retried by the next flush.
func flushDiscovery(ctx context.Context) {
	if err := observer.Flush(ctx); err != nil {
		slog.Error("failed flushing the discovery table", "err", err)
	}
}

func gated() bool { return discoveryEnv.REFTABLE_GATE }

// seedRecord is the parking.v1 record that, overlaid by stationRules, rebuilds
// the station as published without one: the station is created at 0,0 under its
// provider-derived name.
func seedRecord(name, code string) map[string]any {
	// Only what the provider stated: the 0,0 the station carries without a
	// record, or a name that is only the code, would become operator data.
	if name == "" || name == code {
		return nil
	}
	return map[string]any{"name": name}
}

// entryFor reports what Transform does with a station: published when it has a
// record or the gate is open, withheld otherwise.
func entryFor(code string, hasRec bool, name string, capacity *int) discovery.Entry {
	e := discovery.Entry{Name: name, Seed: seedRecord(name, code)}
	if capacity != nil {
		e.Data = map[string]any{"capacity": *capacity}
	}
	switch {
	case hasRec:
		e.Status = discovery.StatusPublished
	case gated():
		e.Status, e.Reason = discovery.StatusWithheld, "no-record"
	default:
		e.Status, e.Reason = discovery.StatusPublished, "no-record"
	}
	return e
}

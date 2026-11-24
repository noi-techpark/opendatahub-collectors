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

	// REFTABLE_GATE withholds a station without an enrichment record. When
	// false, every station the provider sends is published.
	REFTABLE_GATE bool `default:"false"`
}

type noopObserver struct{}

func (noopObserver) Observe(string, discovery.Entry) {}
func (noopObserver) Flush(context.Context) error     { return nil }
func (noopObserver) Complete()                       {}

// discoveryObserver is replaced by a discovery.Recorder in tests.
var discoveryObserver discovery.Observer = noopObserver{}

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
	discoveryObserver = r
	return nil
}

// flushDiscovery never fails the message: a failed write stays pending and is
// retried by the next flush.
func flushDiscovery(ctx context.Context) {
	if err := discoveryObserver.Flush(ctx); err != nil {
		slog.Error("failed flushing the discovery table", "err", err)
	}
}

func gated() bool { return discoveryEnv.REFTABLE_GATE }

// seedRecord is the parking.v1 record that, overlaid by stationRules, rebuilds
// the station as published without one: it is created at 0,0. The name is not
// seeded because $.standard_name also writes metaData.standard_name, a field an
// unenriched station does not carry.
// seedRecord is empty: the provider states no coordinates (the station sits on
// 0,0 without a record), and the name cannot be seeded because the
// standard_name rule would add metaData.standard_name.
func seedRecord() map[string]any { return nil }

// hasRecord matches enrichStation: an empty record enriches nothing.
func hasRecord(providerID string) bool {
	if refTable == nil {
		return false
	}
	record, ok := refTable.Get(providerID)
	return ok && len(record) > 0
}

// withheldReason says why the gate keeps a station out of the silo, or "" when
// it is published. A carpark also needs its facility: a station whose parent is
// not published has nothing to hang from.
func withheldReason(providerID, facilityID string, isCarpark bool) string {
	if !gated() {
		return ""
	}
	if !hasRecord(providerID) {
		return "no-record"
	}
	if isCarpark && !hasRecord(facilityID) {
		return "parent-incomplete"
	}
	return ""
}

// reportStation tells the discovery table what the registry holds for a station.
// Capacity is left out: for some carparks it is a live quota that changes every
// few minutes, which would rewrite the document on every event.
func reportStation(providerID string, e entry) {
	d := discovery.Entry{Name: e.base.Name, Seed: seedRecord()}
	isCarpark := e.carparkID >= 0
	if isCarpark {
		d.Parent = e.facilityID
		d.Data = map[string]any{"facility_id": e.facilityID, "carpark_id": e.carparkID}
	}
	if reason := withheldReason(providerID, e.facilityID, isCarpark); reason != "" {
		d.Status, d.Reason = discovery.StatusWithheld, reason
	} else {
		d.Status = discovery.StatusPublished
		if !hasRecord(providerID) {
			d.Reason = "no-record"
		}
	}
	discoveryObserver.Observe(providerID, d)
}

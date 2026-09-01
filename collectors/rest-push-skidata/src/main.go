// SPDX-FileCopyrightText: 2024 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os/signal"
	"syscall"
	"time"

	"github.com/noi-techpark/opendatahub-go-sdk/ingest/dc"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/ms"
	"github.com/noi-techpark/opendatahub-go-sdk/ingest/rdb"
	"github.com/noi-techpark/opendatahub-go-sdk/tel"
)

// PushPayload carries the raw JSON body together with the facilityId from the URL.
type PushPayload struct {
	Body json.RawMessage
}

var env struct {
	dc.Env

	INBOUND_AUTH_USER string `required:"true"`
	INBOUND_AUTH_PASS string `required:"true"`

	SKIDATA_BASE_URL string `required:"true"`

	// Counting categories are collected as a second flow under their own
	// provider, so they land in their own collection and can be read back
	// keyed by facility.
	SKIDATA_CATEGORIES_PROVIDER string        `default:"skidata/counting-categories"`
	SKIDATA_CATEGORY_REFRESH    time.Duration `default:"1h"`

	// The credential system of record. The address is cluster DNS and the same
	// in every environment, so it defaults rather than being repeated in each
	// values file; override it to point somewhere else. The credentials have no
	// sensible default and come from the mirrored Secret.
	BACKOFFICE_URL       string `default:"http://opendatahub-parking-control-tower.parking.svc.cluster.local:8091"`
	BACKOFFICE_AUTH_USER string `required:"true"`
	BACKOFFICE_AUTH_PASS string `required:"true"`

	// How often the whole set is re-read. This is the correctness guarantee:
	// there is no push, so this interval is the worst case between an operator
	// saving a credential and this collector using it (R9.8).
	BACKOFFICE_REFRESH time.Duration `default:"60s"`
}

var collector *dc.Dc[PushPayload]

// credentialSource returns how this process learns its credentials.
func credentialSource() func(context.Context) ([]FacilityCredential, error) {
	b := &Backoffice{
		URL:    env.BACKOFFICE_URL,
		User:   env.BACKOFFICE_AUTH_USER,
		Pass:   env.BACKOFFICE_AUTH_PASS,
		Client: &http.Client{Timeout: 20 * time.Second},
	}
	return b.Fetch
}

// refreshLoop re-reads the whole set and reconciles what is running against it.
//
// A failed read is logged and skipped, never applied: an unreachable backoffice
// means "unknown", and treating it as an empty set would unsubscribe every
// facility because the network blipped.
func refreshLoop(ctx context.Context, s *Supervisor, fetch func(context.Context) ([]FacilityCredential, error)) {
	defer tel.FlushOnPanic()

	t := time.NewTicker(env.BACKOFFICE_REFRESH)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		creds, err := fetch(ctx)
		if err != nil {
			slog.Error("Could not refresh credentials; keeping the current set", "err", err)
			continue
		}
		added, removed, restarted := s.Apply(ctx, creds)
		if added+removed+restarted > 0 {
			slog.Info("Credential set changed",
				"added", added, "removed", removed, "restarted", restarted, "total", len(creds))
		}
	}
}

func main() {
	ms.InitWithEnv(context.Background(), "", &env)
	slog.Info("Starting rest-push-skidata collector...")

	defer tel.FlushOnPanic()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	source := credentialSource()
	creds, err := source(ctx)
	// Fatal on purpose: a collector that starts with no credentials subscribes
	// to nothing and looks healthy doing it, which is worse than not starting.
	ms.FailOnError(ctx, err, "failed to load credentials")
	slog.Info("Loaded facility credentials", "count", len(creds))

	collector = dc.NewDc[PushPayload](context.Background(), env.Env)

	go func() {
		defer tel.FlushOnPanic()
		collector.Start(context.Background(), func(ctx context.Context, p PushPayload) (*rdb.RawAny, error) {
			// Stated rather than left to the SDK default. The content type
			// decides how the writer stores the payload — a text type is kept
			// verbatim as a string, anything else becomes BSON binary — and the
			// transformer decodes it as a string. Leaving that to a default
			// somewhere else couples the two ends through a value neither of
			// them declares.
			return &rdb.RawAny{
				Provider:    env.PROVIDER,
				Timestamp:   time.Now(),
				Rawdata:     p.Body,
				ContentType: "application/json",
			}, nil
		})
	}()

	supervisor := NewSupervisor(runFacility)
	defer supervisor.Stop()
	added, _, _ := supervisor.Apply(ctx, creds)
	slog.Info("Managing facilities", "count", added)

	go refreshLoop(ctx, supervisor, source)

	serve(collector.GetInputChannel())
}

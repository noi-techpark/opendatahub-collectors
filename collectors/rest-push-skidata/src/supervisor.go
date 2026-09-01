// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"sync"
)

// Supervisor owns one goroutine per facility and reconciles that set against
// the credentials the backoffice reports.
//
// The point is that changing one facility's password disturbs no other. Before
// this, credentials arrived in SKIDATA_CREDENTIALS_JSON and were read once at
// startup, so a single edit meant redeploying a pod holding twenty-five live
// subscriptions -- every one of them dropped and rebuilt to fix one.
type Supervisor struct {
	mu      sync.Mutex
	running map[string]*facilityRun
	run     func(context.Context, FacilityCredential)
}

type facilityRun struct {
	cancel context.CancelFunc
	cred   FacilityCredential
	done   chan struct{}
}

func NewSupervisor(run func(context.Context, FacilityCredential)) *Supervisor {
	return &Supervisor{running: map[string]*facilityRun{}, run: run}
}

// Apply reconciles the running set with the given credentials: added ones start,
// removed ones stop, and a changed username or password restarts only that one.
//
// Untouched facilities are never restarted. That is the entire point: with
// thousands of credentials, reacting to one edit by rebuilding every
// subscription is indistinguishable from the redeploy this replaced.
func (s *Supervisor) Apply(ctx context.Context, creds []FacilityCredential) (added, removed, restarted int) {
	wanted := make(map[string]FacilityCredential, len(creds))
	for _, c := range creds {
		wanted[c.Facility] = c
	}

	s.mu.Lock()
	// Cancel first, wait later. Waiting here would hold the lock across a
	// facility's in-flight HTTP call -- twenty seconds worst case -- and with
	// thousands of facilities those waits would add up serially while every
	// other reconciliation queued behind them.
	stopping := map[string]*facilityRun{}
	starting := []FacilityCredential{}
	for facility, current := range s.running {
		next, keep := wanted[facility]
		switch {
		case !keep:
			current.cancel()
			stopping[facility] = current
			delete(s.running, facility)
			removed++
		case next != current.cred:
			// Restart rather than mutate: the goroutine is mid-flight in a
			// health check or a subscribe, and swapping the credential under it
			// would use half the old one and half the new.
			current.cancel()
			stopping[facility] = current
			delete(s.running, facility)
			starting = append(starting, next)
			restarted++
		}
	}
	for facility, cred := range wanted {
		if _, ok := s.running[facility]; !ok {
			if _, beingRestarted := stopping[facility]; !beingRestarted {
				starting = append(starting, cred)
				added++
			}
		}
	}
	s.mu.Unlock()

	// Every cancellation is already issued, so these unwind concurrently and
	// the wait is the slowest one, not the sum.
	for facility, r := range stopping {
		<-r.done
		slog.Info("Stopped managing facility", "facility", facility)
	}

	s.mu.Lock()
	for _, cred := range starting {
		s.start(ctx, cred)
	}
	s.mu.Unlock()
	return added, removed, restarted
}

// start assumes the lock is held.
func (s *Supervisor) start(ctx context.Context, cred FacilityCredential) {
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	s.running[cred.Facility] = &facilityRun{cancel: cancel, cred: cred, done: done}
	go func() {
		defer close(done)
		s.run(runCtx, cred)
	}()
}

// Facilities lists what is currently managed.
func (s *Supervisor) Facilities() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.running))
	for f := range s.running {
		out = append(out, f)
	}
	return out
}

// Stop cancels everything and waits for it to unwind.
func (s *Supervisor) Stop() {
	s.mu.Lock()
	stopping := s.running
	s.running = map[string]*facilityRun{}
	for _, r := range stopping {
		r.cancel()
	}
	s.mu.Unlock()

	for facility, r := range stopping {
		<-r.done
		slog.Info("Stopped managing facility", "facility", facility)
	}
}

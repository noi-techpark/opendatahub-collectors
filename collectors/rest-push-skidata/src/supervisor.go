// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"log/slog"
	"sort"
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
	// Serialises whole reconciliations. Apply releases `mu` while it waits for
	// cancelled goroutines to unwind -- it must not hold a lock across a
	// facility's in-flight HTTP call -- and in that window a second Apply used
	// to see the facility in neither the running set nor the stopping set, call
	// it new, and start a duplicate whose cancel func the first one then
	// overwrote. That goroutine became unreachable: no later Apply and no Stop
	// could ever end it.
	//
	// Two callers race by construction: the refresh ticker and the pushed set.
	reconcile sync.Mutex

	mu      sync.Mutex
	running map[string]*facilityRun
	// Set by Stop, so a reconciliation already in flight cannot start
	// goroutines that outlive it.
	stopped bool
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

// Reconciliation names what changed, not just how much.
//
// Counts alone made the logs useless for the question actually asked of them --
// "did MY edit land?" -- because "restarted=1" is the same line whichever of a
// thousand facilities it was.
type Reconciliation struct {
	Added     []string
	Removed   []string
	Restarted []string
}

// Changed reports whether anything happened, which is what decides if the
// reconciliation is worth a log line at all.
func (r Reconciliation) Changed() bool {
	return len(r.Added)+len(r.Removed)+len(r.Restarted) > 0
}

// Apply reconciles the running set with the given credentials: added ones start,
// removed ones stop, and a changed username or password restarts only that one.
//
// Untouched facilities are never restarted. That is the entire point: with
// thousands of credentials, reacting to one edit by rebuilding every
// subscription is indistinguishable from the redeploy this replaced.
func (s *Supervisor) Apply(ctx context.Context, creds []FacilityCredential) Reconciliation {
	s.reconcile.Lock()
	defer s.reconcile.Unlock()

	wanted := make(map[string]FacilityCredential, len(creds))
	for _, c := range creds {
		wanted[c.Facility] = c
	}

	s.mu.Lock()
	// Cancel first, wait later. Waiting here would hold the lock across a
	// facility's in-flight HTTP call -- twenty seconds worst case -- and with
	// thousands of facilities those waits would add up serially while every
	// other reconciliation queued behind them.
	out := Reconciliation{Added: []string{}, Removed: []string{}, Restarted: []string{}}
	stopping := map[string]*facilityRun{}
	starting := []FacilityCredential{}
	for facility, current := range s.running {
		next, keep := wanted[facility]
		switch {
		case !keep:
			current.cancel()
			stopping[facility] = current
			delete(s.running, facility)
			out.Removed = append(out.Removed, facility)
		case next != current.cred:
			// Restart rather than mutate: the goroutine is mid-flight in a
			// health check or a subscribe, and swapping the credential under it
			// would use half the old one and half the new.
			current.cancel()
			stopping[facility] = current
			delete(s.running, facility)
			starting = append(starting, next)
			out.Restarted = append(out.Restarted, facility)
		}
	}
	for facility, cred := range wanted {
		if _, ok := s.running[facility]; !ok {
			if _, beingRestarted := stopping[facility]; !beingRestarted {
				starting = append(starting, cred)
				out.Added = append(out.Added, facility)
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
	if !s.stopped {
		for _, cred := range starting {
			s.start(ctx, cred)
		}
	}
	s.mu.Unlock()

	// Sorted so the same reconciliation reads the same way twice; map iteration
	// order would otherwise shuffle the names between identical deploys.
	sort.Strings(out.Added)
	sort.Strings(out.Removed)
	sort.Strings(out.Restarted)
	return out
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
	s.reconcile.Lock()
	defer s.reconcile.Unlock()

	s.mu.Lock()
	s.stopped = true
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

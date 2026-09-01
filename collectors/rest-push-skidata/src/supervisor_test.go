// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"sync"
	"testing"
	"time"
)

// recorder stands in for manageFacility: it records every start and stop, and
// blocks until cancelled exactly as the real loop does.
type recorder struct {
	mu      sync.Mutex
	starts  []FacilityCredential
	stopped []string
	live    map[string]bool
}

func newRecorder() *recorder { return &recorder{live: map[string]bool{}} }

func (r *recorder) run(ctx context.Context, cred FacilityCredential) {
	r.mu.Lock()
	r.starts = append(r.starts, cred)
	r.live[cred.Facility] = true
	r.mu.Unlock()

	<-ctx.Done()

	r.mu.Lock()
	r.stopped = append(r.stopped, cred.Facility)
	r.live[cred.Facility] = false
	r.mu.Unlock()
}

func (r *recorder) startCount(facility string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := 0
	for _, s := range r.starts {
		if s.Facility == facility {
			n++
		}
	}
	return n
}

func (r *recorder) isLive(facility string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.live[facility]
}

// waitFor polls until cond holds. Starting a facility is asynchronous by
// design -- Apply must not block on a goroutine reaching its first line -- so
// the test waits for the observable effect rather than assuming it.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", what)
}

func cred(facility, password string) FacilityCredential {
	return FacilityCredential{Facility: facility, Username: "u-" + facility, Password: password}
}

// The whole reason this exists: editing one credential must not disturb the
// others. It used to mean redeploying a pod holding twenty-five subscriptions.
func TestChangingOneCredentialRestartsOnlyThatFacility(t *testing.T) {
	r := newRecorder()
	s := NewSupervisor(r.run)
	ctx := context.Background()
	t.Cleanup(s.Stop)

	s.Apply(ctx, []FacilityCredential{cred("A", "1"), cred("B", "1"), cred("C", "1")})
	waitFor(t, "the initial three to start", func() bool {
		return r.isLive("A") && r.isLive("B") && r.isLive("C")
	})

	added, removed, restarted := s.Apply(ctx, []FacilityCredential{
		cred("A", "1"), cred("B", "2"), cred("C", "1"),
	})
	if added != 0 || removed != 0 || restarted != 1 {
		t.Fatalf("added=%d removed=%d restarted=%d, want 0/0/1", added, removed, restarted)
	}
	waitFor(t, "B to restart", func() bool { return r.startCount("B") == 2 })
	for _, f := range []string{"A", "C"} {
		if n := r.startCount(f); n != 1 {
			t.Errorf("%s restarted (%d starts) for a change to B", f, n)
		}
		if !r.isLive(f) {
			t.Errorf("%s was stopped by a change to B", f)
		}
	}
}

func TestAddedAndRemovedFacilitiesStartAndStop(t *testing.T) {
	r := newRecorder()
	s := NewSupervisor(r.run)
	ctx := context.Background()
	t.Cleanup(s.Stop)

	s.Apply(ctx, []FacilityCredential{cred("A", "1"), cred("B", "1")})
	waitFor(t, "A and B to start", func() bool { return r.isLive("A") && r.isLive("B") })

	added, removed, restarted := s.Apply(ctx, []FacilityCredential{cred("A", "1"), cred("C", "1")})

	if added != 1 || removed != 1 || restarted != 0 {
		t.Fatalf("added=%d removed=%d restarted=%d, want 1/1/0", added, removed, restarted)
	}
	if r.isLive("B") {
		t.Error("B is still running after being removed from the set")
	}
	waitFor(t, "C to start", func() bool { return r.isLive("C") })
}

// Applying the same set repeatedly is what the refresh loop does every minute.
// If it were not a no-op, every facility would drop its subscription once a
// minute for no reason.
func TestReapplyingAnUnchangedSetDoesNothing(t *testing.T) {
	r := newRecorder()
	s := NewSupervisor(r.run)
	ctx := context.Background()
	t.Cleanup(s.Stop)

	set := []FacilityCredential{cred("A", "1"), cred("B", "1")}
	s.Apply(ctx, set)
	waitFor(t, "the set to start", func() bool { return r.isLive("A") && r.isLive("B") })

	for i := 0; i < 5; i++ {
		if a, rm, rs := s.Apply(ctx, set); a+rm+rs != 0 {
			t.Fatalf("reapply %d changed something: %d/%d/%d", i, a, rm, rs)
		}
	}
	if n := r.startCount("A"); n != 1 {
		t.Errorf("A started %d times across five identical applies", n)
	}
}

// A restart must not leave two goroutines health-checking the same facility.
func TestRestartWaitsForTheOldGoroutineToUnwind(t *testing.T) {
	r := newRecorder()
	s := NewSupervisor(r.run)
	ctx := context.Background()
	t.Cleanup(s.Stop)

	s.Apply(ctx, []FacilityCredential{cred("A", "1")})
	waitFor(t, "A to start", func() bool { return r.isLive("A") })
	s.Apply(ctx, []FacilityCredential{cred("A", "2")})

	r.mu.Lock()
	stopped := len(r.stopped)
	r.mu.Unlock()
	if stopped != 1 {
		t.Fatalf("the old goroutine had not unwound when Apply returned (%d stops)", stopped)
	}
	waitFor(t, "A to be running again", func() bool { return r.isLive("A") })
}

func TestStopEndsEverything(t *testing.T) {
	r := newRecorder()
	s := NewSupervisor(r.run)
	s.Apply(context.Background(), []FacilityCredential{cred("A", "1"), cred("B", "1")})
	waitFor(t, "both to start", func() bool { return r.isLive("A") && r.isLive("B") })
	s.Stop()

	for _, f := range []string{"A", "B"} {
		if r.isLive(f) {
			t.Errorf("%s survived Stop", f)
		}
	}
	if got := s.Facilities(); len(got) != 0 {
		t.Errorf("still managing %v", got)
	}
}

// pause is how every wait in the facility loop becomes cancellable. A bare
// time.Sleep here is what made a credential change need a pod restart.
func TestPauseReturnsImmediatelyWhenCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	start := time.Now()
	if pause(ctx, time.Hour) {
		t.Error("pause reported that it slept through a cancelled context")
	}
	if time.Since(start) > time.Second {
		t.Error("pause waited for the timer instead of the cancellation")
	}
}

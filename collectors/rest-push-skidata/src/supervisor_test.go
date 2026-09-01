// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"context"
	"fmt"
	"reflect"
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

	got := s.Apply(ctx, []FacilityCredential{cred("A", "1"), cred("B", "2"), cred("C", "1")})
	// Named, not counted: "restarted=1" is the same line whichever facility it
	// was, which is useless for confirming that one edit landed.
	if !reflect.DeepEqual(got.Restarted, []string{"B"}) {
		t.Fatalf("restarted %v, want [B]", got.Restarted)
	}
	if len(got.Added) != 0 || len(got.Removed) != 0 {
		t.Fatalf("added=%v removed=%v, want neither", got.Added, got.Removed)
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

	got := s.Apply(ctx, []FacilityCredential{cred("A", "1"), cred("C", "1")})

	if !reflect.DeepEqual(got.Added, []string{"C"}) || !reflect.DeepEqual(got.Removed, []string{"B"}) {
		t.Fatalf("added=%v removed=%v, want [C] and [B]", got.Added, got.Removed)
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
		if got := s.Apply(ctx, set); got.Changed() {
			t.Fatalf("reapply %d changed something: %+v", i, got)
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

// slowRecorder unwinds only after a delay, the way a facility mid-request does.
// The bug needs that window: Apply drops its lock while cancelled goroutines
// finish, and a goroutine that stops instantly closes it before anything can
// race through.
type slowRecorder struct {
	mu     sync.Mutex
	starts map[string]int
	stops  map[string]int
	unwind time.Duration
}

func newSlowRecorder(unwind time.Duration) *slowRecorder {
	return &slowRecorder{starts: map[string]int{}, stops: map[string]int{}, unwind: unwind}
}

func (r *slowRecorder) run(ctx context.Context, cred FacilityCredential) {
	r.mu.Lock()
	r.starts[cred.Facility]++
	r.mu.Unlock()

	<-ctx.Done()
	time.Sleep(r.unwind)

	r.mu.Lock()
	r.stops[cred.Facility]++
	r.mu.Unlock()
}

// balance reports started-minus-stopped, which must be zero once everything has
// been told to stop. A leaked goroutine shows up here and nowhere else: a map
// keyed by facility cannot tell one live goroutine from two.
func (r *slowRecorder) balance(facility string) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.starts[facility] - r.stops[facility]
}

// Two callers reconcile concurrently by construction: the refresh ticker and a
// pushed set. Apply releases its lock while cancelled goroutines unwind, and a
// second Apply used to see the facility in neither the running nor the stopping
// set, call it new, and start a duplicate -- whose cancel func the first Apply
// then overwrote, leaving a goroutine nothing could ever stop.
func TestConcurrentAppliesDoNotDuplicateAFacility(t *testing.T) {
	for attempt := 0; attempt < 40; attempt++ {
		r := newSlowRecorder(20 * time.Millisecond)
		s := NewSupervisor(r.run)
		ctx := context.Background()

		s.Apply(ctx, []FacilityCredential{cred("A", "1")})
		waitFor(t, "A to start", func() bool { return r.balance("A") == 1 })

		var wg sync.WaitGroup
		wg.Add(2)
		for i := 2; i < 4; i++ {
			go func(gen int) {
				defer wg.Done()
				s.Apply(ctx, []FacilityCredential{cred("A", fmt.Sprint(gen))})
			}(i)
		}
		wg.Wait()
		s.Stop()

		if got := r.balance("A"); got != 0 {
			t.Fatalf("attempt %d: %d goroutine(s) for A outlived Stop; a duplicate start "+
				"overwrote the tracked cancel func", attempt, got)
		}
	}
}

// Stop clears the running set, then waits. A reconciliation already past its own
// diff would start goroutines into the emptied map and outlive the shutdown.
func TestApplyRacingStopLeavesNothingRunning(t *testing.T) {
	for attempt := 0; attempt < 40; attempt++ {
		r := newSlowRecorder(20 * time.Millisecond)
		s := NewSupervisor(r.run)
		ctx := context.Background()
		s.Apply(ctx, []FacilityCredential{cred("A", "1"), cred("B", "1")})
		waitFor(t, "the pair to start", func() bool { return r.balance("A") == 1 && r.balance("B") == 1 })

		var wg sync.WaitGroup
		wg.Add(2)
		go func() { defer wg.Done(); s.Apply(ctx, []FacilityCredential{cred("A", "2"), cred("C", "1")}) }()
		go func() { defer wg.Done(); s.Stop() }()
		wg.Wait()

		s.Stop()
		for _, f := range []string{"A", "B", "C"} {
			if got := r.balance(f); got != 0 {
				t.Fatalf("attempt %d: %d goroutine(s) for %s outlived Stop", attempt, got, f)
			}
		}
	}
}

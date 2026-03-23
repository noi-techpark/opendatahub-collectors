// SPDX-FileCopyrightText: 2026 NOI Techpark <digital@noi.bz.it>
//
// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"testing"
	"time"
)

var testTs = time.Date(2024, 9, 20, 10, 0, 0, 0, time.UTC)

func TestAggregator_FlowSum(t *testing.T) {
	a := NewAggregator()
	var got float64
	var ok bool
	for i := 0; i < 10; i++ {
		got, _, ok = a.Add("CH:0002.01", "average-flow-light-vehicles", 10.0, testTs)
	}
	if !ok {
		t.Fatal("expected result on 10th sample")
	}
	if got != 100.0 {
		t.Fatalf("expected sum 100, got %v", got)
	}
}

func TestAggregator_SpeedMean(t *testing.T) {
	a := NewAggregator()
	var got float64
	var ok bool
	for i := 0; i < 10; i++ {
		got, _, ok = a.Add("CH:0002.01", "average-speed-light-vehicles", float64(i+1), testTs)
	}
	if !ok {
		t.Fatal("expected result on 10th sample")
	}
	// mean of 1..10 = 5.5
	if got != 5.5 {
		t.Fatalf("expected mean 5.5, got %v", got)
	}
}

func TestAggregator_NoEmitBefore10(t *testing.T) {
	a := NewAggregator()
	for i := 0; i < 9; i++ {
		_, _, ok := a.Add("CH:0002.01", "average-speed", 80.0, testTs)
		if ok {
			t.Fatalf("unexpected emit at sample %d", i+1)
		}
	}
}

func TestAggregator_ResetAfterEmit(t *testing.T) {
	a := NewAggregator()
	for i := 0; i < 10; i++ {
		a.Add("CH:0002.01", "average-flow", 5.0, testTs)
	}
	// 11th sample starts new window — should not emit
	_, _, ok := a.Add("CH:0002.01", "average-flow", 5.0, testTs)
	if ok {
		t.Fatal("11th sample should not emit; new window not complete")
	}
	// Fill remaining 8 samples (making 10 total in the second window)
	for i := 0; i < 8; i++ {
		a.Add("CH:0002.01", "average-flow", 5.0, testTs)
	}
	// The 10th sample of the second window should emit
	_, _, ok = a.Add("CH:0002.01", "average-flow", 5.0, testTs)
	if !ok {
		t.Fatal("second window should emit on its 10th sample")
	}
}

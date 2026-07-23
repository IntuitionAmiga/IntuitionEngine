package main

import "testing"

// TestPageDirtyPublish_ZeroAllocsSteadyState pins the page-dirty write path at
// zero allocations. Every guest RAM write passes through publishRange, so the
// steady state (a page already marked in the open epoch) must be two atomic
// loads and a compare with nothing on the heap.
func TestPageDirtyPublish_ZeroAllocsSteadyState(t *testing.T) {
	tr := newPageDirtyTracker(1 << 20)
	tr.publishRange(0x1000, 4) // prime: page now at the open epoch

	steady := testing.AllocsPerRun(1000, func() {
		tr.publishRange(0x1000, 4)
	})
	if steady != 0 {
		t.Fatalf("steady-state page publish allocates %.0f times per run, want 0", steady)
	}

	// A first touch of a fresh page must also stay off the heap.
	first := testing.AllocsPerRun(1000, func() {
		tr.publishRange(0x2000, 4)
	})
	if first != 0 {
		t.Fatalf("first-touch page publish allocates %.0f times per run, want 0", first)
	}
}

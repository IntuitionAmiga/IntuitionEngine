// cpu_yield_wasm_test.go - throttle prediction for the wasm cooperative yield.

//go:build wasm

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"testing"
	"time"
)

// TestWasmYield_SkipNeverOverrunsTheInterval is the safety property: whatever
// the throttle predicts, replaying the calls it wants to skip at the rate it
// observed must not carry past the end of the interval. Overrunning would hold
// the single wasm thread past a frame boundary, which is what the yield exists
// to prevent.
func TestWasmYield_SkipNeverOverrunsTheInterval(t *testing.T) {
	const interval = 16 * time.Millisecond
	for _, calls := range []int{1, 7, 64, 1000, 100000} {
		for _, elapsed := range []time.Duration{
			time.Microsecond, 100 * time.Microsecond, time.Millisecond,
			8 * time.Millisecond, 15*time.Millisecond + 999*time.Microsecond,
		} {
			skip := yieldSkipFor(calls, elapsed, interval)
			if skip < 0 {
				t.Fatalf("calls=%d elapsed=%v: negative skip %d", calls, elapsed, skip)
			}
			if skip > yieldMaxSkip {
				t.Fatalf("calls=%d elapsed=%v: skip %d exceeds the cap %d", calls, elapsed, skip, yieldMaxSkip)
			}
			// Time the skipped calls would consume at the observed rate.
			perCall := elapsed / time.Duration(calls)
			if perCall == 0 {
				continue
			}
			if spent := elapsed + time.Duration(skip)*perCall; spent > interval {
				t.Fatalf("calls=%d elapsed=%v skip=%d: would reach %v, past the %v interval",
					calls, elapsed, skip, spent, interval)
			}
		}
	}
}

// TestWasmYield_SlowdownAfterEstimateStaysBounded is the case a constant-rate
// test cannot see: the estimate is taken during a cheap phase, and the calls
// that follow become far more expensive, as they do when the JIT dispatcher
// starts chaining long runs of blocks. The skip must then be small enough that
// the clock is rechecked without starving rendering, whatever the slowdown.
func TestWasmYield_SlowdownAfterEstimateStaysBounded(t *testing.T) {
	const interval = 16 * time.Millisecond
	// A cheap phase: many calls in very little time, the most optimistic rate
	// the throttle can observe.
	fastCalls, fastElapsed := 100000, 100*time.Microsecond
	skip := yieldSkipFor(fastCalls, fastElapsed, interval)
	if skip > yieldMaxSkip {
		t.Fatalf("skip %d exceeds the safe batch %d", skip, yieldMaxSkip)
	}

	// Now each call costs far more than the rate the estimate came from. The
	// skipped calls must still be bounded work, not a whole interval of it.
	fastPerCall := fastElapsed / time.Duration(fastCalls)
	for _, slowdown := range []int{10, 100, 1000} {
		perCall := fastPerCall * time.Duration(slowdown)
		spent := time.Duration(skip) * perCall
		if spent > interval {
			t.Fatalf("after a %dx slowdown, %d skipped calls consume %v, past the %v interval",
				slowdown, skip, spent, interval)
		}
	}
}

// TestWasmYield_SkipZeroWhenUnpredictable pins the cases where the throttle
// must fall back to consulting the clock on the next call.
func TestWasmYield_SkipZeroWhenUnpredictable(t *testing.T) {
	const interval = 16 * time.Millisecond
	cases := []struct {
		name    string
		calls   int
		elapsed time.Duration
	}{
		{"no calls yet", 0, time.Millisecond},
		{"no time yet", 10, 0},
		{"interval already spent", 10, interval},
		{"interval overrun", 10, interval * 2},
		{"too few calls to extrapolate", 1, 15 * time.Millisecond},
	}
	for _, tc := range cases {
		if got := yieldSkipFor(tc.calls, tc.elapsed, interval); got != 0 {
			t.Fatalf("%s: skip = %d, want 0", tc.name, got)
		}
	}
}

// TestWasmYield_SkipScalesWithHeadroom pins that the throttle actually saves
// clock reads: early in an interval, with a rate established, it must allow a
// substantial skip, and it must allow less as the interval fills.
func TestWasmYield_SkipScalesWithHeadroom(t *testing.T) {
	const interval = 16 * time.Millisecond
	early := yieldSkipFor(100, time.Millisecond, interval)
	late := yieldSkipFor(100, 15*time.Millisecond, interval)
	if early < late {
		t.Fatalf("skip early in the interval (%d) should exceed skip late in it (%d)", early, late)
	}
	if early < yieldMaxSkip {
		t.Fatalf("with plenty of headroom the throttle allowed only %d skips, short of the safe batch %d", early, yieldMaxSkip)
	}
}

// TestWasmYield_ThrottleConsultsClockRarely drives hostCooperativeYield itself
// and counts how often it reaches the clock. The point of the change is that a
// call in the middle of an interval costs a decrement, not a JS crossing.
func TestWasmYield_ThrottleConsultsClockRarely(t *testing.T) {
	savedLast, savedSkip, savedCalls := lastWasmYield, yieldSkipLeft, yieldCallsSince
	defer func() {
		lastWasmYield, yieldSkipLeft, yieldCallsSince = savedLast, savedSkip, savedCalls
	}()

	wasmResetYieldThrottle()
	skipped := 0
	for range 5000 {
		before := yieldSkipLeft
		hostCooperativeYield()
		// A call that consumed a skip never touched the clock.
		if before > 0 && yieldSkipLeft == before-1 {
			skipped++
		}
		if yieldCallsSince == 0 {
			// A park happened and the interval restarted; stop, since the rest
			// of the loop would measure a fresh interval.
			break
		}
	}
	if skipped == 0 {
		t.Fatal("the throttle never skipped a clock read, so every call still crosses into JS")
	}
}

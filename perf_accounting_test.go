// perf_accounting_test.go — counter aggregation, reset, env-gate.
//
// Pins the contract slice 3b establishes:
//
//   1. Disabled state (perfAcctOn=false): AddJit/AddInterp are no-ops.
//      No write to the underlying atomics; Snapshot reads zero forever.
//
//   2. Enabled state (perfAcctOn=true): AddJit/AddInterp accumulate
//      monotonically; Snapshot returns the accumulated values.
//
//   3. Reset zeroes both counters; subsequent Snapshot returns zero
//      until further adds.
//
//   4. The behavioral invariant for measurement validity (jitNs +
//      interpNs <= wall time when no IO wait) is asserted via a
//      synthetic harness that adds known durations and verifies the
//      sum stays within tolerance. The full real-CPU invariant is
//      validated downstream by the Metric 2 fixture authoring step;
//      this file only pins the counter mechanics.
//
// Tests use perfAcctForceEnableForTest to bypass the env-gate Once
// (otherwise parallel tests would race on os.Getenv).

package main

import (
	"sync"
	"testing"
)

func withPerfAcct(t *testing.T, on bool, fn func()) {
	t.Helper()
	prev := perfAcctOn
	perfAcctForceEnableForTest(on)
	defer func() { perfAcctOn = prev }()
	fn()
}

// TestPerfAcct_DisabledNoOp asserts the disabled fast-path: Add helpers
// must not write to the atomics when perfAcctOn=false. Counters stay at
// zero regardless of how many Add calls happen.
func TestPerfAcct_DisabledNoOp(t *testing.T) {
	withPerfAcct(t, false, func() {
		var p PerfAcct
		for i := 0; i < 100; i++ {
			p.AddJit(1234)
			p.AddInterp(5678)
		}
		snap := p.Snapshot()
		if snap.JitNs != 0 || snap.InterpNs != 0 {
			t.Fatalf("disabled PerfAcct: expected (0,0), got (%d,%d) — "+
				"the IE_PERF_ACCT=0 fast path must not write to atomics",
				snap.JitNs, snap.InterpNs)
		}
	})
}

// TestPerfAcct_EnabledAccumulates asserts adds accumulate when enabled.
func TestPerfAcct_EnabledAccumulates(t *testing.T) {
	withPerfAcct(t, true, func() {
		var p PerfAcct
		p.AddJit(1000)
		p.AddJit(2000)
		p.AddInterp(500)
		snap := p.Snapshot()
		if snap.JitNs != 3000 {
			t.Errorf("JitNs: expected 3000, got %d", snap.JitNs)
		}
		if snap.InterpNs != 500 {
			t.Errorf("InterpNs: expected 500, got %d", snap.InterpNs)
		}
	})
}

// TestPerfAcct_Reset asserts Reset zeroes both counters.
func TestPerfAcct_Reset(t *testing.T) {
	withPerfAcct(t, true, func() {
		var p PerfAcct
		p.AddJit(1000)
		p.AddInterp(2000)
		p.Reset()
		snap := p.Snapshot()
		if snap.JitNs != 0 || snap.InterpNs != 0 {
			t.Fatalf("after Reset: expected (0,0), got (%d,%d)",
				snap.JitNs, snap.InterpNs)
		}
		// Reset followed by Add resumes accumulation correctly.
		p.AddJit(42)
		snap = p.Snapshot()
		if snap.JitNs != 42 {
			t.Errorf("post-Reset Add: expected 42, got %d", snap.JitNs)
		}
	})
}

// TestPerfAcct_ConcurrentAdds asserts atomic-add semantics under
// concurrent writers. 4 goroutines each add 100 × 100ns; total must be
// 40000ns regardless of interleaving.
func TestPerfAcct_ConcurrentAdds(t *testing.T) {
	withPerfAcct(t, true, func() {
		var p PerfAcct
		var wg sync.WaitGroup
		for g := 0; g < 4; g++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					p.AddJit(100)
				}
			}()
		}
		wg.Wait()
		snap := p.Snapshot()
		if snap.JitNs != 4*100*100 {
			t.Fatalf("concurrent adds: expected %d, got %d",
				4*100*100, snap.JitNs)
		}
	})
}

// TestPerfAcct_AggregationInvariant asserts the measurement-validity
// invariant: jitNs + interpNs equals the sum of the durations passed
// to Add helpers. This pins that no double-counting or rounding loss
// is introduced by the counter implementation. Real-CPU validation
// (jitNs + interpNs ≈ wall_time when no IO) is downstream in Metric 2
// fixture authoring.
func TestPerfAcct_AggregationInvariant(t *testing.T) {
	withPerfAcct(t, true, func() {
		var p PerfAcct
		jitDurations := []int64{100, 200, 300, 50, 75}
		interpDurations := []int64{40, 60, 80}
		var expectedJit, expectedInterp int64
		for _, d := range jitDurations {
			p.AddJit(d)
			expectedJit += d
		}
		for _, d := range interpDurations {
			p.AddInterp(d)
			expectedInterp += d
		}
		snap := p.Snapshot()
		if snap.JitNs != expectedJit {
			t.Errorf("JitNs aggregation: expected %d, got %d",
				expectedJit, snap.JitNs)
		}
		if snap.InterpNs != expectedInterp {
			t.Errorf("InterpNs aggregation: expected %d, got %d",
				expectedInterp, snap.InterpNs)
		}
		total := snap.JitNs + snap.InterpNs
		expectedTotal := expectedJit + expectedInterp
		if total != expectedTotal {
			t.Errorf("total: expected %d, got %d", expectedTotal, total)
		}
	})
}

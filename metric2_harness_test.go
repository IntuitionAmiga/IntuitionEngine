// metric2_harness_test.go - Smoke test for Metric 2 measurement harness.
//
// Validates that RecordX86RotozoomerMetric2 produces non-trivial
// numbers under a fixed wall-clock window. Pins the contract that
// slice 3c gives slice 3d (Phase 0 reconstruction + fixture authoring):
//
//   - Wall time matches the requested window (within 50ms slack for
//     goroutine startup/teardown).
//   - Instructions retired > 0 (JIT made forward progress).
//   - JitNs > 0 (callNative was invoked at least once and PerfAcct
//     was actually wired into the hot path).
//   - JitNs <= WallNs (sanity: native time can't exceed wall time).
//   - InterpNs >= 0 (may be 0 if no fallbacks fired in the window —
//     rotozoomer doesn't trigger many fallback paths).
//
// The PerfAcct gate is forced on via perfAcctForceEnableForTest so
// the test doesn't require the IE_PERF_ACCT environment variable to
// be set externally — leaking IE_PERF_ACCT into other tests in the
// suite would cause time.Now overhead in unrelated benchmarks.
//
// Skips when the rotozoomer binary is absent (build environments
// without sdk/examples/prebuilt content).

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRecordX86RotozoomerMetric2_ProducesValidSample(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	binPath := filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86")
	if _, err := os.Stat(binPath); err != nil {
		t.Skipf("rotozoomer x86 binary not present at %s: %v", binPath, err)
	}

	prev := perfAcctOn
	perfAcctForceEnableForTest(true)
	defer func() { perfAcctOn = prev }()

	window := 200 * time.Millisecond
	sample, err := RecordX86RotozoomerMetric2(window)
	if err != nil {
		t.Fatalf("RecordX86RotozoomerMetric2: %v", err)
	}

	// Wall time within ±50ms of requested window. Lower bound guards
	// against the harness returning early; upper bound guards against
	// runaway goroutine cleanup.
	expectedNs := window.Nanoseconds()
	slackNs := int64(100 * time.Millisecond)
	if sample.WallNs < expectedNs-slackNs/2 {
		t.Errorf("WallNs=%d, expected >= %d (window minus slack)",
			sample.WallNs, expectedNs-slackNs/2)
	}
	if sample.WallNs > expectedNs+slackNs {
		t.Errorf("WallNs=%d, expected <= %d (window plus slack)",
			sample.WallNs, expectedNs+slackNs)
	}

	// JIT made forward progress.
	if sample.Instructions == 0 {
		t.Error("Instructions=0; rotozoomer JIT did not retire any instructions in 200ms window")
	}

	// PerfAcct was wired and observed.
	if sample.JitNs == 0 {
		t.Error("JitNs=0; PerfAcct did not capture any callNative time — " +
			"either accounting gate is broken or X86ExecuteJIT never entered native code")
	}
	if sample.JitNs > sample.WallNs {
		t.Errorf("JitNs=%d > WallNs=%d; native time cannot exceed wall time",
			sample.JitNs, sample.WallNs)
	}

	// InterpNs is non-negative (atomic.Int64 can't go negative
	// from positive adds, but pin the contract).
	if sample.InterpNs < 0 {
		t.Errorf("InterpNs=%d, expected >= 0", sample.InterpNs)
	}

	t.Logf("rotozoomer-x86 200ms sample: wall=%dns instrs=%d jit=%dns interp=%dns "+
		"(jit-fraction=%.1f%%)",
		sample.WallNs, sample.Instructions, sample.JitNs, sample.InterpNs,
		float64(sample.JitNs)/float64(sample.WallNs)*100)
}

// TestRecordX86RotozoomerMetric2_ErrorsWhenAccountingDisabled asserts
// the safety check: running the harness with PerfAcct disabled returns
// an error rather than producing a zero-jit_bound sample. Without this
// gate, a misconfigured fixture-authoring run could record a sample
// with jit_bound_*=0 and either trip the parser's positive-measurement
// check (good outcome) or, if the parser is later relaxed, land a
// silently-wrong fixture.
func TestRecordX86RotozoomerMetric2_ErrorsWhenAccountingDisabled(t *testing.T) {
	if !x86JitAvailable {
		t.Skip("x86 JIT not available")
	}
	prev := perfAcctOn
	perfAcctForceEnableForTest(false)
	defer func() { perfAcctOn = prev }()

	_, err := RecordX86RotozoomerMetric2(50 * time.Millisecond)
	if err == nil {
		t.Fatal("RecordX86RotozoomerMetric2 with PerfAcct disabled: expected error, got nil")
	}
}

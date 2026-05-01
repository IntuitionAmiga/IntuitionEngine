// metric2_harness.go - Slice 3c: Metric 2 measurement harness for
// rotozoomer-x86 (the first per-backend real workload to be recorded).
//
// Slice 3b laid the JIT-bound time accounting plumbing (PerfAcct,
// IE_PERF_ACCT env gate, callNative + Step instrumentation in
// X86ExecuteJIT). This slice composes that plumbing with the existing
// rotozoomer x86 binary harness (used by the shadow-parity test) into
// a single function that records the four numbers Metric 2 needs:
//
//   - wallclock_ns_to_milestone
//   - instructions_retired_at_milestone
//   - jit_ns_in_window
//   - interp_ns_in_window
//
// Definition of "milestone" for rotozoomer-x86: instructions retired
// reaches a fixed count. Rotozoomer is a non-terminating render loop
// (no natural completion marker), so a fixed instruction-count
// milestone normalizes the measurement across runs without needing
// frame-counter instrumentation in the video chip. The milestone is
// reached by observing the existing instructionCount accumulator
// inside X86ExecuteJIT — but since that accumulator is local, the
// harness instead polls cpu state at fixed intervals from a goroutine
// and stops the run when an external counter reaches the target.
//
// Why not run X86ExecuteJIT directly until milestone: the JIT loop
// owns its own instructionCount; exposing it would require wiring a
// new field through the CPU struct or adding a poll callback. For
// slice 3c the simpler approach is wall-clock-bounded: run for a
// fixed window, then read PerfAcct + a coarse instruction count
// estimated from JIT block.execCount * block.instrCount aggregation.
//
// What slice 3c deliberately does NOT do:
//   - Author testdata/bench-uniformity/metric2.txt (Phase 0
//     reconstruction needed; depends on git-checkout of pre-summit
//     commit which is out of scope per session "no commit" directive).
//   - Wire other backends (only x86 instrumented so far).
//   - Compute frames_per_sec (would require frame-flip detection).
//
// What this file gives the next slice (3d): a callable function with
// stable inputs/outputs that, when run on both pre-summit (after
// re-applying slice 3b PerfAcct plumbing) and current commits,
// produces the four numbers needed to author one Metric 2 record.

//go:build amd64 && (linux || windows || darwin)

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Metric2Sample is the per-run output of the rotozoomer-x86 harness.
// All times in nanoseconds; instructions in absolute count. Caller
// converts to the units required by metric2.txt (ms, fps, etc.).
type Metric2Sample struct {
	WallNs       int64  // wall-clock duration of the measurement window
	Instructions uint64 // total guest instructions retired (sum of block.execCount * block.instrCount)
	JitNs        int64  // ns spent inside callNative (from PerfAcct)
	InterpNs     int64  // ns spent inside Step() fallbacks (from PerfAcct)
}

// RecordX86RotozoomerMetric2 runs the prebuilt rotozoomer x86 binary
// for the given wall-clock window with JIT enabled, demo-accel
// disabled, and PerfAcct on, returning a Metric2Sample. The caller
// must have IE_PERF_ACCT=1 set at process start (or have called
// perfAcctForceEnableForTest). When PerfAcctEnabled() is false this
// function returns an error rather than silently producing zero
// counters — the alternative would let a misconfigured run land a
// fixture with bogus jit_bound figures.
//
// The harness is deliberately thin: it instantiates the same CPU
// configuration as the shadow-parity test (NewMachineBus, demo-accel
// disabled, EIP=0, ESP=0xFFF0), runs X86ExecuteJIT in a goroutine,
// stops it after the window elapses, and reads counters. Total
// instructions are estimated from the cached JIT block stats —
// rotozoomer's hot loop hits the same ~3-block region millions of
// times, so block.execCount * block.instrCount is a faithful
// retired-instruction count without needing per-Step tracking.
func RecordX86RotozoomerMetric2(window time.Duration) (Metric2Sample, error) {
	if !PerfAcctEnabled() {
		return Metric2Sample{}, fmt.Errorf(
			"PerfAcct disabled; set IE_PERF_ACCT=1 at process start before running this harness")
	}
	if !x86JitAvailable {
		return Metric2Sample{}, fmt.Errorf("x86 JIT not available on this build")
	}

	binPath := filepath.Join("sdk", "examples", "prebuilt", "rotozoomer_x86.ie86")
	data, err := os.ReadFile(binPath)
	if err != nil {
		return Metric2Sample{}, fmt.Errorf("read rotozoomer binary %q: %w", binPath, err)
	}

	bus := NewMachineBus()
	adapter := NewX86BusAdapter(bus)
	cpu := NewCPU_X86(adapter)
	cpu.memory = adapter.GetMemory()
	cpu.x86JitIOBitmap = buildX86IOBitmap(adapter, bus)
	cpu.x86JitEnabled = true
	cpu.x86DemoAccel = x86DemoAccelNone // exercise general dispatch

	cpu.EIP = 0
	cpu.ESP = 0xFFF0
	for i, b := range data {
		if uint32(i) >= uint32(len(cpu.memory)) {
			break
		}
		cpu.memory[i] = b
	}

	cpu.perfAcct.Reset()
	cpu.running.Store(true)
	cpu.Halted = false

	t0 := time.Now()
	done := make(chan struct{})
	go func() {
		cpu.X86ExecuteJIT()
		close(done)
	}()

	time.Sleep(window)
	cpu.running.Store(false)

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		return Metric2Sample{}, fmt.Errorf("X86ExecuteJIT did not stop within 2s after running.Store(false)")
	}
	wallNs := time.Since(t0).Nanoseconds()

	// Read counters before freeX86JIT (deferred inside X86ExecuteJIT)
	// has already cleared the JIT code cache. Instruction count comes
	// from PerfAcct, which is on the CPU struct itself and survives
	// the cache teardown — earlier drafts of this harness summed
	// blk.execCount * blk.instrCount across cpu.x86JitCache.blocks
	// and silently returned zero because freeX86JIT had nil'd the
	// cache by the time the goroutine returned.
	snap := cpu.perfAcct.Snapshot()
	return Metric2Sample{
		WallNs:       wallNs,
		Instructions: snap.Instructions,
		JitNs:        snap.JitNs,
		InterpNs:     snap.InterpNs,
	}, nil
}

// perf_accounting.go — opt-in JIT-bound-time accounting.
//
// Metric 2 (per-backend real-workload acceptance gate) needs a clean
// JIT-bound-vs-interpreter-bound time split for each run. Wall time
// alone conflates JIT compile/exec cost with interpreter fallback cost
// and host-side bookkeeping; without the split, a "JIT-faster" claim
// can't be distinguished from a workload that simply happened to spend
// less time in the interpreter for unrelated reasons.
//
// Design constraints:
//   - Production hot path must not pay atomic-counter cost when
//     accounting is disabled. Gate via the IE_PERF_ACCT environment
//     variable (read once at process start). When unset/empty/"0", the
//     observe helpers are no-ops; when "1", they accumulate.
//   - Counters are per-CPU instance (not global) so concurrent
//     emulator instances or a single instance with a coprocessor
//     don't cross-contaminate readings.
//   - Atomic int64 fits the typical range (a single nanosecond counter
//     accumulates ~292 years before overflow).
//   - Reset is explicit; benchmark harnesses call Reset before the
//     measurement window, Snapshot after. No implicit reset on Read.
//
// What's deliberately out of scope for slice 3b:
//   - ioWaitNs: each backend has different wait boundaries (x86's
//     tryFastMMIOPollLoop, M68K's GEMDOS intercept, IE64's MMIO bails).
//     Mixing them in one counter loses meaning. Add per-backend in
//     a follow-up slice.
//   - Cross-backend wiring: only the x86 CPU is wired here today.
//     Other backends get the field hookup when their Metric 2 workload
//     is recorded.

package main

import (
	"os"
	"sync/atomic"
)

// PerfAcct holds the JIT-vs-interpreter time split for one CPU
// instance. Embed into each runner that needs accounting. Read-only
// helpers are safe across goroutines (atomic loads); writes are atomic
// adds. Reset must not race with a measurement window — call it only
// when the CPU is quiesced.
type PerfAcct struct {
	jitNs    atomic.Int64  // ns spent inside native JIT code (callNative roundtrips)
	interpNs atomic.Int64  // ns spent inside interpreter Step() and per-instr fallbacks
	instrs   atomic.Uint64 // guest instructions retired during the measurement window
}

// PerfAcctSnapshot is a frozen-in-time view of a PerfAcct. Returned
// by Snapshot for reporting; safe to log/format without further locking.
type PerfAcctSnapshot struct {
	JitNs        int64
	InterpNs     int64
	Instructions uint64
}

// Reset zeroes the counters. Call before a measurement window starts.
// Not safe to call concurrently with AddJit/AddInterp; only call when
// the CPU is between runs.
func (p *PerfAcct) Reset() {
	p.jitNs.Store(0)
	p.interpNs.Store(0)
	p.instrs.Store(0)
}

// Snapshot reads both counters atomically (each load is atomic; the
// pair is not, so a concurrent writer may produce a snapshot where
// the two halves are seconds apart). For the Metric 2 use case the
// snapshot is taken when the CPU has halted, so single-thread
// consistency is sufficient.
func (p *PerfAcct) Snapshot() PerfAcctSnapshot {
	return PerfAcctSnapshot{
		JitNs:        p.jitNs.Load(),
		InterpNs:     p.interpNs.Load(),
		Instructions: p.instrs.Load(),
	}
}

// AddInstrs accumulates retired guest instructions. No-op when
// accounting is disabled. The X86ExecuteJIT loop calls this with the
// `executed` count returned by each callNative round-trip (which folds
// ChainCount and RetCount per chain-exit accounting), plus +1 for each
// per-instruction interpreter fallback Step.
func (p *PerfAcct) AddInstrs(n uint64) {
	if !perfAcctOn {
		return
	}
	p.instrs.Add(n)
}

// AddJit accumulates ns into the JIT-bound counter. No-op when
// accounting is disabled (perfAcctEnabled() returned false at startup).
// The disabled fast path is a single global-var load + branch — no
// atomic op, no allocation.
func (p *PerfAcct) AddJit(ns int64) {
	if !perfAcctOn {
		return
	}
	p.jitNs.Add(ns)
}

// AddInterp accumulates ns into the interpreter-bound counter. Same
// disabled fast-path semantics as AddJit.
func (p *PerfAcct) AddInterp(ns int64) {
	if !perfAcctOn {
		return
	}
	p.interpNs.Add(ns)
}

// perfAcctOn is read at process start from IE_PERF_ACCT. The value
// cannot change during a run — flipping mid-run would produce
// inconsistent measurements (some calls counted, some not) and the
// disabled fast path can't observe the env mutation cheaply.
//
// Initialization happens in init() so the hot path is correct even if
// no caller ever invokes PerfAcctEnabled() before X86ExecuteJIT runs.
// A previous lazy sync.Once design left perfAcctOn at the zero value
// until first observation, silently zeroing all counters under
// IE_PERF_ACCT=1 unless an external harness primed the gate.
var perfAcctOn bool

func init() {
	perfAcctOn = os.Getenv("IE_PERF_ACCT") == "1"
}

// PerfAcctEnabled returns whether IE_PERF_ACCT was "1" at process
// start. Exposed for tests and for harnesses that need to skip
// accounting-dependent code paths.
func PerfAcctEnabled() bool {
	return perfAcctOn
}

// perfAcctForceEnableForTest is a test-only hook to override the env
// gate. The production code path never calls this.
func perfAcctForceEnableForTest(on bool) {
	perfAcctOn = on
}

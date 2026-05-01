// jit_poll_cadence.go - shared running/IRQ poll cadence for JIT exec loops
// (Phase 7c of the six-CPU JIT unification plan).
//
// Today every JIT exec loop calls cpu.running.Load() per block exit
// (jit_6502_exec.go:112, jit_z80_exec.go:129, jit_m68k_exec.go:98, etc.).
// On hot loops with small blocks this is one atomic load per few hundred
// guest instructions. The interpreter side (cpu_m68k.go:2405) already
// batches: it polls every 4096 guest instructions instead of every step.
//
// PollCadence centralizes the batching policy. JIT exec loops keep a local
// counter, increment it by the number of guest instructions retired in a
// block exit, and call ShouldPoll(counter) to decide whether to do an
// atomic Load. Between polls, the loop trusts the cached "running" value.
//
// Latency cost: at default cadence (4096 instructions), guest stop request
// takes ~4K-instructions worst-case to take effect. At ~100 MIPS this is
// ~40µs — well below any user-visible threshold.
//
// Wiring into per-backend exec loops is the second-half deliverable of
// Phase 7c; this file is the policy + a tested helper.

//go:build amd64 && (linux || windows || darwin)

package main

// DefaultPollCadence is the number of guest instructions between
// running/IRQ atomic polls. Matches the M68K interpreter's existing
// 4096-instruction batch (cpu_m68k.go:2405).
const DefaultPollCadence = 4096

// PollCadence tracks how many guest instructions have been retired since
// the last poll. Embedded in each per-backend JIT exec context (or used
// as a stack-local) by Phase 7c's wiring patch.
//
// The struct is intentionally not goroutine-safe: each JIT exec loop
// runs on its own goroutine; cross-thread "running" updates go through
// the atomic.Bool on the CPU struct, which the poll path reads.
type PollCadence struct {
	cadence uint32
	count   uint32
}

// NewPollCadence builds a PollCadence with the given threshold. Pass
// DefaultPollCadence unless the call site has a specific reason to
// poll more or less often.
func NewPollCadence(cadence uint32) PollCadence {
	if cadence == 0 {
		cadence = DefaultPollCadence
	}
	return PollCadence{cadence: cadence}
}

// Tick advances the counter by the given number of guest instructions
// retired and returns true if the caller should poll the running flag
// (and clear the counter). Returning true does not itself read any
// atomic — the caller does that, so each backend can read its own
// CPU's running atomic without an interface boundary.
func (p *PollCadence) Tick(retired uint32) bool {
	p.count += retired
	if p.count >= p.cadence {
		p.count = 0
		return true
	}
	return false
}

// Reset clears the internal counter. Called on guest reset / context
// reload so the cadence does not skip a poll right after a long pause.
func (p *PollCadence) Reset() {
	p.count = 0
}

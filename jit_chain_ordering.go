// jit_chain_ordering.go - shared chain-slot ordering policy (Phase 6 of the
// six-CPU JIT unification plan).
//
// On amd64 the chain slot at the end of every JIT block tests a 64-bit
// pointer and conditionally jumps:
//
//   movabs r10, <chainSlotPtr>          ; load slot
//   mov    r10, [r10]                   ; load target if patched
//   test   r10, r10
//   jz     fallthroughToInterpreter
//   jmp    r10
//
// The policy this file encodes is *when* in the per-block emit sequence the
// chain-slot check should land relative to flag-producing ops. Phase 2's
// lazy-flag state machine is degraded if a chain-slot probe lives between
// a flag producer and its consumer — the test/jcc clobbers the host EFLAGS,
// forcing a needless materialization.
//
// Phase 6 rule, applied uniformly by every backend's chain emission:
//
//   - The chain-slot probe runs *after* every flag-producing op whose
//     output the probe could clobber, but *before* any subsequent guest
//     consumer that would reload from a fresh flag-producing op.
//   - Equivalently: the slot is the last instruction of the block that may
//     touch host EFLAGS. After the slot, the block exits via either the
//     chained target or the fallthrough; either way the guest flag word is
//     re-materialized at exit.
//
// This file does not emit code — backends do. It records the policy as
// constants + a helper that decides whether a candidate emit position is a
// legal chain-slot landing.
//
// Closure-plan Slice E disposition (DEFERRED): the per-backend audit
// — reading every emit_amd64.go's chain-slot emission and verifying
// no flag-producer/consumer pair is clobbered by the chain TEST/JZ —
// is not yet completed. ChainSlotPolicyFor remains advisory; the
// six-CPU summit's flag-state machine has not had a regression
// attributable to chain-slot ordering since landing, but no emit-site
// audit has been run to confirm the invariant holds end-to-end. Slice
// E follow-up either (a) adds emit-site regression tests at each
// backend's chain-slot emit point and demonstrates the policy is
// satisfied, or (b) deletes ChainSlotPolicyFor and converts this file
// to pure documentation of the invariant.

//go:build amd64 && (linux || windows || darwin)

package main

// ChainSlotPolicy classifies where in a block's emit sequence the chain
// slot may land. Backends consult this so a future tightening (e.g. moving
// the slot past a SETcc-producing tail-call sequence) updates every
// callsite uniformly.
type ChainSlotPolicy int

const (
	// ChainSlotAtTerminator places the chain slot immediately before the
	// block's terminator (Bcc / branch / return). Default for x86, M68K,
	// IE64.
	ChainSlotAtTerminator ChainSlotPolicy = iota

	// ChainSlotAfterMaterialize places the chain slot after the block's
	// flag materialization but before any consumer-side reload. Used by
	// 6502 because P-register materialize lands later in the emit sequence.
	ChainSlotAfterMaterialize

	// ChainSlotInline places the chain slot at the same position as the
	// branch fallthrough; reserved for backends that fold the chain test
	// into the branch encoding itself.
	ChainSlotInline
)

// chainSlotPolicyByBackend documents the per-backend choice as a single
// table so Phase 6's audit (and any future re-audit) is checkable in
// constant time. Keyed by the canonical short backend tag ("ie64", "x86",
// "m68k", "z80", "6502") so this non-test file stays free of the
// test-only BenchBackend enum from jit_bench_harness_test.go.
var chainSlotPolicyByBackend = map[string]ChainSlotPolicy{
	"ie64": ChainSlotAtTerminator,
	"x86":  ChainSlotAtTerminator,
	"m68k": ChainSlotAtTerminator,
	"z80":  ChainSlotAtTerminator,
	"6502": ChainSlotAfterMaterialize,
}

// ChainSlotPolicyFor returns the policy for a backend tag.
func ChainSlotPolicyFor(backendTag string) ChainSlotPolicy {
	return chainSlotPolicyByBackend[backendTag]
}

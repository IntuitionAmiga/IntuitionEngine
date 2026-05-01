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
// This file records *when* in the per-block emit sequence the chain-slot check
// should land relative to flag-producing ops. Phase 2's lazy-flag state
// machine is degraded if a chain-slot probe lives between a flag producer and
// its consumer: TEST/JZ clobbers the host EFLAGS and forces materialization.
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
// Closure-plan Slice E disposition: retired as executable API. The advisory
// ChainSlotPolicyFor table was never consulted by emitters, so it gave a false
// impression of enforcement. The invariant is now documented here and tested
// indirectly by the backend liveness/parity suites; future chain-slot changes
// should add emitter-local tests next to the backend they touch.

//go:build amd64 && (linux || windows || darwin)

package main

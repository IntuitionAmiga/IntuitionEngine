// jit_x86_eflags_liveness.go - per-instruction EFLAGS liveness for x86 JIT
// (Phase 2c of the six-CPU JIT unification plan).
//
// Sibling backends: 6502 in jit_6502_flags_liveness.go, M68K in
// jit_m68k_ccr_liveness.go, Z80 in jit_z80_flags_liveness.go. The x86
// emitter (jit_x86_emit_amd64.go) currently populates `flagsNeeded`
// inline via x86PeepholeFlags. This file mirrors the cross-backend
// scaffold so Phase 4 region superblocks and Phase 7a fusion have a
// uniform single-file landing zone per backend.
//
// Conservative default: every instruction's EFLAGS output is treated
// as live until proven otherwise. False positives cost a flag-extract
// micro-op; false negatives corrupt guest EFLAGS.

//go:build amd64 && (linux || windows || darwin)

package main

// x86EFLAGSLiveness returns, for each slot i in instrs, whether the
// EFLAGS output of instrs[i] is consumed by a downstream instruction
// in the same block. Phase 2c initial wiring is conservative — every
// slot is true. Phase 4 region superblocks consume the contract; the
// body tightens as the cross-backend consumer table lands.
func x86EFLAGSLiveness(instrs []X86JITInstr) JITFlagLiveness {
	if len(instrs) == 0 {
		return nil
	}
	live := make(JITFlagLiveness, len(instrs))
	for i := range live {
		live[i] = true
	}
	return live
}

// x86EFLAGSConsumers reports whether the x86 instruction at instrs[i]
// is an EFLAGS consumer (Jcc, SETcc, CMOVcc, ADC/SBB, conditional
// pushes). Phase 2c: returns true conservatively. Replaced when the
// real consumer table lands.
func x86EFLAGSConsumers(instrs []X86JITInstr, i int) bool {
	_ = instrs
	_ = i
	return true
}

// jit_x86_eflags_liveness.go - per-instruction EFLAGS liveness for x86 JIT.
//
// The x86 compiler and region compiler share this backend-neutral entry
// point. The instruction classification remains in jit_x86_common.go because
// decoder and emitter admission use the same read/write definitions.
//
// The analysis is deliberately block-local. A block exit is an implicit
// consumer, so the compiler marks the final producer live after calling this
// function. This keeps the reusable analysis honest while preserving the
// externally visible EFLAGS boundary.

//go:build amd64 && (linux || windows || darwin)

package main

// x86EFLAGSLiveness returns, for each slot i in instrs, whether the
// EFLAGS output of instrs[i] is consumed by a downstream instruction
// in the same block.
func x86EFLAGSLiveness(instrs []X86JITInstr) JITFlagLiveness {
	return x86PeepholeFlags(instrs)
}

// x86EFLAGSConsumers reports whether the x86 instruction at instrs[i]
// is an EFLAGS consumer (Jcc, SETcc, CMOVcc, ADC/SBB, rotates through
// carry and conditional string forms).
func x86EFLAGSConsumers(instrs []X86JITInstr, i int) bool {
	if i < 0 || i >= len(instrs) {
		return false
	}
	return x86InstrReadsFlags(&instrs[i])
}

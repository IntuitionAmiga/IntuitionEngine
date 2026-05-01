// jit_ie64_abi.go - canonical IE64 JIT register ABI scaffold
// (Phase 7g of the six-CPU JIT unification plan).
//
// IE64 has no asm interpreter today; this file declares the canonical
// pinning so future asm handlers, the JIT emitter, and the abi-consistency
// test stay in lockstep. Mirrors BackendCanonicalABI["ie64"] and
// jit_emit_amd64.go:22-27.
//
// IE64 has 32 GPRs. Five are pinned (R1, R2, R3, R4, R31). R5..R30 are
// spilled and accessed via emitLoadSpilledRegAMD64 / emitStoreSpilledRegAMD64
// at [RDI + ie64Reg*8].

//go:build amd64 && (linux || windows || darwin)

package main

const (
	IE64ABIRegR1  = "RBX" // mapped IE64 R1
	IE64ABIRegR2  = "RBP" // mapped IE64 R2
	IE64ABIRegR3  = "R12" // mapped IE64 R3
	IE64ABIRegR4  = "R13" // mapped IE64 R4
	IE64ABIRegR31 = "R14" // mapped IE64 SP
	IE64ABIRegPC  = "R15" // IE64 PC / return channel
)

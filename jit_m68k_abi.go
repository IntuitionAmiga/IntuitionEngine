// jit_m68k_abi.go - canonical M68K JIT register ABI scaffold
// (Phase 7g of the six-CPU JIT unification plan).
//
// M68K has no asm interpreter today, so this file declares the ABI for the
// future per-opcode handlers Phase 7h adds. The JIT emitter
// (jit_m68k_emit_amd64.go:31-39) is the source of truth; this file mirrors
// it as exported names so future asm handlers and the
// jit_abi_consistency_test.go cross-check have a single read.
//
// No behavior change in this scaffold — the emitter still references its
// own m68kAMD64Reg* constants. A follow-up sub-phase replaces those at the
// emitter with references to the names below.

//go:build amd64 && (linux || windows || darwin)

package main

// M68KCanonicalABI documents the host-register pinning for M68K JIT and
// (future) asm interpreter handlers. Mirrors BackendCanonicalABI["m68k"].
const (
	M68KABIRegD0       = "RBX" // m68kAMD64RegD0
	M68KABIRegD1       = "RBP" // m68kAMD64RegD1
	M68KABIRegA0       = "R12" // m68kAMD64RegA0
	M68KABIRegA7       = "R13" // m68kAMD64RegA7 (SP)
	M68KABIRegCCR      = "R14" // m68kAMD64RegCCR (5-bit XNZVC)
	M68KABIRegCtx      = "R15" // m68kAMD64RegCtx
	M68KABIRegDataBase = "RDI" // m68kAMD64RegDataBase (&DataRegs[0])
	M68KABIRegMemBase  = "RSI" // m68kAMD64RegMemBase (&memory[0])
	M68KABIRegAddrBase = "R9"  // m68kAMD64RegAddrBase (&AddrRegs[0])
)

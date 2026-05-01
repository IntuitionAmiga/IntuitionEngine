// jit_x86_abi.go - canonical x86 JIT register ABI scaffold
// (Phase 7g of the six-CPU JIT unification plan).
//
// x86 has no asm interpreter today; this file declares the canonical
// pinning so future asm handlers, the JIT emitter, and the abi-consistency
// test stay in lockstep. Mirrors BackendCanonicalABI["x86"] and
// jit_x86_emit_amd64.go:36-45. Guest EBP/ESI/EDI are not pinned — they
// live in JITContext spill slots and are loaded on demand.

//go:build amd64 && (linux || windows || darwin)

package main

const (
	X86ABIRegGuestEAX = "RBX" // x86AMD64RegGuestEAX
	X86ABIRegGuestECX = "RBP" // x86AMD64RegGuestECX
	X86ABIRegGuestEDX = "R12" // x86AMD64RegGuestEDX
	X86ABIRegGuestEBX = "R13" // x86AMD64RegGuestEBX
	X86ABIRegGuestESP = "R14" // x86AMD64RegGuestESP
	X86ABIRegCtx      = "R15" // x86AMD64RegCtx
	X86ABIRegMemBase  = "RSI" // x86AMD64RegMemBase
	X86ABIRegIOBM     = "R9"  // x86AMD64RegIOBM
)

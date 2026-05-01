// jit_6502_abi.go - canonical 6502 JIT register ABI scaffold
// (Phase 7g of the six-CPU JIT unification plan).
//
// The 6502 already has an asm interpreter (cpu_6502_interp_amd64.s, gated
// behind interp6502full). Today the JIT and goasm-interp pin different
// host registers; Phase 7g picks the JIT's existing layout as canonical
// and brings the asm-interp onto it in a follow-up. Mirrors
// BackendCanonicalABI["6502"] and jit_6502_emit_amd64.go:31-36.

//go:build amd64 && (linux || windows || darwin)

package main

const (
	P65ABIRegA   = "RBX" // accumulator
	P65ABIRegX   = "RBP" // index X
	P65ABIRegY   = "R12" // index Y
	P65ABIRegSP  = "R13" // stack pointer
	P65ABIRegPC  = "R14" // program counter
	P65ABIRegSR  = "R15" // status register
	P65ABIRegMem = "RSI" // memory base
)

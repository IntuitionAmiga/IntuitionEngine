// jit_6502_abi.go - canonical 6502 JIT register ABI.
//
// Closure-plan F.2: ABI convergence with cpu_6502_interp_amd64.s is
// retired. The interp ABI is deliberately separate; the supported bail
// path is JIT→cpu.Step() through Go (with mapped registers spilled
// across the call). Constants below are the JIT's canonical register
// pinning and must match jit_6502_emit_amd64.go and the
// BackendCanonicalABI["6502"] entry in jit_abi_common.go.

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

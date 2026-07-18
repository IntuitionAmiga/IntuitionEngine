// jit_m68k_dispatch_arm64.go - M68K JIT execution dispatch (arm64 targets).
//
// Milestone 2 of the M68K JIT parity plan: a dedicated arm64 dispatch seam
// so m68kJitAvailable can flip per target without disturbing the amd64
// path. The arm64 M68K backend does not exist yet (milestone 3), so this
// routes to the interpreter and leaves m68kJitAvailable false. Backend
// bring-up changes only this file.

//go:build arm64 && (linux || windows || darwin)

package main

// m68kJitExecute falls back to the interpreter until the arm64 M68K JIT
// backend lands (parity plan milestone 3).
func (cpu *M68KCPU) m68kJitExecute() {
	cpu.ExecuteInstruction()
}

// freeM68KJIT is a no-op until the arm64 backend owns native resources.
func (cpu *M68KCPU) freeM68KJIT() {}

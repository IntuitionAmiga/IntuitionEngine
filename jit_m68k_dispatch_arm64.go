// jit_m68k_dispatch_arm64.go - M68K JIT execution dispatch (arm64 targets).
//
// Milestone 2 of the M68K JIT parity plan: a dedicated arm64 dispatch seam
// so m68kJitAvailable can flip per target without disturbing the amd64
// path. The arm64 M68K backend does not exist yet (milestone 3), so this
// routes to the interpreter and leaves m68kJitAvailable false. Backend
// bring-up changes only this file.

//go:build arm64 && (linux || windows || darwin)

package main

// m68kJitExecute runs the arm64 JIT dispatcher when the JIT is enabled.
// m68kJitAvailable stays false until the full milestone 3 gate passes
// (differential grids plus AROS boot on real arm64 hardware), so default
// runners keep interpreting; tests and explicit opt-in set m68kJitEnabled.
func (cpu *M68KCPU) m68kJitExecute() {
	if cpu.m68kJitEnabled {
		cpu.M68KExecuteJIT()
	} else {
		cpu.ExecuteInstruction()
	}
}

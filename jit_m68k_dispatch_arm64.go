// jit_m68k_dispatch_arm64.go - M68K JIT execution dispatch (arm64 targets).
//
// Dedicated arm64 dispatch seam for the native M68020 backend.

//go:build arm64 && (linux || windows || darwin)

package main

// m68kJitExecute runs the arm64 JIT dispatcher when the JIT is enabled.
func (cpu *M68KCPU) m68kJitExecute() {
	if cpu.m68kJitEnabled {
		cpu.M68KExecuteJIT()
	} else {
		cpu.ExecuteInstruction()
	}
}

func init() {
	m68kJitAvailable = true
}

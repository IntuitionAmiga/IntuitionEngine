// jit_x86_dispatch.go - x86 JIT execution dispatch (JIT-capable platforms)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build (amd64 || arm64) && linux

package main

// x86JitExecute runs the JIT execution loop if JIT is enabled,
// otherwise falls back to the interpreter.
func (cpu *CPU_X86) x86JitExecute() {
	// Correctness-first fallback: the native x86 JIT is not yet trustworthy on
	// full demo workloads such as the x86 rotozoomer. Keep the JIT plumbing and
	// emitter tests intact, but route runtime execution through the interpreter
	// until the real workload path is covered tightly enough to re-enable it.
	cpu.x86RunInterpreter()
}

func init() {
	x86JitAvailable = true
}

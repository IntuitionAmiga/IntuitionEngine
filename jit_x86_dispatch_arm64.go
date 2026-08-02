// jit_x86_dispatch_arm64.go - x86 JIT execution dispatch (Linux/arm64).
//
// The ARM64 backend deliberately starts with a small direct subset. Forms
// outside that subset return to the canonical one-instruction interpreter
// boundary; this preserves x86 state while the emitter is expanded.

//go:build arm64 && linux

package main

func (cpu *CPU_X86) x86JitExecute() {
	if !x86JitAvailable {
		cpu.x86RunInterpreter()
		return
	}
	cpu.X86ExecuteJIT()
}

func init() {
	// The dispatcher only executes a verified direct prefix. Every remaining
	// instruction resumes through the canonical interpreter boundary, so the
	// public gate is safe while direct coverage continues to expand.
	x86JitAvailable = true
}

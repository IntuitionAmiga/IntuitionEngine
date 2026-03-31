// jit_x86_dispatch.go - x86 JIT execution dispatch (JIT-capable platforms)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build (amd64 || arm64) && linux

package main

// x86JitExecute runs the JIT execution loop if JIT is enabled,
// otherwise falls back to the interpreter.
func (cpu *CPU_X86) x86JitExecute() {
	if cpu.x86JitEnabled {
		cpu.X86ExecuteJIT()
	} else {
		cpu.x86RunInterpreter()
	}
}

func init() {
	x86JitAvailable = true
}

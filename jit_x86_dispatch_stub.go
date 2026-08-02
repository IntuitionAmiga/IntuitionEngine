// jit_x86_dispatch_stub.go - x86 JIT stub for non-JIT platforms.
//
// x86 JIT is unavailable on targets without a native backend. Linux/arm64
// has its own dispatcher and must stay out of this fallback.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build !(amd64 && (linux || windows || darwin)) && !(arm64 && linux)

package main

// x86JitExecute always falls back to the interpreter on non-JIT platforms.
func (cpu *CPU_X86) x86JitExecute() {
	cpu.x86RunInterpreter()
}

// x86RunInterpreter is the fallback interpreter loop.
func (cpu *CPU_X86) x86RunInterpreter() {
	for cpu.Running() && !cpu.Halted {
		cpu.Step()
	}
}

// freeX86JIT is a no-op on non-JIT platforms.
func (cpu *CPU_X86) freeX86JIT() {}

// jit_x86_dispatch_stub.go - x86 JIT stub for non-JIT platforms.
//
// x86 JIT is amd64-only. The arm64-linux build was tagged in earlier
// rounds, but the x86 emitter never landed there, so this stub fills
// every non-amd64 build (including arm64-linux) with an interpreter
// fallback. CPUX86Runner.JITEnabled has no effect outside amd64; the
// runner's r.jit gate falls through to x86RunInterpreter via dispatch.
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build !(amd64 && (linux || windows || darwin))

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

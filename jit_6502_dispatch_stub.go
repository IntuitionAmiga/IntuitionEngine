// jit_6502_dispatch_stub.go - 6502 JIT fallback for non-JIT platforms.
//
// 6502 JIT is amd64-only (per CLAUDE.md: only IE64 has arm64 JIT). The
// arm64-linux build previously tagged the 6502 dispatcher to expect a
// real JIT path, but the arm64 emitter never landed; this stub now
// covers every non-amd64 build (arm64-linux, arm64-darwin, etc.) with
// an interpreter fallback so cross-builds link cleanly.

//go:build !(amd64 && (linux || windows || darwin))

package main

// jit6502Execute falls back to the interpreter on platforms without JIT support.
func (cpu *CPU_6502) jit6502Execute() {
	cpu.Execute()
}

// ExecuteJIT6502 is a no-op stub on non-JIT platforms.
func (cpu *CPU_6502) ExecuteJIT6502() {
	cpu.Execute()
}

// freeJIT6502 is a no-op stub on non-JIT platforms.
func (cpu *CPU_6502) freeJIT6502() {}

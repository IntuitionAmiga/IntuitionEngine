// jit_6502_dispatch_stub.go - 6502 JIT fallback for non-JIT platforms.
//
// This fallback covers every target without a 6502 backend. In particular,
// Windows and macOS intentionally retain interpreter support only.

//go:build !((amd64 || arm64) && linux) && !(js && wasm)

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

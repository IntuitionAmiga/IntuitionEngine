// jit_z80_dispatch_stub.go - Z80 JIT fallback for non-JIT platforms

//go:build !((amd64 && (linux || windows)) || (arm64 && linux))

package main

var z80JitAvailable bool

func init() {
	z80JitAvailable = false
}

// z80JitExecute falls back to the interpreter on platforms without JIT support.
func (cpu *CPU_Z80) z80JitExecute() {
	cpu.Execute()
}

// ExecuteJITZ80 is a no-op stub on non-JIT platforms.
func (cpu *CPU_Z80) ExecuteJITZ80() {
	cpu.Execute()
}

// freeZ80JIT is a no-op stub on non-JIT platforms.
func (cpu *CPU_Z80) freeZ80JIT() {}

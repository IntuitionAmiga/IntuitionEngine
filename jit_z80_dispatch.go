// jit_z80_dispatch.go - Z80 JIT platform dispatch (JIT-capable platforms)

//go:build (amd64 || arm64) && linux

package main

import "runtime"

func init() {
	// Z80 JIT is only functional on amd64. The arm64 emitter is a stub
	// that does not execute instructions — enabling it would silently
	// skip execution. Guard here so arm64 falls back to the interpreter.
	z80JitAvailable = runtime.GOARCH == "amd64"
}

// z80JitExecute routes Z80 execution through JIT or interpreter based on
// platform support, JIT enable flag, and debug mode.
func (cpu *CPU_Z80) z80JitExecute() {
	if cpu.Debug || !cpu.jitEnabled || !z80JitAvailable {
		cpu.Execute()
		return
	}
	cpu.ExecuteJITZ80()
}

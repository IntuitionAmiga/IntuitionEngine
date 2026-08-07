// jit_z80_dispatch.go - Z80 JIT platform dispatch (JIT-capable platforms)

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

import "runtime"

func init() {
	// amd64 has broad direct lowering. Linux ARM64 emits a tested safe subset
	// and routes every remaining form through the frozen canonical helper, so
	// it never skips guest work while direct lowering grows.
	z80JitAvailable = runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64"
}

func z80JITBackend() string { return "native" }

// z80JitExecute routes Z80 execution through JIT or interpreter based on
// platform support, JIT enable flag, and debug mode.
func (cpu *CPU_Z80) z80JitExecute() {
	if cpu.Debug || !cpu.jitEnabled || !z80JitAvailable {
		cpu.Execute()
		return
	}
	cpu.ExecuteJITZ80()
}

// jit_z80_dispatch.go - Z80 JIT platform dispatch (JIT-capable platforms)

//go:build (amd64 || arm64) && linux

package main

func init() {
	z80JitAvailable = true
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

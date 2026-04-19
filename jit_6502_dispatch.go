// jit_6502_dispatch.go - 6502 JIT platform dispatch (JIT-capable platforms)

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

func init() {
	jit6502Available = true
}

// jit6502Execute routes 6502 execution through JIT or interpreter based on
// platform support, JIT enable flag, and debug mode.
func (cpu *CPU_6502) jit6502Execute() {
	if cpu.Debug || !cpu.jitEnabled || !jit6502Available {
		cpu.Execute()
		return
	}
	cpu.ExecuteJIT6502()
}

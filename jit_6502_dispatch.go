// jit_6502_dispatch.go - 6502 JIT platform dispatch (JIT-capable platforms)
//
// 6502 JIT is amd64-only (per CLAUDE.md: only IE64 has arm64 JIT).
// Non-amd64 builds get the stub in jit_6502_dispatch_stub.go.

//go:build amd64 && (linux || windows || darwin)

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

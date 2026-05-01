// jit_x86_dispatch.go - x86 JIT execution dispatch (JIT-capable platforms)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

// x86JitExecute always runs the native x86 JIT. Phase 8 of the JIT-unification
// plan retired the interpreter dispatch gate after shadow-parity confirmed the
// general JIT path runs real x86 binaries byte-equivalent to the interpreter.
// There is no runtime fallback to the interpreter loop: any path that cannot
// be JIT-emitted (initialization failure, scan/compile error) surfaces as a
// panic so the gap is fixed at its source.
//
// Single-instruction bail-and-resume into cpu.Step() (used by MMIO writes
// and the rare unsupported-opcode bail) is part of the JIT↔host-device
// protocol, not an interpreter fallback. It is preserved.
func (cpu *CPU_X86) x86JitExecute() {
	cpu.X86ExecuteJIT()
}

func init() {
	x86JitAvailable = true
}

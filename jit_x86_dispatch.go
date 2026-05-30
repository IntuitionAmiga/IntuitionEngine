// jit_x86_dispatch.go - x86 JIT execution dispatch (JIT-capable platforms)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

// x86JitExecute runs the native x86 JIT. Phase 8 of the JIT-unification plan
// retired the interpreter dispatch gate after shadow-parity confirmed the
// general JIT path runs real x86 binaries byte-equivalent to the interpreter.
// There is no per-block runtime fallback: any path that cannot be JIT-emitted
// (initialization failure, scan/compile error) surfaces as a panic so the gap
// is fixed at its source.
//
// The one exception is host-CPU capability: the x86 emitter uses LAHF/SAHF in
// its REP/Jcc flag plumbing, which a few early x86-64 parts lack in 64-bit
// mode. When x86JitAvailable is false the whole backend falls back to the
// interpreter loop rather than emitting an illegal instruction (SIGILL).
//
// Single-instruction bail-and-resume into cpu.Step() (used by MMIO writes
// and the rare unsupported-opcode bail) is part of the JIT↔host-device
// protocol, not an interpreter fallback. It is preserved.
func (cpu *CPU_X86) x86JitExecute() {
	if !x86JitAvailable {
		cpu.x86RunInterpreter()
		return
	}
	cpu.X86ExecuteJIT()
}

func init() {
	// The x86 guest JIT emits LAHF/SAHF unconditionally. Gate the backend on
	// that host capability so older x86-64 parts that lack it run interpreted
	// instead of hitting SIGILL. (Read of x86Host is safe: it is a package-var
	// initializer, evaluated before any init func.)
	x86JitAvailable = x86Host.HasLAHFSAHF
}

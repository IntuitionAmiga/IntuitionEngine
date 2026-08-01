// jit_x86_dispatch.go - x86 JIT execution dispatch (JIT-capable platforms)
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

//go:build amd64 && (linux || windows || darwin)

package main

// x86JitExecute runs the native x86 JIT. Decoder-admitted forms that have no
// native lowering return through the bounded interpreter path, including the
// decoded x87 helper protocol. Compile errors other than the documented
// no-instruction admission miss remain JIT bugs and surface immediately.
//
// The one exception is host-CPU capability: the x86 emitter uses LAHF/SAHF in
// its REP/Jcc flag plumbing, which a few early x86-64 parts lack in 64-bit
// mode. When x86JitAvailable is false the whole backend falls back to the
// interpreter loop rather than emitting an illegal instruction (SIGILL).
//
// Single-instruction bail-and-resume is part of the JIT and host-device
// protocol. x87 bails use their immutable decoded payload; other forms use
// the ordinary interpreter path.
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

// jit_dispatch.go - JIT execution dispatch (JIT-capable platforms)

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

// jitExecute runs the JIT execution loop if JIT is enabled,
// otherwise falls back to the interpreter.
func (cpu *CPU64) jitExecute() {
	if cpu.jitEnabled {
		cpu.ExecuteJIT()
	} else {
		cpu.Execute()
	}
}

func init() {
	// jitAvailable is the single signal main, scripts, benchmarks, and the
	// IE_REQUIRE_JIT tests use to decide whether the IE64 JIT is usable. Gate it
	// on the host actually supporting every instruction the JIT emits
	// unconditionally (amd64: SSE4.1/ROUNDSS; no-op on arm64) so it reflects
	// reality: on a host that would fall back, jitAvailable is false and callers
	// never report or rely on JIT.
	jitAvailable = checkJITHostFeatures() == nil
}

// jit_dispatch.go - JIT execution dispatch (JIT-capable platforms)

//go:build (amd64 && (linux || windows)) || (arm64 && (linux || windows || darwin))

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
	jitAvailable = true
}

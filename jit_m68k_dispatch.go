// jit_m68k_dispatch.go - M68K JIT execution dispatch (JIT-capable platforms)

//go:build amd64 && (linux || windows || darwin)

package main

// m68kJitExecute runs the JIT execution loop if JIT is enabled,
// otherwise falls back to the interpreter.
func (cpu *M68KCPU) m68kJitExecute() {
	if cpu.m68kJitEnabled {
		cpu.M68KExecuteJIT()
	} else {
		cpu.ExecuteInstruction()
	}
}

func init() {
	m68kJitAvailable = true
}

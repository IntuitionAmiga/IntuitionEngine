// jit_m68k_dispatch_stub.go - M68K JIT stub for non-JIT platforms

//go:build !(amd64 && (linux || windows))

package main

// m68kJitExecute always falls back to the interpreter on non-JIT platforms.
func (cpu *M68KCPU) m68kJitExecute() {
	cpu.ExecuteInstruction()
}

// freeM68KJIT is a no-op on non-JIT platforms.
func (cpu *M68KCPU) freeM68KJIT() {}

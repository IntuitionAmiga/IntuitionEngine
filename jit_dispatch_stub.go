// jit_dispatch_stub.go - JIT stub for non-JIT platforms

//go:build !((amd64 && (linux || windows)) || (arm64 && linux))

package main

// jitExecute always falls back to the interpreter on non-JIT platforms.
func (cpu *CPU64) jitExecute() {
	cpu.Execute()
}

// freeJIT is a no-op on non-JIT platforms.
func (cpu *CPU64) freeJIT() {}

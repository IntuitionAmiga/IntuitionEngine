// jit_dispatch_stub.go - JIT stub for non-JIT platforms.
//
// js/wasm is excluded: it has its own dispatcher (jit_exec_wasm.go) driving
// the wasm bytecode backend.

//go:build !js && !((amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin)))

package main

// jitExecute always falls back to the interpreter on non-JIT platforms.
func (cpu *CPU64) jitExecute() {
	cpu.Execute()
}

// freeJIT is a no-op on non-JIT platforms.
func (cpu *CPU64) freeJIT() {}

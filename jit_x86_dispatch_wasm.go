//go:build js && wasm

// jit_x86_dispatch_wasm.go - x86 JIT execution dispatch (js/wasm).

package main

// The parity plan forbids advertising a partial wasm x86 JIT as available.
// Keep the public availability bit down until the full manifest-backed backend
// is complete, even though subsets below can be developed and differentially
// tested already.
const x86WasmCoverageReady = false

func (cpu *CPU_X86) x86JitExecute() {
	if !x86JitAvailable || !x86WasmJITEnabled() {
		cpu.x86RunInterpreter()
		return
	}
	cpu.X86ExecuteJIT()
}

func init() {
	x86JitAvailable = wasmSIMDSupported() && x86WasmCoverageReady
}

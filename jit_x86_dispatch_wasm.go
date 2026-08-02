//go:build js && wasm

// jit_x86_dispatch_wasm.go - x86 JIT execution dispatch (js/wasm).

package main

// The manifest-backed wasm x86 backend now covers the decoder-supported direct
// and canonical-helper families published by the x86 JIT contract. Public
// availability still remains gated on host SIMD support and the ordinary
// js/wasm runtime checks below.
const x86WasmCoverageReady = true

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

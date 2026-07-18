// jit_m68k_dispatch_wasm.go - M68K JIT execution dispatch (js/wasm).
//
// Milestone 2 of the M68K JIT parity plan: a dedicated wasm dispatch seam
// so m68kJitAvailable can flip per target without disturbing the amd64
// path. The wasm M68K backend does not exist yet (milestone 5), so this
// routes to the interpreter and leaves m68kJitAvailable false. Backend
// bring-up changes only this file.

//go:build js && wasm

package main

// m68kJitExecute falls back to the interpreter until the wasm M68K JIT
// backend lands (parity plan milestone 5).
func (cpu *M68KCPU) m68kJitExecute() {
	cpu.ExecuteInstruction()
}

// freeM68KJIT is a no-op until the wasm backend owns compiled modules.
func (cpu *M68KCPU) freeM68KJIT() {}

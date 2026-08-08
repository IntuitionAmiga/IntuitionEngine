//go:build js && wasm

package main

// The generated module has a wasm32 ABI even when the Go js/wasm runtime uses
// an eight-byte host uintptr. Field offsets must therefore fit the i32 linear
// memory address space. The low 32 bits of a Go js/wasm pointer are its linear
// memory offset, as used by the existing IE64, M68K, 6502, x86, and Z80 wasm
// backends.
var (
	_ [0 - int(ie32JITABIPC>>32)]byte
	_ [0 - int(ie32JITABISP>>32)]byte
	_ [0 - int(ie32JITABIA>>32)]byte
	_ [0 - int(ie32JITABIInterruptEnabled>>32)]byte
	_ [0 - int(ie32JITABIInInterrupt>>32)]byte
	_ [0 - int(ie32JITABIMemory>>32)]byte
)

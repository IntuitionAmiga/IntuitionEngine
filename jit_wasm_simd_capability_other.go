//go:build !(js && wasm)

// jit_wasm_simd_capability_other.go - non-browser SIMD capability default.

package main

// wasmSIMDSupported is meaningful only on the js/wasm runtime path. Native
// tests validate SIMD modules under wazero directly instead of using a host
// browser capability bit.
func wasmSIMDSupported() bool { return false }

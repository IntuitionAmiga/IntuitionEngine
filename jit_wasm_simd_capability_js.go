//go:build js && wasm

// jit_wasm_simd_capability_js.go - browser SIMD capability probe.

package main

import "syscall/js"

// wasmSIMDSupported reports whether the hosting JS engine accepts a SIMD
// module built with the shared encoder. This is the activation gate the x86
// js/wasm backend will use once its lowering is live.
func wasmSIMDSupported() bool {
	global := js.Global()
	wa := global.Get("WebAssembly")
	if !wa.Truthy() {
		return false
	}
	validate := wa.Get("validate")
	if !validate.Truthy() {
		return false
	}
	modBytes := wasmSIMDProbeModuleBytes()
	u8 := global.Get("Uint8Array").New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	return wa.Call("validate", u8).Bool()
}

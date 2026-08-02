//go:build js && wasm

// jit_wasm_simd_js_test.go - js/wasm SIMD capability tests for the shared
// encoder path.

package main

import (
	"math"
	"syscall/js"
	"testing"
)

func TestWasmSIMD_NodeProbeModuleInstantiates(t *testing.T) {
	if !wasmSIMDSupported() {
		t.Fatal("SIMD probe rejected by the hosting JS engine")
	}
	global := js.Global()
	modBytes := wasmSIMDProbeModuleBytes()
	u8 := global.Get("Uint8Array").New(len(modBytes))
	js.CopyBytesToJS(u8, modBytes)
	wa := global.Get("WebAssembly")
	mod := wa.Get("Module").New(u8)
	inst := wa.Get("Instance").New(mod)
	got := inst.Get("exports").Get("probe").Invoke().Float()
	if math.Abs(got-3.75) > 1e-12 {
		t.Fatalf("probe() = %v, want 3.75", got)
	}
}

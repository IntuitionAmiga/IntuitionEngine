// jit_wasm_simd_probe.go - shared SIMD-capability probe module.
//
// The x86 js/wasm backend requires SIMD as a hard capability rather than
// silently degrading to a partial interpreter-only substitute. This tiny
// module exercises the exact encoder features that backend needs: v128 value
// types, the 0xFD SIMD prefix, f64x2 arithmetic and lane extraction.

package main

// wasmSIMDProbeModuleBytes builds a small module exporting probe() -> f64.
// The body computes (1.5 + 2.25) in both f64x2 lanes and returns lane 0 as a
// scalar f64 so runtimes can validate or instantiate it without extra imports.
func wasmSIMDProbeModuleBytes() []byte {
	m := newWasmModuleBuilder()
	typ := m.addType(nil, []byte{wasmTypeF64})
	b := &wasmBody{}
	b.f64Const(1.5)
	b.f64x2Splat()
	b.f64Const(2.25)
	b.f64x2Splat()
	b.f64x2Add()
	b.f64x2ExtractLane(0)
	b.end()
	fn := m.addFunc(typ, nil, b.code)
	m.exportFunc("probe", fn)
	return m.build()
}

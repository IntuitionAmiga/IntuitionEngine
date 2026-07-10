// jit_wasm_stubs.go - inert native-JIT symbols for the js/wasm build.
//
// WebAssembly cannot execute native machine code, so jitAvailable stays false
// and the native JIT backends are absent. IE64 instead has a wasm bytecode
// backend (jit_wasm_encoder.go, jit_wasm_ie64_emit.go, jit_wasm_runtime.go,
// jit_exec_wasm.go): hot blocks are translated to wasm modules and compiled
// by the browser's own engine at runtime. The other CPUs run their
// interpreters. A handful of native-JIT symbols are still referenced by
// untagged shared code that compiles on every target
// (perf_tuning_profiles.go, mmu_ie64.go, jit_mmap_arena.go); this file
// supplies inert definitions so the wasm build links.

//go:build wasm

package main

// execMemAlign matches the native JIT arena alignment. The wasm build never
// allocates an executable arena, but jit_mmap_arena.go's pure-arithmetic
// helpers reference the constant.
const execMemAlign = 16

// flushMicroTLB and invalidateMicroTLBVPN are JIT micro-TLB maintenance hooks
// the shared MMU code (mmu_ie64.go) calls after a translation change. There is
// no JIT micro-TLB on wasm, so both are no-ops.
func (ctx *JITContext) flushMicroTLB()                   {}
func (ctx *JITContext) invalidateMicroTLBVPN(vpn uint64) {}

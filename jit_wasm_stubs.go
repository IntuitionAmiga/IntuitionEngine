// jit_wasm_stubs.go - inert JIT symbols for the js/wasm interpreter-only build.
//
// WebAssembly cannot execute JIT-compiled native code, so jitAvailable is
// false (jit_dispatch_stub.go) and every CPU runs its interpreter. A handful
// of JIT symbols are still referenced by untagged shared code that compiles on
// every target (perf_tuning_profiles.go, mmu_ie64.go, jit_mmap_arena.go). This
// file supplies inert definitions so the wasm build links; none of it runs.

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

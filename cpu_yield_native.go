// cpu_yield_native.go - cooperative yield is a no-op on native builds.
//
// Native targets run each CPU on its own OS thread with async preemption, so
// the interpreter loop never needs to hand the thread back. The empty function
// inlines away to zero cost in the hot loop. The js/wasm build overrides this
// (cpu_yield_wasm.go) because wasm has a single cooperatively-scheduled thread.

//go:build !wasm

package main

func hostCooperativeYield() {}

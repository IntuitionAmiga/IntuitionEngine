// jit_icache_arm64.go - ARM64 instruction cache flush

//go:build arm64 && linux

package main

// flushICacheASM uses DC CVAU + IC IVAU + DSB ISH + ISB to flush
// the instruction cache for the given address range.
// Implemented in jit_icache_arm64.s.
func flushICacheASM(addr, size uintptr)

// flushICache ensures instruction cache coherency for newly written code
// on ARM64. ARM64 does not guarantee icache/dcache coherency, so after
// writing code to memory we must invalidate the icache.
func flushICache(addr, size uintptr) {
	if size == 0 {
		return
	}
	flushICacheASM(addr, size)
}

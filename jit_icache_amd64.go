// jit_icache_amd64.go - x86-64 instruction cache flush (no-op)

//go:build amd64 && linux

package main

// flushICache is a no-op on x86-64. The x86 architecture guarantees
// instruction cache coherency — stores are visible to instruction fetch
// without explicit cache maintenance.
func flushICache(addr, size uintptr) {}

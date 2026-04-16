// jit_icache_amd64.go - x86-64 instruction cache flush (no-op)

//go:build amd64 && linux

package main

// flushICache is a no-op on x86-64. The x86 architecture guarantees
// instruction cache coherency — stores are visible to instruction fetch
// without explicit cache maintenance.
func flushICache(addr, size uintptr) {}

// flushICacheDual is a no-op on x86-64 for the same reason. The
// dual-alias distinction matters only on architectures with
// non-coherent I-cache (notably ARM64); x86 guarantees coherency
// across aliases of the same physical memory as well.
func flushICacheDual(writableAddr, execAddr, size uintptr) {}

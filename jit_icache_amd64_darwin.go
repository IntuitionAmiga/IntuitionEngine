//go:build amd64 && darwin

package main

// flushICache is a no-op on x86-64. The architecture guarantees
// coherent instruction fetch after stores without explicit cache
// invalidation.
func flushICache(addr, size uintptr) {}

// flushICacheDual is also a no-op on x86-64. darwin/amd64 uses a
// single executable mapping, but keeping the dual-view hook avoids
// special cases in shared JIT code.
func flushICacheDual(writableAddr, execAddr, size uintptr) {}

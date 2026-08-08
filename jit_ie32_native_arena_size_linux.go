//go:build linux && (amd64 || arm64)

package main

// ie32JITExecutableArenaSize exposes the native arena capacity to the shared
// policy regression test without coupling wasm builds to Linux mappings.
func ie32JITExecutableArenaSize(cpu *CPU) int {
	if cpu == nil || cpu.jit == nil || cpu.jit.execMem == nil {
		return 0
	}
	return len(cpu.jit.execMem.writable)
}

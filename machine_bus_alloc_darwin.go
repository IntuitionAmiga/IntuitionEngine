// machine_bus_alloc_darwin.go - reset support for darwin anonymous mmap.

//go:build darwin

package main

func resetBusMmapMemory(mem []byte) {
	resetAnonymousMmap(mem)
}

//go:build darwin

package main

import (
	"unsafe"

	"golang.org/x/sys/unix"
)

func resetAnonymousMmap(mem []byte) {
	if len(mem) == 0 {
		return
	}
	addr := unsafe.Pointer(&mem[0])
	ret, err := unix.MmapPtr(-1, 0, addr, uintptr(len(mem)),
		unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_ANON|unix.MAP_PRIVATE|unix.MAP_FIXED)
	if err == nil && ret == addr {
		return
	}
	for i := range mem {
		mem[i] = 0
	}
}

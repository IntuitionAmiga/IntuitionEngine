//go:build arm64 && windows

package main

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32FlushInstructionCache = windows.NewLazySystemDLL("kernel32.dll").NewProc("FlushInstructionCache")
	currentProcess                = windows.CurrentProcess()
)

func flushICache(addr, size uintptr) {
	flushInstructionCache(addr, size)
}

func flushICacheDual(writableAddr, execAddr, size uintptr) {
	flushInstructionCache(execAddr, size)
}

func flushInstructionCache(addr, size uintptr) {
	if size == 0 {
		return
	}
	r1, _, err := kernel32FlushInstructionCache.Call(
		uintptr(currentProcess),
		addr,
		size,
	)
	if r1 == 0 {
		panic(fmt.Sprintf("FlushInstructionCache failed for %p (%d bytes): %v", unsafe.Pointer(addr), size, err))
	}
}

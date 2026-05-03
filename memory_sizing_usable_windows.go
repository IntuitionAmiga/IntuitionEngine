//go:build windows

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

type windowsMemoryStatusEx struct {
	length               uint32
	memoryLoad           uint32
	totalPhys            uint64
	availPhys            uint64
	totalPageFile        uint64
	availPageFile        uint64
	totalVirtual         uint64
	availVirtual         uint64
	availExtendedVirtual uint64
}

func detectUsableRAM() (uint64, string, error) {
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	globalMemoryStatusEx := kernel32.NewProc("GlobalMemoryStatusEx")

	var status windowsMemoryStatusEx
	status.length = uint32(unsafe.Sizeof(status))
	ret, _, err := globalMemoryStatusEx.Call(uintptr(unsafe.Pointer(&status)))
	if ret == 0 {
		return 0, "", fmt.Errorf("GlobalMemoryStatusEx: %w", err)
	}
	return status.availPhys, "windows-GlobalMemoryStatusEx", nil
}

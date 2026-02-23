// jit_mmap.go - Executable memory allocation via syscall.Mmap

//go:build (amd64 || arm64) && linux

package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

// ExecMem manages an mmap'd RWX region for JIT-compiled code.
type ExecMem struct {
	buf  []byte // mmap'd region
	used int    // bump allocator offset
}

const execMemAlign = 16 // 16-byte alignment for all code blocks

// AllocExecMem allocates an RWX memory region of the given size.
func AllocExecMem(size int) (*ExecMem, error) {
	// Round up to page size
	pageSize := syscall.Getpagesize()
	size = (size + pageSize - 1) &^ (pageSize - 1)

	buf, err := syscall.Mmap(-1, 0, size,
		syscall.PROT_READ|syscall.PROT_WRITE|syscall.PROT_EXEC,
		syscall.MAP_PRIVATE|syscall.MAP_ANONYMOUS)
	if err != nil {
		return nil, fmt.Errorf("mmap RWX failed: %w", err)
	}
	return &ExecMem{buf: buf}, nil
}

// Write copies code into the executable region and returns the execution address.
// Successive writes are 16-byte aligned.
func (em *ExecMem) Write(code []byte) (uintptr, error) {
	// Align to 16 bytes
	aligned := (em.used + execMemAlign - 1) &^ (execMemAlign - 1)
	if aligned+len(code) > len(em.buf) {
		return 0, fmt.Errorf("ExecMem exhausted: need %d, have %d", aligned+len(code), len(em.buf))
	}
	em.used = aligned
	copy(em.buf[em.used:], code)
	addr := uintptr(unsafe.Pointer(&em.buf[em.used]))
	em.used += len(code)

	// ARM64: flush instruction cache for the written region
	flushICache(addr, uintptr(len(code)))

	return addr, nil
}

// Reset resets the bump allocator. Existing code becomes invalid.
func (em *ExecMem) Reset() {
	em.used = 0
}

// Free releases the mmap'd memory.
func (em *ExecMem) Free() {
	if em.buf != nil {
		syscall.Munmap(em.buf)
		em.buf = nil
	}
}

// Used returns the number of bytes allocated.
func (em *ExecMem) Used() int {
	return em.used
}

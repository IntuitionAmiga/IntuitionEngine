//go:build darwin && amd64

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

type ExecMem struct {
	writable []byte
	exec     []byte
	used     int
}

const execMemAlign = 16

var (
	execMemsMu sync.RWMutex
	execMems   []*ExecMem
)

func AllocExecMem(size int) (*ExecMem, error) {
	pageSize := unix.Getpagesize()
	size = (size + pageSize - 1) &^ (pageSize - 1)

	mem, err := unix.Mmap(
		-1,
		0,
		size,
		unix.PROT_READ|unix.PROT_WRITE|unix.PROT_EXEC,
		unix.MAP_ANON|unix.MAP_PRIVATE,
	)
	if err != nil {
		return nil, fmt.Errorf("mmap executable memory failed: %w", err)
	}

	em := &ExecMem{
		writable: mem,
		exec:     mem,
	}

	execMemsMu.Lock()
	execMems = append(execMems, em)
	execMemsMu.Unlock()

	return em, nil
}

func (em *ExecMem) Write(code []byte) (uintptr, error) {
	aligned := (em.used + execMemAlign - 1) &^ (execMemAlign - 1)
	if aligned+len(code) > len(em.writable) {
		return 0, fmt.Errorf("ExecMem exhausted: need %d, have %d", aligned+len(code), len(em.writable))
	}
	em.used = aligned
	copy(em.writable[em.used:], code)
	addr := uintptr(unsafe.Pointer(&em.exec[em.used]))
	em.used += len(code)
	flushICache(addr, uintptr(len(code)))
	return addr, nil
}

func (em *ExecMem) Reset() {
	em.used = 0
}

func (em *ExecMem) Free() {
	execMemsMu.Lock()
	for i, e := range execMems {
		if e == em {
			execMems = append(execMems[:i], execMems[i+1:]...)
			break
		}
	}
	execMemsMu.Unlock()

	if em.writable != nil {
		_ = unix.Munmap(em.writable)
		em.writable = nil
		em.exec = nil
	}
}

func (em *ExecMem) Used() int {
	return em.used
}

func (em *ExecMem) execToWritable(execAddr uintptr) (uintptr, bool) {
	if len(em.exec) == 0 {
		return 0, false
	}
	base := uintptr(unsafe.Pointer(&em.exec[0]))
	if execAddr < base || execAddr >= base+uintptr(len(em.exec)) {
		return 0, false
	}
	return execAddr, true
}

func lookupWritable(execAddr uintptr) uintptr {
	execMemsMu.RLock()
	defer execMemsMu.RUnlock()
	for _, em := range execMems {
		if addr, ok := em.execToWritable(execAddr); ok {
			return addr
		}
	}
	return 0
}

func PatchRel32At(patchAddr, targetAddr uintptr) {
	writableAddr := lookupWritable(patchAddr)
	if writableAddr == 0 {
		return
	}
	disp := int32(targetAddr - (patchAddr + 4))
	p := (*[4]byte)(unsafe.Pointer(writableAddr))
	p[0] = byte(disp)
	p[1] = byte(disp >> 8)
	p[2] = byte(disp >> 16)
	p[3] = byte(disp >> 24)
	flushICache(patchAddr, 4)
}

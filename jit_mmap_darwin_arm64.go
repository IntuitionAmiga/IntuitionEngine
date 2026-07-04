//go:build darwin && arm64

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

type ExecMem struct {
	mem    []byte
	used   int
	arenas execMemArenaState
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
		unix.MAP_ANON|unix.MAP_PRIVATE|darwinMAPJIT,
	)
	if err != nil {
		return nil, fmt.Errorf("mmap MAP_JIT failed: %w", err)
	}

	em := &ExecMem{mem: mem}

	execMemsMu.Lock()
	execMems = append(execMems, em)
	execMemsMu.Unlock()

	return em, nil
}

func (em *ExecMem) Write(code []byte) (addr uintptr, err error) {
	offset, reserveErr := em.arenas.reserve(len(em.mem), len(code))
	if reserveErr != nil {
		return 0, reserveErr
	}
	if err := jitPrepareForWrite(); err != nil {
		return 0, err
	}
	defer func() {
		finishErr := jitFinishWrite()
		if err == nil && finishErr != nil {
			err = finishErr
		}
	}()

	copy(em.mem[offset:], code)
	addr = uintptr(unsafe.Pointer(&em.mem[offset]))
	if high := offset + len(code); high > em.used {
		em.used = high
	}
	if err := darwinICacheInvalidate(addr, uintptr(len(code))); err != nil {
		return 0, err
	}
	return addr, nil
}

func (em *ExecMem) Reset() {
	em.used = 0
	em.arenas.reset()
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

	if em.mem != nil {
		_ = unix.Munmap(em.mem)
		em.mem = nil
	}
}

func (em *ExecMem) Used() int {
	return em.used
}

func (em *ExecMem) execBytes(execAddr uintptr, size int) ([]byte, bool) {
	if len(em.mem) == 0 || size < 0 {
		return nil, false
	}
	base := uintptr(unsafe.Pointer(&em.mem[0]))
	if execAddr < base {
		return nil, false
	}
	offset := execAddr - base
	if offset > uintptr(len(em.mem)) || uintptr(size) > uintptr(len(em.mem))-offset {
		return nil, false
	}
	return em.mem[offset : offset+uintptr(size)], true
}

func (em *ExecMem) writableBytes(execAddr uintptr, size int) ([]byte, uintptr, bool) {
	b, ok := em.execBytes(execAddr, size)
	if !ok {
		return nil, 0, false
	}
	return b, execAddr, true
}

func lookupWritableBytes(execAddr uintptr, size int) ([]byte, uintptr, bool) {
	execMemsMu.RLock()
	defer execMemsMu.RUnlock()
	for _, em := range execMems {
		if b, addr, ok := em.writableBytes(execAddr, size); ok {
			return b, addr, true
		}
	}
	return nil, 0, false
}

func lookupExecBytes(execAddr uintptr, size int) ([]byte, bool) {
	execMemsMu.RLock()
	defer execMemsMu.RUnlock()
	for _, em := range execMems {
		if b, ok := em.execBytes(execAddr, size); ok {
			return b, true
		}
	}
	return nil, false
}

func releaseExecMemAddr(execAddr uintptr) bool {
	execMemsMu.RLock()
	defer execMemsMu.RUnlock()
	for _, em := range execMems {
		if len(em.mem) == 0 {
			continue
		}
		base := uintptr(unsafe.Pointer(&em.mem[0]))
		if execAddr < base {
			continue
		}
		offset := execAddr - base
		if offset > uintptr(len(em.mem)) {
			continue
		}
		return em.arenas.releaseOffset(int(offset))
	}
	return false
}

func PatchRel32At(patchAddr, targetAddr uintptr) {
	p, _, ok := lookupWritableBytes(patchAddr, 4)
	if !ok {
		return
	}
	if err := jitPrepareForWrite(); err != nil {
		panic(err)
	}
	defer func() {
		if err := jitFinishWrite(); err != nil {
			panic(err)
		}
	}()

	disp := int32(targetAddr - (patchAddr + 4))
	p[0] = byte(disp)
	p[1] = byte(disp >> 8)
	p[2] = byte(disp >> 16)
	p[3] = byte(disp >> 24)

	if err := darwinICacheInvalidate(patchAddr, 4); err != nil {
		panic(err)
	}
}

//go:build windows && (amd64 || arm64)

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

type ExecMem struct {
	writable []byte
	exec     []byte
	used     int
	mapping  windows.Handle
}

const execMemAlign = 16

var (
	execMemsMu sync.RWMutex
	execMems   []*ExecMem
)

func AllocExecMem(size int) (*ExecMem, error) {
	pageSize := windows.Getpagesize()
	size = (size + pageSize - 1) &^ (pageSize - 1)
	size64 := uint64(size)

	mapping, err := windows.CreateFileMapping(
		windows.InvalidHandle,
		nil,
		windows.PAGE_EXECUTE_READWRITE,
		uint32(size64>>32),
		uint32(size64),
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateFileMapping failed: %w", err)
	}

	writableAddr, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_WRITE, 0, 0, uintptr(size))
	if err != nil {
		_ = windows.CloseHandle(mapping)
		return nil, fmt.Errorf("MapViewOfFile RW failed: %w", err)
	}

	execAddr, err := windows.MapViewOfFile(mapping, windows.FILE_MAP_READ|windows.FILE_MAP_EXECUTE, 0, 0, uintptr(size))
	if err != nil {
		_ = windows.UnmapViewOfFile(writableAddr)
		_ = windows.CloseHandle(mapping)
		return nil, fmt.Errorf("MapViewOfFile RX failed: %w", err)
	}

	em := &ExecMem{
		writable: unsafe.Slice((*byte)(unsafe.Pointer(writableAddr)), size),
		exec:     unsafe.Slice((*byte)(unsafe.Pointer(execAddr)), size),
		mapping:  mapping,
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
	writableAddr := uintptr(unsafe.Pointer(&em.writable[em.used]))
	execAddr := uintptr(unsafe.Pointer(&em.exec[em.used]))
	em.used += len(code)
	flushICacheDual(writableAddr, execAddr, uintptr(len(code)))
	return execAddr, nil
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

	if len(em.exec) != 0 {
		_ = windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&em.exec[0])))
		em.exec = nil
	}
	if len(em.writable) != 0 {
		_ = windows.UnmapViewOfFile(uintptr(unsafe.Pointer(&em.writable[0])))
		em.writable = nil
	}
	if em.mapping != 0 {
		_ = windows.CloseHandle(em.mapping)
		em.mapping = 0
	}
}

func (em *ExecMem) Used() int {
	return em.used
}

func (em *ExecMem) execBytes(execAddr uintptr, size int) ([]byte, bool) {
	if len(em.exec) == 0 || size < 0 {
		return nil, false
	}
	execBase := uintptr(unsafe.Pointer(&em.exec[0]))
	if execAddr < execBase {
		return nil, false
	}
	offset := execAddr - execBase
	if offset > uintptr(len(em.exec)) || uintptr(size) > uintptr(len(em.exec))-offset {
		return nil, false
	}
	return em.exec[offset : offset+uintptr(size)], true
}

func (em *ExecMem) writableBytes(execAddr uintptr, size int) ([]byte, uintptr, bool) {
	if len(em.exec) == 0 || size < 0 {
		return nil, 0, false
	}
	execBase := uintptr(unsafe.Pointer(&em.exec[0]))
	if execAddr < execBase {
		return nil, 0, false
	}
	offset := execAddr - execBase
	if offset > uintptr(len(em.exec)) || uintptr(size) > uintptr(len(em.exec))-offset {
		return nil, 0, false
	}
	writableAddr := uintptr(unsafe.Pointer(&em.writable[offset]))
	return em.writable[offset : offset+uintptr(size)], writableAddr, true
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

func PatchRel32At(patchAddr, targetAddr uintptr) {
	p, writableAddr, ok := lookupWritableBytes(patchAddr, 4)
	if !ok {
		return
	}
	disp := int32(targetAddr - (patchAddr + 4))
	p[0] = byte(disp)
	p[1] = byte(disp >> 8)
	p[2] = byte(disp >> 16)
	p[3] = byte(disp >> 24)
	flushICacheDual(writableAddr, patchAddr, 4)
}

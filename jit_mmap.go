// jit_mmap.go - Dual-mapped executable memory (W^X) for JIT code cache.
//
// The JIT host memory backend satisfies the M15.6 G1 W^X invariant: the
// same physical pages are mapped twice, once writable (RW, not
// executable) for emit and patch paths, once executable (RX, not
// writable) for dispatch. No single mapping ever has both PROT_WRITE
// and PROT_EXEC.
//
// Backing: memfd_create(2) anonymous file, MAP_SHARED for both views.
// Aliasing: writes to the writable view are visible through the exec
// view, because both map the same kernel-side object.

//go:build (amd64 || arm64) && linux

package main

import (
	"fmt"
	"sync"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ExecMem manages a pair of aliased mappings for JIT code: a writable
// view used by emit/patch paths and an executable view used by
// dispatch. Callers hold execution-view addresses (returned by Write
// and stored in chain slots); writes through these addresses go
// through the writable view via PatchRel32At's lookup.
type ExecMem struct {
	writable []byte // RW view; emit and PatchRel32At write here
	exec     []byte // RX view; dispatch jumps here
	used     int    // bump allocator offset (shared for both views)
	fd       int    // memfd backing both mappings
}

const execMemAlign = 16 // 16-byte alignment for all code blocks

// execMems is a package-level registry of every live ExecMem, used by
// PatchRel32At to translate an execution-view address (which is what
// callers store in chain slots) to the corresponding writable-view
// address for the actual byte write.
var (
	execMemsMu sync.RWMutex
	execMems   []*ExecMem
)

// AllocExecMem allocates a dual-mapped code region of at least the
// given size. The returned ExecMem exposes a writable view and an
// executable view over the same physical memory.
func AllocExecMem(size int) (*ExecMem, error) {
	pageSize := unix.Getpagesize()
	size = (size + pageSize - 1) &^ (pageSize - 1)

	// Request an executable memfd. On hardened kernels with
	// vm.memfd_noexec >= 1 the default is non-executable, which would
	// cause the later PROT_EXEC mmap to fail; MFD_EXEC overrides that
	// for this specific fd. On older kernels (< Linux 6.3) MFD_EXEC is
	// an unknown flag and memfd_create returns EINVAL — fall back to
	// the legacy flag set, which still succeeds in the unhardened
	// default policy.
	fd, err := unix.MemfdCreate("intuition-jit", unix.MFD_CLOEXEC|unix.MFD_EXEC)
	if err == unix.EINVAL {
		fd, err = unix.MemfdCreate("intuition-jit", unix.MFD_CLOEXEC)
	}
	if err != nil {
		return nil, fmt.Errorf("memfd_create failed: %w", err)
	}
	if err := unix.Ftruncate(fd, int64(size)); err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("ftruncate memfd failed: %w", err)
	}

	writable, err := unix.Mmap(fd, 0, size,
		unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_SHARED)
	if err != nil {
		_ = unix.Close(fd)
		return nil, fmt.Errorf("mmap RW view failed: %w", err)
	}

	exec, err := unix.Mmap(fd, 0, size,
		unix.PROT_READ|unix.PROT_EXEC,
		unix.MAP_SHARED)
	if err != nil {
		_ = unix.Munmap(writable)
		_ = unix.Close(fd)
		return nil, fmt.Errorf("mmap RX view failed: %w", err)
	}

	em := &ExecMem{
		writable: writable,
		exec:     exec,
		fd:       fd,
	}

	execMemsMu.Lock()
	execMems = append(execMems, em)
	execMemsMu.Unlock()

	return em, nil
}

// Write copies code into the writable view and returns the
// execution-view address for the emitted block. Successive writes are
// 16-byte aligned.
func (em *ExecMem) Write(code []byte) (uintptr, error) {
	aligned := (em.used + execMemAlign - 1) &^ (execMemAlign - 1)
	if aligned+len(code) > len(em.writable) {
		return 0, fmt.Errorf("ExecMem exhausted: need %d, have %d",
			aligned+len(code), len(em.writable))
	}
	em.used = aligned
	copy(em.writable[em.used:], code)
	writableAddr := uintptr(unsafe.Pointer(&em.writable[em.used]))
	execAddr := uintptr(unsafe.Pointer(&em.exec[em.used]))
	em.used += len(code)

	// ARM64 cache coherency under dual aliasing: the stores above went
	// into D-cache lines tagged by writableAddr; the CPU will fetch
	// instructions through execAddr. DC CVAU must target the writable
	// VA (where the dirty lines live) and IC IVAU must target the exec
	// VA (where the fetch will occur). flushICacheDual issues both with
	// the required DSB ISH / ISB barriers. No-op on amd64.
	flushICacheDual(writableAddr, execAddr, uintptr(len(code)))

	return execAddr, nil
}

// Reset resets the bump allocator. Existing code becomes invalid.
func (em *ExecMem) Reset() {
	em.used = 0
}

// Free releases both mappings and closes the backing memfd.
func (em *ExecMem) Free() {
	execMemsMu.Lock()
	for i, e := range execMems {
		if e == em {
			execMems = append(execMems[:i], execMems[i+1:]...)
			break
		}
	}
	execMemsMu.Unlock()

	if em.exec != nil {
		_ = unix.Munmap(em.exec)
		em.exec = nil
	}
	if em.writable != nil {
		_ = unix.Munmap(em.writable)
		em.writable = nil
	}
	if em.fd != 0 {
		_ = unix.Close(em.fd)
		em.fd = 0
	}
}

// Used returns the number of bytes allocated.
func (em *ExecMem) Used() int {
	return em.used
}

// execBytes returns an execution-view byte slice for an execution-view
// address. The returned slice is backed by em's mmap.
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

// writableBytes returns a writable-view byte slice for an execution-view
// address. The returned slice is backed by em's mmap.
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

// PatchRel32At overwrites the 4-byte relative displacement at
// patchAddr (an execution-view address) so that a JMP/Jcc rel32 at
// (patchAddr-1) branches to targetAddr. The displacement is computed
// against the execution-view address (because the CPU reads the rel32
// from the execution view) but written through the writable view.
//
// If patchAddr is not within any registered ExecMem, the call is a
// no-op. This guards tests that hold synthetic chain-slot addresses
// for structure-only assertions and never execute the patched code.
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

	// Dual-alias icache coherency for ARM64: clean the D-cache line
	// via the writable VA, then invalidate the I-cache line via the
	// exec VA. No-op on amd64.
	flushICacheDual(writableAddr, patchAddr, 4)
}

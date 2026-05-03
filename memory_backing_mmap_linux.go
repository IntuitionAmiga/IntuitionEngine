// memory_backing_mmap_linux.go - PLAN_MAX_RAM slice 10a.
//
// MmapBacking on Linux uses anonymous private mmap with MADV_DONTNEED for
// Reset. The underlying memory is not Go-managed; Close calls munmap.

//go:build linux

package main

import (
	"encoding/binary"
	"errors"
	"fmt"

	"golang.org/x/sys/unix"
)

// ErrHighRangeBackingUnsupported is returned by NewMmapBacking on platforms
// that cannot allocate a large mmap-backed Backing. AllocateBacking treats
// this as a non-retryable signal and bootGuestRAMFromComputed soft-falls back
// to the bus.memory window when it sees this sentinel.
var ErrHighRangeBackingUnsupported = errors.New("mmap-backed high-range guest RAM unsupported on this platform")

// MmapBacking is an anonymous-mmap-backed Backing. The underlying mapping is
// not Go-managed; callers must invoke Close to munmap when done.
type MmapBacking struct {
	mem    []byte
	closed bool
}

// NewMmapBacking allocates an anonymous private mmap of the requested size.
// size must be a non-zero multiple of MMU_PAGE_SIZE. Returns the underlying
// mmap error wrapped if the kernel rejects the mapping (RLIMIT_AS, VA
// fragmentation, etc.); the caller (AllocateBacking) is expected to halve
// and retry.
func NewMmapBacking(size uint64) (Backing, error) {
	return newMmapBackingWithMmap(size, func(length, prot, flags int) ([]byte, error) {
		return unix.Mmap(-1, 0, length, prot, flags)
	})
}

func newMmapBackingWithMmap(size uint64, mmapFn func(length, prot, flags int) ([]byte, error)) (Backing, error) {
	if size == 0 {
		return nil, fmt.Errorf("%w: size=0", ErrInvalidSizeArg)
	}
	if size%uint64(MMU_PAGE_SIZE) != 0 {
		return nil, fmt.Errorf("%w: size %d not aligned to MMU_PAGE_SIZE=%d",
			ErrInvalidSizeArg, size, MMU_PAGE_SIZE)
	}
	maxInt := int(^uint(0) >> 1)
	if size > uint64(maxInt) {
		return nil, fmt.Errorf("%w: size %d exceeds int max %d", ErrInvalidSizeArg, size, maxInt)
	}
	mem, err := mmapFn(int(size),
		unix.PROT_READ|unix.PROT_WRITE,
		unix.MAP_ANON|unix.MAP_PRIVATE|unix.MAP_NORESERVE)
	if err != nil {
		return nil, fmt.Errorf("mmap anon failed: %w", err)
	}
	return &MmapBacking{mem: mem}, nil
}

func (b *MmapBacking) Size() uint64 { return uint64(len(b.mem)) }

func (b *MmapBacking) assertOpen() {
	if b.closed {
		panic("MmapBacking use after Close")
	}
}

func (b *MmapBacking) inRange(addr, length uint64) bool {
	b.assertOpen()
	end := addr + length
	if end < addr {
		return false
	}
	return end <= uint64(len(b.mem))
}

func (b *MmapBacking) Read8(addr uint64) byte {
	if !b.inRange(addr, 1) {
		return 0
	}
	return b.mem[addr]
}

func (b *MmapBacking) Write8(addr uint64, v byte) {
	if !b.inRange(addr, 1) {
		return
	}
	b.mem[addr] = v
}

func (b *MmapBacking) Read32(addr uint64) uint32 {
	if !b.inRange(addr, 4) {
		return 0
	}
	return binary.LittleEndian.Uint32(b.mem[addr : addr+4])
}

func (b *MmapBacking) Write32(addr uint64, v uint32) {
	if !b.inRange(addr, 4) {
		return
	}
	binary.LittleEndian.PutUint32(b.mem[addr:addr+4], v)
}

func (b *MmapBacking) Read64(addr uint64) uint64 {
	if !b.inRange(addr, 8) {
		return 0
	}
	return binary.LittleEndian.Uint64(b.mem[addr : addr+8])
}

func (b *MmapBacking) Write64(addr uint64, v uint64) {
	if !b.inRange(addr, 8) {
		return
	}
	binary.LittleEndian.PutUint64(b.mem[addr:addr+8], v)
}

func (b *MmapBacking) ReadBytes(addr uint64, dst []byte) {
	if !b.inRange(addr, uint64(len(dst))) {
		for i := range dst {
			dst[i] = 0
		}
		return
	}
	copy(dst, b.mem[addr:addr+uint64(len(dst))])
}

func (b *MmapBacking) WriteBytes(addr uint64, src []byte) {
	if !b.inRange(addr, uint64(len(src))) {
		return
	}
	copy(b.mem[addr:addr+uint64(len(src))], src)
}

// Reset zeroes all backed memory by releasing the pages back to the kernel
// via MADV_DONTNEED. Subsequent reads return zero (demand-paged) and the
// resident-set drops. The mapping itself is not unmapped.
func (b *MmapBacking) Reset() {
	b.assertOpen()
	if len(b.mem) == 0 {
		return
	}
	_ = unix.Madvise(b.mem, unix.MADV_DONTNEED)
}

// Close unmaps the backing. Subsequent reads/writes are undefined.
func (b *MmapBacking) Close() error {
	if b.closed {
		return nil
	}
	err := unix.Munmap(b.mem)
	b.mem = nil
	b.closed = true
	return err
}

// memory_backing.go - Backing-store abstraction for guest RAM.
//
// PLAN_MAX_RAM.md slice 2: introduce a memory backing interface that can be
// implemented by either a contiguous []byte (legacy/low-memory fast path) or
// a sparse page-keyed map (above-4-GiB IE64 tests without a giant []byte).
// Bus and CPU integration is intentionally deferred to slice 3.

package main

import (
	"encoding/binary"
	"fmt"
)

// Backing is the byte-level guest physical memory abstraction. Addresses are
// uint64 so above-4-GiB IE64 paths can use the same interface as legacy
// low-memory paths. Implementations must accept any address; out-of-range
// reads return zero and out-of-range writes are silently ignored.
type Backing interface {
	// Size returns the page-aligned advertised size in bytes.
	Size() uint64

	Read8(addr uint64) byte
	Write8(addr uint64, v byte)
	Read32(addr uint64) uint32
	Write32(addr uint64, v uint32)
	Read64(addr uint64) uint64
	Write64(addr uint64, v uint64)

	ReadBytes(addr uint64, dst []byte)
	WriteBytes(addr uint64, src []byte)

	// Reset zeroes all backed memory and frees lazily-allocated pages where
	// applicable.
	Reset()
}

// ContiguousBacking is a flat []byte backing store. Used for legacy/low-memory
// fast paths and small unit tests.
type ContiguousBacking struct {
	mem []byte
}

// NewContiguousBacking allocates a contiguous []byte of the requested size.
// size must be a non-zero multiple of MMU_PAGE_SIZE.
func NewContiguousBacking(size uint64) (*ContiguousBacking, error) {
	if size == 0 {
		return nil, fmt.Errorf("%w: size=0", ErrInvalidSizeArg)
	}
	if size%uint64(MMU_PAGE_SIZE) != 0 {
		return nil, fmt.Errorf("%w: size %d not aligned to MMU_PAGE_SIZE=%d",
			ErrInvalidSizeArg, size, MMU_PAGE_SIZE)
	}
	return &ContiguousBacking{mem: make([]byte, size)}, nil
}

func (b *ContiguousBacking) Size() uint64 { return uint64(len(b.mem)) }

func (b *ContiguousBacking) inRange(addr, length uint64) bool {
	end := addr + length
	if end < addr {
		return false
	}
	return end <= uint64(len(b.mem))
}

func (b *ContiguousBacking) Read8(addr uint64) byte {
	if !b.inRange(addr, 1) {
		return 0
	}
	return b.mem[addr]
}

func (b *ContiguousBacking) Write8(addr uint64, v byte) {
	if !b.inRange(addr, 1) {
		return
	}
	b.mem[addr] = v
}

func (b *ContiguousBacking) Read32(addr uint64) uint32 {
	if !b.inRange(addr, 4) {
		return 0
	}
	return binary.LittleEndian.Uint32(b.mem[addr : addr+4])
}

func (b *ContiguousBacking) Write32(addr uint64, v uint32) {
	if !b.inRange(addr, 4) {
		return
	}
	binary.LittleEndian.PutUint32(b.mem[addr:addr+4], v)
}

func (b *ContiguousBacking) Read64(addr uint64) uint64 {
	if !b.inRange(addr, 8) {
		return 0
	}
	return binary.LittleEndian.Uint64(b.mem[addr : addr+8])
}

func (b *ContiguousBacking) Write64(addr uint64, v uint64) {
	if !b.inRange(addr, 8) {
		return
	}
	binary.LittleEndian.PutUint64(b.mem[addr:addr+8], v)
}

func (b *ContiguousBacking) ReadBytes(addr uint64, dst []byte) {
	if !b.inRange(addr, uint64(len(dst))) {
		for i := range dst {
			dst[i] = 0
		}
		return
	}
	copy(dst, b.mem[addr:addr+uint64(len(dst))])
}

func (b *ContiguousBacking) WriteBytes(addr uint64, src []byte) {
	if !b.inRange(addr, uint64(len(src))) {
		return
	}
	copy(b.mem[addr:addr+uint64(len(src))], src)
}

func (b *ContiguousBacking) Reset() {
	for i := range b.mem {
		b.mem[i] = 0
	}
}

// SparseBacking is a page-keyed sparse backing store. Pages are allocated on
// first write; reads of unwritten pages return zero. The advertised Size()
// is the upper bound used for in-range checks; total resident memory is
// proportional to AllocatedPages(), not Size().
//
// Used for IE64 above-4-GiB unit tests where allocating a giant []byte is
// neither possible nor required.
type SparseBacking struct {
	advertisedSize uint64
	pages          map[uint64][]byte
	pageSize       uint64
}

// NewSparseBacking returns a sparse backing of the advertised size in bytes.
// The advertised size is rounded down to MMU_PAGE_SIZE.
func NewSparseBacking(advertisedSize uint64) *SparseBacking {
	pageSize := uint64(MMU_PAGE_SIZE)
	return &SparseBacking{
		advertisedSize: advertisedSize &^ (pageSize - 1),
		pages:          make(map[uint64][]byte),
		pageSize:       pageSize,
	}
}

func (b *SparseBacking) Size() uint64 { return b.advertisedSize }

// AllocatedPages reports the number of resident pages. Used by tests to
// confirm that the sparse backing does not retain unused pages.
func (b *SparseBacking) AllocatedPages() int { return len(b.pages) }

func (b *SparseBacking) inRange(addr, length uint64) bool {
	end := addr + length
	if end < addr {
		return false
	}
	return end <= b.advertisedSize
}

func (b *SparseBacking) page(pageIdx uint64, allocate bool) []byte {
	if p, ok := b.pages[pageIdx]; ok {
		return p
	}
	if !allocate {
		return nil
	}
	p := make([]byte, b.pageSize)
	b.pages[pageIdx] = p
	return p
}

func (b *SparseBacking) Read8(addr uint64) byte {
	if !b.inRange(addr, 1) {
		return 0
	}
	pageIdx := addr / b.pageSize
	off := addr % b.pageSize
	p := b.page(pageIdx, false)
	if p == nil {
		return 0
	}
	return p[off]
}

func (b *SparseBacking) Write8(addr uint64, v byte) {
	if !b.inRange(addr, 1) {
		return
	}
	pageIdx := addr / b.pageSize
	off := addr % b.pageSize
	p := b.page(pageIdx, true)
	p[off] = v
}

func (b *SparseBacking) readSpan(addr uint64, dst []byte) {
	for i := range dst {
		dst[i] = b.Read8(addr + uint64(i))
	}
}

func (b *SparseBacking) writeSpan(addr uint64, src []byte) {
	for i := range src {
		b.Write8(addr+uint64(i), src[i])
	}
}

func (b *SparseBacking) Read32(addr uint64) uint32 {
	if !b.inRange(addr, 4) {
		return 0
	}
	var buf [4]byte
	b.readSpan(addr, buf[:])
	return binary.LittleEndian.Uint32(buf[:])
}

func (b *SparseBacking) Write32(addr uint64, v uint32) {
	if !b.inRange(addr, 4) {
		return
	}
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	b.writeSpan(addr, buf[:])
}

func (b *SparseBacking) Read64(addr uint64) uint64 {
	if !b.inRange(addr, 8) {
		return 0
	}
	var buf [8]byte
	b.readSpan(addr, buf[:])
	return binary.LittleEndian.Uint64(buf[:])
}

func (b *SparseBacking) Write64(addr uint64, v uint64) {
	if !b.inRange(addr, 8) {
		return
	}
	var buf [8]byte
	binary.LittleEndian.PutUint64(buf[:], v)
	b.writeSpan(addr, buf[:])
}

func (b *SparseBacking) ReadBytes(addr uint64, dst []byte) {
	if !b.inRange(addr, uint64(len(dst))) {
		for i := range dst {
			dst[i] = 0
		}
		return
	}
	b.readSpan(addr, dst)
}

func (b *SparseBacking) WriteBytes(addr uint64, src []byte) {
	if !b.inRange(addr, uint64(len(src))) {
		return
	}
	b.writeSpan(addr, src)
}

func (b *SparseBacking) Reset() {
	b.pages = make(map[uint64][]byte)
}

// AllocateBacking calls allocator with the requested page-aligned size. On
// allocation failure the size is halved (rounded down to MMU_PAGE_SIZE) and
// retried until either an allocator call succeeds or the candidate size
// drops below MIN_GUEST_RAM. Returns the backing, the size that was
// successfully allocated, and any error.
//
// The retry policy is deterministic so any final reported total/active value
// matches what the bus and guest discovery paths see.
func AllocateBacking(requested uint64, allocator func(size uint64) (Backing, error)) (Backing, uint64, error) {
	if requested == 0 {
		return nil, 0, fmt.Errorf("%w: requested=0", ErrInvalidSizeArg)
	}
	if requested%uint64(MMU_PAGE_SIZE) != 0 {
		return nil, 0, fmt.Errorf("%w: requested %d not aligned to MMU_PAGE_SIZE",
			ErrInvalidSizeArg, requested)
	}
	if requested < MIN_GUEST_RAM {
		return nil, 0, fmt.Errorf("%w: requested=%d", ErrGuestRAMBelowMinimum, requested)
	}

	pageMask := uint64(MMU_PAGE_SIZE) - 1
	size := requested
	var lastErr error
	for size >= MIN_GUEST_RAM {
		b, err := allocator(size)
		if err == nil {
			return b, size, nil
		}
		lastErr = err
		next := (size / 2) &^ pageMask
		if next == size {
			next = size - uint64(MMU_PAGE_SIZE)
			next &^= pageMask
		}
		size = next
	}
	if lastErr != nil {
		return nil, 0, fmt.Errorf("%w: all retries failed (last allocator error: %v)",
			ErrGuestRAMBelowMinimum, lastErr)
	}
	return nil, 0, fmt.Errorf("%w: cannot allocate above floor", ErrGuestRAMBelowMinimum)
}

// AllocateGuestRAM combines AllocateBacking with the bus single-source-of-
// truth wiring. On success the returned MemorySizing reflects the actual
// backed size (which may be less than requested if a retry was needed) and
// is also published on the bus. On failure the bus sizing is left untouched
// so callers can fail clearly without partial state.
//
// active_visible_ram is re-clamped to the new total and to the visible
// ceiling. If a smaller backed total drops below MIN_GUEST_RAM the function
// fails clearly via AllocateBacking's policy.
func AllocateGuestRAM(bus *MachineBus, requested MemorySizing, allocator func(size uint64) (Backing, error)) (Backing, MemorySizing, error) {
	backing, finalSize, err := AllocateBacking(requested.TotalGuestRAM, allocator)
	if err != nil {
		return nil, MemorySizing{}, err
	}
	final := requested
	final.TotalGuestRAM = finalSize
	if final.ActiveVisibleRAM > finalSize {
		final.ActiveVisibleRAM = finalSize
	}
	if final.VisibleCeiling != 0 && final.ActiveVisibleRAM > final.VisibleCeiling {
		final.ActiveVisibleRAM = final.VisibleCeiling
	}
	final.ActiveVisibleRAM = pageAlignDown(final.ActiveVisibleRAM)
	if final.ActiveVisibleRAM < MIN_GUEST_RAM {
		return nil, MemorySizing{}, fmt.Errorf("%w: post-retry active=%d",
			ErrGuestRAMBelowMinimum, final.ActiveVisibleRAM)
	}
	bus.SetSizing(final)
	bus.SetBacking(backing)
	RegisterSysInfoMMIOFromBus(bus)
	return backing, final, nil
}

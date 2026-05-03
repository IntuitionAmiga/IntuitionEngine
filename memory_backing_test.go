// memory_backing_test.go - Tests for the guest RAM backing abstraction.
//
// PLAN_MAX_RAM.md slice 2 (RED phase).

package main

import (
	"errors"
	"sync"
	"testing"
)

const (
	bMiB = 1024 * 1024
	bGiB = 1024 * 1024 * 1024
)

// --- ContiguousBacking ---

func TestContiguousBacking_SizeIsPageAligned(t *testing.T) {
	b, err := NewContiguousBacking(64 * bMiB)
	if err != nil {
		t.Fatalf("NewContiguousBacking: %v", err)
	}
	if b.Size() != 64*bMiB {
		t.Fatalf("Size = %d, want %d", b.Size(), 64*bMiB)
	}
	if b.Size()%uint64(MMU_PAGE_SIZE) != 0 {
		t.Fatalf("Size %d not page-aligned", b.Size())
	}
}

func TestContiguousBacking_RejectsUnalignedSize(t *testing.T) {
	if _, err := NewContiguousBacking(uint64(MMU_PAGE_SIZE) - 1); err == nil {
		t.Fatalf("expected error for unaligned size, got nil")
	}
	if _, err := NewContiguousBacking(uint64(MMU_PAGE_SIZE) + 1); err == nil {
		t.Fatalf("expected error for unaligned size, got nil")
	}
}

func TestContiguousBacking_Read8Write8(t *testing.T) {
	b, err := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	if err != nil {
		t.Fatalf("NewContiguousBacking: %v", err)
	}
	b.Write8(0x10, 0xAB)
	if got := b.Read8(0x10); got != 0xAB {
		t.Fatalf("Read8 = %#x, want 0xAB", got)
	}
}

func TestContiguousBacking_Read32Write32_LittleEndian(t *testing.T) {
	b, _ := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	b.Write32(0x20, 0xDEADBEEF)
	if got := b.Read32(0x20); got != 0xDEADBEEF {
		t.Fatalf("Read32 = %#x, want 0xDEADBEEF", got)
	}
	if b.Read8(0x20) != 0xEF || b.Read8(0x21) != 0xBE || b.Read8(0x22) != 0xAD || b.Read8(0x23) != 0xDE {
		t.Fatalf("Write32 not little-endian: %02x %02x %02x %02x",
			b.Read8(0x20), b.Read8(0x21), b.Read8(0x22), b.Read8(0x23))
	}
}

func TestContiguousBacking_Read64Write64(t *testing.T) {
	b, _ := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	b.Write64(0x40, 0xCAFEBABEDEADBEEF)
	if got := b.Read64(0x40); got != 0xCAFEBABEDEADBEEF {
		t.Fatalf("Read64 = %#x, want 0xCAFEBABEDEADBEEF", got)
	}
}

func TestContiguousBacking_OutOfRangeReadsZero(t *testing.T) {
	b, _ := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	if got := b.Read32(uint64(MMU_PAGE_SIZE) + 0x10); got != 0 {
		t.Fatalf("out-of-range Read32 = %#x, want 0", got)
	}
}

func TestContiguousBacking_OutOfRangeWritesIgnored(t *testing.T) {
	b, _ := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	b.Write32(uint64(MMU_PAGE_SIZE)+0x10, 0xFFFFFFFF)
	// No panic, no growth.
	if b.Size() != uint64(MMU_PAGE_SIZE) {
		t.Fatalf("Size grew after OOR write: %d", b.Size())
	}
}

func TestContiguousBacking_Reset(t *testing.T) {
	b, _ := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	b.Write32(0x100, 0x12345678)
	b.Reset()
	if got := b.Read32(0x100); got != 0 {
		t.Fatalf("Reset did not zero memory: Read32 = %#x", got)
	}
}

func TestContiguousBacking_ReadBytesWriteBytes(t *testing.T) {
	b, _ := NewContiguousBacking(uint64(MMU_PAGE_SIZE))
	src := []byte{1, 2, 3, 4, 5, 6, 7, 8}
	b.WriteBytes(0x80, src)
	dst := make([]byte, 8)
	b.ReadBytes(0x80, dst)
	for i := range src {
		if dst[i] != src[i] {
			t.Fatalf("ReadBytes[%d] = %d, want %d", i, dst[i], src[i])
		}
	}
}

// --- SparseBacking ---

func TestSparseBacking_AdvertisedSize(t *testing.T) {
	const advertised = uint64(8 * bGiB)
	b := NewSparseBacking(advertised)
	if b.Size() != advertised {
		t.Fatalf("Size = %d, want %d", b.Size(), advertised)
	}
}

func TestSparseBacking_RoundTripAt8GiB(t *testing.T) {
	b := NewSparseBacking(16 * bGiB)
	addr := uint64(8 * bGiB)
	b.Write32(addr, 0xFEEDFACE)
	if got := b.Read32(addr); got != 0xFEEDFACE {
		t.Fatalf("Read32 at 8 GiB = %#x, want 0xFEEDFACE", got)
	}
}

func TestSparseBacking_UnwrittenReadsReturnZero(t *testing.T) {
	b := NewSparseBacking(8 * bGiB)
	if got := b.Read32(uint64(4 * bGiB)); got != 0 {
		t.Fatalf("unwritten Read32 = %#x, want 0", got)
	}
	if got := b.Read64(uint64(4 * bGiB)); got != 0 {
		t.Fatalf("unwritten Read64 = %#x, want 0", got)
	}
}

func TestSparseBacking_Read64Write64AcrossPageBoundary(t *testing.T) {
	b := NewSparseBacking(8 * bGiB)
	addr := uint64(MMU_PAGE_SIZE) - 4 // 4 bytes in this page, 4 bytes in next
	b.Write64(addr, 0x0102030405060708)
	if got := b.Read64(addr); got != 0x0102030405060708 {
		t.Fatalf("page-spanning Read64 = %#x, want 0x0102030405060708", got)
	}
}

func TestSparseBacking_Read32Write32AcrossPageBoundary(t *testing.T) {
	b := NewSparseBacking(8 * bGiB)
	addr := uint64(MMU_PAGE_SIZE) - 2
	b.Write32(addr, 0xDEADBEEF)
	if got := b.Read32(addr); got != 0xDEADBEEF {
		t.Fatalf("page-spanning Read32 = %#x, want 0xDEADBEEF", got)
	}
}

func TestSparseBacking_NoGiantAllocation(t *testing.T) {
	// Touch only two pages 8 GiB apart. The sparse impl must only retain
	// those two pages, not allocate the entire advertised size.
	b := NewSparseBacking(64 * bGiB)
	b.Write8(0x100, 0xAA)
	b.Write8(uint64(8*bGiB)+0x200, 0xBB)
	if b.AllocatedPages() != 2 {
		t.Fatalf("AllocatedPages = %d, want 2", b.AllocatedPages())
	}
	resident := uint64(b.AllocatedPages()) * uint64(MMU_PAGE_SIZE)
	if resident > 16*bMiB {
		t.Fatalf("sparse backing retained %d bytes, expected page-scoped", resident)
	}
}

func TestSparseBacking_OutOfRangeReadsZeroWritesIgnored(t *testing.T) {
	b := NewSparseBacking(uint64(MMU_PAGE_SIZE))
	if got := b.Read32(2 * uint64(MMU_PAGE_SIZE)); got != 0 {
		t.Fatalf("OOR Read32 = %#x, want 0", got)
	}
	b.Write32(2*uint64(MMU_PAGE_SIZE), 0xFFFFFFFF)
	if b.AllocatedPages() != 0 {
		t.Fatalf("OOR write allocated a page (got %d)", b.AllocatedPages())
	}
}

func TestSparseBacking_Reset(t *testing.T) {
	b := NewSparseBacking(8 * bGiB)
	b.Write32(uint64(4*bGiB), 0x1234)
	b.Reset()
	if got := b.Read32(uint64(4 * bGiB)); got != 0 {
		t.Fatalf("Reset did not zero high page")
	}
	if b.AllocatedPages() != 0 {
		t.Fatalf("Reset did not free pages: %d", b.AllocatedPages())
	}
}

func TestSparseBacking_ConcurrentPageFault_Race(t *testing.T) {
	b := NewSparseBacking(8 * bGiB)
	var wg sync.WaitGroup
	for _, addr := range []uint64{0x1000, uint64(4*bGiB) + 0x2000} {
		wg.Add(1)
		go func(addr uint64) {
			defer wg.Done()
			for i := 0; i < 1000; i++ {
				b.Write8(addr+uint64(i%64), byte(i))
			}
		}(addr)
	}
	wg.Wait()
}

func TestSparseBacking_ConcurrentReadWriteSamePage_Race(t *testing.T) {
	b := NewSparseBacking(8 * bGiB)
	const addr = uint64(0x3000)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			b.Write32(addr, uint32(i))
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 1000; i++ {
			_ = b.Read32(addr)
		}
	}()
	wg.Wait()
}

// --- AllocateBacking retry/fail policy ---

func TestAllocateBacking_FirstAttemptSucceeds(t *testing.T) {
	calls := 0
	alloc := func(size uint64) (Backing, error) {
		calls++
		return NewContiguousBacking(size)
	}
	got, finalSize, err := AllocateBacking(64*bMiB, alloc)
	if err != nil {
		t.Fatalf("AllocateBacking: %v", err)
	}
	if calls != 1 {
		t.Fatalf("expected 1 alloc call, got %d", calls)
	}
	if finalSize != 64*bMiB {
		t.Fatalf("finalSize = %d, want %d", finalSize, 64*bMiB)
	}
	if got.Size() != 64*bMiB {
		t.Fatalf("Backing.Size = %d, want %d", got.Size(), 64*bMiB)
	}
}

func TestAllocateBacking_RetriesOnFailureWithSmallerAlignedSize(t *testing.T) {
	failingThreshold := uint64(128 * bMiB)
	var sizes []uint64
	alloc := func(size uint64) (Backing, error) {
		sizes = append(sizes, size)
		if size >= failingThreshold {
			return nil, errors.New("simulated alloc failure")
		}
		return NewContiguousBacking(size)
	}
	got, finalSize, err := AllocateBacking(512*bMiB, alloc)
	if err != nil {
		t.Fatalf("AllocateBacking: %v", err)
	}
	if len(sizes) < 2 {
		t.Fatalf("expected retry, got sizes=%v", sizes)
	}
	if finalSize >= failingThreshold {
		t.Fatalf("finalSize=%d, expected below failing threshold", finalSize)
	}
	if got.Size() != finalSize {
		t.Fatalf("Backing.Size=%d, finalSize=%d", got.Size(), finalSize)
	}
	for _, s := range sizes {
		if s%uint64(MMU_PAGE_SIZE) != 0 {
			t.Fatalf("retry size %d not page-aligned", s)
		}
		if s < MIN_GUEST_RAM {
			t.Fatalf("retry size %d below MIN_GUEST_RAM", s)
		}
	}
}

func TestAllocateBacking_FailsClearlyWhenBelowMin(t *testing.T) {
	alloc := func(size uint64) (Backing, error) {
		return nil, errors.New("always fails")
	}
	_, _, err := AllocateBacking(64*bMiB, alloc)
	if err == nil {
		t.Fatalf("expected error when allocator always fails")
	}
	if !errors.Is(err, ErrGuestRAMBelowMinimum) && !errors.Is(err, ErrInsufficientHostRAM) {
		t.Fatalf("expected ErrGuestRAMBelowMinimum or ErrInsufficientHostRAM, got %v", err)
	}
}

func TestAllocateBacking_RejectsRequestBelowMinUpfront(t *testing.T) {
	alloc := func(size uint64) (Backing, error) {
		return NewContiguousBacking(size)
	}
	_, _, err := AllocateBacking(MIN_GUEST_RAM-uint64(MMU_PAGE_SIZE), alloc)
	if err == nil {
		t.Fatalf("expected error for request below MIN_GUEST_RAM")
	}
	if !errors.Is(err, ErrGuestRAMBelowMinimum) {
		t.Fatalf("expected ErrGuestRAMBelowMinimum, got %v", err)
	}
}

func TestAllocateBacking_RejectsUnalignedRequest(t *testing.T) {
	alloc := func(size uint64) (Backing, error) {
		return NewContiguousBacking(size)
	}
	_, _, err := AllocateBacking(64*bMiB+1, alloc)
	if err == nil {
		t.Fatalf("expected error for unaligned request")
	}
}

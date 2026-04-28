// memory_backing_mmap_test.go - PLAN_MAX_RAM slice 10a TDD coverage.
//
// Validates the MmapBacking allocator: round-trip, reset semantics, input
// validation, and OOB behaviour. Tests are written against the Backing
// interface so they run on every platform; the underlying implementation is
// per-OS (linux/darwin = mmap, others = ContiguousBacking fallback or
// ErrHighRangeBackingUnsupported sentinel).

package main

import (
	"errors"
	"testing"
)

func TestMmapBacking_AllocReadWriteRoundTrip(t *testing.T) {
	const size uint64 = 256 * 1024 * 1024
	b, err := NewMmapBacking(size)
	if err != nil {
		t.Fatalf("NewMmapBacking(256MiB) failed: %v", err)
	}
	defer func() { _ = b.Close() }()
	if got := b.Size(); got != size {
		t.Fatalf("Size = %d, want %d", got, size)
	}

	addrs := []uint64{0, 0x1000, size / 2, size - 8}
	for _, a := range addrs {
		b.Write64(a, 0xDEADBEEFCAFEBABE)
	}
	for _, a := range addrs {
		if got := b.Read64(a); got != 0xDEADBEEFCAFEBABE {
			t.Fatalf("Read64(0x%x) = 0x%x, want 0xDEADBEEFCAFEBABE", a, got)
		}
	}
}

func TestMmapBacking_ResetReturnsZero(t *testing.T) {
	const size uint64 = 16 * 1024 * 1024
	b, err := NewMmapBacking(size)
	if err != nil {
		t.Fatalf("NewMmapBacking failed: %v", err)
	}
	defer func() { _ = b.Close() }()
	b.Write32(0x1000, 0xCAFEBABE)
	b.Write32(size-4, 0x12345678)
	b.Reset()
	if got := b.Read32(0x1000); got != 0 {
		t.Fatalf("after Reset Read32(0x1000)=0x%x, want 0", got)
	}
	if got := b.Read32(size - 4); got != 0 {
		t.Fatalf("after Reset Read32(size-4)=0x%x, want 0", got)
	}
}

func TestMmapBacking_ResetIdempotent(t *testing.T) {
	const size uint64 = 16 * 1024 * 1024
	b, err := NewMmapBacking(size)
	if err != nil {
		t.Fatalf("NewMmapBacking failed: %v", err)
	}
	defer func() { _ = b.Close() }()
	b.Reset()
	b.Reset()
	if got := b.Read32(0); got != 0 {
		t.Fatalf("Read32(0) = 0x%x, want 0", got)
	}
}

func TestMmapBacking_RejectsZeroSize(t *testing.T) {
	_, err := NewMmapBacking(0)
	if !errors.Is(err, ErrInvalidSizeArg) {
		t.Fatalf("NewMmapBacking(0) err=%v, want ErrInvalidSizeArg", err)
	}
}

func TestMmapBacking_RejectsUnalignedSize(t *testing.T) {
	_, err := NewMmapBacking(uint64(MMU_PAGE_SIZE) + 1)
	if !errors.Is(err, ErrInvalidSizeArg) {
		t.Fatalf("NewMmapBacking(unaligned) err=%v, want ErrInvalidSizeArg", err)
	}
}

func TestMmapBacking_OOBReadsZero(t *testing.T) {
	const size uint64 = 4 * 1024 * 1024
	b, err := NewMmapBacking(size)
	if err != nil {
		t.Fatalf("NewMmapBacking failed: %v", err)
	}
	defer func() { _ = b.Close() }()
	if got := b.Read32(size); got != 0 {
		t.Fatalf("Read32(size)=0x%x, want 0", got)
	}
	if got := b.Read64(size - 4); got != 0 {
		t.Fatalf("Read64 straddling OOB=0x%x, want 0", got)
	}
}

func TestMmapBacking_OOBWritesIgnored(t *testing.T) {
	const size uint64 = 4 * 1024 * 1024
	b, err := NewMmapBacking(size)
	if err != nil {
		t.Fatalf("NewMmapBacking failed: %v", err)
	}
	defer func() { _ = b.Close() }()
	b.Write32(size, 0xCAFEBABE)
	b.Write64(size-4, 0xFFFFFFFFFFFFFFFF)
	if got := b.Read32(size - 4); got != 0 {
		t.Fatalf("after OOB write Read32(size-4)=0x%x, want 0", got)
	}
}

// TestAllocateBacking_HighRangeUnsupportedSentinel_NoRetry pins
// AllocateBacking's non-retryable behaviour for the platform-unsupported
// sentinel.
func TestAllocateBacking_HighRangeUnsupportedSentinel_NoRetry(t *testing.T) {
	calls := 0
	allocator := func(size uint64) (Backing, error) {
		calls++
		return nil, ErrHighRangeBackingUnsupported
	}
	const requested uint64 = 256 * 1024 * 1024
	_, _, err := AllocateBacking(requested, allocator)
	if !errors.Is(err, ErrHighRangeBackingUnsupported) {
		t.Fatalf("AllocateBacking err=%v, want ErrHighRangeBackingUnsupported", err)
	}
	if calls != 1 {
		t.Fatalf("allocator invoked %d times, want exactly 1 (no halving retry)", calls)
	}
}

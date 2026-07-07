package main

import (
	"bytes"
	"sync"
	"testing"
)

// Characterisation tests pinning the exact per-byte semantics of SparseBacking
// readSpan/writeSpan before the page-chunked refactor. readSpan/writeSpan are
// per-byte: absent pages read as zero, out-of-range reads yield zero, and
// out-of-range writes are dropped byte-wise (unlike ReadBytes/WriteBytes, which
// are all-or-nothing). These must stay byte-identical after chunking.

func newSpanTestBacking(t *testing.T, pages uint64) *SparseBacking {
	t.Helper()
	return NewSparseBacking(pages * uint64(MMU_PAGE_SIZE))
}

func filledSpan(n int, seed byte) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = seed + byte(i)
	}
	return b
}

func TestSparseBackingWriteSpanCrossesPages(t *testing.T) {
	ps := uint64(MMU_PAGE_SIZE)
	for _, tc := range []struct {
		name string
		addr uint64
		size int
	}{
		{"two-page straddle", ps - 8, 16},
		{"three-page straddle", ps - 4, int(ps) + 8},
		{"page-aligned two pages", ps, 2 * int(ps)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := newSpanTestBacking(t, 5)
			src := filledSpan(tc.size, 0x11)
			b.writeSpan(tc.addr, src)
			got := make([]byte, tc.size)
			for i := range got {
				got[i] = b.read8(tc.addr + uint64(i))
			}
			if !bytes.Equal(got, src) {
				t.Fatalf("readback mismatch at %#x len %d", tc.addr, tc.size)
			}
		})
	}
}

func TestSparseBackingReadSpanCrossesPages(t *testing.T) {
	ps := uint64(MMU_PAGE_SIZE)
	b := newSpanTestBacking(t, 5)
	// Seed a 3-page region byte by byte, then read it in one span.
	base := ps - 4
	size := int(ps)*2 + 8
	for i := 0; i < size; i++ {
		b.write8(base+uint64(i), byte(0x40+i))
	}
	dst := make([]byte, size)
	b.readSpan(base, dst)
	for i := 0; i < size; i++ {
		if dst[i] != byte(0x40+i) {
			t.Fatalf("byte %d: got %#x want %#x", i, dst[i], byte(0x40+i))
		}
	}
}

func TestSparseBackingReadSpanAbsentPageZeroFills(t *testing.T) {
	ps := uint64(MMU_PAGE_SIZE)
	b := newSpanTestBacking(t, 5)
	dst := filledSpan(int(ps)*2+16, 0xAA) // pre-dirty to prove zero-fill
	b.readSpan(ps-8, dst)
	for i, v := range dst {
		if v != 0 {
			t.Fatalf("absent-page read byte %d = %#x, want 0", i, v)
		}
	}
	if got := b.AllocatedPages(); got != 0 {
		t.Fatalf("read of absent pages allocated %d pages, want 0", got)
	}
}

func TestSparseBackingSpanPartiallyOutOfRange(t *testing.T) {
	b := newSpanTestBacking(t, 2) // advertised size = 2 pages
	end := b.Size()
	// Read straddling the end: in-range bytes come from memory (zero here),
	// out-of-range bytes are zero. Whole span therefore zero, no panic.
	dst := filledSpan(32, 0xBB)
	b.readSpan(end-16, dst)
	for i, v := range dst {
		if v != 0 {
			t.Fatalf("straddle read byte %d = %#x, want 0", i, v)
		}
	}
	// Write straddling the end: in-range bytes land, out-of-range bytes drop.
	src := filledSpan(32, 0x70)
	b.writeSpan(end-16, src)
	for i := 0; i < 16; i++ {
		if got := b.read8(end - 16 + uint64(i)); got != src[i] {
			t.Fatalf("in-range write byte %d = %#x, want %#x", i, got, src[i])
		}
	}
	// Bytes at/after end must remain unwritten (still zero, no allocation past end).
	if got := b.read8(end); got != 0 {
		t.Fatalf("OOB write leaked byte at end = %#x, want 0", got)
	}
}

func TestSparseBackingSpanEmpty(t *testing.T) {
	b := newSpanTestBacking(t, 2)
	b.writeSpan(0, nil)
	b.readSpan(0, nil)
	b.writeSpan(0, []byte{})
	b.readSpan(0, []byte{})
	if got := b.AllocatedPages(); got != 0 {
		t.Fatalf("empty span ops allocated %d pages, want 0", got)
	}
}

func benchSpanSizes() []struct {
	name string
	size int
} {
	return []struct {
		name string
		size int
	}{
		{"4K", 4 * 1024},
		{"64K", 64 * 1024},
	}
}

func BenchmarkSparseBackingReadSpan(b *testing.B) {
	for _, sz := range benchSpanSizes() {
		b.Run(sz.name, func(b *testing.B) {
			backing := NewSparseBacking(uint64(sz.size) + uint64(MMU_PAGE_SIZE)*2)
			backing.writeSpan(0x40, filledSpan(sz.size, 0x33)) // make pages present
			dst := make([]byte, sz.size)
			b.SetBytes(int64(sz.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				backing.readSpan(0x40, dst)
			}
		})
	}
}

func BenchmarkSparseBackingWriteSpan(b *testing.B) {
	for _, sz := range benchSpanSizes() {
		b.Run(sz.name, func(b *testing.B) {
			backing := NewSparseBacking(uint64(sz.size) + uint64(MMU_PAGE_SIZE)*2)
			src := filledSpan(sz.size, 0x55)
			b.SetBytes(int64(sz.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				backing.writeSpan(0x40, src)
			}
		})
	}
}

// Per-byte reference benchmarks reproduce the pre-refactor read8/write8 loops so
// benchstat can compare chunked vs per-byte within one binary.
func BenchmarkSparseBackingReadSpanPerByte(b *testing.B) {
	for _, sz := range benchSpanSizes() {
		b.Run(sz.name, func(b *testing.B) {
			backing := NewSparseBacking(uint64(sz.size) + uint64(MMU_PAGE_SIZE)*2)
			backing.writeSpan(0x40, filledSpan(sz.size, 0x33))
			dst := make([]byte, sz.size)
			b.SetBytes(int64(sz.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := range dst {
					dst[j] = backing.read8(0x40 + uint64(j))
				}
			}
		})
	}
}

func BenchmarkSparseBackingWriteSpanPerByte(b *testing.B) {
	for _, sz := range benchSpanSizes() {
		b.Run(sz.name, func(b *testing.B) {
			backing := NewSparseBacking(uint64(sz.size) + uint64(MMU_PAGE_SIZE)*2)
			src := filledSpan(sz.size, 0x55)
			b.SetBytes(int64(sz.size))
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for j := range src {
					backing.write8(0x40+uint64(j), src[j])
				}
			}
		})
	}
}

func TestSparseBackingSpanConcurrentWithWord(t *testing.T) {
	ps := uint64(MMU_PAGE_SIZE)
	b := newSpanTestBacking(t, 8)
	var wg sync.WaitGroup
	for g := 0; g < 8; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			base := uint64(g) * ps
			src := filledSpan(int(ps)+64, byte(g))
			for i := 0; i < 50; i++ {
				b.writeSpan(base+32, src)
				dst := make([]byte, len(src))
				b.readSpan(base+32, dst)
				b.Write32(base+8, uint32(g*1000+i))
				_ = b.Read32(base + 8)
			}
		}(g)
	}
	wg.Wait()
}

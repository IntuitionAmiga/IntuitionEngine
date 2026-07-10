// wasm_guest_ram_test.go - guest RAM sizing and non-mmap backing guards for
// the js/wasm interpreter-only build (Phase 2).
//
// These exercise the shared sizing and bus-construction seams the wasm entry
// relies on, so they run on the host as ordinary tests and, once the Layer C
// harness lands (Phase 4), under Node against the wasm target too. The wasm-
// only constants (busMemBootClamp, NewMmapBacking heap fallback) are pinned
// separately in wasm_guest_ram_wasm_test.go.

package main

import "testing"

// TestWasmGuestRAM_FallbackSizingResolvesTo256MiB pins the sizing decision the
// browser and CI entry points reach when host RAM discovery is unavailable
// (no /proc/meminfo on js). The synthesised fallback (1 GiB usable, 768 MiB
// reserve) must land exactly on the IE64 BASIC minimum so the REPL boots.
func TestWasmGuestRAM_FallbackSizingResolvesTo256MiB(t *testing.T) {
	ms, err := ComputeMemorySizing(^uint64(0), SizingOverrides{
		SkipPlatformCheck:   true,
		DetectedUsableRAM:   1 << 30,
		HostReserveBytes:    768 * 1024 * 1024,
		HostReserveExplicit: true,
	})
	if err != nil {
		t.Fatalf("ComputeMemorySizing fallback: %v", err)
	}
	if got, want := ms.TotalGuestRAM, uint64(ehbasicMinRequiredRAM); got != want {
		t.Fatalf("fallback TotalGuestRAM = %d, want %d (IE64 BASIC minimum)", got, want)
	}
}

// TestWasmGuestRAM_HeapBusServesBASICMinimum proves a BASIC-sized bus can be
// built from a plain Go-heap allocator (the wasm path, which has no mmap) and
// that the full window is addressable, including a byte near the top.
func TestWasmGuestRAM_HeapBusServesBASICMinimum(t *testing.T) {
	heap := func(n uint64) []byte { return make([]byte, n) }
	bus, err := newMachineBusSizedWithAllocator(uint64(ehbasicMinRequiredRAM), heap)
	if err != nil {
		t.Fatalf("newMachineBusSizedWithAllocator(256 MiB, heap): %v", err)
	}
	if got := uint64(len(bus.memory)); got != uint64(ehbasicMinRequiredRAM) {
		t.Fatalf("len(bus.memory) = %d, want %d", got, ehbasicMinRequiredRAM)
	}
	top := uint32(ehbasicMinRequiredRAM) - 4
	bus.Write32(top, 0xDEADBEEF)
	if got := bus.Read32(top); got != 0xDEADBEEF {
		t.Fatalf("round-trip at offset %#x = %#x, want 0xDEADBEEF", top, got)
	}
}

// TestWasmGuestRAM_ContiguousBackingRoundTrips guards the heap Backing that the
// wasm NewMmapBacking delegates to for sub-clamp sizes: it must allocate and
// serve reads and writes without an mmap region.
func TestWasmGuestRAM_ContiguousBackingRoundTrips(t *testing.T) {
	const size = 256 * 1024 * 1024
	backing, err := NewContiguousBacking(size)
	if err != nil {
		t.Fatalf("NewContiguousBacking(256 MiB): %v", err)
	}
	defer backing.Close()
	if got := backing.Size(); got != size {
		t.Fatalf("Size() = %d, want %d", got, size)
	}
	const addr = size - 8
	backing.Write32(addr, 0xCAFEF00D)
	if got := backing.Read32(addr); got != 0xCAFEF00D {
		t.Fatalf("backing round-trip at %#x = %#x, want 0xCAFEF00D", addr, got)
	}
}

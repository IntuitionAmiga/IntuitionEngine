// wasm_guest_ram_wasm_test.go - js/wasm-only guards pinning the non-mmap
// memory path the browser build depends on (Phase 2).
//
// These assert the values in machine_bus_alloc_other.go and
// memory_backing_mmap_other.go that only compile on the js/wasm (and windows)
// target, so they cannot run on a linux or darwin host. The Layer C harness
// (make test-wasm, Phase 4) executes them under Node.

//go:build wasm

package main

import (
	"errors"
	"testing"
)

// TestWasmMemoryBootClampIs256MiB pins the non-mmap boot clamp to the IE64
// BASIC minimum. If this drifts below 256 MiB the browser REPL cannot boot;
// if it grows, a browser tab eagerly commits a larger Go-heap slice.
func TestWasmMemoryBootClampIs256MiB(t *testing.T) {
	if got, want := busMemBootClamp, uint64(256*1024*1024); got != want {
		t.Fatalf("busMemBootClamp = %d, want %d", got, want)
	}
	if got, want := busMemBootClamp, uint64(ehbasicMinRequiredRAM); got != want {
		t.Fatalf("busMemBootClamp = %d, want ehbasicMinRequiredRAM %d", got, want)
	}
}

// TestWasmMemoryHeapAllocator proves the default bus allocator is a plain Go
// heap allocation on js (no mmap), returning exactly the requested length.
func TestWasmMemoryHeapAllocator(t *testing.T) {
	const size = 1 << 20
	buf := defaultBusMemAllocator(size)
	if got := uint64(len(buf)); got != size {
		t.Fatalf("defaultBusMemAllocator(%d) len = %d", size, got)
	}
}

// TestWasmMemoryHighRangeSoftFallback pins the wasm NewMmapBacking contract:
// sizes up to the clamp allocate a heap-backed ContiguousBacking, and anything
// above returns the soft sentinel so bootGuestRAMFromComputed clamps to the
// bus.memory window rather than committing an oversized slice.
func TestWasmMemoryHighRangeSoftFallback(t *testing.T) {
	backing, err := NewMmapBacking(busMemBootClamp)
	if err != nil {
		t.Fatalf("NewMmapBacking(clamp) err = %v, want nil", err)
	}
	if backing == nil {
		t.Fatal("NewMmapBacking(clamp) returned nil backing")
	}
	backing.Close()

	if _, err := NewMmapBacking(busMemBootClamp + 1); !errors.Is(err, ErrHighRangeBackingUnsupported) {
		t.Fatalf("NewMmapBacking(clamp+1) err = %v, want ErrHighRangeBackingUnsupported", err)
	}
}

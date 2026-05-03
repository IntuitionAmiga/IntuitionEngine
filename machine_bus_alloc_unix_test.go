//go:build linux || darwin

package main

import "testing"

func TestAllocateBusMemory_CustomHeapAllocatorDoesNotInstallMmapReset(t *testing.T) {
	mem, reset := allocateBusMemory(busMemMmapThreshold, func(size uint64) []byte {
		return make([]byte, size)
	})
	if len(mem) != int(busMemMmapThreshold) {
		t.Fatalf("len(mem)=%d, want %d", len(mem), busMemMmapThreshold)
	}
	if reset != nil {
		t.Fatal("custom heap allocator received mmap reset hook")
	}
}

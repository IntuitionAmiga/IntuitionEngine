package main

import (
	"testing"
	"unsafe"
)

// TestCPUZ80CacheLineLayout verifies the Z80 running atomic gets full
// cache-line isolation regardless of the CPU struct's base alignment.
// See cpu_x86_layout_test.go for the alignment-vs-cache-line argument.
// Wrapper must be ≥ 2*CacheLineSize and the atomic must sit one
// cache-line into the wrapper so a full line of self-padding precedes
// it.
func TestCPUZ80CacheLineLayout(t *testing.T) {
	var cpu CPU_Z80
	wrapSize := unsafe.Sizeof(cpu.running)
	if wrapSize < 2*CacheLineSize {
		t.Fatalf("CacheLineIsolatedBool size %d < 2*CacheLineSize %d — false-sharing isolation not guaranteed",
			wrapSize, 2*CacheLineSize)
	}
	innerOff := unsafe.Offsetof(cpu.running.v)
	if innerOff != CacheLineSize {
		t.Fatalf("CacheLineIsolatedBool.v at offset %d inside wrapper, want %d (one cache line of pre-pad)",
			innerOff, CacheLineSize)
	}
}

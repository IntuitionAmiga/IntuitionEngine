package main

import (
	"testing"
	"unsafe"
)

// TestCPUX86CacheLineLayout verifies the running atomic gets full
// cache-line isolation regardless of the CPU struct's base alignment.
// Plain offset-mod-64 checks are insufficient because Go aligns structs
// to their max-field alignment (8 bytes), not to cache-line size — the
// struct base can sit at any 8-byte boundary, so an inline-padded atomic
// at field-offset 64 still shares a line with neighbours when base mod
// 64 != 0.
//
// CacheLineIsolatedBool wraps the atomic with 64 bytes of pad on each
// side, total ≥ 128 bytes. This test pins both the wrapper size and the
// position of the atomic inside it.
func TestCPUX86CacheLineLayout(t *testing.T) {
	var cpu CPU_X86
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

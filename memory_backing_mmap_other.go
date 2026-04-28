// memory_backing_mmap_other.go - PLAN_MAX_RAM slice 10a fallback.
//
// Platforms without an mmap-backed implementation (notably windows). High-
// range allocations above busMemMaxBytes return ErrHighRangeBackingUnsupported
// so AllocateBacking does not silently halve down to a small Go-heap
// allocation, and bootGuestRAMFromComputed soft-falls back to the bus.memory
// window. Small sizes fall through to ContiguousBacking so tests and small-
// profile boots still work without a separate code path.

//go:build !linux && !darwin

package main

import "errors"

// ErrHighRangeBackingUnsupported is returned by NewMmapBacking on platforms
// that cannot allocate a large mmap-backed Backing.
var ErrHighRangeBackingUnsupported = errors.New("mmap-backed high-range guest RAM unsupported on this platform")

// NewMmapBacking on non-mmap platforms: only sizes up to busMemBootClamp
// fall through to a ContiguousBacking (Go heap). Anything above the boot
// clamp returns the sentinel so AllocateBacking will halve-and-retry
// (and bootGuestRAMFromComputed soft-falls back to the bus.memory window
// if every retry above the clamp also returns the sentinel). This pins
// the non-mmap clamp end-to-end: a guest profile asking for e.g. AROS at
// 2 GiB on windows can never silently commit a 2 GiB Go heap.
func NewMmapBacking(size uint64) (Backing, error) {
	if size > busMemBootClamp {
		return nil, ErrHighRangeBackingUnsupported
	}
	return NewContiguousBacking(size)
}

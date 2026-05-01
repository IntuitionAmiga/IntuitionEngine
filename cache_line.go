// cache_line.go - cache-line isolation primitive for cross-thread atomics.
//
// Go's runtime aligns structs to their largest field's required alignment
// (8 bytes on amd64/arm64), NOT to cache-line size. So a `_ [N]byte; v
// atomic.Bool` inline padding inside a CPU struct only ensures the field
// offset is a multiple of 64 within the struct — the struct base address
// may sit at any 8-byte boundary. If base mod 64 != 0, the atomic still
// shares a cache line with neighbouring fields no matter how much padding
// precedes it.
//
// CacheLineIsolatedBool wraps an atomic.Bool with 64 bytes of padding on
// each side. Total size is ≥ 128 bytes. Regardless of where the wrapper is
// placed inside its enclosing struct (and regardless of the enclosing
// struct's base alignment), the cache line containing the atomic is fully
// covered by the wrapper's own padding bytes — no foreign field can share
// the line.
//
// Mathematical guarantee: let B be the absolute address of the wrapper's
// first byte. The atomic sits at B + 64. The cache line containing the
// atomic is [floor((B+64)/64)*64 .. that+63]. Since B is at least
// 8-aligned, the line start lies in [B .. B+63] and the line end lies in
// [B+64 .. B+127]. The wrapper occupies [B .. B+128], so the entire line
// fits inside the wrapper's own bytes — guaranteed false-sharing
// elimination.

package main

import "sync/atomic"

// CacheLineSize is the assumed L1-cache line size on supported hosts.
// 64 bytes covers amd64, arm64, and most modern x86/ARM cores. Hosts with
// larger lines (e.g. some POWER9 at 128) still benefit because the
// wrapper's two-line total padding subsumes the larger line size as long
// as it does not exceed 64 bytes (which 128-byte lines do — but 128-byte
// lines are not in IE's supported host set).
const CacheLineSize = 64

// CacheLineIsolatedBool guarantees that its embedded atomic.Bool sits on
// a cache line containing no other field of the enclosing struct. Use in
// place of a bare `atomic.Bool` for any flag that is written by one
// goroutine and polled by another (e.g. CPU.running set by the bus stop
// signal and polled by the execution loop).
//
// Access via Load / Store / CompareAndSwap; the wrapper does not promote
// the atomic.Bool methods directly so callers must go through the
// wrapper API (avoids accidentally taking the address of the inner
// atomic and re-introducing false-sharing through aliasing).
type CacheLineIsolatedBool struct {
	_ [CacheLineSize]byte
	v atomic.Bool
	_ [CacheLineSize]byte
}

func (b *CacheLineIsolatedBool) Load() bool         { return b.v.Load() }
func (b *CacheLineIsolatedBool) Store(val bool)     { b.v.Store(val) }
func (b *CacheLineIsolatedBool) Swap(val bool) bool { return b.v.Swap(val) }
func (b *CacheLineIsolatedBool) CompareAndSwap(old, new bool) bool {
	return b.v.CompareAndSwap(old, new)
}

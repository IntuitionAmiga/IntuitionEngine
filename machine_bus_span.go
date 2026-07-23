// machine_bus_span.go - bulk span transfers across the guest bus.
//
// Device paths that move whole files, ROM images, sound buffers and display
// data through the bus one byte at a time pay the full per-access dispatch for
// every byte: bounds tests, the I/O page bitmap lookup, the debug activity
// test, and on the high side an interface call plus a sparse page lookup. The
// measurement benchmarks in bus_span_bench_test.go put that at 139 MB/s for a
// low-memory staging loop against 11 GB/s for the bulk copy, and 24 MB/s
// against 12 GB/s for a sparse backing write. The byte loop is not a share of
// those operations, it is essentially all of them.
//
// ReadSpan and WriteSpan take the bulk path only when it is indistinguishable
// from the loop it replaces. Anything that would make a byte observable
// individually sends the whole span back to the per-byte path: MMIO anywhere in
// the range, an active debug service, a span crossing the low memory and
// backing seam, or a span that is not wholly mapped RAM. The fallback is not a
// slow corner to be tidied away later; it is what keeps the fast path honest.
//
// The bulk path is on by default, with IE_BUS_SPANS=0 as the kill switch. It
// was landed opt-in and flipped once the differentials passed with each
// eligibility guard proven load-bearing by removing it and watching a test
// fail, and once the benchmarks showed the byte loop was not a share of these
// transfers but essentially all of them.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"os"
	"sync/atomic"
)

// busSpansEnabled reports whether the bulk path may be taken. Reading the
// environment once and caching it keeps the check to an atomic load on a path
// that is called per transfer, not per byte, but the setter exists so the
// differential tests can drive both settings in one process.
var busSpansEnabledFlag atomic.Bool

func init() {
	busSpansEnabledFlag.Store(os.Getenv("IE_BUS_SPANS") != "0")
}

func busSpansEnabled() bool {
	return busSpansEnabledFlag.Load()
}

// setBusSpansEnabled overrides the switch and returns a function restoring the
// previous setting. Tests only.
func setBusSpansEnabled(enabled bool) func() {
	previous := busSpansEnabledFlag.Load()
	busSpansEnabledFlag.Store(enabled)
	return func() { busSpansEnabledFlag.Store(previous) }
}

// spanBulkEligible reports whether [addr, addr+length) can be moved as one
// block with no observable difference from the byte loop. It answers only for
// the bulk path; a false result is always safe.
func (bus *MachineBus) spanBulkEligible(addr, length uint64) bool {
	if bus == nil || length == 0 {
		return false
	}
	end := addr + length
	if end < addr {
		return false
	}
	if bus.debugAccessActive() {
		// Guards, watchpoints and access history all observe individual
		// accesses, so the span has to be delivered as individual accesses.
		return false
	}
	lowEnd := uint64(len(bus.memory))
	if end <= lowEnd {
		if addr > 0xFFFFFFFF {
			return false
		}
		// hasMappedLegacyRange consults the I/O page bitmap first, so a span
		// over plain RAM costs one bitmap test per 256 bytes, not per byte.
		return !bus.hasMappedLegacyRange(uint32(addr), length)
	}
	// Above low memory the backing owns the span, and addrInBacking is what
	// rejects both a span crossing the seam (it requires the start to be at or
	// above the end of bus.memory) and one running off the end of the backing.
	return bus.addrInBacking(addr, length)
}

// spanIntersectsStrictMMIOWindow reports whether any part of the span lies
// inside a configured strict MMIO window. Callers whose per-byte path used the
// fault-checked accessors need this on top of spanBulkEligible: those accessors
// fault on an unmapped address inside a strict window, whereas the bulk
// eligibility test only rejects pages that have a region mapped over them, so
// an unmapped strict-window page passes it. The test is deliberately whole-span
// and does not ask whether each address is mapped, because strict windows are
// rare and being conservative here costs a fallback, not a wrong answer.
func (bus *MachineBus) spanIntersectsStrictMMIOWindow(addr, length uint64) bool {
	if bus == nil || length == 0 || len(bus.strictMMIOWindows) == 0 {
		return false
	}
	end := addr + length - 1
	if end < addr {
		return true
	}
	for _, window := range bus.strictMMIOWindows {
		if addr <= uint64(window.End) && end >= uint64(window.Start) {
			return true
		}
	}
	return false
}

// spanFaultCheckedEligible is spanBulkEligible narrowed to what the
// fault-checked byte accessors would have accepted.
//
// Read8WithFault and Write8WithFault only ever touch bus.memory: both reject
// every address at or above its end outright, including addresses the backing
// would happily serve, and both fault on an unmapped address inside a strict
// MMIO window. A caller replacing a loop of those accessors must not widen what
// its guest can reach, so a buffer they would have rejected has to keep being
// rejected rather than quietly succeeding against backing memory.
func (bus *MachineBus) spanFaultCheckedEligible(addr, length uint64) bool {
	if bus == nil || length == 0 {
		return false
	}
	end := addr + length
	if end < addr || end > uint64(len(bus.memory)) {
		return false
	}
	if bus.spanIntersectsStrictMMIOWindow(addr, length) {
		return false
	}
	return bus.spanBulkEligible(addr, length)
}

// ReadSpan fills dst from guest memory starting at the physical address addr.
// It is equivalent to calling ReadPhys8 for each byte, including the warnings
// and zero fill an out-of-range address produces, because that is exactly what
// it falls back to.
func (bus *MachineBus) ReadSpan(addr uint64, dst []byte) {
	if bus == nil || len(dst) == 0 {
		return
	}
	if busSpansEnabled() && bus.spanBulkEligible(addr, uint64(len(dst))) {
		if addr+uint64(len(dst)) <= uint64(len(bus.memory)) {
			copy(dst, bus.memory[addr:addr+uint64(len(dst))])
			return
		}
		bus.backing.ReadBytes(addr, dst)
		return
	}
	for i := range dst {
		dst[i] = bus.ReadPhys8(addr + uint64(i))
	}
}

// WriteSpan stores src into guest memory starting at the physical address addr,
// equivalently to calling WritePhys8 for each byte.
//
// The bulk path publishes the bytes before it queues the JIT invalidation, for
// the reason WriteGuestBytes documents: invalidating first would let a live
// dispatcher drain the invalidation, miss the cache and recompile the old bytes
// in the gap, leaving a stale block with nothing left to invalidate it.
func (bus *MachineBus) WriteSpan(addr uint64, src []byte) {
	if bus == nil || len(src) == 0 {
		return
	}
	if busSpansEnabled() && bus.spanBulkEligible(addr, uint64(len(src))) {
		length := uint64(len(src))
		locked := bus.beginM68KJITRAMWrite(addr, length)
		if addr+length <= uint64(len(bus.memory)) {
			copy(bus.memory[addr:addr+length], src)
		} else {
			bus.backing.WriteBytes(addr, src)
		}
		bus.invalidateM68KJITRAMWrite(addr, length)
		bus.endM68KJITRAMWrite(locked)
		return
	}
	for i, v := range src {
		bus.WritePhys8(addr+uint64(i), v)
	}
}

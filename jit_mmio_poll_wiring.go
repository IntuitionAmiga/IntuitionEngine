// jit_mmio_poll_wiring.go - Phase 7f-bis exec-loop wiring.
//
// Phase 7f landed the shared TryFastMMIOPoll matcher and per-backend
// PollPattern descriptors with nil AddressIsMMIOPredicate placeholders.
// Phase 7f-bis (this file) populates those predicates from each backend's
// authoritative MMIO classifier at the point its JIT exec loop starts.
//
// Wiring is per-backend because the classifiers differ:
//
//   - x86  : per-page IOBitmap built at initX86JIT (translateIO + VGA
//            windows), most precise classifier.
//   - IE64 : MachineBus.IsIOAddress (mapped MMIO regions live above
//            IO_REGION_BASE; the bus knows which pages are mapped).
//   - M68K : Bus32 underlying *MachineBus — IsIOAddress when the bus
//            is a MachineBus, otherwise the static range fallback.
//   - Z80  : Z80BusAdapter.bus is *MachineBus.
//   - 6502 : Bus6502Adapter.bus is Bus32, type-assert to *MachineBus.
//
// Each backend's exec-entry calls its enable* helper once before the
// dispatch loop. The helpers are idempotent — calling twice rewrites the
// predicate to the latest captured bus, which is the desired behaviour
// when a CPU is reset and re-entered.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "sync/atomic"

// pollPredicateGen is bumped each time a backend's predicate is rewired
// so tests can verify wiring fired without inspecting closure identity.
var pollPredicateGen [5]atomic.Uint64

const (
	pollGenX86 = iota
	pollGenIE64
	pollGenM68K
	pollGenZ80
	pollGen6502
)

// enableX86PollWiring populates X86PollPattern.AddressIsMMIOPredicate
// from the CPU's per-page IOBitmap. The bitmap is built at initX86JIT
// time so this must run after that.
func enableX86PollWiring(cpu *CPU_X86) {
	bitmap := cpu.x86JitIOBitmap
	X86PollPattern.AddressIsMMIOPredicate = func(addr uint32) bool {
		page := addr >> 8
		if page >= uint32(len(bitmap)) {
			// Out-of-range probes default to MMIO so a malformed
			// poll trace cannot accidentally match RAM.
			return true
		}
		return bitmap[page] != 0
	}
	pollPredicateGen[pollGenX86].Add(1)
}

// enableIE64PollWiring populates IE64PollPattern.AddressIsMMIOPredicate
// from the CPU's bus.
func enableIE64PollWiring(cpu *CPU64) {
	bus := cpu.bus
	if bus == nil {
		return
	}
	IE64PollPattern.AddressIsMMIOPredicate = func(addr uint32) bool {
		return bus.IsIOAddress(addr)
	}
	pollPredicateGen[pollGenIE64].Add(1)
}

// enableM68KPollWiring populates M68KPollPattern.AddressIsMMIOPredicate
// from the CPU's bus when the bus is a MachineBus; otherwise installs
// the static range predicate so the matcher still has a usable
// classifier under test buses.
func enableM68KPollWiring(cpu *M68KCPU) {
	if mb, ok := cpu.bus.(*MachineBus); ok {
		M68KPollPattern.AddressIsMMIOPredicate = func(addr uint32) bool {
			return mb.IsIOAddress(addr)
		}
	} else {
		M68KPollPattern.AddressIsMMIOPredicate = IsIOAddress
	}
	pollPredicateGen[pollGenM68K].Add(1)
}

// enableZ80PollWiring populates Z80PollPattern.AddressIsMMIOPredicate
// from the Z80BusAdapter's underlying MachineBus. Z80 polls usually
// target I/O ports rather than memory-mapped registers, but the matcher
// is still the right shape gate for memory-mapped MMIO that Z80
// emitters route through Read8.
func enableZ80PollWiring(adapter *Z80BusAdapter) {
	if adapter == nil || adapter.bus == nil {
		return
	}
	bus := adapter.bus
	Z80PollPattern.AddressIsMMIOPredicate = func(addr uint32) bool {
		return bus.IsIOAddress(addr)
	}
	pollPredicateGen[pollGenZ80].Add(1)
}

// enable6502PollWiring populates P65PollPattern.AddressIsMMIOPredicate
// from the Bus6502Adapter's underlying MachineBus. The adapter holds a
// Bus32 interface; type-assert to MachineBus and fall back to the
// static range predicate when the underlying bus is a test stub.
func enable6502PollWiring(adapter *Bus6502Adapter) {
	if adapter == nil {
		return
	}
	if mb, ok := adapter.bus.(*MachineBus); ok {
		P65PollPattern.AddressIsMMIOPredicate = func(addr uint32) bool {
			return mb.IsIOAddress(addr)
		}
	} else {
		P65PollPattern.AddressIsMMIOPredicate = IsIOAddress
	}
	pollPredicateGen[pollGen6502].Add(1)
}

// pollPredicateGenSnapshot returns the current wiring generation for
// the given backend slot. Tests use this to confirm a wiring helper
// actually ran.
func pollPredicateGenSnapshot(slot int) uint64 {
	if slot < 0 || slot >= len(pollPredicateGen) {
		return 0
	}
	return pollPredicateGen[slot].Load()
}

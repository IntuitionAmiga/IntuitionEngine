// machine_bus_span_fault_test.go - the span fast path may not widen guest reach.
//
// Call sites whose per-byte path used Read8WithFault and Write8WithFault carry
// an extra obligation on top of "the bytes come out the same": a buffer those
// helpers would have refused must keep being refused. Both of them only ever
// touch bus.memory and reject every address at or above its end, so on a
// machine with a backing bound the general bulk eligibility test is more
// permissive than they are, and a GEMDOS buffer above the legacy window would
// silently succeed against backing memory instead of returning GEMDOS_EIMBA.
//
// The oracle here is the fault helpers themselves.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"math/rand"
	"testing"
)

// faultSpanTestBus returns a bus with a backing bound well above bus.memory,
// which is the configuration that exposes the difference. The 32 MiB legacy
// slice is the test rig's; the backing stands in for autodetected guest RAM.
func faultSpanTestBus() *MachineBus {
	bus := NewMachineBus()
	bus.SetBacking(NewSparseBacking(64 << 20))
	return bus
}

// byteLoopWriteFaultOK reports whether the per-byte fault-checked write path
// would have accepted every byte of the span.
func byteLoopWriteFaultOK(bus *MachineBus, addr uint32, count int) bool {
	for i := range count {
		if !bus.Write8WithFault(addr+uint32(i), 0) {
			return false
		}
	}
	return true
}

// byteLoopReadFaultOK is the read-side oracle.
func byteLoopReadFaultOK(bus *MachineBus, addr uint32, count int) bool {
	for i := range count {
		if _, ok := bus.Read8WithFault(addr + uint32(i)); !ok {
			return false
		}
	}
	return true
}

// TestGemdosSpan_RejectsBufferAboveLegacyMemory is the targeted regression. A
// buffer sitting above bus.memory but inside the backing is exactly what the
// fault helpers refuse and what the general bulk test accepts.
func TestGemdosSpan_RejectsBufferAboveLegacyMemory(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	bus := faultSpanTestBus()
	above := uint32(len(bus.memory)) + 0x1000
	const count = 0x200

	if byteLoopWriteFaultOK(bus, above, count) {
		t.Fatal("the fault-checked write path accepted a buffer above bus.memory; the premise of this test is wrong")
	}
	if byteLoopReadFaultOK(bus, above, count) {
		t.Fatal("the fault-checked read path accepted a buffer above bus.memory; the premise of this test is wrong")
	}
	if !bus.spanBulkEligible(uint64(above), count) {
		t.Fatal("the general bulk test rejected a backing span; the premise of this test is wrong")
	}

	interceptor := &GemdosInterceptor{bus: bus}
	if interceptor.spanEligible(above, count) {
		t.Fatal("GEMDOS would take the bulk path for a buffer the fault-checked helpers reject, " +
			"so Fread and Fwrite would reach backing memory instead of returning GEMDOS_EIMBA")
	}

	// A buffer wholly inside the legacy window is still allowed the fast path,
	// or the fix would have been to disable it.
	if !interceptor.spanEligible(0x100000, count) {
		t.Fatal("a buffer inside bus.memory was refused the bulk path")
	}
}

// TestGemdosSpan_RejectsStrictMMIOWindow covers the second way the general test
// is more permissive: the fault helpers fault on an unmapped address inside a
// strict MMIO window, while the bulk test only rejects pages that have a region
// mapped over them.
func TestGemdosSpan_RejectsStrictMMIOWindow(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	bus := NewMachineBus()
	const windowStart, windowEnd = 0x200000, 0x2000FF
	bus.SetStrictMMIOWindows([]AddrRange{{Start: windowStart, End: windowEnd}})

	if byteLoopWriteFaultOK(bus, windowStart, 0x40) {
		t.Fatal("the fault-checked write path accepted an unmapped strict-window address; the premise of this test is wrong")
	}
	if !bus.spanBulkEligible(windowStart, 0x40) {
		t.Fatal("the general bulk test rejected an unmapped strict-window span; the premise of this test is wrong")
	}

	interceptor := &GemdosInterceptor{bus: bus}
	if interceptor.spanEligible(windowStart, 0x40) {
		t.Fatal("GEMDOS would take the bulk path across a strict MMIO window the fault-checked helpers refuse")
	}
}

// TestGemdosSpan_NeverWiderThanFaultHelpers is the general property, over
// randomised spans against both bus shapes: whenever the fast path is taken,
// every byte of it would have been accepted by the helpers it replaces.
func TestGemdosSpan_NeverWiderThanFaultHelpers(t *testing.T) {
	restore := setBusSpansEnabled(true)
	defer restore()

	rng := rand.New(rand.NewSource(4242))
	shapes := map[string]func() *MachineBus{
		"WithBacking": faultSpanTestBus,
		"LegacyOnly":  func() *MachineBus { return NewMachineBus() },
		"StrictWindow": func() *MachineBus {
			bus := faultSpanTestBus()
			bus.SetStrictMMIOWindows([]AddrRange{{Start: 0x300000, End: 0x3000FF}})
			return bus
		},
	}

	for name, build := range shapes {
		t.Run(name, func(t *testing.T) {
			bus := build()
			bus.MapIO(0x400000, 0x4000FF, func(uint32) uint32 { return 0 }, func(uint32, uint32) {})
			interceptor := &GemdosInterceptor{bus: bus}

			legacy := uint32(len(bus.memory))
			eligible := 0
			for range 3000 {
				// Draw addresses that straddle the interesting boundaries:
				// the end of bus.memory, the strict window and the mapping.
				var addr uint32
				switch rng.Intn(4) {
				case 0:
					addr = uint32(rng.Intn(int(legacy) + 0x4000))
				case 1:
					addr = legacy - uint32(rng.Intn(0x400)) + uint32(rng.Intn(0x400))
				case 2:
					addr = 0x300000 - uint32(rng.Intn(0x200)) + uint32(rng.Intn(0x200))
				default:
					addr = 0x400000 - uint32(rng.Intn(0x200)) + uint32(rng.Intn(0x200))
				}
				count := 1 + rng.Intn(0x180)

				if !interceptor.spanEligible(addr, count) {
					continue
				}
				eligible++
				if !byteLoopWriteFaultOK(bus, addr, count) {
					t.Fatalf("span at 0x%X len %d took the bulk path, but the fault-checked write path rejects it", addr, count)
				}
				if !byteLoopReadFaultOK(bus, addr, count) {
					t.Fatalf("span at 0x%X len %d took the bulk path, but the fault-checked read path rejects it", addr, count)
				}
			}
			if eligible == 0 {
				t.Fatal("no span was ever eligible, so this test proved nothing")
			}
		})
	}
}

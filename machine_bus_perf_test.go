// machine_bus_perf_test.go - measurement benchmarks for the memory and bus tranche.
//
// These benchmarks exist to decide whether an optimisation is worth making, not
// to prove one already made. Tranche 3 gates two items on numbers taken here:
// item 9 (a per-page capability pre-filter) is only built if the debug-activity
// load and the unsealed map load cost something measurable on the read path, and
// item 10 (bulk span APIs) is only built where the per-byte loop is a material
// share of the operation it sits in.
//
// (c) 2024 - 2026 Zayn Otley
// https://github.com/IntuitionAmiga/IntuitionEngine
// License: GPLv3 or later

package main

import (
	"testing"
)

// benchBusWithDebug returns a bus whose debug access service is attached, and
// active when watch is true. An attached but idle service is the normal state
// for a machine that has ever opened the monitor, so it is measured separately
// from having no service at all.
func benchBusWithDebug(watch bool) *MachineBus {
	bus := NewMachineBus()
	access := NewDebugAccessService()
	bus.SetDebugAccessService(access)
	if watch {
		// A watchpoint on an address the benchmark never touches: the pre-filter
		// question is what an access pays to discover that it is not watched.
		access.Watch(0, 0x7F000, 1, WatchWrite)
	}
	return bus
}

// BenchmarkRead32_NonIO_Variants measures the non-I/O read path across the four
// states item 9 cares about: no debug service, an attached idle service, an
// attached active service, and the map sealed versus unsealed.
func BenchmarkRead32_NonIO_Variants(b *testing.B) {
	cases := []struct {
		name   string
		build  func() *MachineBus
		sealed bool
	}{
		{name: "NoDebug/Unsealed", build: func() *MachineBus { return NewMachineBus() }},
		{name: "NoDebug/Sealed", build: func() *MachineBus { return NewMachineBus() }, sealed: true},
		{name: "DebugIdle/Unsealed", build: func() *MachineBus { return benchBusWithDebug(false) }},
		{name: "DebugIdle/Sealed", build: func() *MachineBus { return benchBusWithDebug(false) }, sealed: true},
		{name: "DebugActive/Unsealed", build: func() *MachineBus { return benchBusWithDebug(true) }},
		{name: "DebugActive/Sealed", build: func() *MachineBus { return benchBusWithDebug(true) }, sealed: true},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			bus := tc.build()
			bus.Write32(0x1000, 0x12345678)
			if tc.sealed {
				bus.SealMappings()
			}
			b.ResetTimer()
			for range b.N {
				_ = bus.Read32(0x1000)
			}
		})
	}
}

// BenchmarkWrite32_NonIO_Variants is the write-side counterpart. Writes carry
// the old-value read that the debug path needs, so the active case is expected
// to cost more here than on the read side.
func BenchmarkWrite32_NonIO_Variants(b *testing.B) {
	cases := []struct {
		name   string
		build  func() *MachineBus
		sealed bool
	}{
		{name: "NoDebug/Unsealed", build: func() *MachineBus { return NewMachineBus() }},
		{name: "NoDebug/Sealed", build: func() *MachineBus { return NewMachineBus() }, sealed: true},
		{name: "DebugIdle/Sealed", build: func() *MachineBus { return benchBusWithDebug(false) }, sealed: true},
		{name: "DebugActive/Sealed", build: func() *MachineBus { return benchBusWithDebug(true) }, sealed: true},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			bus := tc.build()
			if tc.sealed {
				bus.SealMappings()
			}
			b.ResetTimer()
			for i := range b.N {
				bus.Write32(0x1000, uint32(i))
			}
		})
	}
}

// BenchmarkDebugAccessActive isolates the activity test itself, so its cost can
// be compared against the whole access it guards rather than inferred from the
// difference between two noisy end-to-end numbers.
func BenchmarkDebugAccessActive(b *testing.B) {
	b.Run("Attached", func(b *testing.B) {
		bus := benchBusWithDebug(false)
		b.ResetTimer()
		for range b.N {
			if bus.debugAccessActive() {
				b.Fatal("service should be idle")
			}
		}
	})
	b.Run("Detached", func(b *testing.B) {
		bus := NewMachineBus()
		b.ResetTimer()
		for range b.N {
			if bus.debugAccessActive() {
				b.Fatal("no service attached")
			}
		}
	})
}

// BenchmarkCurrentMapSnapshot isolates the map lookup that every sized access
// makes, sealed against unsealed. Sealed reads a plain field; unsealed reads an
// atomic pointer.
func BenchmarkCurrentMapSnapshot(b *testing.B) {
	for _, sealed := range []bool{false, true} {
		name := "Unsealed"
		if sealed {
			name = "Sealed"
		}
		b.Run(name, func(b *testing.B) {
			bus := NewMachineBus()
			if sealed {
				bus.SealMappings()
			}
			b.ResetTimer()
			for range b.N {
				if bus.currentMapSnapshot() == nil {
					b.Fatal("nil snapshot")
				}
			}
		})
	}
}

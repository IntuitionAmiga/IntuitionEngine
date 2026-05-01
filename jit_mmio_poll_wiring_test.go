// jit_mmio_poll_wiring_test.go - Phase 7f-bis wiring gates.
//
// Each enable*PollWiring helper must populate its backend's PollPattern
// with a non-nil predicate that classifies the IO_REGION as MMIO and a
// known-RAM address as non-MMIO. Tests construct a MachineBus with the
// bus-driven sizing path and exercise the helpers directly so wiring is
// verified independent of the per-backend exec loop machinery.

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && (linux || windows || darwin))

package main

import "testing"

func newWiringBus(t *testing.T) *MachineBus {
	t.Helper()
	bus := NewMachineBus()
	bus.MapIO(IO_REGION_BASE, IO_REGION_BASE+0x100,
		func(addr uint32) uint32 { return 0 },
		func(addr uint32, value uint32) {})
	bus.SealMappings()
	return bus
}

func TestPollWiring_X86PopulatesPredicate(t *testing.T) {
	X86PollPattern.AddressIsMMIOPredicate = nil
	gen := pollPredicateGenSnapshot(pollGenX86)

	cpu := &CPU_X86{}
	cpu.x86JitIOBitmap = make([]byte, 256)
	mmioPage := uint32(IO_REGION_BASE >> 8)
	if mmioPage < uint32(len(cpu.x86JitIOBitmap)) {
		cpu.x86JitIOBitmap[mmioPage] = 1
	} else {
		cpu.x86JitIOBitmap = make([]byte, mmioPage+1)
		cpu.x86JitIOBitmap[mmioPage] = 1
	}
	enableX86PollWiring(cpu)

	if X86PollPattern.AddressIsMMIOPredicate == nil {
		t.Fatal("x86 wiring left predicate nil")
	}
	if !X86PollPattern.AddressIsMMIOPredicate(IO_REGION_BASE) {
		t.Errorf("x86 predicate failed to classify IO_REGION_BASE as MMIO")
	}
	if X86PollPattern.AddressIsMMIOPredicate(0x100) {
		t.Errorf("x86 predicate misclassified low RAM as MMIO")
	}
	if pollPredicateGenSnapshot(pollGenX86) <= gen {
		t.Errorf("x86 wiring did not bump generation counter")
	}
}

func TestPollWiring_IE64PopulatesPredicate(t *testing.T) {
	IE64PollPattern.AddressIsMMIOPredicate = nil
	gen := pollPredicateGenSnapshot(pollGenIE64)

	bus := newWiringBus(t)
	cpu := &CPU64{bus: bus}
	enableIE64PollWiring(cpu)

	if IE64PollPattern.AddressIsMMIOPredicate == nil {
		t.Fatal("IE64 wiring left predicate nil")
	}
	if !IE64PollPattern.AddressIsMMIOPredicate(IO_REGION_BASE) {
		t.Errorf("IE64 predicate failed to classify IO_REGION_BASE as MMIO")
	}
	if pollPredicateGenSnapshot(pollGenIE64) <= gen {
		t.Errorf("IE64 wiring did not bump generation counter")
	}
}

func TestPollWiring_M68KPopulatesPredicate(t *testing.T) {
	M68KPollPattern.AddressIsMMIOPredicate = nil
	gen := pollPredicateGenSnapshot(pollGenM68K)

	bus := newWiringBus(t)
	cpu := &M68KCPU{bus: bus}
	enableM68KPollWiring(cpu)

	if M68KPollPattern.AddressIsMMIOPredicate == nil {
		t.Fatal("M68K wiring left predicate nil")
	}
	if !M68KPollPattern.AddressIsMMIOPredicate(IO_REGION_BASE) {
		t.Errorf("M68K predicate failed to classify IO_REGION_BASE as MMIO")
	}
	if pollPredicateGenSnapshot(pollGenM68K) <= gen {
		t.Errorf("M68K wiring did not bump generation counter")
	}
}

func TestPollWiring_Z80PopulatesPredicate(t *testing.T) {
	Z80PollPattern.AddressIsMMIOPredicate = nil
	gen := pollPredicateGenSnapshot(pollGenZ80)

	bus := newWiringBus(t)
	adapter := &Z80BusAdapter{bus: bus}
	enableZ80PollWiring(adapter)

	if Z80PollPattern.AddressIsMMIOPredicate == nil {
		t.Fatal("Z80 wiring left predicate nil")
	}
	if !Z80PollPattern.AddressIsMMIOPredicate(IO_REGION_BASE) {
		t.Errorf("Z80 predicate failed to classify IO_REGION_BASE as MMIO")
	}
	if pollPredicateGenSnapshot(pollGenZ80) <= gen {
		t.Errorf("Z80 wiring did not bump generation counter")
	}
}

func TestPollWiring_6502PopulatesPredicate(t *testing.T) {
	P65PollPattern.AddressIsMMIOPredicate = nil
	gen := pollPredicateGenSnapshot(pollGen6502)

	bus := newWiringBus(t)
	adapter := &Bus6502Adapter{bus: bus}
	enable6502PollWiring(adapter)

	if P65PollPattern.AddressIsMMIOPredicate == nil {
		t.Fatal("6502 wiring left predicate nil")
	}
	if !P65PollPattern.AddressIsMMIOPredicate(IO_REGION_BASE) {
		t.Errorf("6502 predicate failed to classify IO_REGION_BASE as MMIO")
	}
	if pollPredicateGenSnapshot(pollGen6502) <= gen {
		t.Errorf("6502 wiring did not bump generation counter")
	}
}

func TestPollWiring_AllRegistryPredicatesPopulatedAfterWiring(t *testing.T) {
	bus := newWiringBus(t)

	cpuX := &CPU_X86{x86JitIOBitmap: make([]byte, 256)}
	enableX86PollWiring(cpuX)
	enableIE64PollWiring(&CPU64{bus: bus})
	enableM68KPollWiring(&M68KCPU{bus: bus})
	enableZ80PollWiring(&Z80BusAdapter{bus: bus})
	enable6502PollWiring(&Bus6502Adapter{bus: bus})

	for tag, pat := range BackendPollPatterns {
		if pat.AddressIsMMIOPredicate == nil {
			t.Errorf("backend %q predicate nil after wiring", tag)
		}
	}
}

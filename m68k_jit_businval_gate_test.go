//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// Cross-thread bus writes (coprocessor workers, DMA, video) must not enqueue
// M68K JIT invalidations for regions containing no compiled code: each enqueue
// bumps the invalidation generation, and the dispatcher re-loops whenever the
// generation moves between block lookup and native entry, so a worker
// streaming data into guest RAM can livelock the dispatcher. The bus
// invalidator applies the same O(1) code-envelope reject as the CPU's own
// write path, through an atomically published envelope.

func newBusInvalGateRig(t *testing.T) (*MachineBus, *M68KCPU) {
	t.Helper()
	bus := NewMachineBus()
	cpu := NewM68KCPU(bus)
	// Route enqueues (not synchronous invalidation) as during dispatch.
	cpu.m68kJitDispatchActive.Store(true)
	return bus, cpu
}

func TestM68KBusInvalidator_SkipsWithNoCompiledCode(t *testing.T) {
	bus, cpu := newBusInvalGateRig(t)

	gen := cpu.m68kJitInvalGen.Load()
	bus.invalidateM68KJITRAMWrite(0x500000, 64)

	if got := cpu.m68kJitInvalGen.Load(); got != gen {
		t.Fatalf("data-only bus write bumped inval generation (%d -> %d) with an empty code envelope", gen, got)
	}
}

func TestM68KBusInvalidator_SkipsOutsideCodeEnvelope(t *testing.T) {
	bus, cpu := newBusInvalGateRig(t)
	cpu.m68kMarkJITCodeRanges(&JITBlock{startPC: 0x1000, endPC: 0x1100})

	gen := cpu.m68kJitInvalGen.Load()
	bus.invalidateM68KJITRAMWrite(0x10000000, 4096) // far above any code

	if got := cpu.m68kJitInvalGen.Load(); got != gen {
		t.Fatalf("write far outside the code envelope bumped inval generation (%d -> %d)", gen, got)
	}
}

func TestM68KBusInvalidator_StillEnqueuesForCodeWrites(t *testing.T) {
	bus, cpu := newBusInvalGateRig(t)
	cpu.m68kMarkJITCodeRanges(&JITBlock{startPC: 0x1000, endPC: 0x1100})

	gen := cpu.m68kJitInvalGen.Load()
	bus.invalidateM68KJITRAMWrite(0x1040, 4) // inside compiled code

	if got := cpu.m68kJitInvalGen.Load(); got == gen {
		t.Fatal("write inside the code envelope must still enqueue an invalidation")
	}
}

func TestM68KBusInvalidator_EnvelopeResetsWithCache(t *testing.T) {
	bus, cpu := newBusInvalGateRig(t)
	cpu.m68kMarkJITCodeRanges(&JITBlock{startPC: 0x1000, endPC: 0x1100})
	cpu.m68kResetJITCodeCache()

	gen := cpu.m68kJitInvalGen.Load()
	bus.invalidateM68KJITRAMWrite(0x1040, 4)

	if got := cpu.m68kJitInvalGen.Load(); got != gen {
		t.Fatalf("post-reset write bumped inval generation (%d -> %d); envelope should be empty again", gen, got)
	}
}

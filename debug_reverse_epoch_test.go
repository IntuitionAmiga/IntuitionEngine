package main

import (
	"testing"
)

// TestReverseHistory_UsesPageEpochs_StateMatchesLegacy drives an identical write
// sequence through a legacy monitor and an epoch-driven monitor and asserts that
// the reconstructed final state is byte-identical. The epoch delta copies only
// the pages the cursor reports dirty; if that set ever missed a written page the
// materialised state would diverge here.
func TestReverseHistory_UsesPageEpochs_StateMatchesLegacy(t *testing.T) {
	newRig := func(epoch bool) (*MachineMonitor, *CPU64, *MachineBus) {
		bus, err := NewMachineBusSized(uint64(DEFAULT_MEMORY_SIZE))
		if err != nil {
			t.Fatal(err)
		}
		cpu := NewCPU64(bus)
		mon := NewMachineMonitor(bus)
		if epoch {
			mon.epochHistory = true
			mon.enableEpochHistoryLocked()
		}
		mon.wholeCheckpointInterval = 4 // exercise both deltas and rebaselining checkpoints
		mon.RegisterCPU("ie64", NewDebugIE64(cpu))
		return mon, cpu, bus
	}

	monL, cpuL, busL := newRig(false)
	monE, cpuE, busE := newRig(true)

	if !monE.busEpochCursor.Active() {
		t.Fatal("epoch cursor not active")
	}

	// A schedule that touches several pages, rewrites some, and zeroes one back
	// so the zero-page (delete) path is exercised.
	type write struct{ addr, val uint32 }
	steps := [][]write{
		{{0x1000, 0xDEADBEEF}, {0x9000, 0x11111111}},
		{{0x1000, 0xCAFEBABE}, {0x40000, 0x22222222}},
		{{0x9000, 0}, {0x80000, 0x33333333}},
		{{0x1004, 0x44444444}},
		{{0x40000, 0x55555555}, {0x120000, 0x66666666}},
		{{0x80000, 0}, {0x1000, 0x77777777}},
		{{0x200000, 0x88888888}},
		{{0x120000, 0x99999999}, {0x1008, 0xAAAAAAAA}},
	}

	apply := func(bus *MachineBus, cpu *CPU64, ws []write, pc uint64) {
		for _, w := range ws {
			bus.Write32(w.addr, w.val)
		}
		cpu.PC = pc
		cpu.regs[1] = pc ^ 0x5555
	}

	for i, ws := range steps {
		pc := uint64(0x2000 + i*0x10)
		apply(busL, cpuL, ws, pc)
		monL.recordWholeMachineHistory()
		apply(busE, cpuE, ws, pc)
		monE.recordWholeMachineHistory()
	}

	lastL, err := monL.materializeWholeMachineSnapshotLocked(monL.wholeHistory[len(monL.wholeHistory)-1])
	if err != nil {
		t.Fatalf("materialise legacy: %v", err)
	}
	lastE, err := monE.materializeWholeMachineSnapshotLocked(monE.wholeHistory[len(monE.wholeHistory)-1])
	if err != nil {
		t.Fatalf("materialise epoch: %v", err)
	}
	if !wholeSnapshotsEquivalent(lastL, lastE) {
		t.Fatalf("epoch reconstruction diverged from legacy:\n legacy bus pages=%d\n epoch  bus pages=%d",
			len(lastL.Bus.Pages), len(lastE.Bus.Pages))
	}

	// The reconstruction must also match the live machine, not just the other
	// history, so a shared bug in both paths cannot hide.
	live, err := monE.takeWholeMachineSnapshotLocked()
	if err != nil {
		t.Fatalf("live snapshot: %v", err)
	}
	if !wholeSnapshotsEquivalent(lastE, live) {
		t.Fatal("epoch reconstruction diverged from the live machine state")
	}
}

// TestMonitorDisabled_ZeroAllocsSteadyState pins the disabled monitor hook
// boundary at zero allocations. With no guard, watchpoint or history armed the
// access service must fall out on one predictable branch and allocate nothing.
func TestMonitorDisabled_ZeroAllocsSteadyState(t *testing.T) {
	s := NewDebugAccessService() // nothing armed, so active stays false

	allocs := testing.AllocsPerRun(1000, func() {
		s.OnRead(0, 0x1000, 4)
		s.OnWrite(0, 0x2000, 4, 0, 0xFFFF)
		s.OnFetch(0, 0x3000, 4)
	})
	if allocs != 0 {
		t.Fatalf("disabled monitor hooks allocate %.0f times per run, want 0", allocs)
	}
	if s.AnyActive(0) {
		t.Fatal("service reports active with nothing armed")
	}
}

// BenchmarkMonitorHooks_Disabled measures the disabled hook overhead: the cost a
// running guest pays for the monitor being compiled in but switched off.
func BenchmarkMonitorHooks_Disabled(b *testing.B) {
	s := NewDebugAccessService()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.OnWrite(0, 0x2000, 4, 0, uint64(i))
	}
}

// TestReverseHistory_EpochResyncsAfterRestore covers the restore hazard: a
// reverse restore rewrites bus memory directly, bypassing the dirty cursor, so
// the next epoch capture must fall back to a full scan rather than trust a stale
// dirty set. Without the resync the delta would keep pages the restore removed.
func TestReverseHistory_EpochResyncsAfterRestore(t *testing.T) {
	bus, err := NewMachineBusSized(uint64(DEFAULT_MEMORY_SIZE))
	if err != nil {
		t.Fatal(err)
	}
	cpu := NewCPU64(bus)
	mon := NewMachineMonitor(bus)
	mon.epochHistory = true
	mon.enableEpochHistoryLocked()
	mon.RegisterCPU("ie64", NewDebugIE64(cpu))

	bus.Write32(0x1000, 0xAAAA)
	mon.recordWholeMachineHistory()
	bus.Write32(0x2000, 0xBBBB)
	mon.recordWholeMachineHistory()

	// Restore the oldest state (only 0x1000 written), rewriting bus memory
	// directly and dropping 0x2000.
	old, err := mon.materializeWholeMachineSnapshotLocked(mon.wholeHistory[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := mon.restoreWholeMachineSnapshotLocked(old); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if !mon.epochForceFull {
		t.Fatal("restore did not force a full epoch capture")
	}

	// A fresh write, then capture: the materialised snapshot must equal the live
	// machine, which no longer holds 0x2000.
	bus.Write32(0x3000, 0xCCCC)
	mon.recordWholeMachineHistory()

	latest, err := mon.materializeWholeMachineSnapshotLocked(mon.wholeHistory[len(mon.wholeHistory)-1])
	if err != nil {
		t.Fatal(err)
	}
	live, err := mon.takeWholeMachineSnapshotLocked()
	if err != nil {
		t.Fatal(err)
	}
	if !wholeSnapshotsEquivalent(latest, live) {
		t.Fatal("epoch history diverged from live state after a reverse restore")
	}
}

//go:build headless

package main

// Phase 3: capability / version / selected-instance / worker-window discovery
// registers in the EXT2 block (0xF23C0..0xF23DF).

import "testing"

func TestCoprocRegs_CapabilityReporting(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	cases := []struct {
		cpuType uint32
		limit   uint32
	}{
		{EXEC_TYPE_M68K, 2}, {EXEC_TYPE_X86, 2}, {EXEC_TYPE_IE64, 2},
		{EXEC_TYPE_IE32, 1}, {EXEC_TYPE_6502, 1}, {EXEC_TYPE_Z80, 1},
	}
	for _, c := range cases {
		bus.Write32(COPROC_CPU_TYPE, c.cpuType)
		if got := bus.Read32(COPROC_INSTANCE_LIMIT); got != c.limit {
			t.Errorf("COPROC_INSTANCE_LIMIT for type %d = %d, want %d", c.cpuType, got, c.limit)
		}
	}
	if got := bus.Read32(COPROC_MAILBOX_VERSION); got != COPROC_LAYOUT_VERSION {
		t.Errorf("COPROC_MAILBOX_VERSION = %d, want %d", got, COPROC_LAYOUT_VERSION)
	}
}

func TestCoprocRegs_WorkerWindowDiscovery(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	for _, inst := range []uint32{0, 1} {
		bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
		bus.Write32(COPROC_INSTANCE, inst)
		wantBase, wantEnd, _, _ := workerWindow(EXEC_TYPE_M68K, inst)
		wantRing := ringBaseAddr(coprocRingIndex(EXEC_TYPE_M68K, inst))
		if got := bus.Read32(COPROC_WORKER_BASE); got != wantBase {
			t.Errorf("M68K#%d COPROC_WORKER_BASE = %#x, want %#x", inst, got, wantBase)
		}
		if got := bus.Read32(COPROC_WORKER_END); got != wantEnd {
			t.Errorf("M68K#%d COPROC_WORKER_END = %#x, want %#x", inst, got, wantEnd)
		}
		if got := bus.Read32(COPROC_WORKER_RING); got != wantRing {
			t.Errorf("M68K#%d COPROC_WORKER_RING = %#x, want %#x", inst, got, wantRing)
		}
	}

	// A type with no instance 1 reports zero for the window.
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_IE32)
	bus.Write32(COPROC_INSTANCE, 1)
	if got := bus.Read32(COPROC_WORKER_BASE); got != 0 {
		t.Errorf("IE32#1 COPROC_WORKER_BASE = %#x, want 0", got)
	}
}

func TestCoprocRegs_SelectedState(t *testing.T) {
	bus, mgr := newTestBusAndManagerExt(t)
	defer mgr.StopAll()

	startM68KInstanceViaMMIO(t, bus, mgr, 1)

	// Instance 1 selected: online.
	bus.Write32(COPROC_CPU_TYPE, EXEC_TYPE_M68K)
	bus.Write32(COPROC_INSTANCE, 1)
	if got := bus.Read32(COPROC_SELECTED_STATE); got != 1 {
		t.Errorf("selected state for running M68K#1 = %d, want 1", got)
	}
	// Instance 0 selected: offline.
	bus.Write32(COPROC_INSTANCE, 0)
	if got := bus.Read32(COPROC_SELECTED_STATE); got != 0 {
		t.Errorf("selected state for idle M68K#0 = %d, want 0", got)
	}
}

// TestCoprocEXT2_MMIOMap asserts the new block is aligned and sits in the free
// gap between the CPU Wait region and the SFX extended aliases, clear of the
// AROS IRQ diagnostics region.
func TestCoprocEXT2_MMIOMap(t *testing.T) {
	// Must not overlap the AROS IRQ diagnostics block (0xF23C0..0xF23DF).
	if COPROC_EXT2_BASE <= IRQ_DIAG_REGION_END && IRQ_DIAG_REGION_BASE <= COPROC_EXT2_END {
		t.Errorf("EXT2 %#x..%#x overlaps IRQ_DIAG %#x..%#x",
			COPROC_EXT2_BASE, COPROC_EXT2_END, IRQ_DIAG_REGION_BASE, IRQ_DIAG_REGION_END)
	}
	// Must sit above CPU Wait and below the SFX extended aliases (0xF2600).
	if COPROC_EXT2_BASE <= CPU_WAIT_REGION_END {
		t.Errorf("EXT2 base %#x overlaps CPU Wait end %#x", COPROC_EXT2_BASE, CPU_WAIT_REGION_END)
	}
	if COPROC_EXT2_END >= IE_SFX_EXT_REGION_BASE {
		t.Errorf("EXT2 end %#x overlaps SFX extended base %#x", COPROC_EXT2_END, IE_SFX_EXT_REGION_BASE)
	}
	regs := []uint32{
		COPROC_INSTANCE_LIMIT, COPROC_SELECTED_STATE, COPROC_MAILBOX_VERSION,
		COPROC_WORKER_BASE, COPROC_WORKER_END, COPROC_WORKER_RING,
	}
	for _, r := range regs {
		if r < COPROC_EXT2_BASE || r > COPROC_EXT2_END {
			t.Errorf("register %#x outside EXT2 block", r)
		}
		if r%4 != 0 {
			t.Errorf("register %#x not 4-byte aligned", r)
		}
	}
}
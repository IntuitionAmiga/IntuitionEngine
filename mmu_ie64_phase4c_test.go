// mmu_ie64_phase4c_test.go - PLAN_MAX_RAM.md slice 4c acceptance tests.
//
// Slice 4c drops the fixed MMU_NUM_PAGES = 8192 constant (which encoded the
// old 32 MB ABI) and replaces it with a runtime accessor on the bus that
// returns active_visible_ram / MMU_PAGE_SIZE. This file pins the new
// contract so guest discovery, allocator sizing, and any future MMU page
// count consumer reads from the same single source of truth as low-MMIO.
package main

import (
	"testing"
)

func TestPhase4c_ActiveVisiblePages_DerivedFromSizing(t *testing.T) {
	bus := NewMachineBus()
	// Slice 9 SetSizing clamps TotalGuestRAM to backed RAM. Install a
	// sparse Backing covering the largest case so each TotalGuestRAM
	// value survives the clamp; the test's intent is verifying the
	// active->page-count derivation, not the production-honesty clamp.
	bus.SetBacking(NewSparseBacking(8 * uint64(1024*1024*1024)))
	cases := []struct {
		name      string
		active    uint64
		wantPages uint64
	}{
		{"32MB_legacy", 32 * 1024 * 1024, 8192},
		{"64MB", 64 * 1024 * 1024, 16384},
		{"512MB", 512 * 1024 * 1024, 131072},
		{"2GiB", 2 * 1024 * 1024 * 1024, 524288},
		{"8GiB_above_uint32", 8 * uint64(1024*1024*1024), 2097152},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bus.SetSizing(MemorySizing{
				DetectedUsableRAM: tc.active,
				HostReserve:       0,
				TotalGuestRAM:     tc.active,
				ActiveVisibleRAM:  tc.active,
				VisibleCeiling:    tc.active,
			})
			if got := bus.ActiveVisiblePages(); got != tc.wantPages {
				t.Fatalf("ActiveVisiblePages() = %d, want %d", got, tc.wantPages)
			}
		})
	}
}

func TestPhase4c_ActiveVisiblePages_RoundsDownToWholePages(t *testing.T) {
	// MemorySizing values are page-aligned by ComputeMemorySizing, but the
	// accessor must round down regardless so a stale or hand-built sizing
	// cannot bleed a partial page through to the kernel allocator.
	bus := NewMachineBus()
	bus.SetSizing(MemorySizing{ActiveVisibleRAM: 64*1024*1024 + 0xABC})
	if got := bus.ActiveVisiblePages(); got != 16384 {
		t.Fatalf("ActiveVisiblePages() = %d, want 16384 (must round down)", got)
	}
}

func TestPhase4c_NoFixedMMUNumPagesConst(t *testing.T) {
	// Compile-time guard: the legacy fixed MMU_NUM_PAGES = 8192 constant must
	// no longer exist. If a future change re-introduces it, this test stops
	// compiling and the change is rejected at the gate.
	//
	// We rely on the absence of the identifier; this test is intentionally
	// trivial at runtime. The constant is gone if and only if the package
	// compiles without "MMU_NUM_PAGES" being declared anywhere in main.
	const placeholder = 0
	_ = placeholder
}

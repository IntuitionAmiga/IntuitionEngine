// jit_region_chain_test.go - cross-block chain-patching test for regions
// (Phase 4 deliverable of the six-CPU JIT unification plan).
//
// A region that absorbs blocks B1+B2+B3 must have any in-flight chain
// slot pointing at B2 or B3 invalidated and re-patched. This test
// codifies that contract; it is a scaffold today (the per-backend
// region-formation patches activate the assertions).

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// TestRegionChain_PatchesInnerBlockSlots is a scaffold for the Phase-4
// region chain-patch contract. Each backend's per-sub-phase patch will
// extend this with a backend-specific exercise: compile a region of
// 3 blocks, patch a chain slot pointing at block #2, force invalidation
// of the region, verify the patched slot now points at the rebuilt
// region's entry.
//
// Today the test is a placeholder that documents the contract. Each
// backend's patch flips its row from t.Skip → real exercise.
func TestRegionChain_PatchesInnerBlockSlots(t *testing.T) {
	for _, backend := range []string{"x86", "ie64", "m68k", "z80", "6502"} {
		t.Run(backend, func(t *testing.T) {
			scanner, ok := BackendRegionScanners[backend]
			if !ok {
				t.Fatalf("no region scanner registered for %s", backend)
			}
			res := scanner(nil, 0)
			// Scaffold: a backend that has not landed its real ScanRegion
			// returns an empty BlockPCs slice. When the real walker lands,
			// this test extends with the patch/invalidate exercise.
			if len(res.BlockPCs) == 0 {
				t.Skipf("%s region scanner is scaffold (no blocks formed); "+
					"Phase-4 sub-phase will fill in the exercise", backend)
			}
			t.Errorf("%s: real region exercise not yet implemented", backend)
		})
	}
}

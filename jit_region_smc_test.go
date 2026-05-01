// jit_region_smc_test.go - region-level self-modifying-code invalidation
// (Phase 4 deliverable of the six-CPU JIT unification plan).
//
// A guest STORE landing inside a region (when the region's source pages
// are marked dirty by the existing code-page bitmap) must invalidate the
// entire enclosing region, not just the single block whose PC contains
// the modified byte. Cases:
//
//   (a) STORE to first byte of a region's last block.
//   (b) STORE crossing a page boundary inside a region.
//   (c) STORE that modifies only an immediate operand inside an already-
//       compiled region.
//
// Scaffold today; per-backend region-formation patches activate.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

func TestRegionSMC_StoreInvalidatesEnclosingRegion(t *testing.T) {
	cases := []struct {
		name string
	}{
		{"FirstByteOfLastBlock"},
		{"CrossPageBoundary"},
		{"ImmediateOperandRewrite"},
	}
	for _, backend := range []string{"x86", "ie64", "m68k", "z80", "6502"} {
		for _, c := range cases {
			t.Run(backend+"/"+c.name, func(t *testing.T) {
				scanner, ok := BackendRegionScanners[backend]
				if !ok {
					t.Fatalf("no region scanner registered for %s", backend)
				}
				res := scanner(nil, 0)
				if len(res.BlockPCs) == 0 {
					t.Skipf("%s region scanner is scaffold; Phase-4 "+
						"sub-phase will fill in this exercise", backend)
				}
				t.Errorf("%s/%s: real SMC exercise not yet implemented",
					backend, c.name)
			})
		}
	}
}

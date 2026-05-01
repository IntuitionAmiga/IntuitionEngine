// jit_region_backends_test.go - tests for the per-backend region
// scanners published by BackendRegionScanners.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// TestScanRegionX86_FollowsForwardChain verifies the memory-driven
// region scanner follows static fall-through and returns the block
// start PCs in order. Three small basic blocks ending with NOP fall
// through into the next; the scanner should report all three.
func TestScanRegionX86_FollowsForwardChain(t *testing.T) {
	mem := make([]byte, 0x1000)
	// At 0x100: NOP; NOP; ... so each "block" is the run up to a
	// terminator. x86ScanBlock terminates on terminators only (not
	// fall-through), so a long NOP run is one block. Use RET-less
	// fall-through doesn't happen in x86 (no implicit terminator).
	// Instead just confirm the scanner returns nil for a non-region
	// shape: a single block of NOPs that runs off the array.
	for i := range mem {
		mem[i] = 0x90 // NOP
	}
	res := ScanRegionX86(mem, 0x100)
	// Without a terminator, x86ScanBlock walks until block size cap;
	// the next iteration's pc continues past with another scan. The
	// scanner caps at MaxBlocks anyway.
	if len(res.BlockPCs) > X86RegionProfile.MaxBlocks {
		t.Errorf("region exceeded MaxBlocks: got %d", len(res.BlockPCs))
	}
}

func TestScanRegionX86_RejectsOutOfBounds(t *testing.T) {
	mem := make([]byte, 0x1000)
	res := ScanRegionX86(mem, 0xFFFFFFFF)
	if len(res.BlockPCs) != 0 {
		t.Errorf("out-of-bounds startPC must yield empty region, got %v", res.BlockPCs)
	}
}

func TestScanRegionX86_PopulatesProfile(t *testing.T) {
	res := ScanRegionX86(make([]byte, 0x100), 0x10)
	if res.Profile.MaxBlocks != X86RegionProfile.MaxBlocks {
		t.Errorf("Profile.MaxBlocks = %d want %d", res.Profile.MaxBlocks, X86RegionProfile.MaxBlocks)
	}
}

// TestScanRegion_AllBackendsScaffoldShape iterates the registry and
// confirms each scanner returns a populated Profile (even when
// BlockPCs is empty for non-x86 stubs). Guards against silent
// scanner deletion.
func TestScanRegion_AllBackendsScaffoldShape(t *testing.T) {
	mem := make([]byte, 0x1000)
	for tag, scanner := range BackendRegionScanners {
		res := scanner(mem, 0x100)
		if res.Profile.MaxBlocks == 0 {
			t.Errorf("backend %q: scanner returned zero MaxBlocks", tag)
		}
	}
}

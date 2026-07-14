// jit_epilogue_dirty_amd64_test.go - Technique 1 (native exits): the amd64
// epilogue must store back only the resident IE64 registers that the block
// actually wrote, matching the ARM64 backend, rather than unconditionally
// spilling all five resident registers on every exit.

//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// epilogueLen emits a standalone epilogue for the given dirty-register mask and
// returns the byte length of the emitted sequence.
func epilogueLen(storeRegs uint32) int {
	cb := NewCodeBuffer(64)
	emitEpilogue(cb, storeRegs, 0)
	return cb.Len()
}

func TestAMD64_Epilogue_StoresOnlyDirtyResidentRegs(t *testing.T) {
	none := epilogueLen(0)
	all := epilogueLen((1 << 1) | (1 << 2) | (1 << 3) | (1 << 4) | (1 << 31))

	if none >= all {
		t.Fatalf("epilogue with no dirty regs (%d bytes) should be smaller than with all five resident regs dirty (%d bytes); stores are not gated on storeRegs", none, all)
	}

	// Each additional dirty resident register must add exactly one store, so
	// the length must grow monotonically as we add resident registers.
	prev := none
	mask := uint32(0)
	for _, r := range []uint{1, 2, 3, 4, 31} {
		mask |= 1 << r
		got := epilogueLen(mask)
		if got <= prev {
			t.Fatalf("adding resident R%d to dirty mask did not grow the epilogue (%d -> %d bytes)", r, prev, got)
		}
		prev = got
	}
	if prev != all {
		t.Fatalf("incremental mask length %d != full mask length %d", prev, all)
	}

	// A non-resident register (e.g. R5) is spilled inline at write time and
	// must not add any store to the epilogue.
	if got := epilogueLen(1 << 5); got != none {
		t.Fatalf("non-resident R5 in dirty mask changed epilogue length (%d != %d)", got, none)
	}
}

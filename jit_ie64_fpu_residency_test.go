//go:build amd64 && (linux || windows || darwin)

package main

import "testing"

// TestIE64ClassifyFPUsage_WidthsAndBarriers checks the FP usage classifier splits
// operands by width and flags helper/memory/fault opcodes as residency barriers.
func TestIE64ClassifyFPUsage_WidthsAndBarriers(t *testing.T) {
	// FP32 binary: two single reads, one single write, no barrier.
	fadd := ie64ClassifyFPUsage(&JITInstr{opcode: OP_FADD, rd: 1, rs: 2, rt: 3})
	if fadd.barrier {
		t.Fatal("FADD must not be a residency barrier")
	}
	if len(fadd.readSingles) != 2 || fadd.writeSingles[0] != 1 {
		t.Fatalf("FADD usage wrong: %+v", fadd)
	}
	if len(fadd.readPairs)+len(fadd.writePairs) != 0 {
		t.Fatalf("FADD must not touch pairs: %+v", fadd)
	}

	// FP64 binary: pair reads/writes keyed by even base (D5 -> base 4).
	dadd := ie64ClassifyFPUsage(&JITInstr{opcode: OP_DADD, rd: 5, rs: 2, rt: 6})
	if dadd.barrier {
		t.Fatal("DADD must not be a residency barrier")
	}
	if dadd.writePairs[0] != 4 { // D5 -> even base 4
		t.Fatalf("DADD write pair base = %d, want 4", dadd.writePairs[0])
	}
	if dadd.readPairs[0] != 2 || dadd.readPairs[1] != 6 {
		t.Fatalf("DADD read pairs wrong: %+v", dadd.readPairs)
	}
	if len(dadd.readSingles)+len(dadd.writeSingles) != 0 {
		t.Fatalf("DADD must not touch singles: %+v", dadd)
	}

	// Cross-format producers/consumers touch only their FP side.
	if u := ie64ClassifyFPUsage(&JITInstr{opcode: OP_FMOVI, rd: 7, rs: 3}); len(u.readSingles) != 0 || u.writeSingles[0] != 7 {
		t.Fatalf("FMOVI usage wrong (rs is a GPR): %+v", u)
	}
	if u := ie64ClassifyFPUsage(&JITInstr{opcode: OP_DCVTFI, rd: 3, rs: 8}); len(u.writePairs) != 0 || u.readPairs[0] != 8 {
		t.Fatalf("DCVTFI usage wrong (rd is a GPR): %+v", u)
	}

	// Barriers: memory, transcendental, compare, FPSR move.
	for _, op := range []byte{OP_FLOAD, OP_DSTORE, OP_FSIN, OP_DSQRT, OP_FCMP, OP_FMOVSR, OP_FCVTSD} {
		if u := ie64ClassifyFPUsage(&JITInstr{opcode: op, rd: 1, rs: 2, rt: 3}); !u.barrier {
			t.Fatalf("opcode 0x%02X must be a residency barrier", op)
		}
	}

	// Non-FP opcode: no FP file access at all.
	if u := ie64ClassifyFPUsage(&JITInstr{opcode: OP_ADD, rd: 1, rs: 2, rt: 3}); u.barrier ||
		len(u.readSingles)+len(u.writeSingles)+len(u.readPairs)+len(u.writePairs) != 0 {
		t.Fatalf("ADD must have empty FP usage: %+v", u)
	}
}

// TestIE64BuildFPResidencyPlan_NoOverlap verifies the ownership map never assigns
// a file slot to two residents and that a pair excludes conflicting singles.
func TestIE64BuildFPResidencyPlan_NoOverlap(t *testing.T) {
	var sw [16]int
	var pw [8]int
	// D2 (slots 2,3) very hot; F3 (slot 3) also wanted but conflicts with D2.
	pw[1] = 100 // pair base 2
	sw[3] = 90  // single slot 3 -> conflicts with D2's odd slot
	sw[8] = 50  // single slot 8 -> independent, should be selected
	plan := ie64BuildFPResidencyPlan(sw, pw)

	// Each binding's declared slots must all point back to that same binding
	// index in the owner map: no slot is shared between two residents.
	for idx, b := range plan.bindings {
		for _, s := range b.slots() {
			if plan.owner[s] != int8(idx) {
				t.Fatalf("binding %d owns slot %d but owner[%d]=%d", idx, s, s, plan.owner[s])
			}
		}
	}

	// D2 must win slot 3 over the single F3; F3 single must be dropped.
	if b, ok := plan.resident(3); !ok || b.kind != ie64FPResPair || b.baseSlot != 2 {
		t.Fatalf("slot 3 should belong to pair D2, got %+v ok=%v", b, ok)
	}
	// Independent single F8 must still be resident.
	if b, ok := plan.resident(8); !ok || b.kind != ie64FPResSingle {
		t.Fatalf("F8 single should be resident, got %+v ok=%v", b, ok)
	}
}

// TestIE64BuildFPResidencyPlan_XMMCap checks no more than eight residents are
// selected and each binds a distinct XMM8..15.
func TestIE64BuildFPResidencyPlan_XMMCap(t *testing.T) {
	var sw [16]int
	var pw [8]int
	for i := range sw {
		sw[i] = 10 + i // sixteen candidate singles, all hot, distinct weights
	}
	plan := ie64BuildFPResidencyPlan(sw, pw)
	if len(plan.bindings) != len(ie64FPResidentHostXMMs) {
		t.Fatalf("selected %d residents, want %d (XMM cap)", len(plan.bindings), len(ie64FPResidentHostXMMs))
	}
	usedXMM := map[byte]bool{}
	for _, b := range plan.bindings {
		if b.xmm < 8 || b.xmm > 15 {
			t.Fatalf("resident bound to non-resident XMM%d", b.xmm)
		}
		if usedXMM[b.xmm] {
			t.Fatalf("XMM%d bound twice", b.xmm)
		}
		usedXMM[b.xmm] = true
	}
	// Hottest single (slot 15, weight 25) must be selected.
	if _, ok := plan.resident(15); !ok {
		t.Fatal("hottest single F15 should be resident")
	}
}

// TestIE64BuildBlockFPPlan_Eligibility checks B1 block eligibility: an FP32
// arithmetic block qualifies; any residency barrier, any FP64 usage, or no FP at
// all disqualifies it.
func TestIE64BuildBlockFPPlan_Eligibility(t *testing.T) {
	fp32 := []JITInstr{
		{opcode: OP_FADD, rd: 1, rs: 2, rt: 3},
		{opcode: OP_FMUL, rd: 1, rs: 1, rt: 4},
		{opcode: OP_HALT64},
	}
	if plan, ok := ie64BuildBlockFPPlan(fp32); !ok || len(plan.bindings) == 0 {
		t.Fatalf("FP32 arithmetic block should be eligible, ok=%v bindings=%d", ok, len(plan.bindings))
	}
	if _, ok := ie64BuildBlockFPPlan([]JITInstr{
		{opcode: OP_FADD, rd: 1, rs: 2, rt: 3},
		{opcode: OP_FLOAD, rd: 5, rs: 6},
	}); ok {
		t.Fatal("FLOAD barrier must disqualify the block")
	}
	// FP64 arithmetic is eligible (B2): the pair becomes a resident.
	if plan, ok := ie64BuildBlockFPPlan([]JITInstr{
		{opcode: OP_DADD, rd: 0, rs: 2, rt: 4},
		{opcode: OP_HALT64},
	}); !ok || len(plan.bindings) == 0 {
		t.Fatalf("FP64 arithmetic block should be eligible, ok=%v bindings=%d", ok, len(plan.bindings))
	}
	// A block mixing an FP64 op with a barrier (DLOAD) is still ineligible.
	if _, ok := ie64BuildBlockFPPlan([]JITInstr{
		{opcode: OP_DADD, rd: 0, rs: 2, rt: 4},
		{opcode: OP_DLOAD, rd: 6, rs: 7},
	}); ok {
		t.Fatal("DLOAD barrier must disqualify the block")
	}
	if _, ok := ie64BuildBlockFPPlan([]JITInstr{{opcode: OP_ADD, rd: 1, rs: 2, rt: 3}}); ok {
		t.Fatal("non-FP block must be ineligible for FP residency")
	}

	// Aliasing: F5 (slot 5) and D4 (pair slots 4,5) share storage. When a block
	// touches both, neither may be resident (else one's XMM spill clobbers the
	// other's memory slot); an independent F8 stays resident.
	plan, ok := ie64BuildBlockFPPlan([]JITInstr{
		{opcode: OP_DADD, rd: 4, rs: 4, rt: 6}, // D4 pair (slots 4,5)
		{opcode: OP_FMUL, rd: 5, rs: 2, rt: 2}, // F5 single (slot 5) aliases D4
		{opcode: OP_FADD, rd: 8, rs: 8, rt: 9}, // F8 independent
		{opcode: OP_HALT64},
	})
	if !ok {
		t.Fatal("mixed FP block should still be eligible (F8 resident)")
	}
	if _, r := plan.resident(5); r {
		t.Error("F5/slot 5 must not be resident (aliases pair D4)")
	}
	if _, r := plan.resident(4); r {
		t.Error("D4/slot 4 must not be resident (aliased by single F5)")
	}
	if _, r := plan.resident(8); !r {
		t.Error("independent F8 should be resident")
	}
}

// TestIE64AccumulateFPWeights_BarrierContributesNothing confirms a barrier op's
// operands do not accrue residency weight.
func TestIE64AccumulateFPWeights_BarrierContributesNothing(t *testing.T) {
	var sw [16]int
	var pw [8]int
	// FCMP is a barrier: its FP32 operands must not gain weight.
	ie64AccumulateFPWeights(&JITInstr{opcode: OP_FCMP, rd: 1, rs: 2, rt: 3}, &sw, &pw)
	for i, w := range sw {
		if w != 0 {
			t.Fatalf("barrier FCMP gave slot %d weight %d", i, w)
		}
	}
	// A normal FADD does contribute.
	ie64AccumulateFPWeights(&JITInstr{opcode: OP_FADD, rd: 1, rs: 2, rt: 3}, &sw, &pw)
	if sw[1] == 0 || sw[2] == 0 || sw[3] == 0 {
		t.Fatalf("FADD should weight its operands: %v", sw[:4])
	}
}

package main

import (
	"strings"
	"testing"
)

// FP5 / FP6 scratch-overlay spill — Phase 1 of the two-gap FPU closeout.
//
// IE64 ScrFP1 (f10) and ScrFP2 (f12) overlay m68k FP5 and FP6. Any synthesis
// op that touches scratch must spill the live FP5/FP6 to dedicated memory
// slots first, then restore (unless destination overlaps).
//
// Slots: __m68kto64_fp5_save / __m68kto64_fp6_save (8B each, BSS, allocated
// lazily on first use via needsFP56Save).

const (
	spillStoreFP5 = "dstore f10, (r16)  ; spill FP5"
	spillStoreFP6 = "dstore f12, (r16)  ; spill FP6"
	spillLoadFP5  = "dload f10, (r16)  ; restore FP5"
	spillLoadFP6  = "dload f12, (r16)  ; restore FP6"
)

// FSCALE writes f10 in its body. Wrapper must spill FP5 before and restore
// after when destination is not FP5.
func TestFP56Spill_FSCALE_PreservesFP5(t *testing.T) {
	out := convertOneInstr(t, "\tfscale.l fp2,fp1")
	mustContain(t, out, "la r16, __m68kto64_fp5_save")
	mustContain(t, out, spillStoreFP5)
	mustContain(t, out, spillLoadFP5)
}

// FSCALE.dst=FP5: prologue still spills FP5 (in case src reads FP5), but
// epilogue must NOT restore FP5 because the just-computed result lives there.
func TestFP56Spill_FSCALE_DstFP5_NoRestore(t *testing.T) {
	out := convertOneInstr(t, "\tfscale.l fp2,fp5")
	mustContain(t, out, spillStoreFP5)
	mustNotContain(t, out, spillLoadFP5)
}

// FTST body uses f12 for the zero constant. Wrapper must spill FP6.
func TestFP56Spill_FTST_PreservesFP6(t *testing.T) {
	out := convertOneInstr(t, "\tftst.x fp1")
	mustContain(t, out, "la r16, __m68kto64_fp6_save")
	mustContain(t, out, spillStoreFP6)
	mustContain(t, out, spillLoadFP6)
}

// FTST source FP5: source must be read into a register before the spill
// prologue overwrites the live FP5 slot copy. Validate via spill comment
// ordering — FP6 store happens before the dcmp; the dcmp uses srcReg which
// for fp5 is f10 (ScrFP1, also FP5). FP5 itself isn't on the spill path
// because materializeFPSrc returns the FPn host reg directly for FPn input
// (no scratch use). So the only spill set is FP6 (body uses f12).
func TestFP56Spill_FTST_LiveOperandFP5(t *testing.T) {
	out := convertOneInstr(t, "\tftst.x fp5")
	mustContain(t, out, spillStoreFP6)
	// Body still references fp5 host name f10 via the dcmp.
	mustContain(t, out, "dcmp r17, f10, f12")
	// FP5 itself is NOT spilled because the FPn fast path bypasses scratch.
	mustNotContain(t, out, spillStoreFP5)
}

// FNEG with non-FPn source — materializeFPSrc loads into f10, clobbering
// FP5. Spill wrapper must catch this.
func TestFP56Spill_FNEG_MemSrc_PreservesFP5(t *testing.T) {
	out := convertOneInstr(t, "\tfneg.d (a0),fp1")
	mustContain(t, out, spillStoreFP5)
	mustContain(t, out, spillLoadFP5)
}

// FNEG with FPn source and no downstream FPCC consumer: materializeFPSrc
// returns the FPn directly (no scratch), and the liveness pass elides the
// shadow update — so no spill is needed.
func TestFP56Spill_FNEG_FPnSrc_NoConsumer_NoSpill(t *testing.T) {
	out := convertOneInstr(t, "\tfneg.x fp2,fp1")
	mustNotContain(t, out, spillStoreFP5)
	mustNotContain(t, out, spillStoreFP6)
}

// FNEG with FPn source AND a downstream FBcc consumer: shadow FPCC update
// runs (uses f12), so FP6 must spill.
func TestFP56Spill_FNEG_FPnSrc_LiveConsumer_FP6(t *testing.T) {
	src := "\tfneg.x fp2,fp1\n\tfbne done\n"
	out := convertSrc(t, src)
	mustNotContain(t, out, spillStoreFP5)
	mustContain(t, out, spillStoreFP6)
}

// FSCALE preserves FP6 via shadow-update path: needs a live FPCC consumer
// downstream so emitShadowFPCCFromResult fires and clobbers f12.
func TestFP56Spill_FSCALE_PreservesFP6(t *testing.T) {
	src := "\tfscale.l fp2,fp1\n\tfbne done\n"
	out := convertSrc(t, src)
	mustContain(t, out, "la r16, __m68kto64_fp6_save")
	mustContain(t, out, spillStoreFP6)
	mustContain(t, out, spillLoadFP6)
}

// FNEG with non-FPn source and no downstream FBcc consumer: materialize uses
// f10 (FP5) but body only emits `dneg`; FP6 stays untouched (no shadow update).
// Validates that the scratchSet logic distinguishes FP1-only from FP12.
func TestFP56Spill_FNEG_OneScratchOnly(t *testing.T) {
	src := "\tfneg.d (a0),fp1\n"
	out := convertSrc(t, src)
	mustContain(t, out, spillStoreFP5)
	mustNotContain(t, out, spillStoreFP6)
}

// FGETMAN body uses both f10 AND f12 directly.
func TestFP56Spill_FGETMAN_BothScratch(t *testing.T) {
	out := convertOneInstr(t, "\tfgetman.x fp2,fp1")
	mustContain(t, out, spillStoreFP5)
	mustContain(t, out, spillStoreFP6)
	mustContain(t, out, spillLoadFP5)
	mustContain(t, out, spillLoadFP6)
}

// SINH uses emitHyperbolicHelper which writes both f10 and f12.
func TestFP56Spill_SINH_BothScratch(t *testing.T) {
	out := convertOneInstr(t, "\tfsinh.x fp1")
	mustContain(t, out, spillStoreFP5)
	mustContain(t, out, spillLoadFP5)
	mustContain(t, out, spillStoreFP6)
	mustContain(t, out, spillLoadFP6)
}

// Back-to-back FSCALE: every op has its own save/restore (no merge or hoist).
// Guards against a future liveness optimisation that might hoist.
func TestFP56Spill_Idempotent(t *testing.T) {
	out := convertSrc(t, "\tfscale.l fp2,fp1\n\tfscale.l fp2,fp1\n")
	if strings.Count(out, spillStoreFP5) < 2 {
		t.Errorf("back-to-back FSCALE must spill FP5 twice, got:\n%s", out)
	}
	if strings.Count(out, spillLoadFP5) < 2 {
		t.Errorf("back-to-back FSCALE must restore FP5 twice, got:\n%s", out)
	}
}

// FSCALE inside a loop body: spill stays in the loop, not hoisted to
// pre-header. Future-proofs against a misguided liveness optimisation.
func TestFP56Spill_LoopBody_NotHoisted(t *testing.T) {
	src := "loop:\n\tfscale.l fp2,fp1\n\tdbra d0,loop\n"
	out := convertSrc(t, src)
	// The spill should appear AFTER the loop: label, not before it.
	loopIdx := strings.Index(out, "loop:")
	spillIdx := strings.Index(out, spillStoreFP5)
	if loopIdx < 0 || spillIdx < 0 {
		t.Fatalf("missing loop: or spill in output:\n%s", out)
	}
	if spillIdx < loopIdx {
		t.Errorf("FP5 spill hoisted above loop: label (spillIdx=%d < loopIdx=%d):\n%s",
			spillIdx, loopIdx, out)
	}
}

// Pure-FP0 program: no op touches scratch → BSS slots not allocated.
func TestFP56Spill_NoOpForFP0Only(t *testing.T) {
	// fmove between FPn regs has no scratch use and shouldn't trigger.
	out := convertOneInstr(t, "\tfmove.x fp0,fp1")
	// __m68kto64_fp5_save / fp6_save BSS labels should NOT appear.
	if strings.Contains(out, "__m68kto64_fp5_save:") {
		t.Errorf("fp5_save BSS label allocated for op that does not need it:\n%s", out)
	}
	if strings.Contains(out, "__m68kto64_fp6_save:") {
		t.Errorf("fp6_save BSS label allocated for op that does not need it:\n%s", out)
	}
}

// When at least one wrapped op runs, BSS slots ARE allocated in the footer.
func TestFP56Spill_BSSAllocatedWhenUsed(t *testing.T) {
	out := convertOneInstr(t, "\tfscale.l fp2,fp1")
	mustContain(t, out, "__m68kto64_fp5_save:")
	mustContain(t, out, "__m68kto64_fp6_save:")
}

package main

import (
	"strings"
	"testing"
)

// Phase 2 — RTE-walkback handler detection + FP slot save/restore.
//
// Default off. Opt in via Converter.fpIrqWrap (CLI: -fp-irq-wrap).

func convertSrcWithIRQWrap(t *testing.T, src string) string {
	t.Helper()
	c := NewConverter()
	c.noHeader = true
	c.fpIrqWrap = true
	out, _ := c.ConvertSource(src)
	return out
}

// Default flag off: no save/restore stubs even with handler-shaped input.
func TestIRQWrap_DefaultOff_NoStubs(t *testing.T) {
	src := "handler:\n\trte\n"
	out := convertSrc(t, src) // fpIrqWrap=false
	mustNotContain(t, out, "FP-slot save")
	mustNotContain(t, out, "FP-slot save (")
	mustNotContain(t, out, "restore FP slots before RTE")
}

// Simple handler: save stub after label, restore stub before RTE.
func TestIRQWrap_SimpleHandler_WrapsEntryAndExit(t *testing.T) {
	src := "handler:\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "handler:")
	mustContain(t, out, "; m68kto64: handler at handler wrapped with FP-slot save")
	mustContain(t, out, "; m68kto64: restore FP slots before RTE")
	// Order: label first, then save stub.
	labelIdx := strings.Index(out, "handler:")
	saveIdx := strings.Index(out, "FP-slot save")
	restoreIdx := strings.Index(out, "restore FP slots")
	rteIdx := strings.Index(out, "load.w r17, (r30)") // existing RTE first line
	if !(labelIdx < saveIdx && saveIdx < restoreIdx && restoreIdx < rteIdx) {
		t.Errorf("ordering wrong: label=%d save=%d restore=%d rte=%d\nout:\n%s",
			labelIdx, saveIdx, restoreIdx, rteIdx, out)
	}
}

// Frame size 16B when no FP5/FP6 ops trigger needsFP56Save.
func TestIRQWrap_FrameSize16_NoFP56(t *testing.T) {
	src := "handler:\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "sub.l r30, r30, #16")
	mustContain(t, out, "add.l r30, r30, #16")
	// 4 load.l / 4 store.l in entry stub (2 slots × 2 halves).
	if strings.Count(out, "load.l r17") < 8 {
		t.Errorf("expected ≥8 load.l in 16B-frame wrap (4 entry, 4 exit), got:\n%s", out)
	}
}

// Frame size 32B when handler body (or another op) triggers needsFP56Save.
// Use FSCALE which always allocates the FP5/FP6 slots.
func TestIRQWrap_FrameSize32_WithFP56(t *testing.T) {
	src := "handler:\n\tfscale.l fp2,fp1\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "sub.l r30, r30, #32")
	mustContain(t, out, "add.l r30, r30, #32")
	mustContain(t, out, "__m68kto64_fp5_save")
	mustContain(t, out, "__m68kto64_fp6_save")
}

// Handler with no FP ops still gets wrap (correctness: caller may have left
// dirty slots).
func TestIRQWrap_NoFPInHandler_StillWraps(t *testing.T) {
	src := "handler:\n\tmove.l #1,d0\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "FP-slot save")
}

// Block ending in RTS, not RTE → no wrap.
func TestIRQWrap_RTSBlock_NotWrapped(t *testing.T) {
	src := "subr:\n\tmove.l #1,d0\n\trts\n"
	out := convertSrcWithIRQWrap(t, src)
	mustNotContain(t, out, "FP-slot save")
}

// Two consecutive handlers, each independently wrapped.
func TestIRQWrap_TwoHandlersBackToBack(t *testing.T) {
	src := "h1:\n\trte\nh2:\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	if strings.Count(out, "FP-slot save") != 2 {
		t.Errorf("expected 2 save stubs (one per handler), got:\n%s", out)
	}
	if strings.Count(out, "restore FP slots") != 2 {
		t.Errorf("expected 2 restore stubs, got:\n%s", out)
	}
}

// Handler containing JSR…RTS continues to be detected as handler-via-RTE.
func TestIRQWrap_NestedJSR_DoesNotBreakWalkback(t *testing.T) {
	// JSR inside a handler: control returns via RTS of callee, then RTE.
	// The handler's own block has no RTS terminator — only the embedded
	// instruction line is `jsr`, which is not a block terminator.
	src := "handler:\n\tjsr inner\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "FP-slot save")
}

// Phase F1.1 — handler frame grows to 40B when ftanh appears in the file
// (FP7 slot adds 8B on top of the FP5/FP6 32B frame).
func TestFP7Spill_IRQWrap_FrameSize40(t *testing.T) {
	src := "handler:\n\tftanh.x fp1\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "sub.l r30, r30, #40")
	mustContain(t, out, "add.l r30, r30, #40")
	// FP7 slot lives at offset 32 (after FP5 at 16 and FP6 at 24).
	mustContain(t, out, "store.l r17, 32(r30)")
	mustContain(t, out, "store.l r17, 36(r30)")
	mustContain(t, out, "__m68kto64_fp7_save")
}

// Stack balance for the 40B frame size.
func TestFP7Spill_IRQWrap_StackBalance40(t *testing.T) {
	src := "h:\n\tftanh.x fp1\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	if strings.Count(out, "sub.l r30, r30, #40") != 1 {
		t.Errorf("expected exactly one sub.l #40, got:\n%s", out)
	}
	if strings.Count(out, "add.l r30, r30, #40") != 1 {
		t.Errorf("expected exactly one add.l #40, got:\n%s", out)
	}
}

// Save order: FP7 saved AFTER FP5/FP6 in entry stub; restored BEFORE
// FP5/FP6 in exit stub (matches plan §"Frame layout").
func TestFP7Spill_IRQWrap_SaveOrder(t *testing.T) {
	src := "h:\n\tftanh.x fp1\n\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	// Entry order: FP5 store at offset 16, FP6 at 24, FP7 at 32.
	fp5Store := strings.Index(out, "store.l r17, 16(r30)")
	fp6Store := strings.Index(out, "store.l r17, 24(r30)")
	fp7Store := strings.Index(out, "store.l r17, 32(r30)")
	if !(fp5Store < fp6Store && fp6Store < fp7Store) {
		t.Errorf("entry order wrong: fp5@%d, fp6@%d, fp7@%d:\n%s",
			fp5Store, fp6Store, fp7Store, out)
	}
	// Exit order (reverse): FP7 load before FP5/FP6 loads.
	fp7Load := strings.Index(out, "load.l r17, 32(r30)")
	fp6Load := strings.Index(out, "load.l r17, 24(r30)")
	if !(fp7Load < fp6Load) {
		t.Errorf("exit order wrong: fp7-load@%d should precede fp6-load@%d:\n%s",
			fp7Load, fp6Load, out)
	}
}

// Orphan RTE: RTE with no preceding label cannot be wrapped. Under
// -strict -fp-irq-wrap the converter must error.
func TestIRQWrap_OrphanRTE_StrictErrors(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.fpIrqWrap = true
	c.strict = true
	out, errs := c.ConvertSource("\trte\n")
	if errs == 0 {
		t.Errorf("orphan RTE under -strict -fp-irq-wrap should error, got 0 errors. Output:\n%s", out)
	}
	if !strings.Contains(out, "ERROR") {
		t.Errorf("expected ; ERROR: diagnostic for orphan RTE, got:\n%s", out)
	}
}

// Orphan RTE: under default (non-strict) -fp-irq-wrap, emit a diag and
// fall through to the regular RTE lowering (no save/restore stubs).
func TestIRQWrap_OrphanRTE_NonStrictDiagAndSkip(t *testing.T) {
	c := NewConverter()
	c.noHeader = true
	c.fpIrqWrap = true
	c.strict = false
	out, errs := c.ConvertSource("\trte\n")
	if errs != 0 {
		t.Errorf("orphan RTE under non-strict -fp-irq-wrap should not error, got %d errors", errs)
	}
	mustContain(t, out, "orphan RTE")
	mustNotContain(t, out, "FP-slot save")
	mustNotContain(t, out, "restore FP slots before RTE")
	// Regular RTE lowering still emits.
	mustContain(t, out, "load.w r17, (r30)")
}

// Stack balance: sub.l #N at entry must match add.l #N at exit for both
// 16 and 32 frame sizes.
func TestIRQWrap_StackBalance(t *testing.T) {
	for _, tc := range []struct {
		name string
		src  string
		want int
	}{
		{"16B", "h:\n\trte\n", 16},
		{"32B", "h:\n\tfscale.l fp2,fp1\n\trte\n", 32},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out := convertSrcWithIRQWrap(t, tc.src)
			subStr := "sub.l r30, r30, #" + itoaStr(tc.want)
			addStr := "add.l r30, r30, #" + itoaStr(tc.want)
			if !strings.Contains(out, subStr) {
				t.Errorf("missing %q in:\n%s", subStr, out)
			}
			if !strings.Contains(out, addStr) {
				t.Errorf("missing %q in:\n%s", addStr, out)
			}
		})
	}
}

func itoaStr(n int) string {
	if n == 16 {
		return "16"
	}
	if n == 32 {
		return "32"
	}
	return ""
}

package main

import (
	"strings"
	"testing"
)

// Phase F4 — vector-table handler heuristic.
//
// Pre-marks labels referenced as vector-table targets, even when the
// handler block has no preceding source-level RTE fall-through path.
// Active only under -fp-irq-wrap.

// move.l #handler,(vec).l before the handler body — handler gets wrapped.
func TestVectorHeuristic_ImmediateMove_MarksLabel(t *testing.T) {
	src := "" +
		"\tmove.l #myhandler,($80).l\n" +
		"\trts\n" +
		"myhandler:\n" +
		"\tmove.l #1,d0\n" +
		"\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "; m68kto64: handler at myhandler wrapped with FP-slot save")
}

// lea handler,a0 ; move.l a0,(vec).l → marked.
func TestVectorHeuristic_LEAThenMove_MarksLabel(t *testing.T) {
	src := "" +
		"\tlea myhandler,a0\n" +
		"\tmove.l a0,($80).l\n" +
		"\trts\n" +
		"myhandler:\n" +
		"\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "; m68kto64: handler at myhandler wrapped with FP-slot save")
}

// a0 clobbered between LEA and MOVE — NOT marked.
func TestVectorHeuristic_LEAClobbered_NotMarked(t *testing.T) {
	src := "" +
		"\tlea myhandler,a0\n" +
		"\tmove.l #99,a0\n" +
		"\tmove.l a0,($80).l\n" +
		"\trts\n" +
		"myhandler:\n" +
		"\tmove.l #1,d0\n" +
		"\trts\n"
	out := convertSrcWithIRQWrap(t, src)
	// myhandler block has no RTE so RTE-walkback can't mark it; the
	// LEA tracker was clobbered so the vector heuristic also can't.
	// Either path active = bug.
	if strings.Contains(out, "handler at myhandler wrapped") {
		t.Errorf("LEA-clobbered tracker should not mark handler; got:\n%s", out)
	}
}

// JSR between LEA and MOVE wipes the An→label map.
func TestVectorHeuristic_LEAClobberedByJSR_NotMarked(t *testing.T) {
	src := "" +
		"\tlea myhandler,a0\n" +
		"\tjsr setup\n" +
		"\tmove.l a0,($80).l\n" +
		"\trts\n" +
		"myhandler:\n" +
		"\tmove.l #1,d0\n" +
		"\trts\n"
	out := convertSrcWithIRQWrap(t, src)
	if strings.Contains(out, "handler at myhandler wrapped") {
		t.Errorf("JSR boundary should wipe LEA tracker; got:\n%s", out)
	}
}

// Numeric immediate (move.l #$2700,...) is NOT a label — not marked.
func TestVectorHeuristic_NumericImmediateSkipped(t *testing.T) {
	src := "\tmove.l #$2700,($80).l\n\trts\n"
	out := convertSrcWithIRQWrap(t, src)
	// No handler label exists; trivially nothing should be marked.
	if strings.Contains(out, "FP-slot save") {
		t.Errorf("numeric immediate should not mark a handler; got:\n%s", out)
	}
}

// move.l #foo,d0 (dst is Dn, not absolute) — not vector-pattern.
func TestVectorHeuristic_NonVectorMoveSkipped(t *testing.T) {
	src := "\tmove.l #foo,d0\n\trts\nfoo:\n\trts\n"
	out := convertSrcWithIRQWrap(t, src)
	if strings.Contains(out, "handler at foo wrapped") {
		t.Errorf("non-vector move should not mark handler; got:\n%s", out)
	}
}

// Default -fp-irq-wrap off: vector scan not active.
func TestVectorHeuristic_DefaultOff_NoMark(t *testing.T) {
	src := "" +
		"\tmove.l #myhandler,($80).l\n" +
		"\trts\n" +
		"myhandler:\n" +
		"\trte\n"
	out := convertSrc(t, src) // fpIrqWrap false
	if strings.Contains(out, "FP-slot save") {
		t.Errorf("vector heuristic should not run without -fp-irq-wrap; got:\n%s", out)
	}
}

// Orphan RTE (no preceding label) now resolved when an upstream vector
// write names a label that... wait, the handler MUST exist as a label
// for the save stub to attach. If the vector points at a label that
// reaches RTE via fall-through, fine. If the label doesn't exist or
// RTE is truly orphan (no label nearby), the orphan diag still fires.
// Test the "label exists, vector-write marks it, RTE wraps cleanly" path.
func TestVectorHeuristic_NoFallthroughRTE_StillMarked(t *testing.T) {
	// myhandler is NOT preceded by a fall-through RTE block from
	// scanRTEHandlerBlocks's perspective (the prior `rts` terminates).
	// Without the vector-write heuristic, myhandler would not be a
	// handler. With it, myhandler IS marked because of the move.l #.
	src := "" +
		"\tmove.l #myhandler,($80).l\n" +
		"\trts\n" +
		"myhandler:\n" +
		"\trte\n"
	out := convertSrcWithIRQWrap(t, src)
	mustContain(t, out, "handler at myhandler wrapped")
}

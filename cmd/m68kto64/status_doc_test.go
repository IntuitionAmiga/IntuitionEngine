package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// status_doc_test.go — guard against doc/code drift for the §8 instruction
// reference in sdk/docs/m68Kto64.md.
//
// Rules enforced:
//
//  1. Every mnemonic listed as ✅ or ⚠️ must produce output (no "unknown
//     mnemonic" error) when fed through `convertOneInstr` with a minimal
//     synthetic operand pattern.
//
//  2. Every mnemonic listed as ❌ must reach the unknown-mnemonic path (so
//     the doc claim "❌ unsupported" stays honest).
//
//  3. Specific mnemonics that flipped status during this work must appear in
//     the doc with their new status marker. Catches the "code shipped, doc
//     forgot to update" failure mode the same-commit doc-flip rule guards.

// statusCheck is the catalogue of mnemonics whose doc-status this test pins.
// Each entry is exercised through the converter and the markdown is scanned
// for the canonical status marker.
type statusCheck struct {
	mnemonic string // m68k mnemonic to feed through the transpiler
	src      string // synthetic source line (post-tab, no trailing newline)
	wantOK   bool   // true if the converter must accept (✅/⚠️); false if must error (❌)
}

var statusCatalogue = []statusCheck{
	// ✅ — Phases 1–4.
	{"addx", "\taddx.l d0,d1", true},
	{"subx", "\tsubx.l d0,d1", true},
	{"negx", "\tnegx.l d0", true},
	{"roxl", "\troxl.l #1,d0", true},
	{"roxr", "\troxr.l #1,d0", true},
	{"chk2", "\tchk2.l (a0),d0", true},
	{"trapeq", "\ttrapeq", true},
	{"trapne", "\ttrapne.w #$1234", true},
	{"trapgt", "\ttrapgt", true},
	// ⚠️ — Phase 5 / 6.
	{"cas2", "\tcas2.l d0:d1,d2:d3,(a0):(a1)", true},
	{"moves", "\tmoves.l (a0),d1", true},
	{"callm", "\tcallm #4,(a0)", true},
	{"rtm", "\trtm d0", true},
	{"retm", "\tretm #8", true},
	// Mainline integer ops promoted in the same wave.
	{"bset", "\tbset #3,d1", true},
	{"bclr", "\tbclr #4,(a0)", true},
	{"bchg", "\tbchg d0,d1", true},
	{"tas", "\ttas d2", true},
	{"exg", "\texg d0,d1", true},
	{"cmpm", "\tcmpm.l (a0)+,(a1)+", true},
	{"illegal", "\tillegal", true},
	{"rte", "\trte", true},
	{"stop", "\tstop #$2700", true},
	{"reset", "\treset", true},
	{"fsave", "\tfsave (a0)", true},
	{"frestore", "\tfrestore (a0)", true},
}

func TestStatusDoc_MnemonicsConvert(t *testing.T) {
	for _, c := range statusCatalogue {
		t.Run(c.mnemonic, func(t *testing.T) {
			cv := NewConverter()
			cv.noHeader = true
			out, errs := cv.ConvertSource(c.src + "\n")
			gotOK := errs == 0
			if gotOK != c.wantOK {
				t.Errorf("%s: wantOK=%v gotOK=%v, output:\n%s",
					c.mnemonic, c.wantOK, gotOK, out)
			}
		})
	}
}

// TestStatusDoc_DocMarkersMatch scans sdk/docs/m68Kto64.md for each catalogued
// mnemonic and asserts the surrounding row contains an explicit status marker
// matching code reality (✅ for OK ops with no caveat, ⚠ for approximations).
//
// This is the cheap insurance against orphan ✅ claims when a future change
// regresses or removes a lowering.
func TestStatusDoc_DocMarkersMatch(t *testing.T) {
	docPath := filepath.Join("..", "..", "sdk", "docs", "m68Kto64.md")
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	doc := string(raw)

	want := map[string]string{
		// mnemonic → expected status marker present in the §8 row covering it
		"ADDX":   "✅",
		"SUBX":   "✅",
		"NEGX":   "✅",
		"ROXL":   "✅",
		"ROXR":   "✅",
		"CHK2":   "✅",
		"TRAPcc": "✅",
		"CAS2":    "⚠",
		"MOVES":   "⚠",
		"CALLM":   "⚠",
		"RTM":     "⚠",
		"RETM":    "⚠",
		"BSET":    "✅",
		"BCLR":    "✅",
		"BCHG":    "✅",
		"TAS":     "✅",
		"EXG":     "✅",
		"CMPM":    "✅",
		"ILLEGAL": "✅",
		"RTE":     "✅",
		"STOP":    "✅",
		"RESET":   "✅",
		"FSAVE":   "✅",
		"FRESTORE": "✅",
	}
	for mn, marker := range want {
		// Find at least one line containing the mnemonic AND the marker.
		ok := false
		for _, ln := range strings.Split(doc, "\n") {
			if strings.Contains(ln, mn) && strings.Contains(ln, marker) {
				ok = true
				break
			}
		}
		if !ok {
			t.Errorf("doc drift: %s missing %s marker in §8 row of %s",
				mn, marker, docPath)
		}
	}
}

// TestStatusDoc_FPGapsClosed pins the doc claims for the two FPU gaps closed
// by the m68kto64 FPU closeout plan: FP5/FP6 auto-spill (Phase 1) and the
// -fp-irq-wrap handler save/restore (Phase 2). The phrases below must appear
// somewhere in m68Kto64.md or the doc has silently regressed against the
// implementation in fpu.go / fpu_arith.go / fpu_shadow.go /
// fpu_transcendental.go / irq_wrap.go.
func TestStatusDoc_FPGapsClosed(t *testing.T) {
	docPath := filepath.Join("..", "..", "sdk", "docs", "m68Kto64.md")
	raw, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read %s: %v", docPath, err)
	}
	doc := string(raw)

	phrases := []string{
		// Phase 1 — FP5/FP6 scratch overlay closure (doc uses hyphen, not em-dash)
		"FP5/FP6 scratch overlay",
		"__m68kto64_fp5_save",
		"__m68kto64_fp6_save",
		"needsFP56Save",
		// Phase 2 — interrupt-aware FP-slot save/restore
		"-fp-irq-wrap",
		"RTE-walkback",
		"integer scratch r17",
		// Phase F1 — FTANH FP7 sub-gap
		"__m68kto64_fp7_save",
		"FTANH FP7 sub-gap",
		// Phase F2 — (xxx).w sign-extension
		"maybeSignExtAbsW",
		"CLOSED in Phase F2",
		// Phase F3 — ANDI/ORI/EORI #imm,CCR/SR consumer
		"ccr_imm.go",
		"CLOSED in Phase F3",
		// Phase F4 — vector-table handler heuristic
		"scanVectorTableHandlers",
		// Phase F5 — MOVEP
		"emitMovep",
		"CLOSED in Phase F5",
	}
	for _, p := range phrases {
		if !strings.Contains(doc, p) {
			t.Errorf("doc drift: m68Kto64.md missing FP-gap-closure anchor %q", p)
		}
	}
}

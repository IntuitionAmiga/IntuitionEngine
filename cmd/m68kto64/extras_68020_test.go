package main

import "testing"

func TestTrap(t *testing.T) {
	// TRAP #5 (m68k instruction-encoded immediate) → syscall #5. Not the
	// same as m68k vector 5 (zero-divide), which lives at syscall #16 in
	// the locked range.
	out := convertSrc(t, "\ttrap #5\n")
	mustContain(t, out, "syscall #5")
}

func TestMovec_Stripped(t *testing.T) {
	out := convertSrc(t, "\tmovec.l vbr,d0\n")
	mustContain(t, out, "stripped movec")
}

func TestChk_BoundsTest(t *testing.T) {
	out := convertSrc(t, "\tchk.w d0,d1\n")
	// dst (d1=r2) negative or > src (d0=r1) → fail label → syscall #6
	mustContain(t, out, "bltz")
	mustContain(t, out, "bgt")
	mustContain(t, out, "syscall #17")
}

func TestMuluL_Pair(t *testing.T) {
	out := convertSrc(t, "\tmulu.l d0,d2:d3\n")
	mustContain(t, out, "mulu.l r4, r4, r1")
	mustContain(t, out, "mulhu.q r3, r4, r1")
}

func TestMulsL_Pair(t *testing.T) {
	out := convertSrc(t, "\tmuls.l d0,d2:d3\n")
	mustContain(t, out, "muls.l r4, r4, r1")
	mustContain(t, out, "mulhs.q r3, r4, r1")
}

func TestDivuL_Pair(t *testing.T) {
	out := convertSrc(t, "\tdivu.l d0,d2:d3\n")
	mustContain(t, out, "divu.l r4, r18, r1")
	mustContain(t, out, "mod.l r3, r18, r1")
}

func TestMulu_NonPair_Phase2_FallThrough(t *testing.T) {
	// mulu.w d0,d1 → Phase 2 ALU lowering
	out := convertSrc(t, "\tmulu.w d0,d1\n")
	// Phase 2 emits ALU3 form for .w — partial-update merge.
	mustContain(t, out, "and.l")
}

func TestBfextu_Basic(t *testing.T) {
	// Extract 8 bits at offset 4 from d0 into d1.
	out := convertSrc(t, "\tbfextu d0{#4:#8},d1\n")
	mustContain(t, out, "lsr.l r2, r1, r17")
	mustContain(t, out, "and.l r2, r2, r18")
}

func TestBfexts_SignedExtract(t *testing.T) {
	out := convertSrc(t, "\tbfexts d0{#0:#8},d1\n")
	// Signed: lsl up + asr down.
	mustContain(t, out, "lsl.q r2, r1, r18")
	mustContain(t, out, "asr.q r2, r2, r18")
}

package main

import "strings"

// =====================================================================
// Integer shadow-CCR liveness (Phase H)
//
// Mirrors fpu_liveness.go's ShadowFPCC pass for the integer side. The
// shadow N/Z/C/V/X update sequence is heavy (10-15 IE64 ops per m68k
// arith op); when no downstream consumer reads any of those flags
// before the next producer overwrites them, the entire shadow emission
// is dead and can be skipped.
//
// Algorithm (backward pass):
//
//	live := false
//	for i := N-1 .. 0:
//	    l := lines[i]
//	    if l is integer-flag consumer: live = true
//	    else if l is integer-flag producer: liveAt[i] = live; live = false
//	    if l carries a Label or l.Kind == LineLabelOnly: live = true
//
// Labels force "all live" because branches can reach the next consumer
// through a path that doesn't pass through this routine.
//
// Conservative class boundaries: every Bcc / DBcc / Scc / addx-class /
// roxl-class / bcd consumer reads at least one of N/Z/C/V/X. Every
// arith / logical / shift / move / clr producer overwrites at least
// N/Z; the more-expensive C/V/X helpers piggyback on the same emit
// site, so the "any live" criterion covers them too. Splitting N vs
// Z vs C vs V vs X liveness would let us skip more aggressively but
// is left to a follow-up.
// =====================================================================

// computeIntegerCCLiveness returns a map keyed by line index. liveAt[i]
// is true iff some integer-flag consumer reads the flags produced at
// line i before any subsequent producer overwrites them.
func computeIntegerCCLiveness(lines []Line) map[int]bool {
	live := false
	out := map[int]bool{}
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if isIntegerCCConsumer(l) {
			live = true
		} else if isIntegerCCProducer(l) {
			out[i] = live
			live = false
		}
		if l.Label != "" || l.Kind == LineLabelOnly {
			live = true
		}
	}
	return out
}

// isIntegerCCConsumer reports whether `l` reads any of N/Z/C/V/X.
func isIntegerCCConsumer(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	m := l.Mnemonic
	// Bcc family — read condition bits.
	switch m {
	case "beq", "bne", "blt", "bge", "bgt", "ble", "bhi", "bls",
		"bcc", "bcs", "bmi", "bpl", "bvs", "bvc",
		"dbeq", "dbne", "dblt", "dbge", "dbgt", "dble", "dbhi", "dbls",
		"dbcc", "dbcs", "dbmi", "dbpl", "dbvs", "dbvc", "dbt", "dbf",
		"seq", "sne", "slt", "sge", "sgt", "sle", "shi", "sls",
		"scc", "scs", "smi", "spl", "svs", "svc", "st", "sf",
		"trapeq", "trapne", "traplt", "trapge", "trapgt", "traple",
		"traphi", "trapls", "trapcc", "trapcs", "trapmi", "trappl",
		"trapvs", "trapvc", "trapt", "trapf":
		return true
	}
	// addx/subx/negx read X; roxl/roxr read X.
	switch m {
	case "addx", "subx", "negx", "roxl", "roxr":
		return true
	}
	// BCD chain — read X.
	switch m {
	case "abcd", "sbcd", "nbcd":
		return true
	}
	// MOVE-from-CCR / MOVE-from-SR — reads all CCR bits.
	if m == "move" && len(l.Operands) >= 1 {
		src := strings.ToLower(strings.TrimSpace(l.Operands[0]))
		if src == "ccr" || src == "sr" {
			return true
		}
	}
	// dbra/dbf with the F suffix doesn't read flags but the generic
	// dbcc list above already covers dbt/dbf — keep them on the
	// consumer list to avoid mis-classifying as flag-killers.
	if m == "dbra" {
		// DBRA proper tests counter only, not flags — NOT a consumer.
		return false
	}
	return false
}

// isIntegerCCProducer reports whether `l` writes any of N/Z (every
// recognised producer writes at least these two; the C/V/X cases ride
// in the same emit site so the coarse "any-write" classification is
// sufficient for the live/dead decision).
func isIntegerCCProducer(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	switch l.Mnemonic {
	// Arith.
	case "add", "adda", "addi", "addq", "addx",
		"sub", "suba", "subi", "subq", "subx",
		"neg", "negx",
		"cmp", "cmpa", "cmpi", "cmpm",
		"mulu", "muls", "divu", "divs", "divul", "divsl",
		"mulu.l", "muls.l":
		return true
	// Logical / bit.
	case "and", "andi", "or", "ori", "eor", "eori", "not",
		"clr", "ext", "extb", "swap",
		"btst", "bset", "bclr", "bchg":
		return true
	// Shift / rotate.
	case "lsl", "lsr", "asl", "asr", "rol", "ror", "roxl", "roxr":
		return true
	// MOVE / MOVEA / MOVEQ — N/Z only. MOVEA does NOT affect flags;
	// MOVE does. We can't disambiguate without per-op classification,
	// so treat MOVE as a producer and MOVEA as not.
	case "move", "moveq":
		return true
	case "tst":
		return true
	// BCD chain.
	case "abcd", "sbcd", "nbcd":
		return true
	// TAS, CHK etc. — flag-affecting.
	case "tas", "chk", "chk2":
		return true
	}
	return false
}

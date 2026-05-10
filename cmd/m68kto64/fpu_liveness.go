package main

import "strings"

// =====================================================================
// ShadowFPCC liveness pass (Phase 7.4)
//
// Mirrors the integer-side Phase A liveness: the 4-instruction ShadowFPCC
// update sequence is emitted only when some downstream consumer reads it
// before the next producer overwrites it. Otherwise it's dead code and
// skipped.
//
// Producers (cc-affecting FPU ops): FCMP, FTST, and every arithmetic /
// transcendental m68k spec marks as FPSR-cc-affecting — FADD/FSUB/FMUL/
// FDIV/FMOD/FREM/FNEG/FABS/FSQRT/FINT/FINTRZ/FSCALE/FGETMAN/FGETEXP/
// FSGLDIV/FSGLMUL plus every transcendental.
//
// Consumers: FBcc, FDBcc, FScc, FTRAPcc, FMOVE.L FPSR,Dn.
//
// Algorithm (backward pass):
//   live := false
//   for i := N-1 .. 0:
//       l := lines[i]
//       if l is consumer: live = true
//       else if l is producer: liveAt[i] = live; live = false
//       if l carries a Label or l.Kind == LineLabelOnly: live = true
//
// Labels force "all live" because control flow may reach the next consumer
// through a path that doesn't include this routine.
// =====================================================================

func computeFPCCLiveness(lines []Line) map[int]bool {
	live := false
	out := map[int]bool{}
	for i := len(lines) - 1; i >= 0; i-- {
		l := lines[i]
		if isFPCCConsumer(l) {
			live = true
		} else if isFPCCProducer(l) {
			out[i] = live
			live = false
		}
		if l.Label != "" || l.Kind == LineLabelOnly {
			live = true
		}
	}
	return out
}

// isFPCCConsumer reports whether `l` reads ShadowFPCC.
func isFPCCConsumer(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	m := l.Mnemonic
	if strings.HasPrefix(m, "fb") || strings.HasPrefix(m, "fdb") ||
		strings.HasPrefix(m, "ftrap") {
		return true
	}
	// FScc — fs* but excluding fsave/fsqrt/fsin/fscale/fsglmul/fsgldiv etc.
	if strings.HasPrefix(m, "fs") {
		switch m {
		case "fsave", "fsqrt", "fsin", "fsinh", "fscale", "fsglmul", "fsgldiv":
			return false
		}
		return true
	}
	if m == "fmove" {
		// FMOVE.L FPSR,Dn — consumer
		if len(l.Operands) == 2 && strings.EqualFold(strings.TrimSpace(l.Operands[0]), "fpsr") {
			return true
		}
	}
	return false
}

// isFPCCProducer reports whether `l` writes ShadowFPCC (i.e. updates m68k
// FPSR cc bits 27:24).
func isFPCCProducer(l Line) bool {
	if l.Kind != LineInstruction {
		return false
	}
	switch l.Mnemonic {
	case "fcmp", "ftst",
		"fadd", "fsub", "fmul", "fdiv", "fmod", "frem",
		"fneg", "fabs", "fsqrt", "fint", "fintrz",
		"fscale", "fgetexp", "fgetman", "fsglmul", "fsgldiv",
		"fsin", "fcos", "ftan", "fatan", "facos", "fasin",
		"fcosh", "fsinh", "ftanh", "fatanh",
		"fetox", "fetoxm1",
		"flogn", "flog10", "flog2", "flognp1",
		"ftentox", "ftwotox":
		return true
	}
	// FMOVE.L Dn,FPSR — producer of cc bits (split fold).
	if l.Mnemonic == "fmove" && len(l.Operands) == 2 &&
		strings.EqualFold(strings.TrimSpace(l.Operands[1]), "fpsr") {
		return true
	}
	return false
}

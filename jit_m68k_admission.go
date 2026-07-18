// jit_m68k_admission.go - backend-neutral M68020 native-admission analysis.
//
// Extracted from jit_m68k_exec.go and jit_m68k_emit_amd64.go (M68K JIT
// parity plan, milestone 2). Everything here is pure opcode/IR analysis:
// which blocks are production-native-safe, which instructions may take the
// generic IO fallback path, and the A7/control-flow taint walk. No emitter
// state, no native memory — every M68020 backend shares these predicates.
//
// This file must stay free of build tags and emitter symbols.

package main

func m68kInstrMaySetGenericIOFallback(ji *M68KJITInstr) bool {
	opcode := ji.opcode
	group := opcode >> 12

	switch group {
	case 0x0: // CMPI
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		return opcode&0xFF00 == 0x0C00 && (opcode>>6)&3 != 3 &&
			m68kEAMayUseMemHelper(srcMode, srcReg, false)
	case 0x1, 0x2, 0x3: // MOVE
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return m68kEAMayUseMemHelper(srcMode, srcReg, false) || m68kEAMayUseMemHelper(dstMode, dstReg, true)
	case 0x9, 0xB, 0xD: // SUB/CMP/ADD EA,Dn paths
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		opmode := (opcode >> 6) & 7
		if group == 0xB {
			return opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
		if group == 0x9 || group == 0xD {
			return opmode <= 2 && m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
	case 0x4: // Misc family
		if opcode&0xFF80 == 0x4C00 { // MULL/DIVL
			srcMode := (opcode >> 3) & 7
			srcReg := opcode & 7
			return m68kEAMayUseMemHelper(srcMode, srcReg, false)
		}
	}
	return false
}

func m68kBlockMayUseGenericIOFallback(instrs []M68KJITInstr) bool {
	for i := range instrs {
		if m68kInstrGenericIOFallbackUnsafe(&instrs[i]) {
			return true
		}
	}
	return false
}

func m68kInstrGenericIOFallbackUnsafe(ji *M68KJITInstr) bool {
	if !m68kInstrMaySetGenericIOFallback(ji) {
		return false
	}

	opcode := ji.opcode
	group := opcode >> 12
	switch group {
	case 0x0: // CMPI is safe to re-execute only for non-mutating EAs.
		if m68kIsImmediateLogicDn(opcode) {
			return false
		}
		if m68kIsNativeSupportedImmediateLogicEA(opcode) {
			return false
		}
		if m68kIsBTSTImmAnDisp(opcode) {
			return false
		}
		if m68kIsNativeSupportedCMPI(opcode) {
			return false
		}
		if opcode&0xFF00 == 0x0C00 && (opcode>>6)&3 != 3 {
			mode := (opcode >> 3) & 7
			return mode == 3 || mode == 4
		}
		return true
	case 0x1, 0x2, 0x3: // MOVE
		if m68kIsNativeSupportedMOVEA(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEGuarded(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
			return false
		}
		if m68kIsMoveLongStackDispToReg(opcode) || m68kIsMoveLongRegToStackDisp(opcode) ||
			m68kIsMoveLongRegToStackPredec(opcode) || m68kIsMoveLongStackDispToStackPredec(opcode) ||
			m68kIsMoveLongStackPostincToStackPredec(opcode) || m68kIsMoveLongStackIndirectToStackPostinc(opcode) ||
			m68kIsMoveLongStackDispToAddressIndirect(opcode) || m68kIsMoveLongAuditedStackMemory(opcode) ||
			m68kIsMovePostincPostinc(opcode) {
			return false
		}
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		if m68kEAMayUseMemHelper(srcMode, srcReg, false) {
			if dstMode == 0 || dstMode == 1 {
				return false
			}
			if srcMode != 3 && srcMode != 4 && dstMode != 4 {
				return false
			}
			return true
		}
		if m68kEAMayUseMemHelper(dstMode, dstReg, true) {
			return dstMode == 4
		}
		return false
	case 0x5: // ADDQ/SUBQ Dn/An/(An)/-(An)/d16(An) are audited native paths.
		if m68kIsNativeSupportedDBcc(opcode) {
			return false
		}
		if opcode&0xF0C0 == 0x50C0 && opcode&0xF0F8 != 0x50C8 && opcode&0xF0F8 != 0x50F8 {
			return !m68kIsNativeSupportedScc(opcode)
		}
		if opcode&0x00C0 != 0x00C0 {
			mode := (opcode >> 3) & 7
			return mode != 0 && mode != 1 && mode != 2 && mode != 3 && mode != 4 && mode != 5
		}
		return true
	case 0xB: // CMP <ea>,Dn is replay-safe; audited EOR Dn,<ea> owns memory writes.
		opmode := (opcode >> 6) & 7
		if m68kIsNativeSupportedCMPM(opcode) {
			return false
		}
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedCMPA(opcode) {
			return false
		}
		if opmode <= 2 && m68kIsNativeSupportedCMPToDn(opcode) {
			return false
		}
		if opmode >= 4 && opmode <= 6 && m68kIsNativeSupportedLogicDnToEA(opcode) {
			return false
		}
		return opmode > 2
	case 0x8: // OR <ea>,Dn is safe only when the source EA read can be replayed.
		opmode := (opcode >> 6) & 7
		if opmode <= 2 {
			if m68kIsNativeSupportedLogicEAToDn(opcode) {
				return false
			}
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		if m68kIsNativeSupportedLogicDnToEA(opcode) {
			return false
		}
		return true
	case 0xC: // AND <ea>,Dn / AND Dn,<ea>
		if opcode&0xF0C0 == 0xC0C0 && m68kIsNativeSupportedMULW(opcode) {
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		opmode := (opcode >> 6) & 7
		if opmode <= 2 {
			if m68kIsNativeSupportedLogicEAToDn(opcode) {
				return false
			}
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		if m68kIsNativeSupportedLogicDnToEA(opcode) {
			return false
		}
		return true
	case 0x9, 0xD: // SUB/ADD <ea>,Dn write Dn after a source read.
		opmode := (opcode >> 6) & 7
		if opmode <= 2 {
			return m68kEAToDnALUSourceFallbackUnsafe(opcode)
		}
		if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedAddrArithA(opcode) {
			return false
		}
		if m68kIsNativeSupportedArithDnToEA(opcode) {
			return false
		}
		return true
	case 0x4:
		if m68kIsNativeSupportedPEA(opcode) || m68kIsNativeSupportedCLR(opcode) ||
			m68kIsNativeSupportedTST(opcode) || m68kIsNativeSupportedNOT(opcode) ||
			m68kIsNativeSupportedNEG(opcode) || m68kIsNativeSupportedNEGX(opcode) ||
			m68kIsNativeSupportedTAS(opcode) || m68kIsNativeSupportedNBCD(opcode) ||
			m68kIsNativeSupportedMOVEFromStatus(opcode) || m68kIsNativeSupportedMOVEToCCR(opcode) {
			return false
		}
		if m68kIsNativeSupportedCHK(opcode) {
			return false
		}
		if opcode&0xFF80 == 0x4C00 && m68kIsNativeSupportedMULLDIVL(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEM(opcode) {
			return false
		}
		if m68kIsNativeSupportedMOVEToSR(opcode) {
			return false
		}
		if m68kIsTSTAnDisp(opcode) {
			return false
		}
	}
	return true
}

func m68kBlockProductionNativeSafe(instrs []M68KJITInstr) bool {
	if len(instrs) == 0 {
		return false
	}
	for i := range instrs {
		if !m68kInstrProductionNativeSafe(&instrs[i]) {
			return false
		}
	}
	return true
}

func m68kInstrTouchesA7OrControl(ji *M68KJITInstr) bool {
	if ji == nil {
		return false
	}
	opcode := ji.opcode
	group := opcode >> 12
	if group == 0x1 || group == 0x2 || group == 0x3 {
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return m68kModeTouchesSP(srcMode, srcReg) || m68kModeTouchesSP(dstMode, dstReg)
	}
	if group == 0x5 {
		mode := (opcode >> 3) & 7
		reg := opcode & 7
		return opcode&0x00C0 != 0x00C0 && m68kModeTouchesSP(mode, reg)
	}
	if group == 0x6 {
		return opcode&0xFF00 == 0x6100 // BSR
	}
	if group == 0x9 || group == 0xD {
		opmode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return (opmode == 3 || opmode == 7) && dstReg == 7 // SUBA/ADDA ...,A7
	}
	if group != 0x4 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch {
	case opcode == 0x4E70 || opcode&0xFFF0 == 0x4E40 || opcode&0xFFF0 == 0x4E60 || opcode&0xFFFE == 0x4E7A: // RESET/TRAP/MOVE USP/MOVEC
		return true
	case opcode == 0x4E75 || opcode == 0x4E73 || opcode == 0x4E77: // RTS/RTE/RTR
		return true
	case opcode&0xFFC0 == 0x4E80 || opcode&0xFFC0 == 0x4EC0: // JSR/JMP
		return true
	case opcode&0xFFF8 == 0x4E50 || opcode&0xFFF8 == 0x4808 || opcode&0xFFF8 == 0x4E58: // LINK/UNLK
		return true
	case opcode&0xF1C0 == 0x41C0 && ((opcode>>9)&7) == 7: // LEA ...,A7
		return true
	case opcode&0xFFC0 == 0x4840: // PEA
		return true
	case opcode&0xFB80 == 0x4880 && m68kModeTouchesSP(mode, reg): // MOVEM with A7 EA
		return true
	default:
		return m68kModeTouchesSP(mode, reg)
	}
}

func m68kBlockTouchesA7OrControl(instrs []M68KJITInstr) bool {
	for i := range instrs {
		if m68kInstrTouchesA7OrControl(&instrs[i]) {
			return true
		}
	}
	return false
}

// m68kCanUseProductionNativeBlock decides whether a block runs as native code.
// Per M68K_JIT_FALLBACK_REMOVAL_PLAN.md the broad conservative gate
// (m68kNeedsConservativeFallback) no longer decides production admission: a
// block is native iff every instruction has an emitter (!m68kNeedsFallback) and
// is individually production-native-safe (m68kBlockProductionNativeSafe), with
// MMIO/guarded forms handled by per-instruction self-bail at run time rather
// than blanket block rejection. m68kNeedsConservativeFallback remains only as a
// diagnostic/reporting predicate (see m68kConservativeFallbackDiagnostic and the
// AROS diagnostic tests); it is intentionally NOT consulted here.
func m68kCanUseProductionNativeBlock(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	return len(instrs) > 0 &&
		!m68kNeedsFallback(instrs) &&
		m68kBlockProductionNativeSafe(instrs) &&
		!m68kBlockMayUseGenericIOFallback(instrs)
}

// m68kConservativeFallbackDiagnostic exposes the retired conservative gate for
// diagnostics/reporting only. It must never gate production native admission.
func m68kConservativeFallbackDiagnostic(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	return m68kNeedsConservativeFallback(memory, startPC, instrs)
}

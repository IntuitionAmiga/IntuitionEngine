// jit_m68k_ccr_liveness.go - per-instruction CCR liveness for M68K JIT
// (Phase 2b/2c of the six-CPU JIT unification plan).
//
// M68K's CCR has 5 bits (X-N-Z-V-C) but they are NOT all written by
// every flag-producing instruction. The split that matters for
// liveness is X-vs-NZ-vs-VC:
//
//   - Arithmetic-shape ops (ADD, SUB, NEG, NEGX, ADDQ, SUBQ, ASL, LSL,
//     ASR, LSR, ROXL, ROXR, ABCD, SBCD, NBCD, MULU, MULS, DIVU, DIVS)
//     write BOTH X and NZVC.
//   - Logical/move/CMP/TST/CLR/MOVEQ ops preserve X.
//   - The current interpreter's AND/OR/EOR paths update only N/Z and
//     preserve X/V/C via SetFlagsNZ; the JIT mirrors that oracle.
//   - ROL/ROR write NZ+C+V (V=0) but preserve X.
//
// The reverse-walk therefore tracks two independent demands (demandX,
// demandNZVC) and a producer is dead only when ALL bits it writes are
// shadowed. Conflating the two led to a subtle bug where an
// arithmetic ADD followed by a logical AND would be marked dead — but
// AND preserves X, so ADD's X-bit output is still live and the
// downstream guest CCR loses ADD's carry-into-X.
//
// Confident producers (per group/opmode), confident consumers (Bcc/Scc/
// DBcc/TRAPcc with cc>=2), and confident overwriters (RTE, RTR,
// MOVE-to-CCR, MOVE-to-SR) are decoded from the opcode. Anything else
// is treated as "passes both demands through, keeps live=true".

//go:build amd64 && (linux || windows || darwin)

package main

// m68kCCRBits is a small bitmask describing which CCR bits an
// instruction writes. The two bits matter independently for the
// liveness walk:
//
//	m68kCCRBitNZ — N+Z bits
//	m68kCCRBitVC — V+C bits
//	m68kCCRBitX  — X bit (extend / second-carry)
type m68kCCRBits uint8

const (
	m68kCCRBitNZ m68kCCRBits = 1 << iota
	m68kCCRBitVC
	m68kCCRBitX

	m68kCCRBitNZVC = m68kCCRBitNZ | m68kCCRBitVC
)

// m68kClassifyCCR returns the CCR effect of a single opcode:
//
//	writes      — bitmask of which CCR groups the instruction writes
//	consumer    — instruction reads CCR (control-flow consumer or
//	              explicit reader like TRAPV)
//	overwriter  — instruction installs a fresh CCR independent of
//	              prior state (RTE/RTR/MOVE-to-CCR/MOVE-to-SR)
//
// "writes == 0" means non-producer. The reverse walk treats the
// caller-side "live" bit as the OR of (writesX && demandX) and
// (writesNZVC && demandNZVC).
func m68kClassifyCCR(opcode uint16) (writes m68kCCRBits, consumer, overwriter bool) {
	// RTE / RTR overwrite SR (and therefore CCR) from the stack.
	if opcode == 0x4E73 || opcode == 0x4E77 {
		overwriter = true
		return
	}
	// TRAPV reads V.
	if opcode == 0x4E76 {
		consumer = true
		return
	}
	// MOVE to CCR (0x44C0..0x44FF) and MOVE to SR (0x46C0..0x46FF) —
	// install fresh status from source.
	if (opcode & 0xFFC0) == 0x44C0 {
		overwriter = true
		return
	}
	if (opcode & 0xFFC0) == 0x46C0 {
		overwriter = true
		return
	}
	group := opcode >> 12
	switch group {
	case 0:
		if m68kIsImmediateArithmeticDn(opcode) {
			writes = m68kCCRBitX | m68kCCRBitNZVC
		} else if m68kIsImmediateLogicDn(opcode) {
			// Interpreter ORI/ANDI/EORI paths update only N/Z and
			// preserve X/V/C via SetFlagsNZ.
			writes = m68kCCRBitNZ
		} else if opcode&0xFF00 == 0x0C00 && (opcode>>6)&3 != 3 {
			// CMPI.B/W/L preserves X and writes NZVC.
			writes = m68kCCRBitNZVC
		}
	case 1: // MOVE.B — preserves X.
		writes = m68kCCRBitNZVC
	case 2, 3: // MOVE.L (2), MOVE.W (3) — preserves X. MOVEA = no CCR.
		dstMode := (opcode >> 6) & 7
		if dstMode != 1 {
			writes = m68kCCRBitNZVC
		}
	case 4:
		// Group 4 mixed.
		hi := opcode & 0xFF00
		switch hi {
		case 0x4000, 0x4040, 0x4080:
			// NEGX.B/W/L — reads X, writes X+NZVC. The X-read makes
			// it an explicit CCR consumer in addition to producer.
			writes = m68kCCRBitX | m68kCCRBitNZVC
			consumer = true
		case 0x4200, 0x4240, 0x4280:
			// CLR.B/W/L — sets N=0, Z=1, V=0, C=0; preserves X.
			writes = m68kCCRBitNZVC
		case 0x4400, 0x4440, 0x4480:
			// NEG.B/W/L — writes X+NZVC (NEG sets X=C).
			writes = m68kCCRBitX | m68kCCRBitNZVC
		case 0x4600, 0x4640, 0x4680:
			// NOT.B/W/L — interpreter updates N/Z and preserves X/V/C.
			writes = m68kCCRBitNZ
		case 0x4A00, 0x4A40, 0x4A80:
			// TST.B/W/L — preserves X.
			writes = m68kCCRBitNZVC
		}
		// EXT.W (0x4880-0x4887), EXT.L (0x48C0-0x48C7), EXTB.L (0x49C0-0x49C7)
		// preserve X.
		if (opcode&0xFFF8) == 0x4880 || (opcode&0xFFF8) == 0x48C0 || (opcode&0xFFF8) == 0x49C0 {
			writes = m68kCCRBitNZVC
		}
		// TAS — sets N+Z, V=C=0; preserves X.
		if (opcode&0xFFC0) == 0x4AC0 && opcode != 0x4AFC {
			writes = m68kCCRBitNZVC
		}
		// NBCD reads X, writes X+NZVC, and uses sticky Z.
		if (opcode&0xFFC0) == 0x4800 && opcode&0xFFF8 != 0x4808 {
			writes = m68kCCRBitX | m68kCCRBitNZVC
			consumer = true
		}
		// CHK — partial CCR (mostly N); preserves X.
		if (opcode&0xF1C0) == 0x4180 || (opcode&0xF1C0) == 0x4100 {
			writes = m68kCCRBitNZVC
		}
	case 5:
		if (opcode & 0x00C0) == 0x00C0 {
			// Scc / DBcc / TRAPcc family.
			cc := (opcode >> 8) & 0xF
			if cc >= 2 {
				consumer = true
			}
		} else {
			// ADDQ / SUBQ. Destination mode in bits 5-3:
			//   mode 1 (An) → ADDQ/SUBQ to An does NOT modify CCR
			//                 (M68K Programmer's Reference: "If the
			//                 destination is an address register,
			//                 the condition codes are not affected,
			//                 and the entire destination address
			//                 register is used regardless of the
			//                 operation size.")
			//   any other  → ADDQ/SUBQ writes X+NZVC.
			dstMode := (opcode >> 3) & 7
			if dstMode != 1 {
				writes = m68kCCRBitX | m68kCCRBitNZVC
			}
		}
	case 6: // Bcc / BSR / BRA. cc 0=BRA, cc 1=BSR — no CCR read.
		cc := (opcode >> 8) & 0xF
		if cc >= 2 {
			consumer = true
		}
	case 7: // MOVEQ — preserves X.
		if opcode&0x0100 == 0 {
			writes = m68kCCRBitNZVC
		}
	case 8:
		// Group 8: OR.B/W/L (preserves X), DIVU/DIVS (preserves X — division
		// docs: X not affected; N+Z set, V/C set on overflow), SBCD (reads
		// and writes X+NZVC).
		if (opcode & 0xF1F0) == 0x8100 {
			// SBCD — X-reader and X-writer.
			writes = m68kCCRBitX | m68kCCRBitNZVC
			consumer = true
		} else {
			opmode := (opcode >> 6) & 7
			if opmode <= 2 || (opmode >= 4 && opmode <= 6) {
				writes = m68kCCRBitNZ
			} else {
				writes = m68kCCRBitNZVC
			}
		}
	case 9:
		// SUB.B/W/L (opmode 0,1,2,4,5,6) → X+NZVC.
		// SUBA (opmode 3,7) → none.
		// SUBX (op pattern (op & 0xF130) == 0x9100) → reads and writes X+NZVC.
		opmode := (opcode >> 6) & 7
		if opmode != 3 && opmode != 7 {
			writes = m68kCCRBitX | m68kCCRBitNZVC
			if (opcode&0xF130) == 0x9100 && (opcode&0x00C0) != 0x00C0 {
				// SUBX reads X.
				consumer = true
			}
		}
	case 0xB:
		// Group B: CMP.B/W/L, CMPM, CMPA — preserve X.
		// EOR (opmode 4,5,6 with EA mode != An) — preserves X.
		opmode := (opcode >> 6) & 7
		if opmode >= 4 && opmode <= 6 {
			writes = m68kCCRBitNZ
		} else {
			writes = m68kCCRBitNZVC
		}
	case 0xC:
		// Group C: AND/MULU/MULS/ABCD/EXG. Check ABCD first because
		// its mask overlaps the previously-broad EXG mask.
		// ABCD: bits 8-4 = 10000 → mask 0xF1F0 == 0xC100.
		// EXG:  bits 8-4 = 10100 / 10101 / 11000 →
		//       mask 0xF1F8 == 0xC140 / 0xC148 / 0xC188.
		switch {
		case (opcode & 0xF1F0) == 0xC100:
			// ABCD — reads X, writes X+NZVC.
			writes = m68kCCRBitX | m68kCCRBitNZVC
			consumer = true
		case (opcode&0xF1F8) == 0xC140 ||
			(opcode&0xF1F8) == 0xC148 ||
			(opcode&0xF1F8) == 0xC188:
			// EXG — no CCR.
		default:
			opmode := (opcode >> 6) & 7
			if opmode <= 2 || (opmode >= 4 && opmode <= 6) {
				writes = m68kCCRBitNZ
			} else {
				// MUL — preserve X, write NZVC.
				writes = m68kCCRBitNZVC
			}
		}
	case 0xD:
		// ADD.B/W/L (opmode 0,1,2,4,5,6) → X+NZVC.
		// ADDA (opmode 3,7) → none.
		// ADDX (op pattern (op & 0xF130) == 0xD100) → reads X, writes X+NZVC.
		opmode := (opcode >> 6) & 7
		if opmode != 3 && opmode != 7 {
			writes = m68kCCRBitX | m68kCCRBitNZVC
			if (opcode&0xF130) == 0xD100 && (opcode&0x00C0) != 0x00C0 {
				// ADDX reads X.
				consumer = true
			}
		}
	case 0xE:
		// Group E shifts/rotates. ASd/LSd write X+NZVC; ROXL/ROXR
		// READ X and write X+NZVC; ROL/ROR preserve X.
		//
		// Encoding (per M68K reference): rotate-type field is 2 bits.
		//   register-form: bits 4-3 = type (0=AS, 1=LS, 2=ROX, 3=RO).
		//   memory-form: bits 10-9 = type (same encoding).
		// ROXd is the X-reader.
		var rtype uint16
		if opcode&0x00C0 == 0x00C0 {
			rtype = (opcode >> 9) & 3 // memory-form
		} else {
			rtype = (opcode >> 3) & 3 // register-form
		}
		// A REGISTER-count shift/rotate may have a runtime count of 0, where the
		// interpreter returns immediately and leaves CCR entirely unchanged
		// (cpu_m68k.go ExecShiftRotate). So it conditionally PRESERVES the prior
		// CCR — mark it a consumer so an upstream producer is kept live and its
		// value remains available for the count-0 path. Immediate-count forms
		// (count 1..8, never 0) always overwrite and need no such treatment.
		registerCount := opcode&0x00C0 != 0x00C0 && (opcode>>5)&1 == 1
		switch rtype {
		case 3: // RO  (ROL/ROR) — preserves X.
			if opcode&0x00C0 != 0x00C0 {
				size := (opcode >> 6) & 3
				regOrImm := (opcode >> 5) & 1
				count := (opcode >> 9) & 7
				if size == 0 && regOrImm == 0 && count == 0 {
					// Interpreter normalizes immediate byte rotate #8
					// to count 0: no register or CCR change.
					writes = 0
					break
				}
			}
			writes = m68kCCRBitNZVC
			if registerCount {
				consumer = true
			}
		case 2: // ROX (ROXL/ROXR) — reads and writes X.
			writes = m68kCCRBitX | m68kCCRBitNZVC
			consumer = true
		default: // AS / LS — write X (=C), don't read X.
			writes = m68kCCRBitX | m68kCCRBitNZVC
			if registerCount {
				consumer = true
			}
		}
	}
	return
}

// m68kCCRLiveness analyzes a sequence of M68K JIT instructions and
// returns, for each slot i, a boolean indicating whether ANY of the
// CCR bits it writes is consumed by some downstream instruction in
// the same block.
//
// Three-bit demand walk: demandX, demandNZ, and demandVC propagate
// independently in reverse. A producer slot is live if any written bit
// group is demanded. After the live decision, the producer
// clears the demand bits it writes (its output satisfies them).
//
// Consumers (Bcc/Scc/DBcc/TRAPcc/TRAPV) reassert BOTH demands —
// branches read all of N/Z/V/C and TRAPV reads V; we don't separate
// them because the gain from finer split is marginal and the
// correctness cost of mis-classification is high.
//
// Bail-capable instructions (m68kInstrMaySetGenericIOFallback) are
// HIDDEN consumers: when the JIT bails to the interpreter mid-block,
// the bailout epilogue exposes the guest CCR to the interpreter, so
// upstream pending-CCR producers must remain live to be materialised
// at the bail. Treat them like explicit consumers.
//
// Overwriters (RTE/RTR/MOVE-to-CCR/MOVE-to-SR) clear BOTH demands.
//
// Returns nil if instrs is empty.
func m68kCCRLiveness(instrs []M68KJITInstr) JITFlagLiveness {
	if len(instrs) == 0 {
		return nil
	}
	live := make(JITFlagLiveness, len(instrs))
	demandX := true  // block exit observes X
	demandNZ := true // block exit observes N/Z
	demandVC := true // block exit observes V/C
	for i := len(instrs) - 1; i >= 0; i-- {
		writes, consumer, overwriter := m68kClassifyCCR(instrs[i].opcode)
		// Producer-or-overwriter effect (occurs at end of instruction).
		if writes == 0 {
			// Non-producer: pass demands through, keep live=true so
			// any future emit-side consumer (e.g. an undocumented
			// CCR reader) is conservatively served.
			live[i] = true
		} else {
			liveSlot := false
			if writes&m68kCCRBitX != 0 && demandX {
				liveSlot = true
			}
			if writes&m68kCCRBitNZ != 0 && demandNZ {
				liveSlot = true
			}
			if writes&m68kCCRBitVC != 0 && demandVC {
				liveSlot = true
			}
			live[i] = liveSlot
			// Clear the demand bits this slot writes.
			if writes&m68kCCRBitX != 0 {
				demandX = false
			}
			if writes&m68kCCRBitNZ != 0 {
				demandNZ = false
			}
			if writes&m68kCCRBitVC != 0 {
				demandVC = false
			}
		}
		if overwriter {
			demandX = false
			demandNZ = false
			demandVC = false
		}
		// Consumer effect (occurs at start of instruction). Explicit
		// CCR readers reassert demand; bail-capable instructions are
		// hidden consumers because the bailout epilogue surfaces the
		// guest CCR to the interpreter, requiring upstream producers
		// to stay materialisable.
		if consumer || m68kInstrMaySetGenericIOFallback(&instrs[i]) {
			demandX = true
			demandNZ = true
			demandVC = true
		}
	}
	return live
}

// m68kCCRLivenessConsumers reports whether the M68K instruction at
// instrs[i] is a CCR consumer (Bcc/Scc/DBcc/TRAPcc/TRAPV).
func m68kCCRLivenessConsumers(instrs []M68KJITInstr, i int) bool {
	if i < 0 || i >= len(instrs) {
		return false
	}
	_, consumer, _ := m68kClassifyCCR(instrs[i].opcode)
	return consumer
}

// m68kIsCCRProducer reports whether the M68K instruction's opcode
// writes any CCR bit. Used by the compile-loop dead-producer
// pre-materialise gate.
func m68kIsCCRProducer(instr *M68KJITInstr) bool {
	if instr == nil {
		return false
	}
	writes, _, _ := m68kClassifyCCR(instr.opcode)
	return writes != 0
}

// jit68KCCRLivenessEnabled gates the Phase 2c emit-side dead-CCR skip.
// Default false for architectural correctness: SR/CCR is observable at native
// block boundaries by interrupts, exceptions, helper exits, and MMIO callbacks
// even when the following guest instruction does not read CCR. A future
// boundary-aware liveness pass may re-enable this selectively.
var jit68KCCRLivenessEnabled = false

// m68kCurrentLive / m68kCurrentInstrIdx publish the per-block bitmap
// to emitCCR_* helpers. m68kCompileBlockWithMem sets them at the top
// of each block iteration and clears at function exit via defer.
var m68kCurrentLive []bool
var m68kCurrentInstrIdx int

// m68kCCRDeadAtCurrent reports whether the current emit slot's CCR
// output is dead per m68kCurrentLive. Returns false in every fallback
// case (gate off, nil bitmap, out-of-range index) so the safe path
// wins.
func m68kCCRDeadAtCurrent() bool {
	if !jit68KCCRLivenessEnabled {
		return false
	}
	if m68kCurrentLive == nil {
		return false
	}
	if m68kCurrentInstrIdx < 0 || m68kCurrentInstrIdx >= len(m68kCurrentLive) {
		return false
	}
	return !m68kCurrentLive[m68kCurrentInstrIdx]
}

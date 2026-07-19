// jit_m68k_const_fold.go - M68020 block-level constant folding analysis
// (milestone 7 optimisation slice, the M68020 analogue of
// ie64AnalyseConstFold).
//
// A forward constant analysis over one scanned block tracks compile-time
// known data-register values. A whitelisted pure ALU instruction whose
// inputs are all known folds to its precomputed result AND its precomputed
// condition codes: on the M68020 the CCR is an architectural output of
// every folded instruction, so each fold entry carries the exact CCR bits
// the instruction would have produced (the "M68020 CCR proof" the parity
// matrix requires). The backend emits the constant through its normal
// destination-write path and applies the architectural constant CCR update:
// MOVE clears V/C; AND/OR/EOR preserve X and clear V/C; add/sub set all five
// with X=C; CMP leaves X unchanged.
//
// Soundness rules:
//   - Tracking starts unknown at the block head.
//   - Any in-block branch target clears all tracked constants: the taken
//     edge can skip earlier setters.
//   - Any instruction outside the whitelist invalidates every tracked
//     register (conservative; memory reads, helpers and exceptions never
//     feed a fold).
//   - A bail before a folded instruction exits the block, so a fold never
//     executes on a path where its producers did not.
//
// Architecture-neutral and untagged so every backend can consume the same
// plan.

package main

import (
	"os"
	"sync/atomic"
)

// m68kFoldedConstEmits counts folded constants actually emitted by any
// backend. Shape tests read it to prove the fold was applied.
var m68kFoldedConstEmits atomic.Uint64

// Kill switch: IE_M68K_JIT_DISABLE_CONST_FOLD=1 disables the analysis.
var m68kJITConstFoldDisabled = os.Getenv("IE_M68K_JIT_DISABLE_CONST_FOLD") == "1"

// CCR bit masks in M68K order (X N Z V C at bits 4..0).
const (
	m68kFoldCCR_C uint8 = 1 << 0
	m68kFoldCCR_V uint8 = 1 << 1
	m68kFoldCCR_Z uint8 = 1 << 2
	m68kFoldCCR_N uint8 = 1 << 3
	m68kFoldCCR_X uint8 = 1 << 4
)

// m68kFoldEntry is one analysis result aligned with the input instruction
// slice. When folded is true the backend may emit the precomputed register
// value (if setsReg) and replace the ccrMask bits of the CCR with ccrVal
// instead of emitting the instruction body.
type m68kFoldEntry struct {
	folded  bool
	setsReg bool
	reg     uint8  // destination Dn when setsReg
	value   uint32 // full 32-bit register value after the sized merge
	ccrMask uint8  // CCR bits this instruction writes
	ccrVal  uint8  // constant values of those bits
}

type m68kFoldSize int

const (
	m68kFoldSizeB m68kFoldSize = iota
	m68kFoldSizeW
	m68kFoldSizeL
)

func m68kFoldSizedBits(size m68kFoldSize) uint32 {
	switch size {
	case m68kFoldSizeB:
		return 8
	case m68kFoldSizeW:
		return 16
	}
	return 32
}

func m68kFoldSizedMask(size m68kFoldSize) uint32 {
	switch size {
	case m68kFoldSizeB:
		return 0xFF
	case m68kFoldSizeW:
		return 0xFFFF
	}
	return 0xFFFFFFFF
}

// m68kFoldMerge merges a sized result into the previous full register value.
func m68kFoldMerge(old, res uint32, size m68kFoldSize) uint32 {
	m := m68kFoldSizedMask(size)
	return (old &^ m) | (res & m)
}

// m68kFoldNZ computes the N and Z CCR bits of a sized value.
func m68kFoldNZ(res uint32, size m68kFoldSize) uint8 {
	m := m68kFoldSizedMask(size)
	res &= m
	var ccr uint8
	if res == 0 {
		ccr |= m68kFoldCCR_Z
	}
	if res&(1<<(m68kFoldSizedBits(size)-1)) != 0 {
		ccr |= m68kFoldCCR_N
	}
	return ccr
}

// m68kFoldAdd computes dst+src at the given size: sized result plus the
// NZVC bits, matching the interpreter's ADD semantics.
func m68kFoldAdd(dst, src uint32, size m68kFoldSize) (uint32, uint8) {
	bits := m68kFoldSizedBits(size)
	m := m68kFoldSizedMask(size)
	a := dst & m
	b := src & m
	res := (a + b) & m
	ccr := m68kFoldNZ(res, size)
	if uint64(a)+uint64(b) > uint64(m) {
		ccr |= m68kFoldCCR_C
	}
	sign := uint32(1) << (bits - 1)
	if (a&sign) == (b&sign) && (res&sign) != (a&sign) {
		ccr |= m68kFoldCCR_V
	}
	return res, ccr
}

// m68kFoldSub computes dst-src at the given size: sized result plus the
// NZVC bits, matching the interpreter's SUB/CMP semantics.
func m68kFoldSub(dst, src uint32, size m68kFoldSize) (uint32, uint8) {
	bits := m68kFoldSizedBits(size)
	m := m68kFoldSizedMask(size)
	a := dst & m
	b := src & m
	res := (a - b) & m
	ccr := m68kFoldNZ(res, size)
	if b > a {
		ccr |= m68kFoldCCR_C
	}
	sign := uint32(1) << (bits - 1)
	if (a&sign) != (b&sign) && (res&sign) == (b&sign) {
		ccr |= m68kFoldCCR_V
	}
	return res, ccr
}

// m68kFoldInBlockBranchTargets returns the set of pcOffsets that are the
// static target of an in-block Bcc/BRA/BSR/DBcc, keyed by pcOffset.
func m68kFoldInBlockBranchTargets(instrs []M68KJITInstr, startPC uint32, memory []byte) map[uint32]bool {
	var targets map[uint32]bool
	endPC := startPC
	if n := len(instrs); n > 0 {
		last := &instrs[n-1]
		endPC = startPC + last.pcOffset + uint32(last.length)
	}
	add := func(target uint32) {
		if target >= startPC && target < endPC {
			if targets == nil {
				targets = make(map[uint32]bool, 2)
			}
			targets[target-startPC] = true
		}
	}
	for i := range instrs {
		ji := &instrs[i]
		pc := startPC + ji.pcOffset
		op := ji.opcode
		switch {
		case op&0xF000 == 0x6000: // BRA/BSR/Bcc
			disp8 := int32(int8(op & 0xFF))
			switch disp8 {
			case 0:
				if pc+4 <= uint32(len(memory)) {
					d := int32(int16(uint16(memory[pc+2])<<8 | uint16(memory[pc+3])))
					add(uint32(int64(pc) + 2 + int64(d)))
				}
			case -1: // 0xFF: 68020 long displacement
				if pc+6 <= uint32(len(memory)) {
					d := int32(uint32(memory[pc+2])<<24 | uint32(memory[pc+3])<<16 |
						uint32(memory[pc+4])<<8 | uint32(memory[pc+5]))
					add(uint32(int64(pc) + 2 + int64(d)))
				}
			default:
				add(uint32(int64(pc) + 2 + int64(disp8)))
			}
		case op&0xF0F8 == 0x50C8: // DBcc
			if pc+4 <= uint32(len(memory)) {
				d := int32(int16(uint16(memory[pc+2])<<8 | uint16(memory[pc+3])))
				add(uint32(int64(pc) + 2 + int64(d)))
			}
		}
	}
	return targets
}

// m68kFoldImmediate reads a sized immediate whose extension words start at
// extPC. Byte immediates occupy the low byte of one extension word.
func m68kFoldImmediate(memory []byte, extPC uint32, size m68kFoldSize) (uint32, bool) {
	switch size {
	case m68kFoldSizeB:
		if extPC+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint32(memory[extPC+1]), true
	case m68kFoldSizeW:
		if extPC+2 > uint32(len(memory)) {
			return 0, false
		}
		return uint32(memory[extPC])<<8 | uint32(memory[extPC+1]), true
	}
	if extPC+4 > uint32(len(memory)) {
		return 0, false
	}
	return uint32(memory[extPC])<<24 | uint32(memory[extPC+1])<<16 |
		uint32(memory[extPC+2])<<8 | uint32(memory[extPC+3]), true
}

// m68kAnalyseConstFold runs the forward constant analysis over one scanned
// block. Returns a plan aligned with instrs, or nil when nothing folds.
func m68kAnalyseConstFold(instrs []M68KJITInstr, startPC uint32, memory []byte) []m68kFoldEntry {
	if m68kJITConstFoldDisabled || len(instrs) == 0 || memory == nil {
		return nil
	}
	targets := m68kFoldInBlockBranchTargets(instrs, startPC, memory)

	var known [8]bool
	var val [8]uint32
	clearAll := func() {
		for i := range known {
			known[i] = false
		}
	}

	plan := make([]m68kFoldEntry, len(instrs))
	any := false

	for i := range instrs {
		ji := &instrs[i]
		if targets[ji.pcOffset] {
			clearAll()
		}
		if ji.fusedFlag != 0 {
			clearAll()
			continue
		}
		op := ji.opcode
		extPC := startPC + ji.pcOffset + 2

		fold := func(e m68kFoldEntry, dn uint16, full uint32, ok bool) {
			if ok {
				plan[i] = e
				any = true
			}
			if e.setsReg || ok {
				known[dn] = ok
				val[dn] = full
			}
		}

		switch {
		case op&0xF100 == 0x7000: // MOVEQ #imm,Dn
			dn := (op >> 9) & 7
			v := uint32(int32(int8(op & 0xFF)))
			ccr := m68kFoldNZ(v, m68kFoldSizeL)
			fold(m68kFoldEntry{
				folded: true, setsReg: true, reg: uint8(dn), value: v,
				ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
				ccrVal:  ccr,
			}, dn, v, true)

		case (op&0xF000 == 0x1000 || op&0xF000 == 0x2000 || op&0xF000 == 0x3000) &&
			(op>>6)&7 == 0 && (op>>3)&7 == 7 && op&7 == 4:
			// MOVE.{B,W,L} #imm,Dn
			size := m68kFoldMoveSize(op)
			dn := (op >> 9) & 7
			imm, ok := m68kFoldImmediate(memory, extPC, size)
			if !ok {
				known[dn] = false
				continue
			}
			// Byte/word writes merge into the old value; only foldable to a
			// full constant when the old value is known (or size is long).
			if size != m68kFoldSizeL && !known[dn] {
				known[dn] = false
				continue
			}
			full := m68kFoldMerge(val[dn], imm, size)
			ccr := m68kFoldNZ(imm, size)
			fold(m68kFoldEntry{
				folded: true, setsReg: true, reg: uint8(dn), value: full,
				ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
				ccrVal:  ccr,
			}, dn, full, true)

		case (op&0xF000 == 0x1000 || op&0xF000 == 0x2000 || op&0xF000 == 0x3000) &&
			(op>>6)&7 == 0 && (op>>3)&7 == 0:
			// MOVE.{B,W,L} Dm,Dn
			size := m68kFoldMoveSize(op)
			src := op & 7
			dn := (op >> 9) & 7
			if !known[src] || (size != m68kFoldSizeL && !known[dn]) {
				known[dn] = size == m68kFoldSizeL && known[src]
				if known[dn] {
					val[dn] = val[src]
				}
				// Not folded (unknown input): the normal emitter runs.
				if !known[dn] {
					known[dn] = false
				}
				continue
			}
			full := m68kFoldMerge(val[dn], val[src], size)
			ccr := m68kFoldNZ(val[src], size)
			fold(m68kFoldEntry{
				folded: true, setsReg: true, reg: uint8(dn), value: full,
				ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
				ccrVal:  ccr,
			}, dn, full, true)

		case op&0xF138 == 0x5000 && (op>>6)&3 != 3: // ADDQ #q,Dn
			dn := op & 7
			size := m68kFoldSize((op >> 6) & 3)
			q := uint32((op >> 9) & 7)
			if q == 0 {
				q = 8
			}
			if !known[dn] {
				continue
			}
			res, ccr := m68kFoldAdd(val[dn], q, size)
			full := m68kFoldMerge(val[dn], res, size)
			fold(m68kFoldEntry{
				folded: true, setsReg: true, reg: uint8(dn), value: full,
				ccrMask: 0x1F, ccrVal: ccr | (ccr&m68kFoldCCR_C)<<4,
			}, dn, full, true)

		case op&0xF138 == 0x5100 && (op>>6)&3 != 3: // SUBQ #q,Dn
			dn := op & 7
			size := m68kFoldSize((op >> 6) & 3)
			q := uint32((op >> 9) & 7)
			if q == 0 {
				q = 8
			}
			if !known[dn] {
				continue
			}
			res, ccr := m68kFoldSub(val[dn], q, size)
			full := m68kFoldMerge(val[dn], res, size)
			fold(m68kFoldEntry{
				folded: true, setsReg: true, reg: uint8(dn), value: full,
				ccrMask: 0x1F, ccrVal: ccr | (ccr&m68kFoldCCR_C)<<4,
			}, dn, full, true)

		case op&0xFF38 == 0x0600 || op&0xFF38 == 0x0400 || // ADDI/SUBI #imm,Dn
			op&0xFF38 == 0x0200 || op&0xFF38 == 0x0000 || // ANDI/ORI #imm,Dn
			op&0xFF38 == 0x0A00 || op&0xFF38 == 0x0C00: // EORI/CMPI #imm,Dn
			sizeBits := (op >> 6) & 3
			if sizeBits == 3 {
				clearAll()
				continue
			}
			size := m68kFoldSize(sizeBits)
			dn := op & 7
			imm, ok := m68kFoldImmediate(memory, extPC, size)
			if !ok || !known[dn] {
				if op&0x0F00 != 0x0C00 { // CMPI writes no register
					known[dn] = false
				}
				continue
			}
			kind := (op >> 8) & 0xF
			switch kind {
			case 0x6: // ADDI
				res, ccr := m68kFoldAdd(val[dn], imm, size)
				full := m68kFoldMerge(val[dn], res, size)
				fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
					ccrMask: 0x1F, ccrVal: ccr | (ccr&m68kFoldCCR_C)<<4}, dn, full, true)
			case 0x4: // SUBI
				res, ccr := m68kFoldSub(val[dn], imm, size)
				full := m68kFoldMerge(val[dn], res, size)
				fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
					ccrMask: 0x1F, ccrVal: ccr | (ccr&m68kFoldCCR_C)<<4}, dn, full, true)
			case 0x2, 0x0, 0xA: // ANDI/ORI/EORI: N,Z set; V,C cleared; X preserved
				var res uint32
				switch kind {
				case 0x2:
					res = val[dn] & imm
				case 0x0:
					res = val[dn] | imm
				default:
					res = val[dn] ^ imm
				}
				full := m68kFoldMerge(val[dn], res, size)
				ccr := m68kFoldNZ(res, size)
				fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
					ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
					ccrVal:  ccr}, dn, full, true)
			case 0xC: // CMPI: NZVC only, X and register untouched
				_, ccr := m68kFoldSub(val[dn], imm, size)
				plan[i] = m68kFoldEntry{folded: true,
					ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
					ccrVal:  ccr}
				any = true
			}

		case (op&0xF000 == 0xD000 || op&0xF000 == 0x9000 ||
			op&0xF000 == 0xC000 || op&0xF000 == 0x8000 || op&0xF000 == 0xB000) &&
			(op>>6)&7 <= 2 && (op>>3)&7 == 0:
			// ADD/SUB/AND/OR/CMP.{B,W,L} Dm,Dn (EA-to-register opmodes 0-2)
			size := m68kFoldSize((op >> 6) & 3)
			src := op & 7
			dn := (op >> 9) & 7
			isCmp := op&0xF000 == 0xB000
			if !known[src] || !known[dn] {
				if !isCmp {
					known[dn] = false
				}
				continue
			}
			switch op & 0xF000 {
			case 0xD000:
				res, ccr := m68kFoldAdd(val[dn], val[src], size)
				full := m68kFoldMerge(val[dn], res, size)
				fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
					ccrMask: 0x1F, ccrVal: ccr | (ccr&m68kFoldCCR_C)<<4}, dn, full, true)
			case 0x9000:
				res, ccr := m68kFoldSub(val[dn], val[src], size)
				full := m68kFoldMerge(val[dn], res, size)
				fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
					ccrMask: 0x1F, ccrVal: ccr | (ccr&m68kFoldCCR_C)<<4}, dn, full, true)
			case 0xC000, 0x8000:
				var res uint32
				if op&0xF000 == 0xC000 {
					res = val[dn] & val[src]
				} else {
					res = val[dn] | val[src]
				}
				full := m68kFoldMerge(val[dn], res, size)
				ccr := m68kFoldNZ(res, size)
				fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
					ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
					ccrVal:  ccr}, dn, full, true)
			default: // CMP
				_, ccr := m68kFoldSub(val[dn], val[src], size)
				plan[i] = m68kFoldEntry{folded: true,
					ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
					ccrVal:  ccr}
				any = true
			}

		case op&0xF100 == 0xB100 && (op>>6)&3 != 3 && (op>>3)&7 == 0:
			// EOR.{B,W,L} Dm,Dn (register destination form)
			size := m68kFoldSize((op >> 6) & 3)
			src := (op >> 9) & 7
			dn := op & 7
			if !known[src] || !known[dn] {
				known[dn] = false
				continue
			}
			res := val[dn] ^ val[src]
			full := m68kFoldMerge(val[dn], res, size)
			ccr := m68kFoldNZ(res, size)
			fold(m68kFoldEntry{folded: true, setsReg: true, reg: uint8(dn), value: full,
				ccrMask: m68kFoldCCR_N | m68kFoldCCR_Z | m68kFoldCCR_V | m68kFoldCCR_C,
				ccrVal:  ccr}, dn, full, true)

		default:
			// Outside the whitelist: conservatively drop everything.
			clearAll()
		}
	}
	if !any {
		return nil
	}
	return plan
}

func m68kFoldMoveSize(op uint16) m68kFoldSize {
	switch op & 0xF000 {
	case 0x1000:
		return m68kFoldSizeB
	case 0x3000:
		return m68kFoldSizeW
	}
	return m68kFoldSizeL
}

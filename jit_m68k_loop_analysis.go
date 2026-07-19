// jit_m68k_loop_analysis.go - M68020 in-block loop analyses (milestone 7
// optimisation slices: bounded counter-loop budget removal; loop-invariant
// facts for the hoisting slice).
//
// Bounded counter loop: a DBcc whose 16-bit counter is seeded by a
// compile-time constant immediately before the loop head, and whose body
// never rewrites the counter, retires at most (seed.W + 1) iterations.
// When that worst case (plus the block's linear prefix and tail) fits
// inside the per-entry loop budget, the per-iteration budget and safety
// checks are provably redundant and the backend may omit them. The
// retired-count accounting is NOT removable: every committed back-jump
// still accumulates into ChainCount/LoopCount.
//
// Proof obligations, all compile-time:
//   1. The DBcc's static target lies inside the block, at least one
//      instruction after the start (room for the seed).
//   2. instrs[target-1] is MOVEQ #imm,Dn or MOVE.W #imm,Dn seeding the
//      DBcc counter register.
//   3. No instruction in [target, dbcc] writes the counter register
//      (the DBcc decrement itself is the only writer).
//   4. No branch instruction inside the body other than the DBcc itself:
//      a branch into the body could skip the seed or the decrement.
//   5. No other in-block branch targets any instruction in [target, dbcc].
//   6. (trips * bodySize) + len(instrs) <= budget.
//
// Architecture-neutral and untagged so every backend applies the same
// proof.

package main

import (
	"os"
	"sync/atomic"
)

// Kill switch: IE_M68K_JIT_DISABLE_BOUNDED_LOOP=1 keeps the per-iteration
// budget checks on every loop.
var m68kJITBoundedLoopDisabled = os.Getenv("IE_M68K_JIT_DISABLE_BOUNDED_LOOP") == "1"

// m68kInstrWritesDataReg reports whether the instruction certainly-or-
// possibly writes the given data register. Conservative: any instruction
// not understood is treated as writing it.
func m68kInstrWritesDataReg(ji *M68KJITInstr, dn uint16) bool {
	op := ji.opcode
	switch {
	case op == 0x4E71: // NOP
		return false
	case op&0xF100 == 0x7000: // MOVEQ
		return (op>>9)&7 == dn
	case op&0xC000 == 0x0000 && op&0xF000 != 0x0000: // MOVE.B/W/L family (groups 1-3)
		g := op >> 12
		if g == 1 || g == 2 || g == 3 {
			dstMode := (op >> 6) & 7
			dstReg := (op >> 9) & 7
			return dstMode == 0 && dstReg == dn
		}
		return true
	case op&0xF000 == 0xD000 || op&0xF000 == 0x9000 ||
		op&0xF000 == 0xC000 || op&0xF000 == 0x8000: // ADD/SUB/AND/OR
		opmode := (op >> 6) & 7
		if opmode <= 2 { // <ea>,Dn
			return (op>>9)&7 == dn
		}
		return false // Dn,<ea> writes memory (or An for ADDA/SUBA)
	case op&0xF000 == 0xB000: // CMP/CMPA/EOR
		opmode := (op >> 6) & 7
		if opmode <= 2 || opmode == 3 || opmode == 7 { // CMP/CMPA
			return false
		}
		return op&7 == dn && (op>>3)&7 == 0 // EOR Dm,Dn
	case op&0xF100 == 0x5000 || op&0xF100 == 0x5100: // ADDQ/SUBQ
		if (op>>6)&3 == 3 {
			return true // Scc/DBcc handled elsewhere; conservative
		}
		return (op>>3)&7 == 0 && op&7 == dn
	case op&0xFF00 == 0x4A00: // TST
		return false
	case op&0xF138 == 0xE000 || op&0xF138 == 0xE008 ||
		op&0xF138 == 0xE010 || op&0xF138 == 0xE018 ||
		op&0xF138 == 0xE100 || op&0xF138 == 0xE108 ||
		op&0xF138 == 0xE110 || op&0xF138 == 0xE118: // shift/rotate imm form, Dn dest
		return op&7 == dn
	}
	return true
}

// m68kInstrIsBranch reports whether the instruction is any control-flow
// transfer (Bcc/BRA/BSR, DBcc, JMP, JSR, RTS/RTE/RTR).
func m68kInstrIsBranch(op uint16) bool {
	switch {
	case op&0xF000 == 0x6000:
		return true
	case op&0xF0F8 == 0x50C8:
		return true
	case op&0xFFC0 == 0x4EC0 || op&0xFFC0 == 0x4E80: // JMP/JSR
		return true
	case op == 0x4E75 || op == 0x4E73 || op == 0x4E77: // RTS/RTE/RTR
		return true
	}
	return false
}

// ===========================================================================
// Invariant memory-check hoisting (milestone 7 slice)
// ===========================================================================
//
// A (d16,An) access inside a single-entry DBcc loop whose base register is
// never written anywhere up to and including the DBcc has the same
// effective address on every iteration. Its RAM-bounds and I/O guards can
// run once, in a loop precheck at block entry, and be elided inside the
// loop. If the precheck fails the block bails with nothing retired, so the
// dispatcher falls back to the interpreter for the architectural path.
// Store-side SMC checks are never elided.
//
// Instruction hoisting proper is rejected by M68020 semantics: every ALU
// instruction writes the CCR and the CCR is observable at the DBcc on each
// iteration, so a hoisted body instruction would change architectural
// state. The constant-folding slice covers the profitable constant subset.

// m68kLoopHoistEmits counts blocks compiled with a hoisted-guard loop
// precheck. Shape tests read it to prove the hoist was applied.
var m68kLoopHoistEmits atomic.Uint64

// Kill switch: IE_M68K_JIT_DISABLE_LOOP_HOIST=1 keeps every per-iteration
// guard.
var m68kJITLoopHoistDisabled = os.Getenv("IE_M68K_JIT_DISABLE_LOOP_HOIST") == "1"

// m68kColdExitOutlines counts blocks compiled with outlined cold exits
// (milestone 7 cold-exit outlining slice, native backends only). Shape
// tests read it to prove the layout was applied.
var m68kColdExitOutlines atomic.Uint64

// Kill switch: IE_M68K_JIT_DISABLE_COLD_OUTLINE=1 restores inline exits.
var m68kJITColdOutlineDisabled = os.Getenv("IE_M68K_JIT_DISABLE_COLD_OUTLINE") == "1"

// m68kIndirectCacheEmits counts indirect JMP/JSR exits compiled with the
// inline target-cache probe (milestone 7 indirect-target specialisation).
var m68kIndirectCacheEmits atomic.Uint64

// Kill switch: IE_M68K_JIT_DISABLE_INDIRECT_CACHE=1 keeps plain unchained
// exits on dynamic JMP/JSR targets.
var m68kJITIndirectCacheDisabled = os.Getenv("IE_M68K_JIT_DISABLE_INDIRECT_CACHE") == "1"

// m68kRegionResidencyEmits counts regions compiled with a custom pin map
// (milestone 7 region GPR residency, amd64).
var m68kRegionResidencyEmits atomic.Uint64

// Kill switch: IE_M68K_JIT_DISABLE_REGION_RESIDENCY=1 keeps the fixed map
// inside regions.
var m68kJITRegionResidencyDisabled = os.Getenv("IE_M68K_JIT_DISABLE_REGION_RESIDENCY") == "1"

// m68kLoopAccess is one invariant (d16,An) access validated by the loop
// precheck.
type m68kLoopAccess struct {
	an    uint16
	disp  int32
	width uint32
}

// m68kLoopHoistPlan carries the precheck accesses and the set of
// instruction indices whose per-iteration guards are elided.
type m68kLoopHoistPlan struct {
	accesses []m68kLoopAccess
	elide    map[int]bool
}

// m68kLoopInstrMayWriteAddrReg reports whether the instruction may write
// the given address register (including via (An)+ / -(An) side effects).
// Conservative: unknown instructions are treated as writing it. Distinct
// from m68kInstrWritesAddrReg (jit_m68k_exec.go), which under-approximates
// for a different contract.
func m68kLoopInstrMayWriteAddrReg(ji *M68KJITInstr, an uint16) bool {
	op := ji.opcode
	eaTouches := func(mode, reg uint16) bool {
		return (mode == 3 || mode == 4) && reg == an
	}
	switch {
	case op == 0x4E71: // NOP
		return false
	case op&0xF100 == 0x7000: // MOVEQ
		return false
	case op&0xF0F8 == 0x50C8: // DBcc writes only Dn
		return false
	case op&0xC000 == 0x0000 && op>>12 >= 1 && op>>12 <= 3: // MOVE families
		srcMode := (op >> 3) & 7
		srcReg := op & 7
		dstMode := (op >> 6) & 7
		dstReg := (op >> 9) & 7
		if dstMode == 1 && dstReg == an { // MOVEA to An
			return true
		}
		return eaTouches(srcMode, srcReg) || eaTouches(dstMode, dstReg)
	case op&0xF000 == 0xD000 || op&0xF000 == 0x9000: // ADD/SUB incl. ADDA/SUBA
		opmode := (op >> 6) & 7
		if opmode == 3 || opmode == 7 { // ADDA/SUBA
			return (op>>9)&7 == an
		}
		return eaTouches((op>>3)&7, op&7)
	case op&0xF000 == 0xC000 || op&0xF000 == 0x8000 || op&0xF000 == 0xB000:
		// AND/OR/CMP/EOR: CMPA writes nothing; others never write An
		return eaTouches((op>>3)&7, op&7)
	case op&0xF138 == 0x5000 || op&0xF138 == 0x5100: // ADDQ/SUBQ
		mode := (op >> 3) & 7
		reg := op & 7
		if mode == 1 && reg == an { // ADDQ/SUBQ to An
			return true
		}
		return eaTouches(mode, reg)
	case op&0xFF00 == 0x4A00: // TST
		return eaTouches((op>>3)&7, op&7)
	case op&0xF138 == 0xE000 || op&0xF138 == 0xE008 ||
		op&0xF138 == 0xE010 || op&0xF138 == 0xE018 ||
		op&0xF138 == 0xE100 || op&0xF138 == 0xE108 ||
		op&0xF138 == 0xE110 || op&0xF138 == 0xE118: // shift/rotate imm, Dn
		return false
	}
	return true
}

// m68kLoopGuardedMoveAccess recognises the MOVE shapes handled by the
// guarded MOVE emitter with a (d16,An) memory side: MOVE <(d16,An)>,Dn and
// MOVE Dn/#imm,(d16,An). Returns the access on match.
func m68kLoopGuardedMoveAccess(ji *M68KJITInstr, memory []byte, startPC uint32) (m68kLoopAccess, bool) {
	op := ji.opcode
	g := op >> 12
	if g != 1 && g != 2 && g != 3 {
		return m68kLoopAccess{}, false
	}
	var width uint32
	switch g {
	case 1:
		width = 1
	case 3:
		width = 2
	default:
		width = 4
	}
	srcMode := (op >> 3) & 7
	srcReg := op & 7
	dstMode := (op >> 6) & 7
	dstReg := (op >> 9) & 7
	extPC := startPC + ji.pcOffset + 2
	readDisp := func(pc uint32) (int32, bool) {
		if pc+2 > uint32(len(memory)) {
			return 0, false
		}
		return int32(int16(uint16(memory[pc])<<8 | uint16(memory[pc+1]))), true
	}
	if srcMode == 5 && dstMode == 0 { // MOVE (d16,An),Dn
		d, ok := readDisp(extPC)
		if !ok {
			return m68kLoopAccess{}, false
		}
		return m68kLoopAccess{an: srcReg, disp: d, width: width}, true
	}
	if dstMode == 5 && (srcMode == 0 || (srcMode == 7 && srcReg == 4 && g != 1)) {
		// MOVE Dn,(d16,An) or MOVE #imm,(d16,An)
		dispPC := extPC
		if srcMode == 7 { // skip the immediate words
			if g == 2 {
				dispPC += 4
			} else {
				dispPC += 2
			}
		}
		d, ok := readDisp(dispPC)
		if !ok {
			return m68kLoopAccess{}, false
		}
		return m68kLoopAccess{an: dstReg, disp: d, width: width}, true
	}
	return m68kLoopAccess{}, false
}

// m68kAnalyseLoopInvariantGuards finds the first single-entry DBcc loop and
// returns the invariant-guard hoist plan, or nil.
func m68kAnalyseLoopInvariantGuards(instrs []M68KJITInstr, startPC uint32, memory []byte) *m68kLoopHoistPlan {
	if m68kJITLoopHoistDisabled || memory == nil {
		return nil
	}
	for dbccIdx := range instrs {
		ji := &instrs[dbccIdx]
		if ji.opcode&0xF0F8 != 0x50C8 {
			continue
		}
		pc := startPC + ji.pcOffset
		if pc+4 > uint32(len(memory)) {
			continue
		}
		d := int32(int16(uint16(memory[pc+2])<<8 | uint16(memory[pc+3])))
		target := uint32(int64(pc) + 2 + int64(d))
		if target < startPC || target >= pc {
			continue
		}
		targetIdx := -1
		for i := range instrs {
			if instrs[i].pcOffset == target-startPC {
				targetIdx = i
				break
			}
		}
		if targetIdx < 0 || targetIdx >= dbccIdx {
			continue
		}
		// Single entry: no other in-block branch targets the loop range.
		loopLo := instrs[targetIdx].pcOffset
		loopHi := instrs[dbccIdx].pcOffset
		singleEntry := true
		for off := range m68kFoldInBlockBranchTargets(instrs[:dbccIdx], startPC, memory) {
			if off >= loopLo && off <= loopHi {
				singleEntry = false
				break
			}
		}
		if !singleEntry {
			continue
		}
		// No nested control flow inside the body.
		nested := false
		for i := targetIdx; i < dbccIdx; i++ {
			if m68kInstrIsBranch(instrs[i].opcode) || instrs[i].fusedFlag != 0 {
				nested = true
				break
			}
		}
		if nested {
			continue
		}

		plan := &m68kLoopHoistPlan{}
		seen := map[m68kLoopAccess]bool{}
		for i := targetIdx; i < dbccIdx; i++ {
			acc, ok := m68kLoopGuardedMoveAccess(&instrs[i], memory, startPC)
			if !ok {
				continue
			}
			// Base invariant across the whole prefix and loop: its value at
			// block entry is its value on every iteration.
			invariant := true
			for j := 0; j <= dbccIdx; j++ {
				if m68kLoopInstrMayWriteAddrReg(&instrs[j], acc.an) {
					invariant = false
					break
				}
			}
			if !invariant {
				continue
			}
			if !seen[acc] {
				if len(plan.accesses) >= 4 {
					continue // precheck cap; further accesses keep their guards
				}
				seen[acc] = true
				plan.accesses = append(plan.accesses, acc)
			}
			if plan.elide == nil {
				plan.elide = make(map[int]bool, 4)
			}
			plan.elide[i] = true
		}
		if len(plan.accesses) == 0 {
			return nil
		}
		return plan
	}
	return nil
}

// m68kBoundedCounterDBccLoop proves the budget checks redundant for the
// back edge of the DBcc at instrIdx targeting targetIdx. budget is the
// per-entry loop budget the backend enforces (m68kJitBudget on amd64).
func m68kBoundedCounterDBccLoop(instrs []M68KJITInstr, instrIdx, targetIdx int,
	memory []byte, startPC uint32, budget uint32) bool {
	if m68kJITBoundedLoopDisabled || memory == nil {
		return false
	}
	if targetIdx < 1 || instrIdx <= targetIdx || instrIdx >= len(instrs) {
		return false
	}
	dbcc := &instrs[instrIdx]
	if dbcc.opcode&0xF0F8 != 0x50C8 {
		return false
	}
	counter := dbcc.opcode & 7

	// Obligation 2: constant seed of the counter's low word directly
	// before the loop head.
	seed := &instrs[targetIdx-1]
	var lowWord uint32
	switch {
	case seed.opcode&0xF100 == 0x7000 && (seed.opcode>>9)&7 == counter: // MOVEQ
		lowWord = uint32(int32(int8(seed.opcode&0xFF))) & 0xFFFF
	case seed.opcode&0xF1FF == 0x303C && counter == (seed.opcode>>9)&7:
		// MOVE.W #imm,Dn is 0x303C|dn<<9 (dst mode 0, src mode 7 reg 4)
		extPC := startPC + seed.pcOffset + 2
		if extPC+2 > uint32(len(memory)) {
			return false
		}
		lowWord = uint32(memory[extPC])<<8 | uint32(memory[extPC+1])
	default:
		return false
	}
	trips := uint64(lowWord) + 1

	// Obligations 3 and 4 over the body [targetIdx, instrIdx).
	for i := targetIdx; i < instrIdx; i++ {
		ji := &instrs[i]
		if ji.fusedFlag != 0 {
			return false
		}
		if m68kInstrIsBranch(ji.opcode) {
			return false
		}
		if m68kInstrWritesDataReg(ji, counter) {
			return false
		}
	}

	// Obligation 5: no other in-block branch may target the loop range.
	loopLo := instrs[targetIdx].pcOffset
	loopHi := instrs[instrIdx].pcOffset
	targets := m68kFoldInBlockBranchTargets(instrs[:instrIdx], startPC, memory)
	for off := range targets {
		if off >= loopLo && off <= loopHi {
			return false
		}
	}

	// Obligation 6: worst-case retirement fits the budget.
	bodySize := uint64(instrIdx - targetIdx + 1)
	total := trips*bodySize + uint64(len(instrs))
	return total <= uint64(budget)
}

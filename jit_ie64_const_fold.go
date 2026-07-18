// jit_ie64_const_fold.go - Shared constant-only folding analysis for the
// IE64 JIT backends (amd64, ARM64 and wasm).
//
// This file is part of the Intuition Engine project.
// Copyright (c) 2024 - 2026 Zayn Otley
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU Affero General Public License as
// published by the Free Software Foundation, either version 3 of the
// License, or (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU Affero General Public License for more details.
//
// You should have received a copy of the GNU Affero General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package main

import "sync/atomic"

// ie64FoldedConstEmits counts folded constants actually emitted by any
// backend. Structural tests read it to prove the fold was applied.
var ie64FoldedConstEmits atomic.Uint64

// ie64ConstFoldDisabled turns the analysis off. Benchmark-only toggle so the
// folded and unfolded variants run under identical conditions in one binary.
var ie64ConstFoldDisabled bool

// ie64ActiveFoldPlan is the emit-time fold plan for the native backends,
// aligned with the flat instruction index the compilers already track in
// ie64CurrentLoopInstr. Compilers are serialised (see ie64CompileMu and the
// existing ie64ActiveLoopPlan pattern), so package-local state is safe here.
var ie64ActiveFoldPlan []ie64FoldEntry

// ie64FoldEntryAt returns the active plan entry for flat index i, or a zero
// entry when no plan is active or the index is out of range.
func ie64FoldEntryAt(i int) ie64FoldEntry {
	if ie64ActiveFoldPlan == nil || i < 0 || i >= len(ie64ActiveFoldPlan) {
		return ie64FoldEntry{}
	}
	return ie64ActiveFoldPlan[i]
}

// ie64AnalyseRegionConstFold folds each region block independently and
// returns a plan aligned with the region's flat instruction indices. Block
// heads are region-internal branch targets, so tracked constants never cross
// a block boundary. Returns nil when nothing folds.
func ie64AnalyseRegionConstFold(blocks [][]JITInstr, blockPCs []uint64) []ie64FoldEntry {
	var flat []ie64FoldEntry
	any := false
	for bi, blk := range blocks {
		p := ie64AnalyseConstFold(blk, blockPCs[bi])
		if p == nil {
			flat = append(flat, make([]ie64FoldEntry, len(blk))...)
			continue
		}
		any = true
		flat = append(flat, p...)
	}
	if !any {
		return nil
	}
	return flat
}

// ie64FoldEntry is one analysis result aligned with the input instruction
// slice. When folded is true the backend may emit the precomputed value
// through its normal destination-write path instead of the instruction body.
// No source-register facts are exposed.
type ie64FoldEntry struct {
	folded bool
	value  uint64
}

// ie64ConstFoldSupported reports whether the opcode is in the folding
// whitelist. Only pure integer instructions whose interpreter semantics are
// replicated exactly by ie64ConstFoldEval may appear here. The list is
// shared with loop hoisting (ie64HoistableOpcode).
func ie64ConstFoldSupported(op byte) bool {
	return ie64HoistableOpcode(op)
}

// ie64ConstFoldBarrier reports whether the opcode clears all tracked
// constants: memory traffic, FP state, control flow and system/atomic
// instructions. R0 is restored to known zero afterwards by the caller.
func ie64ConstFoldBarrier(op byte) bool {
	switch {
	case op == OP_LOAD || op == OP_STORE:
		return true
	case op >= OP_BRA && op <= OP_JSR_IND: // branches, JMP, JSR/RTS, PUSH/POP
		return true
	case op >= OP_FMOV && op <= OP_DPOW: // FP32 and FP64 families
		return true
	case op >= OP_HALT64 && op <= OP_SUADIS: // system, MMU, atomics
		return true
	}
	return false
}

// ie64AnalyseConstFold runs a forward constant analysis over one basic
// block. It returns a slice aligned with instrs, or nil when nothing folds.
// startPC anchors intra-block branch targets: any instruction that is the
// target of an in-block branch is reachable on a path that skips earlier
// constant setters, so tracked constants are cleared there.
func ie64AnalyseConstFold(instrs []JITInstr, startPC uint64) []ie64FoldEntry {
	if ie64ConstFoldDisabled {
		return nil
	}
	var known uint32 = 1 // R0 is permanently known zero
	var vals [32]uint64

	clearAll := func() {
		known = 1 // restore R0 as known zero
	}

	// Collect intra-block branch-target instruction indices.
	branchTarget := make(map[uint32]bool)
	for i := range instrs {
		in := &instrs[i]
		if in.opcode != OP_BRA && !ie64ConditionalOpcode(in.opcode) {
			continue
		}
		pc := startPC + uint64(in.pcOffset)
		target := uint64(int64(pc) + int64(int32(in.imm32)))
		if target < startPC {
			continue
		}
		off := target - startPC
		if off%IE64_INSTR_SIZE == 0 {
			branchTarget[uint32(off)] = true
		}
	}

	result := make([]ie64FoldEntry, len(instrs))
	anyFolded := false

	for i := range instrs {
		in := &instrs[i]
		if branchTarget[in.pcOffset] {
			clearAll()
		}
		if in.fusedFlag != 0 {
			clearAll()
			continue
		}
		if ie64ConstFoldBarrier(in.opcode) {
			clearAll()
			continue
		}
		if !ie64ConstFoldSupported(in.opcode) {
			known &^= instrWrittenRegs(in)
			continue
		}
		value, ok := ie64ConstFoldEval(in, known, &vals)
		if !ok {
			if in.rd != 0 {
				known &^= 1 << in.rd
			}
			continue
		}
		if in.rd == 0 {
			// The write is architecturally ignored; the normal emit path
			// already no-ops it. R0 stays known zero.
			continue
		}
		known |= 1 << in.rd
		vals[in.rd] = value
		result[i] = ie64FoldEntry{folded: true, value: value}
		anyFolded = true
	}
	if !anyFolded {
		return nil
	}
	return result
}

// ie64ConstFoldEval computes the instruction's architectural result when all
// inputs are known. It mirrors the interpreter's semantics exactly,
// including size masking, sign extension and shift-count masking.
func ie64ConstFoldEval(in *JITInstr, known uint32, vals *[32]uint64) (uint64, bool) {
	regKnown := func(r byte) bool { return known&(1<<r) != 0 }
	reg := func(r byte) uint64 {
		if r == 0 {
			return 0
		}
		return vals[r]
	}
	// operand3 resolves like the interpreter: zero-extended imm32 when
	// xbit is set, otherwise register Rt.
	operand3 := func() (uint64, bool) {
		if in.xbit == 1 {
			return uint64(in.imm32), true
		}
		if !regKnown(in.rt) {
			return 0, false
		}
		return reg(in.rt), true
	}

	switch in.opcode {
	case OP_MOVE:
		if in.xbit == 1 {
			return maskToSize(uint64(in.imm32), in.size), true
		}
		if !regKnown(in.rs) {
			return 0, false
		}
		return maskToSize(reg(in.rs), in.size), true
	case OP_MOVEQ:
		return uint64(int64(int32(in.imm32))), true
	case OP_LEA:
		if !regKnown(in.rs) {
			return 0, false
		}
		return uint64(int64(reg(in.rs)) + int64(int32(in.imm32))), true
	case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR:
		if !regKnown(in.rs) {
			return 0, false
		}
		op3, ok := operand3()
		if !ok {
			return 0, false
		}
		a := reg(in.rs)
		var r uint64
		switch in.opcode {
		case OP_ADD:
			r = a + op3
		case OP_SUB:
			r = a - op3
		case OP_AND64:
			r = a & op3
		case OP_OR64:
			r = a | op3
		case OP_EOR:
			r = a ^ op3
		}
		return maskToSize(r, in.size), true
	case OP_LSL, OP_LSR, OP_ASR:
		if !regKnown(in.rs) {
			return 0, false
		}
		op3, ok := operand3()
		if !ok {
			return 0, false
		}
		shift := op3 & 63
		switch in.opcode {
		case OP_LSL:
			return maskToSize(reg(in.rs)<<shift, in.size), true
		case OP_LSR:
			return maskToSize(reg(in.rs)>>shift, in.size), true
		default: // OP_ASR
			var sval int64
			switch in.size {
			case IE64_SIZE_B:
				sval = int64(int8(reg(in.rs)))
			case IE64_SIZE_W:
				sval = int64(int16(reg(in.rs)))
			case IE64_SIZE_L:
				sval = int64(int32(reg(in.rs)))
			case IE64_SIZE_Q:
				sval = int64(reg(in.rs))
			}
			return maskToSize(uint64(sval>>shift), in.size), true
		}
	}
	return 0, false
}

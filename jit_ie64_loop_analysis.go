package main

import "sync/atomic"

// ie64LoopHoistEmits counts loops compiled with a hoisted invariant
// instruction. Structural tests read it to prove the hoist was applied.
var ie64LoopHoistEmits atomic.Uint64

// ie64LoopHoistDisabled turns hoist selection off. Benchmark-only toggle so
// both layouts run under identical conditions in one binary.
var ie64LoopHoistDisabled bool

// jitFallback values are an internal dispatcher protocol, not guest ABI.
const (
	jitFallbackMemory       uint32 = 1
	jitFallbackLoopPrecheck uint32 = 2
	ie64JITLoopBudget              = 4095
)

type ie64LoopAccess struct {
	base  byte
	disp  int32
	width uint32
}

type ie64LoopPlan struct {
	head, back int
	headPC     uint64
	prefix     uint32
	accesses   []ie64LoopAccess
	hoisted    map[int]bool
	bounded    bool
	count      uint64
	bodySize   uint32
	// hoists lists the loop-invariant integer instructions emitted once
	// before the loop-head label, in program order, and suppressed inside
	// the loop. Dependent invariant chains are allowed when the producer
	// precedes the consumer. Only pure integer single-block loops qualify;
	// region plans always carry an empty list. hoistSet mirrors hoists for
	// O(1) suppression checks at emit time.
	hoists   []int
	hoistSet map[int]bool
}

// Compilers are serialised by ie64CompileMu, so the emit-time plan can remain
// package-local state like the existing FP residency and count-base plans.
var ie64ActiveLoopPlan *ie64LoopPlan
var ie64CurrentLoopInstr = -1

// ie64AnalyseLoop returns the first conservative, single-entry loop in a
// flattened compilation unit. Both native emitters and wasm use this result.
func ie64AnalyseLoop(instrs []JITInstr, startPC uint64) *ie64LoopPlan {
	for back := range instrs {
		br := &instrs[back]
		if br.opcode != OP_BNE && br.opcode != OP_BRA {
			continue
		}
		pc := startPC + uint64(br.pcOffset)
		target := uint64(int64(pc) + int64(int32(br.imm32)))
		if target < startPC || target >= pc || (target-startPC)%IE64_INSTR_SIZE != 0 {
			continue
		}
		head := -1
		for i := range instrs {
			if uint64(instrs[i].pcOffset) == target-startPC {
				head = i
				break
			}
		}
		if head < 0 || head >= back || ie64LoopHasExtraEntry(instrs, startPC, head, back) {
			continue
		}
		p := &ie64LoopPlan{head: head, back: back, headPC: target, prefix: uint32(head), bodySize: uint32(back - head + 1)}
		written := uint32(0)
		for i := head; i <= back; i++ {
			written |= instrWrittenRegs(&instrs[i])
		}
		seen := make(map[ie64LoopAccess]struct{})
		p.hoisted = make(map[int]bool)
		validMem := true
		for i := head; i < back; i++ {
			in := &instrs[i]
			switch in.opcode {
			case OP_LOAD, OP_STORE, OP_DLOAD, OP_DSTORE:
				if in.mmuBail || in.rs == 31 || written&(1<<in.rs) != 0 || (in.opcode == OP_LOAD && in.rd == 0) {
					validMem = false
					continue
				}
				width := ie64AccessBytes(in.size)
				if in.opcode == OP_DLOAD || in.opcode == OP_DSTORE {
					width = 8
				}
				a := ie64LoopAccess{base: in.rs, disp: int32(in.imm32), width: width}
				if _, ok := seen[a]; !ok {
					seen[a] = struct{}{}
					p.accesses = append(p.accesses, a)
				}
				p.hoisted[i] = true
			case OP_PUSH64, OP_POP64, OP_JSR64, OP_JSR_IND, OP_RTS64:
				validMem = false
			}
		}
		if !validMem {
			p.accesses = nil
			p.hoisted = nil
		}
		p.bounded, p.count = ie64BoundedCounterLoop(instrs, p)
		if len(p.accesses) == 0 {
			p.hoists = ie64SelectLoopHoists(instrs, p)
			if len(p.hoists) != 0 {
				p.hoistSet = make(map[int]bool, len(p.hoists))
				for _, i := range p.hoists {
					p.hoistSet[i] = true
				}
			}
		}
		if len(p.accesses) != 0 || p.bounded || len(p.hoists) != 0 {
			return p
		}
	}
	return nil
}

// ie64SelectLoopHoists picks the loop-invariant integer instructions to emit
// once, in program order, before the loop-head label. The loop must be pure
// integer (no memory, FP, helpers, calls, returns or extra branches - the
// body opcodes are restricted to the bounded-integer set). Each candidate
// must come from the constant-folding opcode list, write its destination
// exactly once in the loop, take only immediate inputs, loop-invariant
// register inputs, or the destination of an already-hoisted instruction
// defined EARLIER in the body (a consumer preceding its producer would read
// the pre-loop value on iteration one), and its destination must not be read
// earlier in the body (same iteration-one hazard). Selection iterates to a
// fixpoint so dependent invariant chains hoist entirely. Returns nil when no
// instruction qualifies.
func ie64SelectLoopHoists(instrs []JITInstr, p *ie64LoopPlan) []int {
	if ie64LoopHoistDisabled {
		return nil
	}
	for i := p.head; i < p.back; i++ {
		in := &instrs[i]
		if in.fusedFlag != 0 || in.mmuBail || !ie64BoundedIntegerBodyOpcode(in.opcode) {
			return nil
		}
	}
	var writeCounts [32]int
	writtenInLoop := uint32(0)
	for i := p.head; i <= p.back; i++ {
		w := instrWrittenRegs(&instrs[i])
		writtenInLoop |= w
		for r := 1; r < 32; r++ {
			if w&(1<<r) != 0 {
				writeCounts[r]++
			}
		}
	}
	hoisted := make(map[int]bool)
	var hoistDef [32]int // hoisted defining instruction index per register
	for r := range hoistDef {
		hoistDef[r] = -1
	}
	for changed := true; changed; {
		changed = false
		for i := p.head; i < p.back; i++ {
			in := &instrs[i]
			if hoisted[i] || !ie64HoistableOpcode(in.opcode) || in.rd == 0 || writeCounts[in.rd] != 1 {
				continue
			}
			reads := ie64IntegerReadRegs(in)
			inputsInvariant := true
			for r := 1; r < 32; r++ {
				if reads&(1<<r) == 0 || writtenInLoop&(1<<r) == 0 {
					continue
				}
				if d := hoistDef[r]; d < 0 || d >= i {
					inputsInvariant = false
					break
				}
			}
			if !inputsInvariant {
				continue
			}
			destReadEarlier := false
			for j := p.head; j < i; j++ {
				if ie64IntegerReadRegs(&instrs[j])&(1<<in.rd) != 0 {
					destReadEarlier = true
					break
				}
			}
			if destReadEarlier {
				continue
			}
			hoisted[i] = true
			hoistDef[in.rd] = i
			changed = true
		}
	}
	if len(hoisted) == 0 {
		return nil
	}
	out := make([]int, 0, len(hoisted))
	for i := p.head; i < p.back; i++ {
		if hoisted[i] {
			out = append(out, i)
		}
	}
	return out
}

// ie64HoistableOpcode is the constant-folding opcode list: the pure integer
// instructions eligible for loop hoisting.
func ie64HoistableOpcode(op byte) bool {
	switch op {
	case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA, OP_ADD, OP_SUB,
		OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS,
		OP_NEG, OP_NOT64, OP_AND64, OP_OR64, OP_EOR,
		OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR,
		OP_CLZ, OP_CTZ, OP_POPCNT, OP_BSWAP, OP_SEXT:
		return true
	}
	return false
}

// ie64IntegerReadRegs returns the registers read by a pure integer body
// instruction (the bounded-integer opcode set plus the loop's final BNE).
func ie64IntegerReadRegs(in *JITInstr) uint32 {
	var r uint32
	switch in.opcode {
	case OP_NOP64, OP_MOVEQ:
	case OP_MOVE:
		if in.xbit == 0 {
			r = 1 << in.rs
		}
	case OP_MOVT:
		r = 1 << in.rd // read-modify-write of the low half
	case OP_LEA, OP_NEG, OP_NOT64, OP_CLZ, OP_CTZ, OP_POPCNT, OP_BSWAP, OP_SEXT:
		r = 1 << in.rs
	case OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS,
		OP_MULHU, OP_MULHS, OP_AND64, OP_OR64, OP_EOR,
		OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
		r = 1 << in.rs
		if in.xbit == 0 {
			r |= 1 << in.rt
		}
	case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
		r = 1<<in.rs | 1<<in.rt
	}
	return r &^ 1 // R0 reads are architecturally constant zero
}

func ie64CurrentAccessHoisted() bool {
	return ie64ActiveLoopPlan != nil && ie64ActiveLoopPlan.hoisted[ie64CurrentLoopInstr]
}

func ie64AnalyseRegionLoop(region *ie64Region) (*ie64LoopPlan, []JITInstr) {
	flat := make([]JITInstr, 0)
	for bi, block := range region.blocks {
		for _, in := range block {
			in.pcOffset = uint32(region.blockPCs[bi] + uint64(in.pcOffset) - region.entryPC)
			flat = append(flat, in)
		}
	}
	plan := ie64AnalyseLoop(flat, region.entryPC)
	if plan != nil {
		// Hoisting is single-block only: region back-edge machinery has its
		// own label placement and would bypass the hoisted instructions.
		plan.hoists, plan.hoistSet = nil, nil
		if len(plan.accesses) == 0 && !plan.bounded {
			plan = nil
		}
	}
	return plan, flat
}

func regionWrittenMask(blocks [][]JITInstr) uint32 {
	var w uint32
	for _, b := range blocks {
		for i := range b {
			w |= instrWrittenRegs(&b[i])
		}
	}
	return w
}

func ie64LoopHasExtraEntry(instrs []JITInstr, startPC uint64, head, back int) bool {
	for i := range instrs {
		in := &instrs[i]
		switch in.opcode {
		case OP_BRA, OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
		default:
			continue
		}
		pc := startPC + uint64(in.pcOffset)
		target := uint64(int64(pc) + int64(int32(in.imm32)))
		if target < startPC || (target-startPC)%IE64_INSTR_SIZE != 0 {
			continue
		}
		t := int((target - startPC) / IE64_INSTR_SIZE)
		if t >= head && t <= back && !(i == back && t == head) {
			return true
		}
	}
	return false
}

func ie64BoundedCounterLoop(instrs []JITInstr, p *ie64LoopPlan) (bool, uint64) {
	if p.head == 0 || p.back-p.head < 1 {
		return false, 0
	}
	seed, sub, br := &instrs[p.head-1], &instrs[p.back-1], &instrs[p.back]
	if seed.opcode != OP_MOVE || seed.size != IE64_SIZE_Q || seed.xbit != 1 || seed.rd == 0 || seed.imm32 == 0 ||
		sub.opcode != OP_SUB || sub.size != IE64_SIZE_Q || sub.xbit != 1 || sub.rd != seed.rd || sub.rs != seed.rd || sub.imm32 != 1 ||
		br.opcode != OP_BNE || br.rs != seed.rd || br.rt != 0 {
		return false, 0
	}
	writes := 0
	for i := p.head; i <= p.back; i++ {
		if instrWrittenRegs(&instrs[i])&(1<<seed.rd) != 0 {
			writes++
		}
	}
	if writes != 1 { // the final SUB is the counter's only loop write
		return false, 0
	}
	for i := p.head; i < p.back-1; i++ {
		if !ie64BoundedIntegerBodyOpcode(instrs[i].opcode) {
			return false, 0
		}
	}
	count := uint64(seed.imm32)
	retired := uint64(p.head) + count*uint64(p.bodySize)
	return retired <= ie64JITLoopBudget, count
}

func ie64BoundedIntegerBodyOpcode(op byte) bool {
	switch op {
	case OP_NOP64, OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA,
		OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS,
		OP_NEG, OP_MULHU, OP_MULHS, OP_AND64, OP_OR64, OP_EOR, OP_NOT64,
		OP_LSL, OP_LSR, OP_ASR, OP_CLZ, OP_CTZ, OP_POPCNT, OP_BSWAP, OP_SEXT, OP_ROL, OP_ROR:
		return true
	}
	return false
}

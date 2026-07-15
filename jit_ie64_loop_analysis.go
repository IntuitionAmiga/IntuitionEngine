package main

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
	prefix     uint32
	accesses   []ie64LoopAccess
	bounded    bool
	count      uint64
	bodySize   uint32
}

// Compilers are serialised by ie64CompileMu, so the emit-time plan can remain
// package-local state like the existing FP residency and count-base plans.
var ie64ActiveLoopPlan *ie64LoopPlan

// ie64AnalyseLoop returns the first conservative, single-entry loop in a
// flattened compilation unit. Both native emitters and wasm use this result.
func ie64AnalyseLoop(instrs []JITInstr, startPC uint64) *ie64LoopPlan {
	for back := range instrs {
		br := &instrs[back]
		if br.opcode != OP_BNE {
			continue
		}
		pc := startPC + uint64(br.pcOffset)
		target := uint64(int64(pc) + int64(int32(br.imm32)))
		if target < startPC || target >= pc || (target-startPC)%IE64_INSTR_SIZE != 0 {
			continue
		}
		head := int((target - startPC) / IE64_INSTR_SIZE)
		if head >= back || ie64LoopHasExtraEntry(instrs, startPC, head, back) {
			continue
		}
		p := &ie64LoopPlan{head: head, back: back, prefix: uint32(head), bodySize: uint32(back - head + 1)}
		written := uint32(0)
		for i := head; i <= back; i++ {
			written |= instrWrittenRegs(&instrs[i])
		}
		seen := make(map[ie64LoopAccess]struct{})
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
			case OP_PUSH64, OP_POP64, OP_JSR64, OP_JSR_IND, OP_RTS64:
				validMem = false
			}
		}
		if !validMem {
			p.accesses = nil
		}
		p.bounded, p.count = ie64BoundedCounterLoop(instrs, p)
		if len(p.accesses) != 0 || p.bounded {
			return p
		}
	}
	return nil
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
		br.rs != seed.rd || br.rt != 0 {
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

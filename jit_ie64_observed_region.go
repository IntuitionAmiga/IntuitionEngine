package main

import "errors"

type ie64Region struct {
	blocks   [][]JITInstr
	blockPCs []uint64
	entryPC  uint64
	observed []ie64ObservedBlock
}

const ie64ObservedMaxBlocks = 8

type ie64ObservedTriggerKind uint8

const (
	ie64ObservedNone ie64ObservedTriggerKind = iota
	ie64ObservedConditional
	ie64ObservedIndirectJMP
)

type ie64ObservedRecorder struct {
	entryPC        uint64
	pcs            [ie64ObservedMaxBlocks]uint64
	count          uint8
	active         bool
	rejected       bool
	staticFallback bool
	generation     uint64
}

func (r *ie64ObservedRecorder) start(entry uint64, fallback bool, generation uint64) {
	*r = ie64ObservedRecorder{entryPC: entry, count: 1, active: true, staticFallback: fallback, generation: generation}
	r.pcs[0] = entry
}

func (r *ie64ObservedRecorder) reset() { *r = ie64ObservedRecorder{} }

func (r *ie64ObservedRecorder) path() []uint64 { return r.pcs[:r.count] }

func (r *ie64ObservedRecorder) appendSuccessor(pc uint64) (done, reject bool) {
	if !r.active {
		return false, true
	}
	for i := uint8(0); i < r.count; i++ {
		if r.pcs[i] != pc {
			continue
		}
		if i == 0 && r.count >= 2 {
			r.active = false
			return true, false
		}
		r.active, r.rejected = false, true
		return false, true
	}
	r.pcs[r.count] = pc
	r.count++
	if r.count == ie64ObservedMaxBlocks {
		r.active = false
		return true, false
	}
	return false, false
}

func ie64ConditionalOpcode(op byte) bool {
	switch op {
	case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
		return true
	}
	return false
}

func ie64ObservedTrigger(instrs []JITInstr, entryPC, blockEnd uint64) ie64ObservedTriggerKind {
	for i := range instrs {
		ins := &instrs[i]
		if !ie64ConditionalOpcode(ins.opcode) {
			continue
		}
		pc := entryPC + uint64(ins.pcOffset)
		target := uint64(int64(pc) + int64(int32(ins.imm32)))
		if target < entryPC || target >= blockEnd {
			return ie64ObservedConditional
		}
	}
	if len(instrs) != 0 {
		last := &instrs[len(instrs)-1]
		if last.opcode == OP_JMP && last.rs != 0 {
			return ie64ObservedIndirectJMP
		}
	}
	return ie64ObservedNone
}

type ie64ObservedBlock struct {
	pc              uint64
	instrs          []JITInstr
	hotTarget       uint64
	coldTarget      uint64
	predictedTarget uint64
	kind            ie64ObservedTriggerKind
}

type ie64ObservedRegion struct {
	entryPC    uint64
	blocks     []ie64ObservedBlock
	instrCount int
}

var errIE64ObservedInvalid = errors.New("invalid IE64 observed region")

func ie64BuildObservedRegion(path []uint64, scanned map[uint64][]JITInstr, lowWindow uint64) (*ie64ObservedRegion, error) {
	if len(path) < 2 || len(path) > ie64ObservedMaxBlocks {
		return nil, errIE64ObservedInvalid
	}
	r := &ie64ObservedRegion{entryPC: path[0], blocks: make([]ie64ObservedBlock, 0, len(path))}
	for i, pc := range path {
		if pc >= lowWindow {
			return nil, errIE64ObservedInvalid
		}
		original := scanned[pc]
		if len(original) == 0 || needsFallback(original) {
			return nil, errIE64ObservedInvalid
		}
		for j := range original {
			if original[j].fusedFlag != 0 {
				return nil, errIE64ObservedInvalid
			}
			switch original[j].opcode {
			case OP_JSR64, OP_JSR_IND, OP_RTS64, OP_HALT64, OP_RTI64, OP_WAIT64:
				return nil, errIE64ObservedInvalid
			}
		}
		next := path[(i+1)%len(path)]
		match := -1
		matches := 0
		for j := range original {
			if !ie64ConditionalOpcode(original[j].opcode) {
				continue
			}
			insPC := pc + uint64(original[j].pcOffset)
			target := uint64(int64(insPC) + int64(int32(original[j].imm32)))
			if target == next {
				match, matches = j, matches+1
			}
		}
		block := ie64ObservedBlock{pc: pc, hotTarget: next}
		switch {
		case matches == 1:
			block.instrs = append([]JITInstr(nil), original[:match+1]...)
			block.coldTarget = pc + uint64(original[match].pcOffset) + IE64_INSTR_SIZE
			block.kind = ie64ObservedConditional
		case matches > 1:
			return nil, errIE64ObservedInvalid
		default:
			last := &original[len(original)-1]
			lastPC := pc + uint64(last.pcOffset)
			if last.opcode == OP_JMP && last.rs != 0 {
				block.instrs = append([]JITInstr(nil), original...)
				block.predictedTarget, block.kind = next, ie64ObservedIndirectJMP
			} else if target, ok := ie64ResolveTerminatorTarget(last.opcode, last.rs, last.imm32, lastPC); ok && target == next {
				block.instrs = append([]JITInstr(nil), original...)
			} else {
				return nil, errIE64ObservedInvalid
			}
		}
		if block.hotTarget >= lowWindow || block.coldTarget >= lowWindow && block.coldTarget != 0 {
			return nil, errIE64ObservedInvalid
		}
		r.instrCount += len(block.instrs)
		r.blocks = append(r.blocks, block)
	}
	return r, nil
}

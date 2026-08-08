package main

import (
	"encoding/binary"
	"math/bits"
)

// ie32DecodedInstruction is the backend-neutral representation shared by the
// JIT scanner and emitters. It preserves raw fields because high register bits
// and reserved addressing modes have architectural meanings.
type ie32DecodedInstruction struct {
	PC       uint32
	Opcode   byte
	Reg      byte
	AddrMode byte
	Operand  uint32
	// chasedJump is set only by scanIE32Region for a static forward JMP whose
	// target is emitted immediately afterwards. The emitter retires the JMP but
	// leaves PC unpublished until the region's next observation boundary.
	chasedJump      bool
	fusedLeafCall   bool
	fusedLeafReturn bool
	knownBranch     bool
	branchTaken     bool
	// residentALU marks a contiguous run of immediate arithmetic on one guest
	// register. Backends may retain its value in a host register until the final
	// operation, where residentALUEnd requires the sole architectural spill.
	residentALU      bool
	residentALUStart bool
	residentALUEnd   bool
	// residentALUBranch permits a following conditional to compare the retained
	// host value instead of reloading the guest register.
	residentALUBranch bool
	// rangeProvenRegisterIndirect identifies a register-indirect RAM access
	// whose address range is derived entirely from earlier guest instructions.
	// The backend emits a dynamic host address, while cache admission may retain
	// the block because the proof is independent of entry register contents.
	rangeProvenRegisterIndirect bool
	rangeBaseRegister           byte
	rangeAddressOffset          uint32
}

func decodeIE32Instruction(memory []byte, pc uint32) (ie32DecodedInstruction, bool) {
	if uint64(pc)+INSTRUCTION_SIZE > uint64(len(memory)) {
		return ie32DecodedInstruction{}, false
	}
	off := int(pc)
	return ie32DecodedInstruction{
		PC:       pc,
		Opcode:   memory[off],
		Reg:      memory[off+REG_OFFSET],
		AddrMode: memory[off+ADDRMODE_OFFSET],
		Operand:  binary.LittleEndian.Uint32(memory[off+OPERAND_OFFSET:]),
	}, true
}

func (in ie32DecodedInstruction) registerIndex() byte { return in.Reg & REG_INDEX_MASK }

func (in ie32DecodedInstruction) operandRegisterIndex() byte {
	return byte(in.Operand & REG_INDEX_MASK)
}

// ie32ElideDeadImmediateLoad recognises a safe dead-write peephole. It is used
// only in generated blocks without timer or debug boundaries: the first
// immediate load has no fault or memory effect and a later load in a contiguous
// run of immediate loads overwrites the same register before it can be read.
func ie32ElideDeadImmediateLoad(block []ie32DecodedInstruction, index int) bool {
	if index < 0 || index >= len(block) {
		return false
	}
	first := block[index]
	if first.AddrMode != ADDR_IMMEDIATE {
		return false
	}
	firstReg, firstOK := ie32ImmediateLoadDestination(first)
	if !firstOK {
		return false
	}
	for _, next := range block[index+1:] {
		if next.AddrMode != ADDR_IMMEDIATE {
			return false
		}
		nextReg, ok := ie32ImmediateLoadDestination(next)
		if !ok {
			return false
		}
		if nextReg == firstReg {
			return true
		}
	}
	return false
}

// ie32RegisterLiveness reports whether each guest register is needed after an
// instruction. Block exits conservatively expose every register, so this
// analysis only eliminates a definition proven overwritten before any read.
func ie32RegisterLiveness(block []ie32DecodedInstruction) [][16]bool {
	live := make([][16]bool, len(block))
	var demand [16]bool
	for reg := range demand {
		demand[reg] = true
	}
	for i := len(block) - 1; i >= 0; i-- {
		live[i] = demand
		reads, writes, known := ie32RegisterUseDef(block[i])
		if !known {
			for reg := range demand {
				demand[reg] = true
			}
			continue
		}
		for reg := range writes {
			if writes[reg] {
				demand[reg] = false
			}
		}
		for reg := range reads {
			if reads[reg] {
				demand[reg] = true
			}
		}
	}
	return live
}

func ie32RegisterUseDef(in ie32DecodedInstruction) (reads, writes [16]bool, known bool) {
	reg := int(in.registerIndex())
	operandReg := int(in.operandRegisterIndex())
	readOperand := func() bool {
		if in.AddrMode == ADDR_REGISTER {
			reads[operandReg] = true
			return true
		}
		return in.AddrMode == ADDR_IMMEDIATE || in.AddrMode == ADDR_DIRECT || in.AddrMode == ADDR_MEM_IND || in.AddrMode == ADDR_REG_IND
	}
	if in.isControlFlow() || in.Opcode == WAIT || in.Opcode == SEI || in.Opcode == CLI {
		return reads, writes, false
	}
	if dst, ok := ie32ImmediateLoadDestination(in); ok {
		writes[dst] = true
		return reads, writes, readOperand()
	}
	switch in.Opcode {
	case LOAD:
		writes[reg] = true
		return reads, writes, readOperand()
	case ADD, SUB, MUL, DIV, MOD, AND, OR, XOR, SHL, SHR, NOT:
		reads[reg], writes[reg] = true, true
		if in.Opcode == NOT {
			return reads, writes, true
		}
		return reads, writes, readOperand()
	case STORE:
		reads[reg] = true
		return reads, writes, true
	case INC, DEC:
		if in.AddrMode != ADDR_REGISTER {
			return reads, writes, false
		}
		reads[operandReg], writes[operandReg] = true, true
		return reads, writes, true
	case NOP:
		return reads, writes, true
	}
	if ie32IsNamedStore(in.Opcode) {
		if dst, ok := ie32NamedStoreRegister(in.Opcode); ok {
			reads[dst] = true
			return reads, writes, true
		}
	}
	return reads, writes, false
}

func ie32NamedStoreRegister(opcode byte) (int, bool) {
	switch opcode {
	case STA:
		return 0, true
	case STX:
		return 1, true
	case STY:
		return 2, true
	case STZ:
		return 3, true
	case STB:
		return 4, true
	case STC:
		return 5, true
	case STD:
		return 6, true
	case STE:
		return 7, true
	case STF:
		return 8, true
	case STG:
		return 9, true
	case STH:
		return 10, true
	case STS:
		return 11, true
	case STT:
		return 12, true
	case STU:
		return 13, true
	case STV:
		return 14, true
	case STW:
		return 15, true
	default:
		return 0, false
	}
}

func ie32ElideDeadImmediateLoadWithLiveness(block []ie32DecodedInstruction, live [][16]bool, index int) bool {
	if index < 0 || index >= len(block) || len(live) != len(block) {
		return false
	}
	dst, ok := ie32ImmediateLoadDestination(block[index])
	return ok && block[index].AddrMode == ADDR_IMMEDIATE && !live[index][dst]
}

func ie32ImmediateLoadDestination(in ie32DecodedInstruction) (byte, bool) {
	switch in.Opcode {
	case LOAD:
		return in.registerIndex(), true
	case LDA:
		return 0, true
	case LDX:
		return 1, true
	case LDY:
		return 2, true
	case LDZ:
		return 3, true
	case LDB:
		return 4, true
	case LDC:
		return 5, true
	case LDD:
		return 6, true
	case LDE:
		return 7, true
	case LDF:
		return 8, true
	case LDG:
		return 9, true
	case LDH:
		return 10, true
	case LDS:
		return 11, true
	case LDT:
		return 12, true
	case LDU:
		return 13, true
	case LDV:
		return 14, true
	case LDW:
		return 15, true
	default:
		return 0, false
	}
}

func ie32FoldImmediateALU(block []ie32DecodedInstruction) []ie32DecodedInstruction {
	if len(block) < 2 {
		return block
	}
	out := append([]ie32DecodedInstruction(nil), block...)
	for i := 0; i+1 < len(out); i++ {
		reg, ok := ie32ImmediateLoadDestination(out[i])
		if !ok || out[i].AddrMode != ADDR_IMMEDIATE || out[i+1].AddrMode != ADDR_IMMEDIATE || out[i+1].registerIndex() != reg {
			continue
		}
		value, ok := ie32FoldImmediateALUValue(out[i].Operand, out[i+1].Opcode, out[i+1].Operand)
		if !ok {
			continue
		}
		out[i].Opcode = NOP
		out[i+1] = ie32DecodedInstruction{PC: out[i+1].PC, Opcode: LOAD, Reg: reg, AddrMode: ADDR_IMMEDIATE, Operand: value}
	}
	return out
}

// ie32AnnotateKnownBranches folds a comparison only when the immediately
// preceding immediate load fixes the tested register's value. The load itself
// remains emitted and both guest instructions still retire.
func ie32AnnotateKnownBranches(block []ie32DecodedInstruction) []ie32DecodedInstruction {
	if len(block) < 2 {
		return block
	}
	out := append([]ie32DecodedInstruction(nil), block...)
	for i := 1; i < len(out); i++ {
		branch := &out[i]
		if branch.Opcode < JNZ || branch.Opcode > JLE {
			continue
		}
		reg, ok := ie32ImmediateLoadDestination(out[i-1])
		if !ok || out[i-1].AddrMode != ADDR_IMMEDIATE || reg != branch.registerIndex() {
			continue
		}
		value := out[i-1].Operand
		branch.knownBranch = true
		switch branch.Opcode {
		case JNZ:
			branch.branchTaken = value != 0
		case JZ:
			branch.branchTaken = value == 0
		case JGT:
			branch.branchTaken = int32(value) > 0
		case JGE:
			branch.branchTaken = int32(value) >= 0
		case JLT:
			branch.branchTaken = int32(value) < 0
		case JLE:
			branch.branchTaken = int32(value) <= 0
		}
	}
	return out
}

// ie32AnnotateResidentImmediateALU marks only same-register immediate ALU
// runs. They contain no observation boundary, so retaining the value in a host
// register until the final operation preserves every externally visible guest
// state while avoiding intermediate spills.
func ie32AnnotateResidentImmediateALU(block []ie32DecodedInstruction) []ie32DecodedInstruction {
	out := append([]ie32DecodedInstruction(nil), block...)
	for start := 0; start < len(out); {
		if !ie32ResidentImmediateALU(out[start]) {
			start++
			continue
		}
		run := []int{start}
		end := start + 1
		for end < len(out) {
			if out[end].chasedJump {
				end++
				continue
			}
			if !ie32ResidentImmediateALU(out[end]) || out[end].registerIndex() != out[start].registerIndex() {
				break
			}
			run = append(run, end)
			end++
		}
		if len(run) >= 2 {
			for _, i := range run {
				out[i].residentALU = true
			}
			out[run[0]].residentALUStart = true
			out[run[len(run)-1]].residentALUEnd = true
			next := run[len(run)-1] + 1
			if next < len(out) && out[next].Opcode >= JNZ && out[next].Opcode <= JLE && out[next].registerIndex() == out[start].registerIndex() {
				out[run[len(run)-1]].residentALUBranch = true
				out[next].residentALUBranch = true
			}
		}
		start = end
	}
	return out
}

func ie32ResidentImmediateALU(in ie32DecodedInstruction) bool {
	if in.AddrMode != ADDR_IMMEDIATE {
		return false
	}
	switch in.Opcode {
	case ADD, SUB, MUL, AND, OR, XOR:
		return true
	default:
		return false
	}
}

// ie32ResidentALUSpillsSaved reports actual CPU-register stores avoided by
// fully emitted resident runs. A partial run never contributes because its
// final spill has not been emitted.
func ie32ResidentALUSpillsSaved(block []ie32DecodedInstruction, retired int) uint64 {
	if retired > len(block) {
		retired = len(block)
	}
	start := -1
	var saved uint64
	for i := 0; i < retired; i++ {
		if block[i].residentALUStart {
			start = i
		}
		if block[i].residentALUEnd && start >= 0 {
			saved += uint64(i - start)
			start = -1
		}
	}
	return saved
}

// ie32SpecialiseKnownConstantRegisterAddresses carries immediate register
// values through a block and rewrites register-indirect operands whose base is
// proven constant. This is intentionally frontend-only: runtime RAM/MMIO and
// visible-RAM admission remain mandatory in each backend.
func ie32SpecialiseKnownConstantRegisterAddresses(block []ie32DecodedInstruction) []ie32DecodedInstruction {
	out := append([]ie32DecodedInstruction(nil), block...)
	var known [16]uint32
	var valid [16]bool
	for i := range out {
		in := &out[i]
		if in.AddrMode == ADDR_REG_IND {
			base := in.operandRegisterIndex()
			if valid[base] {
				in.AddrMode = ADDR_DIRECT
				in.Operand = known[base] + (in.Operand &^ uint32(REG_INDEX_MASK))
			}
		}
		if dst, ok := ie32ImmediateLoadDestination(*in); ok && in.AddrMode == ADDR_IMMEDIATE {
			known[dst], valid[dst] = in.Operand, true
			continue
		}
		_, writes, classified := ie32RegisterUseDef(*in)
		if !classified {
			valid = [16]bool{}
			continue
		}
		for reg := range writes {
			if writes[reg] {
				valid[reg] = false
			}
		}
	}
	return out
}

type ie32RegisterRange struct {
	min         uint64
	max         uint64
	lowZeroBits uint8
}

func ie32ExactRegisterRange(value uint32) ie32RegisterRange {
	zeros := uint8(bits.TrailingZeros32(value))
	return ie32RegisterRange{min: uint64(value), max: uint64(value), lowZeroBits: zeros}
}

func ie32UnknownRegisterRange() ie32RegisterRange {
	return ie32RegisterRange{max: uint64(^uint32(0))}
}

func ie32RangeAdd(left, right ie32RegisterRange) ie32RegisterRange {
	if left.max+right.max > uint64(^uint32(0)) {
		return ie32UnknownRegisterRange()
	}
	return ie32RegisterRange{min: left.min + right.min, max: left.max + right.max, lowZeroBits: min(left.lowZeroBits, right.lowZeroBits)}
}

func ie32RangeMul(value ie32RegisterRange, multiplier uint32) ie32RegisterRange {
	if value.max*uint64(multiplier) > uint64(^uint32(0)) {
		return ie32UnknownRegisterRange()
	}
	return ie32RegisterRange{min: value.min * uint64(multiplier), max: value.max * uint64(multiplier), lowZeroBits: min(32, value.lowZeroBits+uint8(bits.TrailingZeros32(multiplier)))}
}

// ie32AnnotateRangeProvenRegisterIndirect marks dynamic register-indirect
// reads whose complete word-access range is ordinary aligned RAM. Its exclusive
// end must remain below the MMIO boundary. The proof is data-flow based rather
// than a snapshot of a changing register, so retained native code remains
// valid across entries. Unsupported arithmetic simply loses the proof and
// resumes through the architectural helper path.
func ie32AnnotateRangeProvenRegisterIndirect(cpu *CPU, block []ie32DecodedInstruction) []ie32DecodedInstruction {
	if cpu == nil || len(block) == 0 {
		return block
	}
	out := append([]ie32DecodedInstruction(nil), block...)
	var ranges [16]ie32RegisterRange
	for i := range ranges {
		ranges[i] = ie32UnknownRegisterRange()
	}
	for i := range out {
		in := &out[i]
		if in.AddrMode == ADDR_REG_IND {
			base := in.operandRegisterIndex()
			range_ := ranges[base]
			offset := in.Operand &^ uint32(REG_INDEX_MASK)
			end := range_.max + uint64(offset) + WORD_SIZE
			if end >= uint64(offset)+WORD_SIZE && range_.lowZeroBits >= 2 && offset&3 == 0 &&
				ie32CanDirectRAMRead(cpu, uint32(range_.min)+offset) &&
				end <= uint64(IO_REGION_START) &&
				end <= uint64(len(cpu.memory)) && uint64(uint32(range_.max)+offset)+WORD_SIZE == end &&
				(uint64(cpuVisibleIE32RAM(cpu)) == 0 || end <= uint64(cpuVisibleIE32RAM(cpu))) {
				in.rangeProvenRegisterIndirect = true
				in.rangeBaseRegister = base
				in.rangeAddressOffset = offset
			}
		}
		ie32UpdateRegisterRange(&ranges, *in)
	}
	return out
}

func cpuVisibleIE32RAM(cpu *CPU) uint64 {
	if bus, ok := cpu.bus.(*MachineBus); ok {
		return bus.ActiveVisibleRAM()
	}
	return 0
}

func ie32UpdateRegisterRange(ranges *[16]ie32RegisterRange, in ie32DecodedInstruction) {
	if dst, ok := ie32ImmediateLoadDestination(in); ok && in.AddrMode == ADDR_IMMEDIATE {
		ranges[dst] = ie32ExactRegisterRange(in.Operand)
		return
	}
	if in.Opcode == LOAD && in.AddrMode == ADDR_REGISTER {
		ranges[in.registerIndex()] = ranges[in.operandRegisterIndex()]
		return
	}
	if in.AddrMode == ADDR_IMMEDIATE {
		reg := in.registerIndex()
		switch in.Opcode {
		case ADD:
			ranges[reg] = ie32RangeAdd(ranges[reg], ie32ExactRegisterRange(in.Operand))
			return
		case MUL:
			ranges[reg] = ie32RangeMul(ranges[reg], in.Operand)
			return
		case AND:
			ranges[reg] = ie32RegisterRange{max: uint64(in.Operand), lowZeroBits: uint8(bits.TrailingZeros32(in.Operand))}
			return
		}
	}
	if in.Opcode == ADD && in.AddrMode == ADDR_REGISTER {
		reg := in.registerIndex()
		ranges[reg] = ie32RangeAdd(ranges[reg], ranges[in.operandRegisterIndex()])
		return
	}
	_, writes, known := ie32RegisterUseDef(in)
	if !known {
		for i := range ranges {
			ranges[i] = ie32UnknownRegisterRange()
		}
		return
	}
	for reg := range writes {
		if writes[reg] {
			ranges[reg] = ie32UnknownRegisterRange()
		}
	}
}

// ie32CountedLoopPlan describes a backward JNZ loop whose bounded body has no
// observation boundary. Direct RAM is admitted only after its one-time guard
// proves the access is outside MMIO, VRAM, and generated code; helpers and
// timing-sensitive forms still need dispatcher observation on every iteration.
type ie32CountedLoopPlan struct {
	head        int
	back        int
	counter     byte
	bodyRetired uint64
}

func ie32AnalyseCountedLoop(block []ie32DecodedInstruction) *ie32CountedLoopPlan {
	if len(block) < 2 {
		return nil
	}
	back := len(block) - 1
	branch := block[back]
	if branch.Opcode != JNZ || branch.AddrMode != ADDR_IMMEDIATE || branch.Operand >= branch.PC {
		return nil
	}
	head := -1
	for i := range block[:back] {
		if block[i].PC == branch.Operand {
			head = i
			break
		}
	}
	if head < 0 || head >= back || back-head < 2 {
		return nil
	}
	counter := branch.registerIndex()
	dec := block[back-1]
	if dec.Opcode != SUB || dec.AddrMode != ADDR_IMMEDIATE || dec.registerIndex() != counter || dec.Operand != 1 {
		return nil
	}
	for i := head; i < back; i++ {
		in := block[i]
		switch in.Opcode {
		case NOP:
		case JSR:
			if !in.fusedLeafCall {
				return nil
			}
		case RTS:
			if !in.fusedLeafReturn {
				return nil
			}
		case LOAD:
			if in.AddrMode != ADDR_IMMEDIATE && in.AddrMode != ADDR_DIRECT {
				return nil
			}
		case STORE:
			if in.AddrMode != ADDR_DIRECT {
				return nil
			}
		case ADD, SUB, MUL, AND, OR, XOR:
			if in.AddrMode != ADDR_IMMEDIATE {
				return nil
			}
		default:
			return nil
		}
		if i != back-1 && in.registerIndex() == counter && in.Opcode != NOP {
			return nil
		}
	}
	return &ie32CountedLoopPlan{head: head, back: back, counter: counter, bodyRetired: uint64(back - head + 1)}
}

func ie32CountedLoopDirectMemoryAdmissible(cpu *CPU, block []ie32DecodedInstruction, plan *ie32CountedLoopPlan) bool {
	if cpu == nil || plan == nil {
		return false
	}
	for i := plan.head; i < plan.back; i++ {
		in := block[i]
		switch in.Opcode {
		case LOAD:
			if in.AddrMode == ADDR_DIRECT && !ie32CanDirectRAMRead(cpu, in.Operand) {
				return false
			}
		case STORE:
			if !ie32CanDirectRAMWrite(cpu, in.Operand) || ie32WriteMutatesRemainingBlock(in, block) {
				return false
			}
		}
	}
	return true
}

func ie32CountedLoopInitialCount(cpu *CPU, block []ie32DecodedInstruction, plan *ie32CountedLoopPlan) (uint32, bool) {
	if cpu == nil || plan == nil {
		return 0, false
	}
	for i := plan.head - 1; i >= 0; i-- {
		in := block[i]
		if dst, ok := ie32ImmediateLoadDestination(in); ok && dst == plan.counter && in.AddrMode == ADDR_IMMEDIATE {
			return in.Operand, true
		}
		_, writes, known := ie32RegisterUseDef(in)
		if !known || writes[plan.counter] {
			return 0, false
		}
	}
	return *cpu.getRegister(plan.counter), true
}

func ie32FoldImmediateALUValue(left uint32, opcode byte, right uint32) (uint32, bool) {
	switch opcode {
	case ADD:
		return left + right, true
	case SUB:
		return left - right, true
	case MUL:
		return left * right, true
	case AND:
		return left & right, true
	case OR:
		return left | right, true
	case XOR:
		return left ^ right, true
	case NOT:
		return ^left, true
	case SHL:
		if right < 32 {
			return left << right, true
		}
	case SHR:
		if right < 32 {
			return left >> right, true
		}
	}
	return 0, false
}

func (in ie32DecodedInstruction) isControlFlow() bool {
	switch in.Opcode {
	case JMP, JNZ, JZ, JGT, JGE, JLT, JLE, JSR, RTS, RTI, HALT:
		return true
	}
	return false
}

func (in ie32DecodedInstruction) isKnownOpcode() bool {
	return ie32KnownOpcode[in.Opcode]
}

const ie32JITMaxBlockInstructions = 64

const ie32JITMaxRegionBlocks = 4

// scanIE32Block forms a bounded straight-line block. Control-flow operations
// are included as the final instruction so an emitter can preserve their
// architectural exit precisely. Unknown opcodes also terminate the block and
// are left for the shared fault path.
func scanIE32Block(memory []byte, startPC uint32, limit int) []ie32DecodedInstruction {
	if limit <= 0 || limit > ie32JITMaxBlockInstructions {
		limit = ie32JITMaxBlockInstructions
	}
	block := make([]ie32DecodedInstruction, 0, limit)
	pc := startPC
	for len(block) < limit {
		in, ok := decodeIE32Instruction(memory, pc)
		if !ok {
			break
		}
		block = append(block, in)
		if !in.isKnownOpcode() || in.isControlFlow() {
			break
		}
		pc += INSTRUCTION_SIZE
	}
	return block
}

// scanIE32FusedBlock replaces a statically proven, register-only JSR leaf
// with its body and a synthetic RTS retirement marker. The guest stack is not
// touched, but JSR, each body instruction, and RTS remain separately retired.
func scanIE32FusedBlock(memory []byte, startPC uint32, limit int) []ie32DecodedInstruction {
	if limit <= 0 || limit > ie32JITMaxBlockInstructions {
		limit = ie32JITMaxBlockInstructions
	}
	block := make([]ie32DecodedInstruction, 0, limit)
	pc := startPC
	for len(block) < limit {
		in, ok := decodeIE32Instruction(memory, pc)
		if !ok {
			break
		}
		if in.Opcode == JSR && in.Operand != pc+INSTRUCTION_SIZE {
			if leaf, ok := ie32AnalyseLeafCall(memory, in.Operand, limit-len(block)-2); ok {
				in.fusedLeafCall = true
				block = append(block, in)
				block = append(block, leaf...)
				block = append(block, ie32DecodedInstruction{PC: in.Operand + uint32(len(leaf))*INSTRUCTION_SIZE, Opcode: RTS, fusedLeafReturn: true})
				pc += INSTRUCTION_SIZE
				continue
			}
		}
		block = append(block, in)
		if !in.isKnownOpcode() || in.isControlFlow() {
			break
		}
		pc += INSTRUCTION_SIZE
	}
	return block
}

func ie32AnalyseLeafCall(memory []byte, target uint32, capacity int) ([]ie32DecodedInstruction, bool) {
	const maxBody = 4
	if capacity < 0 {
		return nil, false
	}
	body := make([]ie32DecodedInstruction, 0, maxBody)
	pc := target
	for len(body) <= maxBody {
		in, ok := decodeIE32Instruction(memory, pc)
		if !ok {
			return nil, false
		}
		if in.Opcode == RTS {
			return body, len(body) <= capacity
		}
		if !ie32LeafFusionSafe(in) || len(body) == maxBody {
			return nil, false
		}
		body = append(body, in)
		pc += INSTRUCTION_SIZE
	}
	return nil, false
}

func ie32LeafFusionSafe(in ie32DecodedInstruction) bool {
	switch in.Opcode {
	case NOP, NOT:
		return true
	case LOAD, LDA, LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW:
		return in.AddrMode == ADDR_IMMEDIATE || in.AddrMode == ADDR_REGISTER
	case ADD, SUB, MUL, AND, OR, XOR:
		return in.AddrMode == ADDR_IMMEDIATE || in.AddrMode == ADDR_REGISTER
	case SHL, SHR:
		return in.AddrMode == ADDR_IMMEDIATE && in.Operand < 32
	default:
		return false
	}
}

// scanIE32Region joins a small number of straight-line blocks through static,
// forward JMPs. Conditional branches, calls, returns, backward targets, and
// every observation boundary remain normal dispatcher exits. The bounded
// shape keeps timer, debugger, cooperative-yield, and external-stop latency
// no worse than the existing block limit.
func scanIE32Region(memory []byte, startPC uint32, limit int) []ie32DecodedInstruction {
	if limit <= 0 || limit > ie32JITMaxBlockInstructions {
		limit = ie32JITMaxBlockInstructions
	}
	region := make([]ie32DecodedInstruction, 0, limit)
	pc := startPC
	for blocks := 0; blocks < ie32JITMaxRegionBlocks && len(region) < limit; blocks++ {
		segment := scanIE32Block(memory, pc, limit-len(region))
		if len(segment) == 0 {
			break
		}
		last := &segment[len(segment)-1]
		region = append(region, segment...)
		if last.Opcode != JMP || last.Operand <= last.PC || uint64(last.Operand)+INSTRUCTION_SIZE > uint64(len(memory)) || len(region) == limit {
			break
		}
		region[len(region)-1].chasedJump = true
		pc = last.Operand
	}
	return region
}

func ie32BlockNextPC(block []ie32DecodedInstruction, retired int) uint32 {
	if retired <= 0 || retired > len(block) {
		return 0
	}
	return block[retired-1].PC + INSTRUCTION_SIZE
}

// ie32BlockResumePC returns the architectural continuation after a partially
// lowered block. A chased JMP has no emitted PC store because its target is
// normally emitted in the same region. If lowering stops at that target's
// helper boundary, execution must resume at the target rather than in the
// skipped source bytes following the JMP.
func ie32BlockResumePC(block []ie32DecodedInstruction, retired int) uint32 {
	if retired <= 0 || retired > len(block) {
		return 0
	}
	last := block[retired-1]
	if last.chasedJump {
		return last.Operand
	}
	return last.PC + INSTRUCTION_SIZE
}

func ie32BlockIsRegion(block []ie32DecodedInstruction, retired int) bool {
	if retired <= 0 || retired > len(block) {
		return false
	}
	for _, in := range block[:retired] {
		if in.chasedJump {
			return true
		}
	}
	return false
}

func ie32BlockEndsInJMP(block []ie32DecodedInstruction, retired int) bool {
	return retired > 0 && retired <= len(block) && block[retired-1].Opcode == JMP
}

// ie32BlockEndsInBackwardJMP identifies a compact loop edge. Forward jumps
// retain the existing hot-region promotion path; a backward jump cannot form
// that region and is therefore retained directly after its first lowering.
func ie32BlockEndsInBackwardJMP(block []ie32DecodedInstruction, retired int) bool {
	if !ie32BlockEndsInJMP(block, retired) {
		return false
	}
	last := block[retired-1]
	return last.Operand <= last.PC
}

func ie32BlockHasLeafFusion(block []ie32DecodedInstruction) bool {
	for _, in := range block {
		if in.fusedLeafCall || in.fusedLeafReturn {
			return true
		}
	}
	return false
}

// ie32CanDirectRAMRead is the common admission proof for direct 32-bit RAM
// reads. The backing slice may be larger than a profile's active visible RAM,
// so len(memory) alone is never an architectural bound.
func ie32CanDirectRAMRead(cpu *CPU, addr uint32) bool {
	if cpu == nil || addr&3 != 0 || addr >= IO_REGION_START {
		return false
	}
	end := uint64(addr) + WORD_SIZE
	if end > uint64(len(cpu.memory)) {
		return false
	}
	if bus, ok := cpu.bus.(*MachineBus); ok {
		visible := bus.ActiveVisibleRAM()
		if visible != 0 && end > visible {
			return false
		}
	}
	return true
}

// ie32CanDirectRAMWrite admits only the raw-RAM Write32 path. Generated code
// must leave MMIO, direct VRAM leases, and debugger-observed accesses to the
// architectural helper path.
func ie32CanDirectRAMWrite(cpu *CPU, addr uint32) bool {
	if !ie32CanDirectRAMRead(cpu, addr) {
		return false
	}
	return cpu.vramDirect == nil || addr+WORD_SIZE <= cpu.vramStart || addr >= cpu.vramEnd
}

// ie32FirstInstructionNeedsHelper avoids building a candidate block for an
// instruction whose direct operand is already known to be MMIO or otherwise
// outside the raw-RAM fast path. Control-flow direct operands are targets, not
// memory addresses, and are deliberately excluded.
func ie32FirstInstructionNeedsHelper(cpu *CPU, in ie32DecodedInstruction) bool {
	if in.AddrMode != ADDR_DIRECT {
		return false
	}
	switch in.Opcode {
	case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
		return !ie32CanDirectRAMWrite(cpu, in.Operand)
	case LOAD, LDA, LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW,
		ADD, SUB, MUL, AND, OR, XOR, DIV, MOD, INC, DEC:
		return !ie32CanDirectRAMRead(cpu, in.Operand)
	default:
		return false
	}
}

// ie32WriteMutatesRemainingBlock prevents a directly lowered store from
// executing instructions which it has just replaced. The next dispatch may
// compile the new bytes, but the current native block must stop at the store
// and let the architectural path publish the write first.
func ie32WriteMutatesRemainingBlock(in ie32DecodedInstruction, block []ie32DecodedInstruction) bool {
	if len(block) == 0 {
		return false
	}
	first := in.PC + INSTRUCTION_SIZE
	last := block[len(block)-1].PC + INSTRUCTION_SIZE
	writeEnd := uint64(in.Operand) + WORD_SIZE
	return uint64(in.Operand) < uint64(last) && writeEnd > uint64(first)
}

func ie32IsNamedStore(opcode byte) bool {
	switch opcode {
	case STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
		return true
	default:
		return false
	}
}

func ie32IsDirectALUOpcode(opcode byte) bool {
	switch opcode {
	case ADD, SUB, MUL, AND, OR, XOR:
		return true
	default:
		return false
	}
}

func ie32StaticRegisterIndirectAddress(cpu *CPU, in ie32DecodedInstruction) (uint32, bool) {
	if cpu == nil || in.AddrMode != ADDR_REG_IND {
		return 0, false
	}
	base := *cpu.getRegister(in.operandRegisterIndex())
	addr := base + (in.Operand & ^uint32(REG_INDEX_MASK))
	return addr, ie32CanDirectRAMRead(cpu, addr)
}

func ie32StaticMemoryIndirectStoreAddress(cpu *CPU, in ie32DecodedInstruction) (uint32, bool) {
	return ie32StaticMemoryIndirectAddress(cpu, in, true)
}

// ie32StaticMemoryIndirectAddress resolves the pointer slot used by IE32
// write-style memory-indirect operations. Both the pointer slot and resulting
// target must be ordinary visible RAM; MMIO and other observable paths remain
// one-instruction helpers.
func ie32StaticMemoryIndirectAddress(cpu *CPU, in ie32DecodedInstruction, write bool) (uint32, bool) {
	if cpu == nil || in.AddrMode != ADDR_MEM_IND || !ie32CanDirectRAMRead(cpu, in.Operand) {
		return 0, false
	}
	addr := cpu.Read32(in.Operand)
	if write {
		return addr, ie32CanDirectRAMWrite(cpu, addr)
	}
	return addr, ie32CanDirectRAMRead(cpu, addr)
}

func ie32MemoryIndirectWrites(opcode byte) bool {
	switch opcode {
	case STORE, STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW, INC, DEC:
		return true
	default:
		return false
	}
}

func ie32SpecialiseFirstIndirect(cpu *CPU, in ie32DecodedInstruction, first bool) (ie32DecodedInstruction, bool) {
	if !first || in.AddrMode != ADDR_MEM_IND || !ie32MemoryIndirectWrites(in.Opcode) {
		return in, true
	}
	addr, ok := ie32StaticMemoryIndirectAddress(cpu, in, true)
	if !ok {
		return in, false
	}
	in.AddrMode, in.Operand = ADDR_DIRECT, addr
	return in, true
}

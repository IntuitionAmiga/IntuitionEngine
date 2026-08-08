package main

import (
	"fmt"
	"sort"
	"testing"
)

func TestIE32JITDecoderPreservesArchitecturalRawFields(t *testing.T) {
	mem := make([]byte, PROG_START+INSTRUCTION_SIZE)
	mem[PROG_START] = LOAD
	mem[PROG_START+REG_OFFSET] = 0xF3
	mem[PROG_START+ADDRMODE_OFFSET] = ADDR_REG_IND
	mem[PROG_START+OPERAND_OFFSET] = 0xF4
	mem[PROG_START+OPERAND_OFFSET+1] = 0x12
	in, ok := decodeIE32Instruction(mem, PROG_START)
	if !ok {
		t.Fatal("decode failed")
	}
	if in.Reg != 0xF3 || in.registerIndex() != 3 {
		t.Fatalf("register decode = %#x/%d", in.Reg, in.registerIndex())
	}
	if in.Operand != 0x12F4 || in.operandRegisterIndex() != 4 {
		t.Fatalf("operand decode = %#x/%d", in.Operand, in.operandRegisterIndex())
	}
}

func TestIE32JITElidesOnlyOverwrittenImmediateLoads(t *testing.T) {
	loads := []ie32DecodedInstruction{
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 2},
	}
	if !ie32ElideDeadImmediateLoad(loads, 0) {
		t.Fatal("overwritten immediate load was not elided")
	}
	loads[1].Reg = REG_X
	if ie32ElideDeadImmediateLoad(loads, 0) {
		t.Fatal("load into a different register was elided")
	}
	loads[1].Reg = REG_A
	loads[1].AddrMode = ADDR_DIRECT
	if ie32ElideDeadImmediateLoad(loads, 0) {
		t.Fatal("memory-reading load was elided")
	}
}

func TestIE32JITRegisterLivenessElidesDeadLoadAcrossRegisterOnlyWork(t *testing.T) {
	block := []ie32DecodedInstruction{
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: ADD, Reg: REG_X, AddrMode: ADDR_IMMEDIATE, Operand: 2},
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 3},
		{Opcode: HALT},
	}
	live := ie32RegisterLiveness(block)
	if !ie32ElideDeadImmediateLoadWithLiveness(block, live, 0) {
		t.Fatal("dead immediate load across register-only work was retained")
	}
	block[1] = ie32DecodedInstruction{Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 2}
	live = ie32RegisterLiveness(block)
	if ie32ElideDeadImmediateLoadWithLiveness(block, live, 0) {
		t.Fatal("immediate load read before overwrite was elided")
	}
}

func TestIE32JITFoldsImmediateLoadAndALU(t *testing.T) {
	block := []ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 7},
		{PC: PROG_START + INSTRUCTION_SIZE, Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 5},
	}
	folded := ie32FoldImmediateALU(block)
	if folded[0].Opcode != NOP || folded[1].Opcode != LOAD || folded[1].Operand != 12 {
		t.Fatalf("folded block = %#v", folded)
	}
	block[1].Reg = REG_X
	if got := ie32FoldImmediateALU(block); got[0].Opcode != LOAD {
		t.Fatal("different-register ALU was folded")
	}
	block[1] = ie32DecodedInstruction{PC: PROG_START + INSTRUCTION_SIZE, Opcode: NOT, Reg: REG_A, AddrMode: ADDR_IMMEDIATE}
	if got := ie32FoldImmediateALU(block); got[0].Opcode != NOP || got[1].Opcode != LOAD || got[1].Operand != ^uint32(7) {
		t.Fatalf("NOT fold = %#v", got)
	}
}

func TestIE32JITAnnotatesKnownImmediateBranch(t *testing.T) {
	block := []ie32DecodedInstruction{
		{PC: PROG_START, Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 0},
		{PC: PROG_START + INSTRUCTION_SIZE, Opcode: JZ, Reg: REG_A, Operand: 0x2000},
	}
	annotated := ie32AnnotateKnownBranches(block)
	if !annotated[1].knownBranch || !annotated[1].branchTaken {
		t.Fatalf("zero branch annotation=%#v", annotated[1])
	}
	block[0].Operand = 1
	annotated = ie32AnnotateKnownBranches(block)
	if !annotated[1].knownBranch || annotated[1].branchTaken {
		t.Fatalf("non-zero branch annotation=%#v", annotated[1])
	}
}

func TestIE32JITAnnotatesResidentImmediateALU(t *testing.T) {
	block := []ie32DecodedInstruction{
		{Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: XOR, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 2},
		{Opcode: SUB, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 3},
		{Opcode: ADD, Reg: REG_X, AddrMode: ADDR_IMMEDIATE, Operand: 4},
	}
	annotated := ie32AnnotateResidentImmediateALU(block)
	if !annotated[0].residentALU || !annotated[0].residentALUStart || annotated[0].residentALUEnd {
		t.Fatalf("first resident ALU annotation = %#v", annotated[0])
	}
	if !annotated[1].residentALU || annotated[1].residentALUStart || annotated[1].residentALUEnd {
		t.Fatalf("middle resident ALU annotation = %#v", annotated[1])
	}
	if !annotated[2].residentALU || annotated[2].residentALUStart || !annotated[2].residentALUEnd {
		t.Fatalf("last resident ALU annotation = %#v", annotated[2])
	}
	if annotated[3].residentALU {
		t.Fatalf("single ALU was marked resident: %#v", annotated[3])
	}
	if got := ie32ResidentALUSpillsSaved(annotated, len(annotated)); got != 2 {
		t.Fatalf("resident spills saved=%d, want 2", got)
	}
	if got := ie32ResidentALUSpillsSaved(annotated, 2); got != 0 {
		t.Fatalf("partial resident run spills saved=%d, want 0", got)
	}
}

func TestIE32JITAnnotatesResidentALUAcrossChasedJump(t *testing.T) {
	block := []ie32DecodedInstruction{
		{Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: JMP, chasedJump: true},
		{Opcode: XOR, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 2},
	}
	annotated := ie32AnnotateResidentImmediateALU(block)
	if !annotated[0].residentALUStart || !annotated[2].residentALUEnd || !annotated[0].residentALU || !annotated[2].residentALU {
		t.Fatalf("chased-jump residency annotation=%#v", annotated)
	}
	if got := ie32ResidentALUSpillsSaved(annotated, len(annotated)); got != 1 {
		t.Fatalf("chased-jump resident spills saved=%d, want 1", got)
	}
}

func TestIE32JITAnnotatesResidentALUBranch(t *testing.T) {
	block := []ie32DecodedInstruction{
		{Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: XOR, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 2},
		{Opcode: JNZ, Reg: REG_A, Operand: 0x200},
	}
	annotated := ie32AnnotateResidentImmediateALU(block)
	if !annotated[1].residentALUBranch || !annotated[2].residentALUBranch {
		t.Fatalf("resident branch annotation=%#v", annotated)
	}
}

func TestIE32JITSpecialisesKnownConstantRegisterAddress(t *testing.T) {
	block := []ie32DecodedInstruction{
		{Opcode: LOAD, Reg: REG_X, AddrMode: ADDR_IMMEDIATE, Operand: 0x400},
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_REG_IND, Operand: 0x40 | REG_X},
		{Opcode: ADD, Reg: REG_X, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_REG_IND, Operand: REG_X},
	}
	specialised := ie32SpecialiseKnownConstantRegisterAddresses(block)
	if specialised[1].AddrMode != ADDR_DIRECT || specialised[1].Operand != 0x440 {
		t.Fatalf("constant register-indirect load=%#v, want direct 0x440", specialised[1])
	}
	if specialised[3].AddrMode != ADDR_REG_IND {
		t.Fatalf("modified pointer register was specialised: %#v", specialised[3])
	}
}

func TestIE32JITAnalysesSafeCountedLoop(t *testing.T) {
	start := uint32(PROG_START)
	body := start + INSTRUCTION_SIZE
	block := []ie32DecodedInstruction{
		{PC: start, Opcode: LOAD, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: 3},
		{PC: body, Opcode: ADD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{PC: body + INSTRUCTION_SIZE, Opcode: SUB, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{PC: body + 2*INSTRUCTION_SIZE, Opcode: JNZ, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: body},
	}
	plan := ie32AnalyseCountedLoop(block)
	if plan == nil || plan.head != 1 || plan.back != 3 || plan.counter != REG_B || plan.bodyRetired != 3 {
		t.Fatalf("counted-loop plan=%#v", plan)
	}
	block[1] = ie32DecodedInstruction{PC: body, Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_DIRECT, Operand: 0x400}
	if plan := ie32AnalyseCountedLoop(block); plan == nil {
		t.Fatal("direct-RAM loop was not admitted as a guarded counted loop")
	}
	block[1] = ie32DecodedInstruction{PC: body, Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_REG_IND, Operand: REG_X}
	if plan := ie32AnalyseCountedLoop(block); plan != nil {
		t.Fatalf("indirect-memory loop admitted as native counted loop: %#v", plan)
	}
}

func TestIE32JITManifestCoversEveryKnownOpcode(t *testing.T) {
	for opcode := range map[byte]struct{}{
		LOAD: {}, STORE: {}, ADD: {}, SUB: {}, MUL: {}, DIV: {}, MOD: {}, AND: {}, OR: {}, XOR: {}, NOT: {}, SHL: {}, SHR: {},
		JMP: {}, JNZ: {}, JZ: {}, JGT: {}, JGE: {}, JLT: {}, JLE: {}, PUSH: {}, POP: {}, JSR: {}, RTS: {}, SEI: {}, CLI: {}, RTI: {}, WAIT: {}, NOP: {}, HALT: {},
		INC: {}, DEC: {}, LDA: {}, LDX: {}, LDY: {}, LDZ: {}, STA: {}, STX: {}, STY: {}, STZ: {}, LDB: {}, LDC: {}, LDD: {}, LDE: {}, LDF: {}, LDG: {}, LDU: {}, LDV: {}, LDW: {}, LDH: {}, LDS: {}, LDT: {}, STB: {}, STC: {}, STD: {}, STE: {}, STF: {}, STG: {}, STU: {}, STV: {}, STW: {}, STH: {}, STS: {}, STT: {},
	} {
		if _, ok := ie32OpcodeManifest[opcode]; !ok {
			t.Fatalf("missing opcode %#02x", opcode)
		}
	}
}

func TestIE32JITFormLedgerClassifiesEveryOpcodeAndAddressingByte(t *testing.T) {
	for opcode := range ie32OpcodeManifest {
		for mode := 0; mode <= 0xFF; mode++ {
			form := ie32OpcodeForm{Opcode: opcode, AddrMode: byte(mode)}
			if _, ok := ie32FormLowering[form]; !ok {
				t.Fatalf("unclassified opcode/form %#02x/%#02x", opcode, mode)
			}
		}
	}
	if got := ie32FormLowering[ie32OpcodeForm{LOAD, ADDR_REG_IND}]; got != ie32LoweringDirect {
		t.Fatalf("guarded register-indirect load = %d, want direct", got)
	}
	if got := ie32FormLowering[ie32OpcodeForm{ADD, ADDR_REG_IND}]; got != ie32LoweringDirect {
		t.Fatalf("guarded register-indirect ALU = %d, want direct", got)
	}
	if got := ie32FormLowering[ie32OpcodeForm{STORE, ADDR_IMMEDIATE}]; got != ie32LoweringDirect {
		t.Fatalf("immediate store = %d, want direct", got)
	}
	if got := ie32FormLowering[ie32OpcodeForm{ADD, ADDR_MEM_IND}]; got != ie32LoweringDirect {
		t.Fatalf("guarded memory-indirect arithmetic = %d, want direct", got)
	}
}

func TestIE32JITScanRegionChasesBoundedForwardJump(t *testing.T) {
	memory := make([]byte, STACK_START)
	start := uint32(PROG_START)
	target := start + 3*INSTRUCTION_SIZE
	putIE32Instruction(memory, start, JMP, 0, ADDR_IMMEDIATE, target)
	putIE32Instruction(memory, target, LDA, 0, ADDR_IMMEDIATE, 0xCAFE)
	putIE32Instruction(memory, target+INSTRUCTION_SIZE, HALT, 0, ADDR_IMMEDIATE, 0)
	region := scanIE32Region(memory, start, 0)
	if len(region) != 3 {
		t.Fatalf("region instruction count=%d, want 3", len(region))
	}
	if !region[0].chasedJump {
		t.Fatal("region did not mark its static jump as chased")
	}
	if region[1].PC != target || region[2].Opcode != HALT {
		t.Fatalf("region target sequence=%#x/%#x, want %#x/HALT", region[1].PC, region[2].Opcode, target)
	}
}

func TestIE32JITFusesSafeLeafCall(t *testing.T) {
	memory := make([]byte, STACK_START)
	start := uint32(PROG_START)
	leaf := start + 4*INSTRUCTION_SIZE
	putIE32Instruction(memory, start, JSR, 0, ADDR_IMMEDIATE, leaf)
	putIE32Instruction(memory, start+INSTRUCTION_SIZE, LDA, 0, ADDR_IMMEDIATE, 9)
	putIE32Instruction(memory, leaf, ADD, REG_A, ADDR_IMMEDIATE, 3)
	putIE32Instruction(memory, leaf+INSTRUCTION_SIZE, RTS, 0, ADDR_IMMEDIATE, 0)
	block := scanIE32FusedBlock(memory, start, 0)
	if len(block) != 4 || !block[0].fusedLeafCall || !block[2].fusedLeafReturn {
		t.Fatalf("fused block=%#v", block)
	}
	if block[3].PC != start+INSTRUCTION_SIZE {
		t.Fatalf("continuation PC=%#x, want %#x", block[3].PC, start+INSTRUCTION_SIZE)
	}
}

func TestIE32JITRejectsObservableLeafCall(t *testing.T) {
	memory := make([]byte, STACK_START)
	start := uint32(PROG_START)
	leaf := start + 4*INSTRUCTION_SIZE
	putIE32Instruction(memory, start, JSR, 0, ADDR_IMMEDIATE, leaf)
	putIE32Instruction(memory, leaf, LOAD, REG_A, ADDR_DIRECT, 0x400)
	putIE32Instruction(memory, leaf+INSTRUCTION_SIZE, RTS, 0, ADDR_IMMEDIATE, 0)
	block := scanIE32FusedBlock(memory, start, 0)
	if len(block) != 1 || block[0].fusedLeafCall {
		t.Fatalf("observable leaf was fused: %#v", block)
	}
}

func TestIE32JITRejectsLeafFusionAtReturnAddress(t *testing.T) {
	memory := make([]byte, STACK_START)
	start := uint32(PROG_START)
	returnAddress := start + INSTRUCTION_SIZE
	putIE32Instruction(memory, start, JSR, 0, ADDR_IMMEDIATE, returnAddress)
	putIE32Instruction(memory, returnAddress, ADD, REG_A, ADDR_IMMEDIATE, 1)
	putIE32Instruction(memory, returnAddress+INSTRUCTION_SIZE, RTS, 0, ADDR_IMMEDIATE, 0)
	block := scanIE32FusedBlock(memory, start, 0)
	if len(block) != 1 || block[0].fusedLeafCall {
		t.Fatalf("return-address JSR was fused: %#v", block)
	}
}

func TestIE32JITDirectManifestFormsMatchStepOne(t *testing.T) {
	if !ie32JITRuntimeAvailable() {
		t.Skip("IE32 JIT unavailable on this platform")
	}
	forms := make([]ie32OpcodeForm, 0)
	for form, kind := range ie32FormLowering {
		if kind == ie32LoweringDirect && form.AddrMode <= ADDR_DIRECT {
			forms = append(forms, form)
		}
	}
	sort.Slice(forms, func(i, j int) bool {
		if forms[i].Opcode != forms[j].Opcode {
			return forms[i].Opcode < forms[j].Opcode
		}
		return forms[i].AddrMode < forms[j].AddrMode
	})
	for _, form := range forms {
		t.Run(fmt.Sprintf("%s/mode-%d", ie32OpcodeName(form.Opcode), form.AddrMode), func(t *testing.T) {
			step := newIE32DirectManifestCPU(form)
			interpreter := newIE32DirectManifestCPU(form)
			jit := newIE32DirectManifestCPU(form)
			step.StepOne()
			interpreter.running.Store(false)
			if err := interpreter.SetJITEnabled(false); err != nil {
				t.Fatalf("disable interpreter JIT: %v", err)
			}
			interpreter.running.Store(true)
			interpreter.Execute()
			assertIE32DirectManifestState(t, step, interpreter, form)
			jit.Execute()
			assertIE32DirectManifestState(t, step, jit, form)
			if got := jit.JITStats().DirectInstructions; got == 0 {
				t.Fatalf("direct manifest form did not execute generated code")
			}
		})
	}
}

func newIE32DirectManifestCPU(form ie32OpcodeForm) *CPU {
	cpu := NewCPU(NewMachineBus())
	cpu.A, cpu.B, cpu.X = 10, 3, 0x400
	cpu.Write32(0x400, 3)
	cpu.Write32(STACK_START-WORD_SIZE, PROG_START+INSTRUCTION_SIZE)
	cpu.SP = STACK_START - WORD_SIZE
	reg, operand := byte(REG_B), uint32(1)
	switch form.Opcode {
	case STORE:
		reg, operand = REG_A, 0x400
	case STA, STX, STY, STZ, STB, STC, STD, STE, STF, STG, STH, STS, STT, STU, STV, STW:
		operand = 0x400
	case LOAD, LDA, LDX, LDY, LDZ, LDB, LDC, LDD, LDE, LDF, LDG, LDH, LDS, LDT, LDU, LDV, LDW:
		switch form.AddrMode {
		case ADDR_REGISTER:
			operand = REG_A
		case ADDR_REG_IND:
			operand = REG_X
		case ADDR_DIRECT, ADDR_MEM_IND:
			operand = 0x400
		}
	case ADD, SUB, MUL, AND, OR, XOR:
		reg = REG_A
		if form.AddrMode == ADDR_REGISTER {
			operand = REG_B
		} else if form.AddrMode == ADDR_DIRECT || form.AddrMode == ADDR_MEM_IND {
			operand = 0x400
		} else {
			operand = 3
		}
	case DIV, MOD:
		reg, operand = REG_A, 3
		if form.AddrMode == ADDR_REGISTER {
			operand = REG_B
		}
	case SHL, SHR:
		reg, operand = REG_A, 1
	case INC, DEC:
		if form.AddrMode == ADDR_DIRECT || form.AddrMode == ADDR_MEM_IND {
			operand = 0x400
			if form.AddrMode == ADDR_MEM_IND {
				cpu.Write32(operand, 0x404)
			}
		} else if form.AddrMode == ADDR_REG_IND {
			operand = REG_X
		} else {
			operand = REG_A
		}
	case JMP, JNZ, JZ, JGT, JGE, JLT, JLE, JSR:
		reg, operand = REG_A, PROG_START+INSTRUCTION_SIZE
	case RTS, RTI:
		cpu.SP = STACK_START - WORD_SIZE
	case PUSH, POP, SEI, CLI, NOT, NOP:
		reg = REG_A
	}
	if (form.Opcode == STORE || ie32IsNamedStore(form.Opcode)) && form.AddrMode == ADDR_REG_IND {
		operand = REG_X
	}
	if (form.Opcode == STORE || ie32IsNamedStore(form.Opcode)) && form.AddrMode == ADDR_MEM_IND {
		operand = 0x400
		cpu.Write32(operand, 0x404)
	}
	putIE32Instruction(cpu.memory, PROG_START, form.Opcode, reg, form.AddrMode, operand)
	cpu.memory[PROG_START+INSTRUCTION_SIZE] = HALT
	return cpu
}

func assertIE32DirectManifestState(t *testing.T, want, got *CPU, form ie32OpcodeForm) {
	t.Helper()
	if want.PC != got.PC || want.SP != got.SP || want.A != got.A || want.X != got.X || want.Y != got.Y || want.Z != got.Z ||
		want.B != got.B || want.C != got.C || want.D != got.D || want.E != got.E || want.F != got.F || want.G != got.G ||
		want.H != got.H || want.S != got.S || want.T != got.T || want.U != got.U || want.V != got.V || want.W != got.W {
		t.Fatalf("architectural state differs for %s mode %d", ie32OpcodeName(form.Opcode), form.AddrMode)
	}
	for _, addr := range []uint32{0x400, 0x404, STACK_START - WORD_SIZE} {
		if w, g := want.Read32(addr), got.Read32(addr); w != g {
			t.Fatalf("memory %#x = %#x, want %#x", addr, g, w)
		}
	}
	if want.interruptEnabled.Load() != got.interruptEnabled.Load() || want.inInterrupt.Load() != got.inInterrupt.Load() {
		t.Fatalf("interrupt state differs for %s mode %d", ie32OpcodeName(form.Opcode), form.AddrMode)
	}
}

func TestIE32JITManifestKeepsObservationOpcodesAtHelperBoundary(t *testing.T) {
	for _, opcode := range []byte{WAIT} {
		if got := ie32OpcodeManifest[opcode].Kind; got != ie32LoweringHelper {
			t.Fatalf("opcode %#x lowering kind = %d, want helper", opcode, got)
		}
	}
	for _, opcode := range []byte{PUSH, POP, JSR, RTS} {
		if got := ie32OpcodeManifest[opcode].Kind; got != ie32LoweringDirect {
			t.Fatalf("stack opcode %#x lowering kind = %d, want direct", opcode, got)
		}
	}
	for _, opcode := range []byte{RTI, SEI, CLI} {
		if got := ie32FormLowering[ie32OpcodeForm{Opcode: opcode, AddrMode: ADDR_IMMEDIATE}]; got != ie32LoweringHelper {
			t.Fatalf("interrupt opcode %#x lowering kind = %d, want helper", opcode, got)
		}
	}
	if got := ie32OpcodeManifest[HALT].Kind; got != ie32LoweringHalt {
		t.Fatalf("HALT lowering kind = %d, want halt", got)
	}
}

func TestIE32JITFormLedgerDistinguishesStoreIndirection(t *testing.T) {
	if got := ie32FormLowering[ie32OpcodeForm{STORE, ADDR_DIRECT}]; got != ie32LoweringDirect {
		t.Fatalf("direct store form kind = %d", got)
	}
	for _, mode := range []byte{ADDR_MEM_IND, ADDR_REG_IND} {
		if got := ie32FormLowering[ie32OpcodeForm{STORE, mode}]; got != ie32LoweringHelper {
			t.Fatalf("indirect store mode %#x kind = %d", mode, got)
		}
	}
}

func TestIE32JITScanBlockStopsAtControlFlowAndBounds(t *testing.T) {
	mem := make([]byte, PROG_START+3*INSTRUCTION_SIZE)
	mem[PROG_START] = NOP
	mem[PROG_START+INSTRUCTION_SIZE] = LDA
	mem[PROG_START+2*INSTRUCTION_SIZE] = JMP
	block := scanIE32Block(mem, PROG_START, 0)
	if len(block) != 3 || block[2].Opcode != JMP {
		t.Fatalf("block = %#v, want NOP,LDA,JMP", block)
	}
	if got := scanIE32Block(mem, PROG_START+3*INSTRUCTION_SIZE, 0); len(got) != 0 {
		t.Fatalf("out-of-bounds block = %#v", got)
	}
}

func TestIE32JITDirectRAMAdmissionUsesActiveVisibleCeiling(t *testing.T) {
	bus := NewMachineBus()
	bus.SetSizing(MemorySizing{ActiveVisibleRAM: 0x2000})
	cpu := NewCPU(bus)
	if !ie32CanDirectRAMRead(cpu, 0x1FFC) {
		t.Fatal("last visible aligned word rejected")
	}
	if ie32CanDirectRAMRead(cpu, 0x2000) {
		t.Fatal("address beyond active visible RAM admitted")
	}
	if ie32CanDirectRAMRead(cpu, IO_REGION_START) {
		t.Fatal("MMIO address admitted")
	}
	if ie32CanDirectRAMRead(cpu, 0x1001) {
		t.Fatal("unaligned address admitted")
	}
}

func TestIE32JITDirectRAMWriteAdmissionRejectsVRAM(t *testing.T) {
	cpu := NewCPU(NewMachineBus())
	cpu.vramDirect = make([]byte, 16)
	cpu.vramStart = 0x400
	cpu.vramEnd = 0x410
	if ie32CanDirectRAMWrite(cpu, 0x400) {
		t.Fatal("direct VRAM write was admitted")
	}
	if !ie32CanDirectRAMWrite(cpu, 0x410) {
		t.Fatal("non-VRAM RAM write was rejected")
	}
	if ie32CanDirectRAMWrite(cpu, IO_REGION_START) {
		t.Fatal("MMIO write was admitted")
	}
}

func TestIE32JITDirectStoreStopsBeforeItMutatesRemainingBlock(t *testing.T) {
	block := []ie32DecodedInstruction{
		{PC: PROG_START, Opcode: STORE, AddrMode: ADDR_DIRECT, Operand: PROG_START + INSTRUCTION_SIZE},
		{PC: PROG_START + INSTRUCTION_SIZE, Opcode: LDA, AddrMode: ADDR_IMMEDIATE},
	}
	if !ie32WriteMutatesRemainingBlock(block[0], block) {
		t.Fatal("write over the following instruction was admitted")
	}
	block[0].Operand = PROG_START - WORD_SIZE
	if ie32WriteMutatesRemainingBlock(block[0], block) {
		t.Fatal("non-overlapping write was rejected")
	}
}

func TestIE32JITElidesOnlyUnobservedImmediateLoads(t *testing.T) {
	block := []ie32DecodedInstruction{
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 1},
		{Opcode: LOAD, Reg: REG_B, AddrMode: ADDR_IMMEDIATE, Operand: 2},
		{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_IMMEDIATE, Operand: 3},
	}
	if !ie32ElideDeadImmediateLoad(block, 0) {
		t.Fatal("immediate load overwritten in an independent immediate run was not elided")
	}

	block[1] = ie32DecodedInstruction{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_REGISTER, Operand: REG_A}
	if ie32ElideDeadImmediateLoad(block, 0) {
		t.Fatal("load whose source is the overwritten register was elided")
	}
	block[1] = ie32DecodedInstruction{Opcode: LOAD, Reg: REG_A, AddrMode: ADDR_REG_IND, Operand: REG_A}
	if ie32ElideDeadImmediateLoad(block, 0) {
		t.Fatal("load whose address is the overwritten register was elided")
	}
}

// jit_wasm_ie64_emit.go - IE64 block translator for the wasm JIT backend.
//
// Translates the scanner's []JITInstr into one wasm function per block. The
// function has signature (ctxPtr i32) -> () and operates on the JITContext
// through the same jitCtxOff* offsets the native emitters use: on js/wasm
// the context, register file and guest RAM all live in Go's linear memory,
// which the generated module imports as env.mem. Interpreter semantics
// (cpu_ie64.go StepOne) are the reference, not the amd64 emitter, wherever
// the two disagree (e.g. Q-size ALU immediates zero-extend here).
//
// Pure Go and untagged: the differential test suite executes the emitted
// blocks under wazero on native hosts.

/*
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"fmt"
	"sort"
)

// Local variable indices inside every generated block function.
const (
	wasmLocCtx     = 0  // i32 param: JITContext address
	wasmLocRegs    = 1  // i32: RegsPtr
	wasmLocMem     = 2  // i32: MemPtr (guest RAM base)
	wasmLocV32     = 3  // i32 scratch
	wasmLocA       = 4  // i64 scratch (memory ops: effective address)
	wasmLocB       = 5  // i64 scratch (memory ops: store value)
	wasmLocT0      = 6  // i64 scratch
	wasmLocT1      = 7  // i64 scratch
	wasmLocBmp     = 8  // i32: IOBitmapPtr
	wasmLocCpb     = 9  // i32: CodePageBitmapPtr
	wasmLocCpbLen  = 10 // i64: CodePageBitmapLen
	wasmLocHighS   = 11 // i64: CodeHighStartPage
	wasmLocHighE   = 12 // i64: CodeHighEndPage
	wasmLocMemSize = 13 // i64: MemSize (zero-extended)
	wasmLocP0      = 14 // i64: SMC probe first page (T0/T1 stay live across smcProbe)
	wasmLocP1      = 15 // i64: SMC probe last page
	wasmLocFpu     = 16 // i32: FPUPtr
	wasmLocSpans   = 17 // i32: CodePageSpansPtr (per-page [min,max] code extents)
	wasmLocS0      = 18 // i32 scratch (SMC probe span clamp)
)

// wasmBlockLocals lists the extra locals (beyond the ctx param) every block
// function declares.
var wasmBlockLocals = []byte{
	wasmTypeI32, wasmTypeI32, wasmTypeI32,
	wasmTypeI64, wasmTypeI64, wasmTypeI64, wasmTypeI64,
	wasmTypeI32, wasmTypeI32,
	wasmTypeI64, wasmTypeI64, wasmTypeI64, wasmTypeI64,
	wasmTypeI64, wasmTypeI64,
	wasmTypeI32,
	wasmTypeI32, wasmTypeI32,
}

const wasmGPRLocalBase = 19

type wasmGPRPlan struct {
	locals   [32]uint32
	dirty    [32]bool
	bindings []byte
}

type wasmFPPlan struct {
	locals   [8]uint32
	dirty    [8]bool
	bindings []byte // even architectural FP slot naming each FP64 pair
}

func (p *wasmFPPlan) local(r byte) uint32 {
	if p == nil || r >= 16 {
		return 0
	}
	return p.locals[(r&0x0e)/2]
}

func wasmBuildFPPlan(instrs []JITInstr, localBase uint32) *wasmFPPlan {
	var uses [8]int
	var dirty [8]bool
	for i := range instrs {
		ins := &instrs[i]
		switch ins.opcode {
		case OP_DMOV, OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV,
			OP_DABS, OP_DNEG, OP_DSQRT, OP_DINT:
			owner := (ins.rd & 0x0e) / 2
			uses[owner]++
			dirty[owner] = true
		case OP_DCMP:
		case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS,
			OP_BRA, OP_JMP, OP_HALT64, OP_NOP64:
			continue
		default:
			return nil
		}
		for _, r := range []byte{ins.rs, ins.rt} {
			if r < 16 {
				uses[(r&0x0e)/2]++
			}
		}
	}
	type candidate struct{ pair, use int }
	var candidates []candidate
	for pair, use := range uses {
		if use != 0 {
			candidates = append(candidates, candidate{pair, use})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].use != candidates[j].use {
			return candidates[i].use > candidates[j].use
		}
		return candidates[i].pair < candidates[j].pair
	})
	p := &wasmFPPlan{}
	for i, c := range candidates {
		p.locals[c.pair] = localBase + uint32(i)
		p.dirty[c.pair] = dirty[c.pair]
		p.bindings = append(p.bindings, byte(c.pair*2))
	}
	return p
}

func (p *wasmGPRPlan) local(r byte) uint32 {
	if p == nil || r >= 32 {
		return 0
	}
	return p.locals[r]
}

// wasmBuildGPRPlan retains hot integer registers only in blocks that cannot
// leave through a helper. This first residency tier therefore has one exit
// invariant: every dirty local is spilled by exit/exitDyn before returning.
func wasmBuildGPRPlan(instrs []JITInstr) *wasmGPRPlan {
	var uses [32]int
	var dirty [32]bool
	for i := range instrs {
		ins := &instrs[i]
		switch ins.opcode {
		case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA,
			OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_DIVU, OP_DIVS,
			OP_MOD64, OP_MODS, OP_NEG, OP_MULHU, OP_MULHS,
			OP_AND64, OP_OR64, OP_EOR, OP_NOT64,
			OP_LSL, OP_LSR, OP_ASR, OP_CLZ, OP_CTZ, OP_POPCNT,
			OP_BSWAP, OP_SEXT, OP_ROL, OP_ROR:
			if ins.rd != 0 && ins.rd != 31 {
				uses[ins.rd]++
				dirty[ins.rd] = true
			}
		case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS,
			OP_BRA, OP_JMP, OP_HALT64, OP_NOP64:
		default:
			return nil
		}
		if ins.rs != 0 && ins.rs != 31 {
			uses[ins.rs]++
		}
		if ins.xbit == 0 && ins.rt != 0 && ins.rt != 31 {
			uses[ins.rt]++
		}
	}
	type candidate struct {
		reg byte
		use int
	}
	var candidates []candidate
	for r := byte(1); r < 31; r++ {
		if uses[r] != 0 {
			candidates = append(candidates, candidate{r, uses[r]})
		}
	}
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].use != candidates[j].use {
			return candidates[i].use > candidates[j].use
		}
		return candidates[i].reg < candidates[j].reg
	})
	if len(candidates) > 16 {
		candidates = candidates[:16]
	}
	p := &wasmGPRPlan{}
	for i, c := range candidates {
		p.locals[c.reg] = wasmGPRLocalBase + uint32(i)
		p.dirty[c.reg] = dirty[c.reg]
		p.bindings = append(p.bindings, c.reg)
	}
	return p
}

// wasmSupportedOpcode is the backend capability allowlist for the current
// milestone. It is deliberately explicit: needsFallback is a block-entry
// validity check, not a capability predicate, and cannot substitute. A block
// containing any opcode outside this set is rejected before translation.
func wasmSupportedOpcode(op byte) bool {
	switch op {
	case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA,
		OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_DIVU, OP_DIVS,
		OP_MOD64, OP_MODS, OP_NEG, OP_MULHU, OP_MULHS,
		OP_AND64, OP_OR64, OP_EOR, OP_NOT64,
		OP_LSL, OP_LSR, OP_ASR, OP_CLZ, OP_CTZ, OP_POPCNT,
		OP_BSWAP, OP_SEXT, OP_ROL, OP_ROR,
		OP_LOAD, OP_STORE,
		OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS,
		OP_BRA, OP_JMP, OP_HALT64,
		OP_JSR64, OP_RTS64, OP_PUSH64, OP_POP64, OP_JSR_IND,
		OP_DMOV, OP_DLOAD, OP_DSTORE,
		OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV,
		OP_DABS, OP_DNEG, OP_DSQRT, OP_DINT,
		OP_DCMP, OP_DCVTIF, OP_DCVTFI,
		OP_NOP64:
		return true
	}
	return false
}

// wasmCompileBlock translates a scanned block into a single-function wasm
// module. The module imports env.mem and exports "block": (ctx i32) -> ().
func wasmCompileBlock(instrs []JITInstr, startPC uint64) ([]byte, error) {
	return wasmCompileBlocks([]wasmRegionBlock{{pc: startPC, instrs: instrs}})
}

const (
	wasmRegionMaxBlocks       = 8
	wasmRegionMaxInstructions = 512
)

type wasmRegionBlock struct {
	pc     uint64
	instrs []JITInstr
}

// wasmFormRegion follows statically resolved BRA edges. Forward edges add new
// blocks; a backward edge closes the region only when it targets a block that
// is already present, allowing wasmCompileBlocks to emit one structured loop.
// Dynamic and out-of-region edges remain ordinary external exits.
func wasmFormRegion(memory []byte, startPC uint64) []wasmRegionBlock {
	pc := startPC
	total := 0
	seen := make(map[uint64]struct{}, wasmRegionMaxBlocks)
	blocks := make([]wasmRegionBlock, 0, wasmRegionMaxBlocks)
	for len(blocks) < wasmRegionMaxBlocks && total < wasmRegionMaxInstructions {
		if _, ok := seen[pc]; ok || pc >= uint64(len(memory)) {
			break
		}
		instrs := scanBlock(memory, pc)
		if len(instrs) == 0 || total+len(instrs) > wasmRegionMaxInstructions {
			break
		}
		valid := true
		for i := range instrs {
			if !wasmInstructionSupported(&instrs[i]) {
				valid = false
				break
			}
		}
		if !valid {
			break
		}
		seen[pc] = struct{}{}
		blocks = append(blocks, wasmRegionBlock{pc: pc, instrs: instrs})
		total += len(instrs)
		last := &instrs[len(instrs)-1]
		instrPC := pc + uint64(last.pcOffset)
		if last.opcode != OP_BRA {
			break
		}
		next := uint64(int64(instrPC) + int64(int32(last.imm32)))
		if next <= pc {
			if _, ok := seen[next]; ok {
				break
			}
			break
		}
		pc = next
	}
	if len(blocks) < 2 {
		return nil
	}
	return blocks
}

func wasmInstructionSupported(ins *JITInstr) bool {
	return wasmSupportedOpcode(ins.opcode) && !ins.mmuBail && ins.fusedFlag == 0 &&
		(!isIE64FPUOpcode(ins.opcode) || validIE64FPUEncoding(ins.opcode, ins.rd, ins.rs, ins.rt))
}

func wasmFPCCSetter(ins *JITInstr) bool {
	switch ins.opcode {
	case OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV, OP_DABS, OP_DNEG, OP_DSQRT,
		OP_DINT, OP_DCVTIF, OP_DLOAD:
		return true
	case OP_DCMP:
		return ins.rd != 0
	}
	return false
}

// wasmFPSRCCLive marks condition-code writes that reach an observable exit.
// Register-only FP64 setters cannot fault, so an earlier write is dead when a
// later setter overwrites it first. Memory operations and control transfers
// are barriers because they may leave the function before that overwrite.
func wasmFPSRCCLive(instrs []JITInstr) []bool {
	emit := make([]bool, len(instrs))
	live := true
	for i := len(instrs) - 1; i >= 0; i-- {
		ins := &instrs[i]
		if wasmFPCCSetter(ins) {
			emit[i] = live
			live = false
		}
		switch ins.opcode {
		case OP_LOAD, OP_STORE, OP_DLOAD, OP_DSTORE, OP_PUSH64, OP_POP64,
			OP_JSR64, OP_JSR_IND, OP_RTS64, OP_BEQ, OP_BNE, OP_BLT, OP_BGE,
			OP_BGT, OP_BLE, OP_BHI, OP_BLS, OP_BRA, OP_JMP, OP_HALT64:
			live = true
		}
	}
	return emit
}

// wasmCompileBlocks emits one wasm function for a static region. The
// flattened residency plans are loaded once in the prologue and spilled only
// at external exits, retaining GPR and FP64 mappings across internal edges and
// structured back edges.
func wasmCompileBlocks(blocks []wasmRegionBlock) ([]byte, error) {
	if len(blocks) == 0 {
		return nil, fmt.Errorf("wasm JIT: empty region")
	}
	instrs := make([]JITInstr, 0)
	for _, block := range blocks {
		if len(block.instrs) == 0 {
			return nil, fmt.Errorf("wasm JIT: empty block at %#x", block.pc)
		}
		instrs = append(instrs, block.instrs...)
	}
	startPC := blocks[0].pc
	if len(instrs) == 0 {
		return nil, fmt.Errorf("wasm JIT: empty block at %#x", startPC)
	}
	// A block that STARTS with HALT must stay on the interpreter (StepOne
	// clears cpu.running); compiled, its RetPC-stays-put exit would make the
	// dispatcher re-enter it forever. Mirrors needsFallback on native, which
	// rejects HALT-first blocks for the same reason.
	if instrs[0].opcode == OP_HALT64 {
		return nil, fmt.Errorf("wasm JIT: block starts with HALT at %#x", startPC)
	}
	for i := range instrs {
		ins := &instrs[i]
		if !wasmSupportedOpcode(ins.opcode) {
			return nil, fmt.Errorf("wasm JIT: unsupported opcode %#02x at +%d", ins.opcode, ins.pcOffset)
		}
		if ins.mmuBail {
			return nil, fmt.Errorf("wasm JIT: mmuBail instruction at +%d", ins.pcOffset)
		}
		if ins.fusedFlag != 0 {
			return nil, fmt.Errorf("wasm JIT: fused instruction at +%d", ins.pcOffset)
		}
		if isIE64FPUOpcode(ins.opcode) && !validIE64FPUEncoding(ins.opcode, ins.rd, ins.rs, ins.rt) {
			// Invalid FP register encodings halt the CPU architecturally;
			// leave them to the interpreter.
			return nil, fmt.Errorf("wasm JIT: invalid FPU encoding at +%d", ins.pcOffset)
		}
	}

	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)

	hasFP := false
	for i := range instrs {
		if isIE64FPUOpcode(instrs[i].opcode) {
			hasFP = true
			break
		}
	}

	needsMem, needsSMC := false, false
	for i := range instrs {
		switch instrs[i].opcode {
		case OP_LOAD, OP_POP64, OP_RTS64, OP_DLOAD:
			needsMem = true
		case OP_STORE, OP_PUSH64, OP_JSR64, OP_JSR_IND, OP_DSTORE:
			needsMem = true
			needsSMC = true
		}
	}

	gprPlan := wasmBuildGPRPlan(instrs)
	localBase := uint32(wasmGPRLocalBase)
	if gprPlan != nil {
		localBase += uint32(len(gprPlan.bindings))
	}
	fpPlan := wasmBuildFPPlan(instrs, localBase)
	if fpPlan != nil {
		localBase += uint32(len(fpPlan.bindings))
	}
	retCountLocal := localBase
	loopPlan := (*ie64LoopPlan)(nil)
	if len(blocks) == 1 {
		loopPlan = ie64AnalyseLoop(instrs, startPC)
	} else {
		r := &ie64Region{entryPC: startPC}
		for _, block := range blocks {
			r.blockPCs = append(r.blockPCs, block.pc)
			r.blocks = append(r.blocks, block.instrs)
		}
		loopPlan, _ = ie64AnalyseRegionLoop(r)
	}
	e := &wasmBlockEmitter{b: &wasmBody{}, needsMem: needsMem, needsSMC: needsSMC, needsFP: hasFP, gprPlan: gprPlan, fpPlan: fpPlan, loopPlan: loopPlan}
	if loopPlan != nil {
		e.retCountLocal = retCountLocal
	}

	loopBlockIdx := -1
	if len(blocks) > 1 {
		lastBlock := &blocks[len(blocks)-1]
		last := &lastBlock.instrs[len(lastBlock.instrs)-1]
		if last.opcode == OP_BRA {
			instrPC := lastBlock.pc + uint64(last.pcOffset)
			target := uint64(int64(instrPC) + int64(int32(last.imm32)))
			for i := range blocks {
				if blocks[i].pc == target {
					loopBlockIdx = i
					e.retCountLocal = retCountLocal
					break
				}
			}
		}
	}
	livenessInstrs := append([]JITInstr(nil), instrs...)
	flatIdx := 0
	for blockIdx := range blocks {
		block := &blocks[blockIdx]
		last := &block.instrs[len(block.instrs)-1]
		instrPC := block.pc + uint64(last.pcOffset)
		target := uint64(int64(instrPC) + int64(int32(last.imm32)))
		internalForward := blockIdx+1 < len(blocks) && target == blocks[blockIdx+1].pc
		internalBack := blockIdx == len(blocks)-1 && loopBlockIdx >= 0 && target == blocks[loopBlockIdx].pc
		if last.opcode == OP_BRA && (internalForward || internalBack) {
			livenessInstrs[flatIdx+len(block.instrs)-1].opcode = OP_NOP64
		}
		flatIdx += len(block.instrs)
	}
	fpCCLive := wasmFPSRCCLive(livenessInstrs)
	if hasFP {
		// Internal helper first, so the block function can call it.
		ccType := m.addType([]byte{wasmTypeI32, wasmTypeI64}, nil)
		e.ccFunc = m.addFunc(ccType, []byte{wasmTypeI32}, wasmEmitCCUpdate64())
	}
	e.prologue()
	idx := uint32(0)
	for blockIdx := range blocks {
		block := &blocks[blockIdx]
		if blockIdx == loopBlockIdx {
			if loopPlan != nil && len(loopPlan.accesses) != 0 {
				e.emitLoopPrechecks()
			}
			e.b.loop()
		}
		for i := range block.instrs {
			ins := &block.instrs[i]
			if len(blocks) == 1 && loopPlan != nil && int(idx) == loopPlan.head {
				if len(loopPlan.accesses) != 0 {
					e.emitLoopPrechecks()
				}
				e.b.loop()
			}
			instrPC := block.pc + uint64(ins.pcOffset)
			internalBRA := i == len(block.instrs)-1 && ins.opcode == OP_BRA && blockIdx+1 < len(blocks) &&
				uint64(int64(instrPC)+int64(int32(ins.imm32))) == blocks[blockIdx+1].pc
			if internalBRA {
				idx++
				continue
			}
			internalBack := i == len(block.instrs)-1 && ins.opcode == OP_BRA && blockIdx == len(blocks)-1 && loopBlockIdx >= 0
			if internalBack {
				// If the dispatch budget is exhausted, expose the loop target and
				// accumulated retired count to the outer driver. Otherwise consume
				// one transition, advance the dynamic count base by the number of
				// instructions in the repeated suffix, and branch to the loop head.
				e.b.localGet(wasmLocCtx)
				e.b.i32Load(2, jitCtxOffChainBudget)
				e.b.op(wasmOpI32Eqz)
				e.b.ifVoid()
				e.exit(blocks[loopBlockIdx].pc, idx+1)
				e.b.op(wasmOpReturn)
				e.b.end()
				e.b.localGet(wasmLocCtx)
				e.b.localGet(wasmLocCtx)
				e.b.i32Load(2, jitCtxOffChainBudget)
				e.b.i32Const(1)
				e.b.op(wasmOpI32Sub)
				e.b.i32Store(2, jitCtxOffChainBudget)
				e.b.localGet(retCountLocal)
				e.b.i32Const(int32(idx + 1 - uint32(flatBlockStart(blocks, loopBlockIdx))))
				e.b.op(wasmOpI32Add)
				e.b.localSet(retCountLocal)
				e.b.br(0)
				idx++
				continue
			}
			e.emitFPCC = fpCCLive[idx]
			e.instr(ins, idx, instrPC)
			idx++
		}
	}
	if len(blocks) == 1 && loopPlan != nil {
		e.b.end()
	}
	if loopBlockIdx >= 0 {
		e.b.end()
	}
	// Fall-off-the-end exit: the block retired every instruction and the
	// next PC is the byte after the last one.
	lastBlock := blocks[len(blocks)-1]
	e.exit(lastBlock.pc+uint64(len(lastBlock.instrs))*8, uint32(len(instrs)))
	e.b.end()

	locals := append([]byte(nil), wasmBlockLocals...)
	if gprPlan != nil {
		for range gprPlan.bindings {
			locals = append(locals, wasmTypeI64)
		}
	}
	if fpPlan != nil {
		for range fpPlan.bindings {
			locals = append(locals, wasmTypeF64)
		}
	}
	if loopBlockIdx >= 0 || loopPlan != nil {
		locals = append(locals, wasmTypeI32)
	}
	fn := m.addFunc(typ, locals, e.b.code)
	m.exportFunc("block", fn)
	return m.build(), nil
}

func flatBlockStart(blocks []wasmRegionBlock, blockIdx int) int {
	start := 0
	for i := 0; i < blockIdx; i++ {
		start += len(blocks[i].instrs)
	}
	return start
}

// wasmBlockEmitter emits the body of one block function.
type wasmBlockEmitter struct {
	b      *wasmBody
	ccFunc uint32 // index of the FPSR condition-code helper (FP blocks only)

	// Prologue trimming: context fields are only loaded into locals when
	// the block contains instructions that read them. Chained dispatch
	// re-enters blocks thousands of times, so the prologue is hot.
	needsMem      bool
	needsSMC      bool
	needsFP       bool
	gprPlan       *wasmGPRPlan
	fpPlan        *wasmFPPlan
	emitFPCC      bool
	retCountLocal uint32
	loopPlan      *ie64LoopPlan
}

func (e *wasmBlockEmitter) prologue() {
	b := e.b
	// Pointer fields wrap to i32: addresses fit 32 bits on wasm even though
	// the fields are 8 bytes wide.
	loadPtr := func(off uint32, local uint32) {
		b.localGet(wasmLocCtx)
		b.i64Load(3, off)
		b.op(wasmOpI32WrapI64)
		b.localSet(local)
	}
	loadPtr(jitCtxOffRegsPtr, wasmLocRegs)
	if e.gprPlan != nil {
		for _, r := range e.gprPlan.bindings {
			b.localGet(wasmLocRegs)
			b.i64Load(3, uint32(r)*8)
			b.localSet(e.gprPlan.local(r))
		}
	}
	if e.needsMem {
		loadPtr(jitCtxOffMemPtr, wasmLocMem)
		loadPtr(jitCtxOffIOBitmapPtr, wasmLocBmp)
		b.localGet(wasmLocCtx)
		b.memOp(wasmOpI64Load32U, 2, jitCtxOffMemSize)
		b.localSet(wasmLocMemSize)
	}
	if e.needsSMC {
		loadPtr(jitCtxOffCodePageBitmapPtr, wasmLocCpb)
		loadPtr(jitCtxOffCodePageSpansPtr, wasmLocSpans)
		b.localGet(wasmLocCtx)
		b.memOp(wasmOpI64Load32U, 2, jitCtxOffCodePageBitmapLen)
		b.localSet(wasmLocCpbLen)
		b.localGet(wasmLocCtx)
		b.i64Load(3, jitCtxOffCodeHighStartPage)
		b.localSet(wasmLocHighS)
		b.localGet(wasmLocCtx)
		b.i64Load(3, jitCtxOffCodeHighEndPage)
		b.localSet(wasmLocHighE)
	}
	if e.needsFP {
		loadPtr(jitCtxOffFPUPtr, wasmLocFpu)
	}
	if e.fpPlan != nil {
		for _, r := range e.fpPlan.bindings {
			b.localGet(wasmLocFpu)
			b.memOp(wasmOpF64Load, 2, fpPairOff(r))
			b.localSet(e.fpPlan.local(r))
		}
	}
}

// exit writes RetPC and RetCount. The caller appends the final end (or
// return) itself where needed.
func (e *wasmBlockEmitter) exit(retPC uint64, retCount uint32) {
	b := e.b
	e.spillGPRs()
	e.spillFPs()
	b.localGet(wasmLocCtx)
	b.i64Const(int64(retPC))
	b.i64Store(3, jitCtxOffRetPC)
	b.localGet(wasmLocCtx)
	e.pushRetCount(retCount)
	b.i32Store(2, jitCtxOffRetCount)
}

func (e *wasmBlockEmitter) pushRetCount(retCount uint32) {
	if e.retCountLocal != 0 {
		e.b.localGet(e.retCountLocal)
		e.b.i32Const(int32(retCount))
		e.b.op(wasmOpI32Add)
		return
	}
	e.b.i32Const(int32(retCount))
}

// loadReg pushes regs[r] as i64; R0 reads as zero.
func (e *wasmBlockEmitter) loadReg(r byte) {
	if r == 0 {
		e.b.i64Const(0)
		return
	}
	if local := e.gprPlan.local(r); local != 0 {
		e.b.localGet(local)
		return
	}
	e.b.localGet(wasmLocRegs)
	e.b.i64Load(3, uint32(r)*8)
}

func (e *wasmBlockEmitter) spillGPRs() {
	if e.gprPlan == nil {
		return
	}
	for _, r := range e.gprPlan.bindings {
		if !e.gprPlan.dirty[r] {
			continue
		}
		e.b.localGet(wasmLocRegs)
		e.b.localGet(e.gprPlan.local(r))
		e.b.i64Store(3, uint32(r)*8)
	}
}

func (e *wasmBlockEmitter) spillFPs() {
	if e.fpPlan == nil {
		return
	}
	for _, r := range e.fpPlan.bindings {
		pair := (r & 0x0e) / 2
		if !e.fpPlan.dirty[pair] {
			continue
		}
		e.b.localGet(wasmLocFpu)
		e.b.localGet(e.fpPlan.local(r))
		e.b.memOp(wasmOpF64Store, 2, fpPairOff(r))
	}
}

func (e *wasmBlockEmitter) storeReg(r byte) {
	if r == 0 {
		e.b.op(wasmOpDrop)
		return
	}
	if local := e.gprPlan.local(r); local != 0 {
		e.b.localSet(local)
		return
	}
	// The computed value is already on the stack. Save it temporarily so the
	// memory address can precede it for i64.store.
	e.b.localSet(wasmLocT0)
	e.b.localGet(wasmLocRegs)
	e.b.localGet(wasmLocT0)
	e.b.i64Store(3, uint32(r)*8)
}

// operand3 pushes the third operand: zero-extended imm32 when xbit is set,
// otherwise regs[rt]. Matches the interpreter for every size, including Q.
func (e *wasmBlockEmitter) operand3(ins *JITInstr) {
	if ins.xbit == 1 {
		e.b.i64Const(int64(uint64(ins.imm32)))
	} else {
		e.loadReg(ins.rt)
	}
}

// mask applies maskToSize to the value on the stack.
func (e *wasmBlockEmitter) mask(size byte) {
	if size == IE64_SIZE_Q {
		return
	}
	e.b.i64Const(int64(ie64SizeMask[size]))
	e.b.op(wasmOpI64And)
}

// sext sign-extends the i64 on the stack from the given size.
func (e *wasmBlockEmitter) sext(size byte) {
	switch size {
	case IE64_SIZE_B:
		e.b.op(wasmOpI64Extend8S)
	case IE64_SIZE_W:
		e.b.op(wasmOpI64Extend16S)
	case IE64_SIZE_L:
		e.b.op(wasmOpI64Extend32S)
	}
}

// instr emits one instruction. Every ALU op computes a value and stores it
// to rd; writes to R0 are dropped (R0 is architecturally zero) and the
// computation is side-effect free, so compute-then-drop is exact. Memory
// ops have their own emitters with fast-path checks and helper exits.
func (e *wasmBlockEmitter) instr(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	switch ins.opcode {
	case OP_NOP64:
		return
	case OP_LOAD:
		e.emitLoad(ins, idx, instrPC)
		return
	case OP_STORE:
		e.emitStore(ins, idx, instrPC)
		return
	case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
		e.emitCondBranch(ins, idx, instrPC)
		return
	case OP_BRA:
		e.exit(uint64(int64(instrPC)+int64(int32(ins.imm32))), idx+1)
		b.op(wasmOpReturn)
		return
	case OP_JMP:
		e.loadReg(ins.rs)
		b.i64Const(int64(int32(ins.imm32)))
		b.op(wasmOpI64Add)
		b.localSet(wasmLocT0)
		e.exitDyn(wasmLocT0, idx+1)
		b.op(wasmOpReturn)
		return
	case OP_HALT64:
		// RetPC is the HALT's own PC and the HALT itself is NOT counted as
		// retired: the dispatcher's interpreter executes it there (StepOne
		// clears cpu.running) and accounts for it exactly once.
		e.exit(instrPC, idx)
		b.op(wasmOpReturn)
		return
	case OP_JSR64, OP_JSR_IND:
		e.emitJSR(ins, idx, instrPC)
		return
	case OP_RTS64:
		e.emitRTS(ins, idx, instrPC)
		return
	case OP_PUSH64:
		e.emitPush(ins, idx, instrPC)
		return
	case OP_POP64:
		e.emitPop(ins, idx, instrPC)
		return
	case OP_DMOV, OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV,
		OP_DABS, OP_DNEG, OP_DSQRT, OP_DINT,
		OP_DCMP, OP_DCVTIF, OP_DCVTFI:
		e.emitFP64(ins)
		return
	case OP_DLOAD:
		e.emitDLoad(ins, idx, instrPC)
		return
	case OP_DSTORE:
		e.emitDStore(ins, idx, instrPC)
		return
	}
	e.value(ins)
	e.storeReg(ins.rd)
}

// exitDyn writes RetPC from an i64 local and RetCount as a constant.
func (e *wasmBlockEmitter) exitDyn(pcLocal uint32, retCount uint32) {
	b := e.b
	e.spillGPRs()
	e.spillFPs()
	b.localGet(wasmLocCtx)
	b.localGet(pcLocal)
	b.i64Store(3, jitCtxOffRetPC)
	b.localGet(wasmLocCtx)
	e.pushRetCount(retCount)
	b.i32Store(2, jitCtxOffRetCount)
}

// emitCondBranch: taken -> block exit at the target; not taken -> native
// fall-through to the next instruction (branches are not terminators).
func (e *wasmBlockEmitter) emitCondBranch(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.loadReg(ins.rs)
	e.loadReg(ins.rt)
	switch ins.opcode {
	case OP_BEQ:
		b.op(wasmOpI64Eq)
	case OP_BNE:
		b.op(wasmOpI64Ne)
	case OP_BLT:
		b.op(wasmOpI64LtS)
	case OP_BGE:
		b.op(wasmOpI64GeS)
	case OP_BGT:
		b.op(wasmOpI64GtS)
	case OP_BLE:
		b.op(wasmOpI64LeS)
	case OP_BHI:
		b.op(wasmOpI64GtU)
	case OP_BLS:
		b.op(wasmOpI64LeU)
	}
	if e.loopPlan != nil && int(idx) == e.loopPlan.back {
		b.ifVoid()
		if !e.loopPlan.bounded {
			b.localGet(e.retCountLocal)
			b.i32Const(int32(e.loopPlan.bodySize))
			b.op(wasmOpI32Add)
			b.i32Const(ie64JITLoopBudget)
			b.op(wasmOpI32GeU)
			b.ifVoid()
			e.exit(e.loopPlan.headPC, idx+1)
			b.op(wasmOpReturn)
			b.end()
		}
		b.localGet(e.retCountLocal)
		b.i32Const(int32(e.loopPlan.bodySize))
		b.op(wasmOpI32Add)
		b.localSet(e.retCountLocal)
		b.br(1) // leave the if and re-enter the surrounding loop
		b.end()
		return
	}
	b.ifVoid()
	e.exit(uint64(int64(instrPC)+int64(int32(ins.imm32))), idx+1)
	b.op(wasmOpReturn)
	b.end()
}

func (e *wasmBlockEmitter) loopAccessHoisted(idx uint32) bool {
	return e.loopPlan != nil && e.loopPlan.hoisted[int(idx)]
}

func (e *wasmBlockEmitter) emitLoopPrechecks() {
	for _, access := range e.loopPlan.accesses {
		e.effAddr(&JITInstr{rs: access.base, imm32: uint32(access.disp)})
		e.memChecks(access.width, func() {
			e.exit(e.loopPlan.headPC, e.loopPlan.prefix)
			e.b.localGet(wasmLocCtx)
			e.b.i32Const(int32(jitFallbackLoopPrecheck))
			e.b.i32Store(2, jitCtxOffNeedIOFallback)
			e.b.op(wasmOpReturn)
		})
	}
}

// stackHelperExit is the helper-exit variant for stack operations: LiveSP
// carries the PRE-mutation SP (the dispatcher redoes the SP arithmetic),
// HelperAddr optionally carries a call target from an i64 local instead of
// the access address in locA.
func (e *wasmBlockEmitter) stackHelperExit(code uint32, rd byte, idx uint32, instrPC uint64, targetLocal int, withVal bool) {
	b := e.b
	b.localGet(wasmLocCtx)
	if targetLocal >= 0 {
		b.localGet(uint32(targetLocal))
	} else {
		b.localGet(wasmLocA)
	}
	b.i64Store(3, jitCtxOffHelperAddr)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(IE64_SIZE_Q))
	b.i32Store(2, jitCtxOffHelperSize)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(rd))
	b.i32Store(2, jitCtxOffHelperRd)
	if withVal {
		b.localGet(wasmLocCtx)
		b.localGet(wasmLocB)
		b.i64Store(3, jitCtxOffHelperVal)
	}
	b.localGet(wasmLocCtx)
	e.loadReg(31)
	b.i64Store(3, jitCtxOffLiveSP)
	b.localGet(wasmLocCtx)
	b.i64Const(int64(instrPC))
	b.i64Store(3, jitCtxOffHelperPC)
	e.exit(instrPC, idx)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(code))
	b.i32Store(2, jitCtxOffNeedHelper)
	b.op(wasmOpReturn)
}

// emitPush: T0 = SP-8; value read AFTER the decrement (PUSH R31 pushes the
// decremented SP); store, SMC probe, then commit SP.
func (e *wasmBlockEmitter) emitPush(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.loadReg(31)
	b.i64Const(8)
	b.op(wasmOpI64Sub)
	b.localSet(wasmLocT0)
	if ins.rs == 31 {
		b.localGet(wasmLocT0)
	} else {
		e.loadReg(ins.rs)
	}
	b.localSet(wasmLocB)
	b.localGet(wasmLocT0)
	b.localSet(wasmLocA)
	e.memChecks(8, func() {
		e.stackHelperExit(HELPER_PUSH, 0, idx, instrPC, -1, true)
	})
	e.guestAddr()
	b.localGet(wasmLocB)
	b.i64Store(0, 0)
	e.smcProbe(8)
	b.localGet(wasmLocRegs)
	b.localGet(wasmLocT0)
	b.i64Store(3, 31*8)
	// SP is committed above, so the dirty exit resumes cleanly at PC+8.
	e.smcExitIfDirty(instrPC, idx)
}

// emitPop: val = [SP]; rd (if not R0) = val; SP += 8.
func (e *wasmBlockEmitter) emitPop(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.loadReg(31)
	b.localSet(wasmLocA)
	e.memChecks(8, func() {
		e.stackHelperExit(HELPER_POP, ins.rd, idx, instrPC, -1, false)
	})
	if ins.rd != 0 {
		b.localGet(wasmLocRegs)
		e.guestAddr()
		b.i64Load(0, 0)
		b.i64Store(3, uint32(ins.rd)*8)
	}
	b.localGet(wasmLocRegs)
	b.localGet(wasmLocA)
	b.i64Const(8)
	b.op(wasmOpI64Add)
	b.i64Store(3, 31*8)
}

// emitJSR handles JSR64 (PC-relative target) and JSR_IND (register target;
// rs==31 sees the decremented SP). Both are terminators: push PC+8, commit
// SP, exit at the target.
func (e *wasmBlockEmitter) emitJSR(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.loadReg(31)
	b.i64Const(8)
	b.op(wasmOpI64Sub)
	b.localSet(wasmLocT0) // new SP
	b.localGet(wasmLocT0)
	b.localSet(wasmLocA) // access address
	b.i64Const(int64(instrPC + 8))
	b.localSet(wasmLocB) // return address
	// Target into T1.
	if ins.opcode == OP_JSR64 {
		b.i64Const(int64(instrPC) + int64(int32(ins.imm32)))
	} else {
		if ins.rs == 31 {
			b.localGet(wasmLocT0)
		} else {
			e.loadReg(ins.rs)
		}
		b.i64Const(int64(int32(ins.imm32)))
		b.op(wasmOpI64Add)
	}
	b.localSet(wasmLocT1)
	helper := HELPER_JSR
	if ins.opcode == OP_JSR_IND {
		helper = HELPER_JSR_IND
	}
	e.memChecks(8, func() {
		e.stackHelperExit(helper, 0, idx, instrPC, wasmLocT1, true)
	})
	e.guestAddr()
	b.localGet(wasmLocB)
	b.i64Store(0, 0)
	e.smcProbe(8)
	b.localGet(wasmLocRegs)
	b.localGet(wasmLocT0)
	b.i64Store(3, 31*8)
	e.exitDyn(wasmLocT1, idx+1)
	b.op(wasmOpReturn)
}

// emitRTS: pop the return address into RetPC; terminator.
func (e *wasmBlockEmitter) emitRTS(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.loadReg(31)
	b.localSet(wasmLocA)
	e.memChecks(8, func() {
		e.stackHelperExit(HELPER_RTS, 0, idx, instrPC, -1, false)
	})
	e.guestAddr()
	b.i64Load(0, 0)
	b.localSet(wasmLocT0)
	b.localGet(wasmLocRegs)
	b.localGet(wasmLocA)
	b.i64Const(8)
	b.op(wasmOpI64Add)
	b.i64Store(3, 31*8)
	e.exitDyn(wasmLocT0, idx+1)
	b.op(wasmOpReturn)
}

// effAddr computes the memory operand address rs + sext(imm32) into locA.
func (e *wasmBlockEmitter) effAddr(ins *JITInstr) {
	e.loadReg(ins.rs)
	e.b.i64Const(int64(int32(ins.imm32)))
	e.b.op(wasmOpI64Add)
	e.b.localSet(wasmLocA)
}

// memChecks emits the fast-path decision chain for the address in locA with
// an access of n bytes. Layout:
//
//	block $FAST
//	  block $HELP
//	    addr < IOStart            -> br $FAST
//	    addr >= MemSize-(n-1)     -> br $HELP
//	    ioPageBitmap[addr>>8]==0  -> br $FAST
//	  end                          ; fall-through = helper
//	  <helper exit, ends in return>
//	end                            ; fast path continues here
func (e *wasmBlockEmitter) memChecks(n uint32, emitHelper func()) {
	b := e.b
	b.block() // $FAST
	b.block() // $HELP
	b.localGet(wasmLocA)
	b.i64Const(int64(IO_REGION_START))
	b.op(wasmOpI64LtU)
	b.brIf(1) // -> $FAST
	b.localGet(wasmLocA)
	b.localGet(wasmLocMemSize)
	b.i64Const(int64(n - 1))
	b.op(wasmOpI64Sub)
	b.op(wasmOpI64GeU)
	b.brIf(0) // -> $HELP
	b.localGet(wasmLocBmp)
	b.localGet(wasmLocA)
	b.i64Const(8)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI32WrapI64)
	b.op(wasmOpI32Add)
	b.memOp(wasmOpI32Load8U, 0, 0)
	b.op(wasmOpI32Eqz)
	b.brIf(1) // -> $FAST (zero means RAM)
	b.end()   // $HELP
	emitHelper()
	b.end() // $FAST
}

// helperExit writes the helper-exit protocol fields and returns from the
// block. RetCount = idx: the bailing instruction did not retire.
func (e *wasmBlockEmitter) helperExit(code uint32, size, rd byte, idx uint32, instrPC uint64, withVal bool) {
	b := e.b
	b.localGet(wasmLocCtx)
	b.localGet(wasmLocA)
	b.i64Store(3, jitCtxOffHelperAddr)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(size))
	b.i32Store(2, jitCtxOffHelperSize)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(rd))
	b.i32Store(2, jitCtxOffHelperRd)
	if withVal {
		b.localGet(wasmLocCtx)
		b.localGet(wasmLocB)
		b.i64Store(3, jitCtxOffHelperVal)
	}
	b.localGet(wasmLocCtx)
	e.loadReg(31)
	b.i64Store(3, jitCtxOffLiveSP)
	b.localGet(wasmLocCtx)
	b.i64Const(int64(instrPC))
	b.i64Store(3, jitCtxOffHelperPC)
	e.exit(instrPC, idx)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(code))
	b.i32Store(2, jitCtxOffNeedHelper)
	b.op(wasmOpReturn)
}

// guestAddr pushes MemPtr + i32(locA), the linear-memory address of the
// guest RAM byte the fast path accesses.
func (e *wasmBlockEmitter) guestAddr() {
	b := e.b
	b.localGet(wasmLocMem)
	b.localGet(wasmLocA)
	b.op(wasmOpI32WrapI64)
	b.op(wasmOpI32Add)
}

func (e *wasmBlockEmitter) emitLoad(ins *JITInstr, idx uint32, instrPC uint64) {
	if ins.rd == 0 {
		// rd==0 skips the load entirely, MMIO side effects included.
		return
	}
	b := e.b
	e.effAddr(ins)
	if _, proven := ie64ConstLowRAMAccess(ins.rs, ins.imm32, ins.size); !proven && !e.loopAccessHoisted(idx) {
		e.memChecks(ie64AccessBytes(ins.size), func() {
			e.helperExit(HELPER_LOAD, ins.size, ins.rd, idx, instrPC, false)
		})
	}
	b.localGet(wasmLocRegs)
	e.guestAddr()
	switch ins.size {
	case IE64_SIZE_B:
		b.memOp(wasmOpI64Load8U, 0, 0)
	case IE64_SIZE_W:
		b.memOp(wasmOpI64Load16U, 0, 0)
	case IE64_SIZE_L:
		b.memOp(wasmOpI64Load32U, 0, 0)
	default:
		b.i64Load(0, 0)
	}
	b.i64Store(3, uint32(ins.rd)*8)
}

func (e *wasmBlockEmitter) emitStore(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	// Value first (size-masked, from rd), so the helper exit can publish it.
	e.loadReg(ins.rd)
	e.mask(ins.size)
	b.localSet(wasmLocB)
	e.effAddr(ins)
	n := ie64AccessBytes(ins.size)
	if _, proven := ie64ConstLowRAMAccess(ins.rs, ins.imm32, ins.size); !proven && !e.loopAccessHoisted(idx) {
		e.memChecks(n, func() {
			e.helperExit(HELPER_STORE, ins.size, ins.rd, idx, instrPC, true)
		})
	}
	e.guestAddr()
	b.localGet(wasmLocB)
	switch ins.size {
	case IE64_SIZE_B:
		b.memOp(wasmOpI64Store8, 0, 0)
	case IE64_SIZE_W:
		b.memOp(wasmOpI64Store16, 0, 0)
	case IE64_SIZE_L:
		b.memOp(wasmOpI64Store32, 0, 0)
	default:
		b.i64Store(0, 0)
	}
	e.smcProbe(n)
	e.smcExitIfDirty(instrPC, idx)
}

// smcExitIfDirty ends the block straight after a store whose probe reported
// a dirty code page. The store retired, so RetPC is the next instruction and
// RetCount includes it. Exiting immediately keeps InvalAddr/InvalSize exact
// (a second dirty store in the same block would otherwise degrade the report
// to a full flush, and full flushes on the page-granular false shares in the
// EhBASIC image caused a permanent recompile storm) and stops the block from
// running on past a write that may have hit its own instructions.
func (e *wasmBlockEmitter) smcExitIfDirty(instrPC uint64, idx uint32) {
	b := e.b
	b.localGet(wasmLocCtx)
	b.i32Load(2, jitCtxOffNeedInval)
	b.ifVoid()
	e.exit(instrPC+8, idx+1)
	b.op(wasmOpReturn)
	b.end()
}

// smcProbe reports a committed store into a compiled code page, matching the
// native emitters: the first dirty store publishes the exact range (callers
// follow the probe with smcExitIfDirty, so in practice each block reports at
// most one); a probe that finds NeedInval already set degrades InvalSize to 0
// (full invalidation) as a defensive fallback. The dispatcher acts on
// NeedInval at the block boundary, and the in-wasm chain driver checks it
// before dispatching another block.
func (e *wasmBlockEmitter) smcProbe(n uint32) {
	b := e.b
	// T0 = first page, T1 = last page of the access.
	b.localGet(wasmLocA)
	b.i64Const(8)
	b.op(wasmOpI64ShrU)
	b.localSet(wasmLocP0)
	b.localGet(wasmLocA)
	b.i64Const(int64(n - 1))
	b.op(wasmOpI64Add)
	b.i64Const(8)
	b.op(wasmOpI64ShrU)
	b.localSet(wasmLocP1)

	// spanLoad pushes spans[page*2+off]: the page's min (off 0) or max
	// (off 1) compiled-byte offset.
	spanLoad := func(loc uint32, off uint32) {
		b.localGet(wasmLocSpans)
		b.localGet(loc)
		b.op(wasmOpI32WrapI64)
		b.i32Const(1)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Add)
		b.memOp(wasmOpI32Load8U, 0, off)
	}
	// aStart pushes the access's first byte offset within its page.
	aStart := func() {
		b.localGet(wasmLocA)
		b.op(wasmOpI32WrapI64)
		b.i32Const(255)
		b.op(wasmOpI32And)
	}

	// pageDirty emits the probes for the page in the given local, branching
	// to $DIRTY (relative depth from the emission point) on a hit. first
	// distinguishes the access's first page from the straddle page: the
	// access's byte extent within the page differs between them.
	pageDirty := func(loc uint32, dirtyDepth uint32, first bool) {
		// Low bitmap: page < CodePageBitmapLen && bitmap[page] != 0 && the
		// accessed bytes overlap the page's compiled-code span [min, max].
		// The span term lets a store into data that merely shares the page
		// with compiled code continue with no exit at all; only writes into
		// the code extent itself go dirty. The bounds check gates the loads
		// (an if frame, so the branch depth inside is one deeper).
		b.localGet(loc)
		b.localGet(wasmLocCpbLen)
		b.op(wasmOpI64LtU)
		b.ifVoid()
		b.localGet(wasmLocCpb)
		b.localGet(loc)
		b.op(wasmOpI32WrapI64)
		b.op(wasmOpI32Add)
		b.memOp(wasmOpI32Load8U, 0, 0)
		// accessEnd >= min, accessEnd clamped to the page.
		if first {
			aStart()
			if n > 1 {
				b.i32Const(int32(n - 1))
				b.op(wasmOpI32Add)
				b.localSet(wasmLocS0)
				b.i32Const(255)
				b.localGet(wasmLocS0)
				b.localGet(wasmLocS0)
				b.i32Const(255)
				b.op(wasmOpI32GtU)
				b.op(wasmOpSelect)
			}
		} else {
			// Straddle page: the access covers its first bytes, so the last
			// accessed offset is (A+n-1) mod 256.
			b.localGet(wasmLocA)
			b.i64Const(int64(n - 1))
			b.op(wasmOpI64Add)
			b.op(wasmOpI32WrapI64)
			b.i32Const(255)
			b.op(wasmOpI32And)
		}
		spanLoad(loc, 0)
		b.op(wasmOpI32GeU)
		b.op(wasmOpI32And)
		if first {
			// accessStart <= max. On the straddle page the access starts at
			// offset 0, which never exceeds max, so the term is elided there.
			aStart()
			spanLoad(loc, 1)
			b.op(wasmOpI32LeU)
			b.op(wasmOpI32And)
		}
		b.brIf(dirtyDepth + 1)
		b.end()
		// High range: CodeHighEndPage != 0 && HS <= page <= HE. No spans are
		// maintained for the high range; stay conservative.
		b.localGet(wasmLocHighE)
		b.i64Const(0)
		b.op(wasmOpI64Ne)
		b.localGet(loc)
		b.localGet(wasmLocHighS)
		b.op(wasmOpI64GeU)
		b.op(wasmOpI32And)
		b.localGet(loc)
		b.localGet(wasmLocHighE)
		b.op(wasmOpI64LeU)
		b.op(wasmOpI32And)
		b.brIf(dirtyDepth)
	}

	b.block() // $CLEAN
	b.block() // $DIRTY
	pageDirty(wasmLocP0, 0, true)
	if n > 1 {
		// Second page probe, gated on a genuine straddle: the straddle
		// variant assumes the access starts at the page's offset 0 and omits
		// the start-vs-max term, so running it against the FIRST page would
		// false-fire on any access ending at or after the page's span. (The
		// pre-span probe emitted it unconditionally as a harmless duplicate;
		// with spans it is no longer harmless.)
		b.localGet(wasmLocP0)
		b.localGet(wasmLocP1)
		b.op(wasmOpI64Ne)
		b.ifVoid()
		pageDirty(wasmLocP1, 1, false)
		b.end()
	}
	b.br(1) // no hit -> $CLEAN
	b.end() // $DIRTY
	// Dirty: NeedInval already set -> degrade to full invalidation.
	b.localGet(wasmLocCtx)
	b.i32Load(2, jitCtxOffNeedInval)
	b.ifVoid()
	b.localGet(wasmLocCtx)
	b.i32Const(0)
	b.i32Store(2, jitCtxOffInvalSize)
	b.elseBranch()
	b.localGet(wasmLocCtx)
	b.i32Const(1)
	b.i32Store(2, jitCtxOffNeedInval)
	b.localGet(wasmLocCtx)
	b.localGet(wasmLocA)
	b.i64Store(3, jitCtxOffInvalAddr)
	b.localGet(wasmLocCtx)
	b.i32Const(int32(n))
	b.i32Store(2, jitCtxOffInvalSize)
	b.end()
	b.end() // $CLEAN
}

// value pushes the instruction's result onto the stack.
func (e *wasmBlockEmitter) value(ins *JITInstr) {
	b := e.b
	size := ins.size
	switch ins.opcode {
	case OP_MOVE:
		if ins.xbit == 1 {
			b.i64Const(int64(uint64(ins.imm32)))
		} else {
			e.loadReg(ins.rs)
		}
		e.mask(size)
	case OP_MOVT:
		e.loadReg(ins.rd)
		b.i64Const(0x00000000FFFFFFFF)
		b.op(wasmOpI64And)
		b.i64Const(int64(uint64(ins.imm32) << 32))
		b.op(wasmOpI64Or)
	case OP_MOVEQ:
		b.i64Const(int64(int32(ins.imm32)))
	case OP_LEA:
		e.loadReg(ins.rs)
		b.i64Const(int64(int32(ins.imm32)))
		b.op(wasmOpI64Add)
	case OP_ADD, OP_SUB, OP_MULU, OP_MULS, OP_AND64, OP_OR64, OP_EOR:
		e.loadReg(ins.rs)
		e.operand3(ins)
		switch ins.opcode {
		case OP_ADD:
			b.op(wasmOpI64Add)
		case OP_SUB:
			b.op(wasmOpI64Sub)
		case OP_MULU, OP_MULS: // identical low 64 bits
			b.op(wasmOpI64Mul)
		case OP_AND64:
			b.op(wasmOpI64And)
		case OP_OR64:
			b.op(wasmOpI64Or)
		case OP_EOR:
			b.op(wasmOpI64Xor)
		}
		e.mask(size)
	case OP_DIVU, OP_MOD64:
		// Divide by zero yields 0 in IE64; wasm div/rem trap, so guard.
		e.operand3(ins)
		b.localSet(wasmLocA)
		b.localGet(wasmLocA)
		b.op(wasmOpI64Eqz)
		b.ifTyped(wasmTypeI64)
		b.i64Const(0)
		b.elseBranch()
		e.loadReg(ins.rs)
		b.localGet(wasmLocA)
		if ins.opcode == OP_DIVU {
			b.op(wasmOpI64DivU)
		} else {
			b.op(wasmOpI64RemU)
		}
		e.mask(size)
		b.end()
	case OP_DIVS:
		// Guard zero (IE64: result 0) and -1 (wasm i64.div_s traps on
		// MinInt64/-1; Go wraps, so emit negation instead).
		e.operand3(ins)
		b.localSet(wasmLocA)
		b.localGet(wasmLocA)
		b.op(wasmOpI64Eqz)
		b.ifTyped(wasmTypeI64)
		b.i64Const(0)
		b.elseBranch()
		b.localGet(wasmLocA)
		b.i64Const(-1)
		b.op(wasmOpI64Eq)
		b.ifTyped(wasmTypeI64)
		b.i64Const(0)
		e.loadReg(ins.rs)
		b.op(wasmOpI64Sub)
		e.mask(size)
		b.elseBranch()
		e.loadReg(ins.rs)
		b.localGet(wasmLocA)
		b.op(wasmOpI64DivS)
		e.mask(size)
		b.end()
		b.end()
	case OP_MODS:
		// Operands sign-extend per size first. rem_s never traps on the
		// MinInt64/-1 pair (result 0, matching Go), only zero needs a guard.
		e.operand3(ins)
		e.sext(size)
		b.localSet(wasmLocA)
		b.localGet(wasmLocA)
		b.op(wasmOpI64Eqz)
		b.ifTyped(wasmTypeI64)
		b.i64Const(0)
		b.elseBranch()
		e.loadReg(ins.rs)
		e.sext(size)
		b.localGet(wasmLocA)
		b.op(wasmOpI64RemS)
		e.mask(size)
		b.end()
	case OP_NEG:
		b.i64Const(0)
		e.loadReg(ins.rs)
		b.op(wasmOpI64Sub)
		e.mask(size)
	case OP_MULHU, OP_MULHS:
		e.loadReg(ins.rs)
		b.localSet(wasmLocA)
		e.operand3(ins)
		b.localSet(wasmLocB)
		e.mulHighU()
		if ins.opcode == OP_MULHS {
			// hi_s = hi_u - (a<0 ? b : 0) - (b<0 ? a : 0)
			b.localGet(wasmLocA)
			b.i64Const(63)
			b.op(wasmOpI64ShrS)
			b.localGet(wasmLocB)
			b.op(wasmOpI64And)
			b.op(wasmOpI64Sub)
			b.localGet(wasmLocB)
			b.i64Const(63)
			b.op(wasmOpI64ShrS)
			b.localGet(wasmLocA)
			b.op(wasmOpI64And)
			b.op(wasmOpI64Sub)
		}
		// No size mask: MULHU/MULHS ignore the size field.
	case OP_NOT64:
		e.loadReg(ins.rs)
		b.i64Const(-1)
		b.op(wasmOpI64Xor)
		e.mask(size)
	case OP_LSL, OP_LSR:
		e.loadReg(ins.rs)
		e.operand3(ins)
		b.i64Const(63)
		b.op(wasmOpI64And)
		if ins.opcode == OP_LSL {
			b.op(wasmOpI64Shl)
		} else {
			b.op(wasmOpI64ShrU)
		}
		e.mask(size)
	case OP_ASR:
		e.loadReg(ins.rs)
		e.sext(size)
		e.operand3(ins)
		b.i64Const(63)
		b.op(wasmOpI64And)
		b.op(wasmOpI64ShrS)
		e.mask(size)
	case OP_CLZ, OP_CTZ, OP_POPCNT:
		// 32-bit operations on the low half, regardless of size.
		e.loadReg(ins.rs)
		b.op(wasmOpI32WrapI64)
		switch ins.opcode {
		case OP_CLZ:
			b.op(wasmOpI32Clz)
		case OP_CTZ:
			b.op(wasmOpI32Ctz)
		case OP_POPCNT:
			b.op(wasmOpI32Popcnt)
		}
		b.op(wasmOpI64ExtendI32U)
	case OP_BSWAP:
		e.loadReg(ins.rs)
		b.op(wasmOpI32WrapI64)
		b.localSet(wasmLocV32)
		b.localGet(wasmLocV32)
		b.i32Const(24)
		b.op(wasmOpI32ShrU)
		b.localGet(wasmLocV32)
		b.i32Const(8)
		b.op(wasmOpI32ShrU)
		b.i32Const(0xFF00)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.localGet(wasmLocV32)
		b.i32Const(8)
		b.op(wasmOpI32Shl)
		b.i32Const(0xFF0000)
		b.op(wasmOpI32And)
		b.op(wasmOpI32Or)
		b.localGet(wasmLocV32)
		b.i32Const(24)
		b.op(wasmOpI32Shl)
		b.op(wasmOpI32Or)
		b.op(wasmOpI64ExtendI32U)
	case OP_SEXT:
		e.loadReg(ins.rs)
		e.sext(size)
	case OP_ROL, OP_ROR:
		if size == IE64_SIZE_Q {
			e.loadReg(ins.rs)
			e.operand3(ins)
			if ins.opcode == OP_ROL {
				b.op(wasmOpI64Rotl)
			} else {
				b.op(wasmOpI64Rotr)
			}
			return
		}
		w := int64(8) << size // 8, 16, 32
		e.loadReg(ins.rs)
		e.mask(size)
		b.localSet(wasmLocA) // V, pre-masked
		e.operand3(ins)
		b.i64Const(w - 1)
		b.op(wasmOpI64And)
		b.localSet(wasmLocB) // N
		b.localGet(wasmLocA)
		b.localGet(wasmLocB)
		if ins.opcode == OP_ROL {
			b.op(wasmOpI64Shl)
		} else {
			b.op(wasmOpI64ShrU)
		}
		b.localGet(wasmLocA)
		b.i64Const(w)
		b.localGet(wasmLocB)
		b.op(wasmOpI64Sub)
		if ins.opcode == OP_ROL {
			b.op(wasmOpI64ShrU)
		} else {
			b.op(wasmOpI64Shl)
		}
		b.op(wasmOpI64Or)
		e.mask(size)
	default:
		// Unreachable: wasmCompileBlock rejected unsupported opcodes.
		panic(fmt.Sprintf("wasm JIT: value() on unhandled opcode %#02x", ins.opcode))
	}
}

// mulHighU pushes the high 64 bits of the unsigned product of locals A and
// B, via four 32x32 partial products.
func (e *wasmBlockEmitter) mulHighU() {
	b := e.b
	const lo32 = 0x00000000FFFFFFFF
	// T0 = (A>>32)*(B&lo) + ((A&lo)*(B&lo))>>32        ("u")
	b.localGet(wasmLocA)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.localGet(wasmLocB)
	b.i64Const(lo32)
	b.op(wasmOpI64And)
	b.op(wasmOpI64Mul)
	b.localGet(wasmLocA)
	b.i64Const(lo32)
	b.op(wasmOpI64And)
	b.localGet(wasmLocB)
	b.i64Const(lo32)
	b.op(wasmOpI64And)
	b.op(wasmOpI64Mul)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI64Add)
	b.localSet(wasmLocT0)
	// T1 = (A&lo)*(B>>32) + (T0&lo)                    ("v")
	b.localGet(wasmLocA)
	b.i64Const(lo32)
	b.op(wasmOpI64And)
	b.localGet(wasmLocB)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI64Mul)
	b.localGet(wasmLocT0)
	b.i64Const(lo32)
	b.op(wasmOpI64And)
	b.op(wasmOpI64Add)
	b.localSet(wasmLocT1)
	// hi = (A>>32)*(B>>32) + (T0>>32) + (T1>>32)
	b.localGet(wasmLocA)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.localGet(wasmLocB)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI64Mul)
	b.localGet(wasmLocT0)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI64Add)
	b.localGet(wasmLocT1)
	b.i64Const(32)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI64Add)
}

// ---------------------------------------------------------------------------
// FP64
// ---------------------------------------------------------------------------
//
// IE64 FP64 values live in even/odd FPRegs pairs; little-endian, a pair is
// exactly one 8-byte load/store at FPUPtr + (idx&0x0E)*4 (4-byte aligned).
// Every arithmetic result updates the FPSR condition codes (bits 27:24) via
// the shared per-module helper emitted by wasmEmitCCUpdate64, and sticky
// exception flags (IO/DZ/OE/UE) replicate the interpreter's predicates
// exactly (fpu_ie64.go).

const (
	wasmFPAbsMask  = 0x7FFFFFFFFFFFFFFF
	wasmFPExpMask  = 0x7FF0000000000000
	wasmFPFracMask = 0x000FFFFFFFFFFFFF
	wasmFPSROff    = 68 // IE64FPU.FPSR byte offset
	wasmFPCROff    = 64 // IE64FPU.FPCR byte offset
)

// wasmEmitCCUpdate64 builds the body of the internal condition-code helper:
// params (fpu i32, bits i64), one extra i32 local for the code. Mirrors
// IE64FPU.setConditionCodesBits64.
func wasmEmitCCUpdate64() []byte {
	b := &wasmBody{}
	const locFpu, locBits, locCC = 0, 1, 2
	// exp == all-ones?
	b.localGet(locBits)
	b.i64Const(wasmFPExpMask)
	b.op(wasmOpI64And)
	b.i64Const(wasmFPExpMask)
	b.op(wasmOpI64Eq)
	b.ifVoid()
	{
		b.localGet(locBits)
		b.i64Const(wasmFPFracMask)
		b.op(wasmOpI64And)
		b.op(wasmOpI64Eqz)
		b.ifVoid() // infinity
		b.localGet(locBits)
		b.i64Const(0)
		b.op(wasmOpI64LtS)
		b.ifTyped(wasmTypeI32)
		b.i32Const(int32(IE64_FPU_CC_I | IE64_FPU_CC_N))
		b.elseBranch()
		b.i32Const(int32(IE64_FPU_CC_I))
		b.end()
		b.localSet(locCC)
		b.elseBranch() // NaN
		b.i32Const(int32(IE64_FPU_CC_NAN))
		b.localSet(locCC)
		b.end()
	}
	b.elseBranch()
	{
		b.localGet(locBits)
		b.i64Const(wasmFPAbsMask)
		b.op(wasmOpI64And)
		b.op(wasmOpI64Eqz)
		b.ifVoid()
		b.i32Const(int32(IE64_FPU_CC_Z))
		b.localSet(locCC)
		b.elseBranch()
		b.localGet(locBits)
		b.i64Const(0)
		b.op(wasmOpI64LtS)
		b.ifVoid()
		b.i32Const(int32(IE64_FPU_CC_N))
		b.localSet(locCC)
		b.end()
		b.end()
	}
	b.end()
	// FPSR = (FPSR & ^0x0F000000) | cc
	b.localGet(locFpu)
	b.localGet(locFpu)
	b.i32Load(2, wasmFPSROff)
	b.i32Const(int32(-0x0F000001))
	b.op(wasmOpI32And)
	b.localGet(locCC)
	b.op(wasmOpI32Or)
	b.i32Store(2, wasmFPSROff)
	b.end()
	return b.code
}

func fpPairOff(idx byte) uint32 { return uint32(idx&0x0E) * 4 }

// loadPairBits pushes the raw bits of an FP pair as i64.
func (e *wasmBlockEmitter) loadPairBits(idx byte) {
	if local := e.fpPlan.local(idx); local != 0 {
		e.b.localGet(local)
		e.b.op(wasmOpI64ReinterpretF64)
		return
	}
	e.b.localGet(wasmLocFpu)
	e.b.i64Load(2, fpPairOff(idx))
}

// storePairBitsFrom stores an i64 local into an FP pair.
func (e *wasmBlockEmitter) storePairBitsFrom(idx byte, local uint32) {
	if resident := e.fpPlan.local(idx); resident != 0 {
		e.b.localGet(local)
		e.b.op(wasmOpF64ReinterpretI64)
		e.b.localSet(resident)
		return
	}
	e.b.localGet(wasmLocFpu)
	e.b.localGet(local)
	e.b.i64Store(2, fpPairOff(idx))
}

// callCC invokes the condition-code helper with the bits in the given local.
func (e *wasmBlockEmitter) callCC(local uint32) {
	if !e.emitFPCC {
		return
	}
	e.b.localGet(wasmLocFpu)
	e.b.localGet(local)
	e.b.call(e.ccFunc)
}

// Predicates on the i64 bits in a local; each leaves an i32 boolean.
func (e *wasmBlockEmitter) fpIsInf(local uint32) {
	e.b.localGet(local)
	e.b.i64Const(wasmFPAbsMask)
	e.b.op(wasmOpI64And)
	e.b.i64Const(wasmFPExpMask)
	e.b.op(wasmOpI64Eq)
}

func (e *wasmBlockEmitter) fpIsNaN(local uint32) {
	e.b.localGet(local)
	e.b.i64Const(wasmFPAbsMask)
	e.b.op(wasmOpI64And)
	e.b.i64Const(wasmFPExpMask)
	e.b.op(wasmOpI64GtU)
}

func (e *wasmBlockEmitter) fpIsZero(local uint32) {
	e.b.localGet(local)
	e.b.i64Const(wasmFPAbsMask)
	e.b.op(wasmOpI64And)
	e.b.op(wasmOpI64Eqz)
}

func (e *wasmBlockEmitter) fpIsNeg(local uint32) {
	e.b.localGet(local)
	e.b.i64Const(0)
	e.b.op(wasmOpI64LtS)
}

func (e *wasmBlockEmitter) not() { e.b.op(wasmOpI32Eqz) }

// fpsrOrIf ORs a sticky exception flag into FPSR when the i32 predicate on
// the stack is true.
func (e *wasmBlockEmitter) fpsrOrIf(flag uint32) {
	b := e.b
	b.ifVoid()
	b.localGet(wasmLocFpu)
	b.localGet(wasmLocFpu)
	b.i32Load(2, wasmFPSROff)
	b.i32Const(int32(flag))
	b.op(wasmOpI32Or)
	b.i32Store(2, wasmFPSROff)
	b.end()
}

// pairF64 pushes an FP pair as f64.
func (e *wasmBlockEmitter) pairF64(local uint32) {
	e.b.localGet(local)
	e.b.op(wasmOpF64ReinterpretI64)
}

// emitFP64 handles the register-to-register FP64 operations. Uses locA for
// the fs bits, locB for ft, locT0 for the result bits, locT1 for integer
// results.
func (e *wasmBlockEmitter) emitFP64(ins *JITInstr) {
	b := e.b
	switch ins.opcode {
	case OP_DMOV:
		// Pure pair copy; condition codes untouched.
		e.loadPairBits(ins.rs)
		b.localSet(wasmLocT0)
		e.storePairBitsFrom(ins.rd, wasmLocT0)
	case OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV:
		e.loadPairBits(ins.rs)
		b.localSet(wasmLocA)
		e.loadPairBits(ins.rt)
		b.localSet(wasmLocB)
		if ins.opcode == OP_DDIV {
			// DZ: t zero, s neither zero nor NaN.
			e.fpIsZero(wasmLocB)
			e.fpIsZero(wasmLocA)
			e.not()
			b.op(wasmOpI32And)
			e.fpIsNaN(wasmLocA)
			e.not()
			b.op(wasmOpI32And)
			e.fpsrOrIf(IE64_FPU_EX_DZ)
		}
		e.pairF64(wasmLocA)
		e.pairF64(wasmLocB)
		switch ins.opcode {
		case OP_DADD:
			b.op(wasmOpF64Add)
		case OP_DSUB:
			b.op(wasmOpF64Sub)
		case OP_DMUL:
			b.op(wasmOpF64Mul)
		case OP_DDIV:
			b.op(wasmOpF64Div)
		}
		b.op(wasmOpI64ReinterpretF64)
		b.localSet(wasmLocT0)
		// OE: result Inf from finite inputs (DDIV: excludes t zero instead
		// of t Inf).
		e.fpIsInf(wasmLocT0)
		e.fpIsInf(wasmLocA)
		e.not()
		b.op(wasmOpI32And)
		if ins.opcode == OP_DDIV {
			e.fpIsZero(wasmLocB)
		} else {
			e.fpIsInf(wasmLocB)
		}
		e.not()
		b.op(wasmOpI32And)
		e.fpsrOrIf(IE64_FPU_EX_OE)
		// IO: NaN produced from non-NaN inputs.
		e.fpIsNaN(wasmLocT0)
		e.fpIsNaN(wasmLocA)
		e.not()
		b.op(wasmOpI32And)
		e.fpIsNaN(wasmLocB)
		e.not()
		b.op(wasmOpI32And)
		e.fpsrOrIf(IE64_FPU_EX_IO)
		// UE: zero produced from non-zero inputs (DMUL; DDIV also excludes
		// t Inf).
		if ins.opcode == OP_DMUL || ins.opcode == OP_DDIV {
			e.fpIsZero(wasmLocT0)
			e.fpIsZero(wasmLocA)
			e.not()
			b.op(wasmOpI32And)
			e.fpIsZero(wasmLocB)
			e.not()
			b.op(wasmOpI32And)
			if ins.opcode == OP_DDIV {
				e.fpIsInf(wasmLocB)
				e.not()
				b.op(wasmOpI32And)
			}
			e.fpsrOrIf(IE64_FPU_EX_UE)
		}
		e.storePairBitsFrom(ins.rd, wasmLocT0)
		e.callCC(wasmLocT0)
	case OP_DABS, OP_DNEG:
		e.loadPairBits(ins.rs)
		b.i64Const(-0x8000000000000000)
		if ins.opcode == OP_DABS {
			b.i64Const(-1)
			b.op(wasmOpI64Xor) // ^signbit mask
			b.op(wasmOpI64And)
		} else {
			b.op(wasmOpI64Xor)
		}
		b.localSet(wasmLocT0)
		e.storePairBitsFrom(ins.rd, wasmLocT0)
		e.callCC(wasmLocT0)
	case OP_DSQRT:
		e.loadPairBits(ins.rs)
		b.localSet(wasmLocA)
		// IO: negative, not zero, not NaN.
		e.fpIsNeg(wasmLocA)
		e.fpIsZero(wasmLocA)
		e.not()
		b.op(wasmOpI32And)
		e.fpIsNaN(wasmLocA)
		e.not()
		b.op(wasmOpI32And)
		e.fpsrOrIf(IE64_FPU_EX_IO)
		e.pairF64(wasmLocA)
		b.op(wasmOpF64Sqrt)
		b.op(wasmOpI64ReinterpretF64)
		b.localSet(wasmLocT0)
		e.storePairBitsFrom(ins.rd, wasmLocT0)
		e.callCC(wasmLocT0)
	case OP_DINT:
		e.loadPairBits(ins.rs)
		b.localSet(wasmLocA)
		// Rounding mode from FPCR bits 1:0.
		b.localGet(wasmLocFpu)
		b.i32Load(2, wasmFPCROff)
		b.i32Const(3)
		b.op(wasmOpI32And)
		b.localSet(wasmLocV32)
		b.localGet(wasmLocV32)
		b.i32Const(int32(IE64_FPU_RND_ZERO))
		b.op(wasmOpI32Eq)
		b.ifTyped(wasmTypeF64)
		e.pairF64(wasmLocA)
		b.op(wasmOpF64Trnc)
		b.elseBranch()
		b.localGet(wasmLocV32)
		b.i32Const(int32(IE64_FPU_RND_FLOOR))
		b.op(wasmOpI32Eq)
		b.ifTyped(wasmTypeF64)
		e.pairF64(wasmLocA)
		b.op(wasmOpF64Flr)
		b.elseBranch()
		b.localGet(wasmLocV32)
		b.i32Const(int32(IE64_FPU_RND_CEIL))
		b.op(wasmOpI32Eq)
		b.ifTyped(wasmTypeF64)
		e.pairF64(wasmLocA)
		b.op(wasmOpF64Ceil)
		b.elseBranch()
		e.pairF64(wasmLocA)
		b.op(wasmOpF64Nrst)
		b.end()
		b.end()
		b.end()
		b.op(wasmOpI64ReinterpretF64)
		b.localSet(wasmLocT0)
		e.storePairBitsFrom(ins.rd, wasmLocT0)
		e.callCC(wasmLocT0)
	case OP_DCMP:
		if ins.rd == 0 {
			// Interpreter skips the whole operation, FPSR included.
			return
		}
		e.loadPairBits(ins.rs)
		b.localSet(wasmLocA)
		e.loadPairBits(ins.rt)
		b.localSet(wasmLocB)
		// Clear the CC nibble first.
		b.localGet(wasmLocFpu)
		b.localGet(wasmLocFpu)
		b.i32Load(2, wasmFPSROff)
		b.i32Const(int32(-0x0F000001))
		b.op(wasmOpI32And)
		b.i32Store(2, wasmFPSROff)
		e.fpIsNaN(wasmLocA)
		e.fpIsNaN(wasmLocB)
		b.op(wasmOpI32Or)
		b.ifVoid()
		{
			b.i32Const(1)
			e.fpsrOrIf(IE64_FPU_CC_NAN | IE64_FPU_EX_IO)
			b.i64Const(0)
			b.localSet(wasmLocT1)
		}
		b.elseBranch()
		{
			e.pairF64(wasmLocA)
			e.pairF64(wasmLocB)
			b.op(wasmOpF64Lt)
			b.ifVoid()
			{
				b.i32Const(1)
				e.fpsrOrIf(IE64_FPU_CC_N)
				b.i64Const(-1)
				b.localSet(wasmLocT1)
			}
			b.elseBranch()
			{
				e.pairF64(wasmLocA)
				e.pairF64(wasmLocB)
				b.op(wasmOpF64Gt)
				b.ifVoid()
				{
					// s > t: CC_I only for positive infinity s.
					e.fpIsInf(wasmLocA)
					e.fpIsNeg(wasmLocA)
					e.not()
					b.op(wasmOpI32And)
					e.fpsrOrIf(IE64_FPU_CC_I)
					b.i64Const(1)
					b.localSet(wasmLocT1)
				}
				b.elseBranch()
				{
					b.i32Const(1)
					e.fpsrOrIf(IE64_FPU_CC_Z)
					e.fpIsInf(wasmLocA)
					e.fpsrOrIf(IE64_FPU_CC_I)
					e.fpIsInf(wasmLocA)
					e.fpIsNeg(wasmLocA)
					b.op(wasmOpI32And)
					e.fpsrOrIf(IE64_FPU_CC_N)
					b.i64Const(0)
					b.localSet(wasmLocT1)
				}
				b.end()
			}
			b.end()
		}
		b.end()
		b.localGet(wasmLocRegs)
		b.localGet(wasmLocT1)
		b.i64Store(3, uint32(ins.rd)*8)
	case OP_DCVTIF:
		e.loadReg(ins.rs)
		b.op(wasmOpF64ConvertI64S)
		b.op(wasmOpI64ReinterpretF64)
		b.localSet(wasmLocT0)
		e.storePairBitsFrom(ins.rd, wasmLocT0)
		e.callCC(wasmLocT0)
	case OP_DCVTFI:
		if ins.rd == 0 {
			return
		}
		e.loadPairBits(ins.rs)
		b.localSet(wasmLocA)
		e.fpIsNaN(wasmLocA)
		b.ifVoid()
		{
			b.i32Const(1)
			e.fpsrOrIf(IE64_FPU_EX_IO)
			b.i64Const(0)
			b.localSet(wasmLocT1)
		}
		b.elseBranch()
		{
			e.pairF64(wasmLocA)
			b.f64Const(9223372036854775808.0) // 2^63 exactly
			b.op(wasmOpF64Gt)
			b.ifVoid()
			{
				b.i32Const(1)
				e.fpsrOrIf(IE64_FPU_EX_IO)
				b.i64Const(0x7FFFFFFFFFFFFFFF)
				b.localSet(wasmLocT1)
			}
			b.elseBranch()
			{
				e.pairF64(wasmLocA)
				b.f64Const(9223372036854775808.0)
				b.op(wasmOpF64Eq)
				b.ifVoid()
				{
					// Interpreter quirk: exactly 2^63 converts through Go's
					// cvttsd2si to MinInt64 with no exception flag.
					b.i64Const(-0x8000000000000000)
					b.localSet(wasmLocT1)
				}
				b.elseBranch()
				{
					e.pairF64(wasmLocA)
					b.f64Const(-9223372036854775808.0)
					b.op(wasmOpF64Lt)
					b.ifVoid()
					{
						b.i32Const(1)
						e.fpsrOrIf(IE64_FPU_EX_IO)
						b.i64Const(-0x8000000000000000)
						b.localSet(wasmLocT1)
					}
					b.elseBranch()
					{
						e.pairF64(wasmLocA)
						b.op(wasmOpI64TruncF64S)
						b.localSet(wasmLocT1)
					}
					b.end()
				}
				b.end()
			}
			b.end()
		}
		b.end()
		b.localGet(wasmLocRegs)
		b.localGet(wasmLocT1)
		b.i64Store(3, uint32(ins.rd)*8)
	}
}

// emitDLoad: 8-byte load into an FP pair with condition-code update; MMIO
// and out-of-window addresses take HELPER_DLOAD.
func (e *wasmBlockEmitter) emitDLoad(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.effAddr(ins)
	if _, proven := ie64ConstLowRAMAccess(ins.rs, ins.imm32, IE64_SIZE_Q); !proven && !e.loopAccessHoisted(idx) {
		e.memChecks(8, func() {
			e.helperExit(HELPER_DLOAD, IE64_SIZE_Q, ins.rd, idx, instrPC, false)
		})
	}
	e.guestAddr()
	b.i64Load(0, 0)
	b.localSet(wasmLocT0)
	e.storePairBitsFrom(ins.rd, wasmLocT0)
	e.callCC(wasmLocT0)
}

// emitDStore: 8-byte store of an FP pair; no condition-code change.
func (e *wasmBlockEmitter) emitDStore(ins *JITInstr, idx uint32, instrPC uint64) {
	b := e.b
	e.loadPairBits(ins.rd)
	b.localSet(wasmLocB)
	e.effAddr(ins)
	if _, proven := ie64ConstLowRAMAccess(ins.rs, ins.imm32, IE64_SIZE_Q); !proven && !e.loopAccessHoisted(idx) {
		e.memChecks(8, func() {
			e.helperExit(HELPER_DSTORE, IE64_SIZE_Q, ins.rd, idx, instrPC, true)
		})
	}
	e.guestAddr()
	b.localGet(wasmLocB)
	b.i64Store(0, 0)
	e.smcProbe(8)
	e.smcExitIfDirty(instrPC, idx)
}

// ---------------------------------------------------------------------------
// Chain driver
// ---------------------------------------------------------------------------

// wasmBuildDriverModule emits the in-wasm dispatch driver. The driver keeps
// block-to-block transfers inside wasm: it reads the next PC from
// ctx.RetPC, looks the PC up in a direct-mapped cache in linear memory
// (16-byte entries: {pc u64, slot+1 u32, pad}), and call_indirects through
// the shared table until the budget runs out, the lookup misses, or a block
// reports a helper exit or SMC invalidation. The measured gap this closes:
// roughly 700 ns per JS-boundary Invoke against roughly 15 ns per in-wasm
// indirect call.
//
// cacheBase and cacheMask (entry-count minus one, power of two) are baked
// into the module; the cache lives at a fixed Go heap address for the
// runtime's lifetime.
func wasmBuildDriverModule(cacheBase uint32, cacheMask uint32) []byte {
	m := newWasmModuleBuilder()
	m.importMemory("env", "mem", 1)
	m.importTable("env", "tab", 1)
	typ := m.addType([]byte{wasmTypeI32}, nil)

	const (
		locCtx = 0 // i32 param
		locPC  = 1 // i64
		locT   = 2 // i32 scratch
	)
	b := &wasmBody{}
	b.localGet(locCtx)
	b.i64Load(3, jitCtxOffRetPC)
	b.localSet(locPC)
	b.block() // $X
	b.loop()  // $L
	// Budget.
	b.localGet(locCtx)
	b.i32Load(2, jitCtxOffChainBudget)
	b.localTee(locT)
	b.op(wasmOpI32Eqz)
	b.brIf(1) // -> $X
	b.localGet(locCtx)
	b.localGet(locT)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.i32Store(2, jitCtxOffChainBudget)
	// Entry address: cacheBase + ((i32(pc>>3) & mask) << 4), kept in locT.
	b.localGet(locPC)
	b.i64Const(3)
	b.op(wasmOpI64ShrU)
	b.op(wasmOpI32WrapI64)
	b.i32Const(int32(cacheMask))
	b.op(wasmOpI32And)
	b.i32Const(4)
	b.op(wasmOpI32Shl)
	b.i32Const(int32(cacheBase))
	b.op(wasmOpI32Add)
	b.localTee(locT)
	// Tag match?
	b.i64Load(3, 0)
	b.localGet(locPC)
	b.op(wasmOpI64Ne)
	b.brIf(1) // -> $X
	// Slot (stored +1; 0 means empty).
	b.localGet(locT)
	b.i32Load(2, 8)
	b.localTee(locT)
	b.op(wasmOpI32Eqz)
	b.brIf(1) // -> $X
	// Call the block.
	b.localGet(locCtx)
	b.localGet(locT)
	b.i32Const(1)
	b.op(wasmOpI32Sub)
	b.callIndirect(typ)
	// Helper exit or SMC report ends the chain.
	b.localGet(locCtx)
	b.i32Load(2, jitCtxOffNeedHelper)
	b.brIf(1)
	b.localGet(locCtx)
	b.i32Load(2, jitCtxOffNeedInval)
	b.brIf(1)
	// ChainCount += RetCount; RetCount = 0; the executed total the Go
	// dispatcher computes (RetCount + ChainCount) is invariant, so the last
	// block's count lands either way.
	b.localGet(locCtx)
	b.localGet(locCtx)
	b.i32Load(2, jitCtxOffChainCount)
	b.localGet(locCtx)
	b.i32Load(2, jitCtxOffRetCount)
	b.op(wasmOpI32Add)
	b.i32Store(2, jitCtxOffChainCount)
	b.localGet(locCtx)
	b.i32Const(0)
	b.i32Store(2, jitCtxOffRetCount)
	// Next PC.
	b.localGet(locCtx)
	b.i64Load(3, jitCtxOffRetPC)
	b.localSet(locPC)
	b.br(0) // -> $L
	b.end() // $L
	b.end() // $X
	b.end()

	fn := m.addFunc(typ, []byte{wasmTypeI64, wasmTypeI32}, b.code)
	m.exportFunc("drive", fn)
	return m.build()
}

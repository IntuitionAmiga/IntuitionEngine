// jit_common.go - JIT compiler infrastructure: CodeBuffer, block scanner, code cache

package main

import (
	"encoding/binary"
	"unsafe"
)

// ===========================================================================
// JITContext — Bridge between Go and JIT-compiled native code
// ===========================================================================

// JITContext is passed to every JIT-compiled block as its sole argument.
// On ARM64 it arrives in X0; on x86-64 in RDI.
type JITContext struct {
	RegsPtr        uintptr // 0:  &cpu.regs[0]
	MemPtr         uintptr // 8:  &cpu.memory[0]
	MemSize        uint32  // 16: len(cpu.memory)
	IOStart        uint32  // 20: IO_REGION_START
	PCPtr          uintptr // 24: &cpu.PC
	LoadMemFn      uintptr // 32: Go helper for I/O reads (future)
	StoreMemFn     uintptr // 40: Go helper for I/O writes (future)
	CpuPtr         uintptr // 48: &cpu for Go callouts
	NeedInval      uint32  // 56: set to 1 when code cache needs invalidation
	NeedIOFallback uint32  // 60: set to 1 when LOAD/STORE hits I/O page
	IOBitmapPtr    uintptr // 64: &cpu.bus.ioPageBitmap[0]
	FPUPtr         uintptr // 72: &cpu.FPU (pointer to IE64FPU struct)
}

// JITContext field offsets (must match struct layout above)
const (
	jitCtxOffRegsPtr        = 0
	jitCtxOffMemPtr         = 8
	jitCtxOffMemSize        = 16
	jitCtxOffIOStart        = 20
	jitCtxOffPCPtr          = 24
	jitCtxOffLoadMemFn      = 32
	jitCtxOffStoreMemFn     = 40
	jitCtxOffCpuPtr         = 48
	jitCtxOffNeedInval      = 56
	jitCtxOffNeedIOFallback = 60
	jitCtxOffIOBitmapPtr    = 64
	jitCtxOffFPUPtr         = 72
)

// jitAvailable is set to true at init time on platforms that support JIT.
var jitAvailable bool

func newJITContext(cpu *CPU64) *JITContext {
	ctx := &JITContext{
		RegsPtr: uintptr(unsafe.Pointer(&cpu.regs[0])),
		MemPtr:  uintptr(unsafe.Pointer(&cpu.memory[0])),
		MemSize: uint32(len(cpu.memory)),
		IOStart: IO_REGION_START,
		PCPtr:   uintptr(unsafe.Pointer(&cpu.PC)),
		CpuPtr:  uintptr(unsafe.Pointer(cpu)),
	}
	if cpu.bus != nil && len(cpu.bus.ioPageBitmap) > 0 {
		ctx.IOBitmapPtr = uintptr(unsafe.Pointer(&cpu.bus.ioPageBitmap[0]))
	}
	if cpu.FPU != nil {
		ctx.FPUPtr = uintptr(unsafe.Pointer(cpu.FPU))
	}
	return ctx
}

// ===========================================================================
// JITInstr — Pre-decoded IE64 instruction for JIT compilation
// ===========================================================================

type JITInstr struct {
	opcode   byte
	rd       byte
	size     byte
	xbit     byte
	rs       byte
	rt       byte
	mmuBail  bool // when true, emit bail-to-interpreter instead of native memory access
	imm32    uint32
	pcOffset uint32 // byte offset from block start
}

// ===========================================================================
// Block Scanner
// ===========================================================================

const jitMaxBlockSize = 256

// isBlockTerminator returns true if the opcode ends a basic block.
func isBlockTerminator(opcode byte) bool {
	switch opcode {
	case OP_BRA, OP_JMP, OP_JSR64, OP_RTS64, OP_JSR_IND, OP_HALT64, OP_RTI64, OP_WAIT64:
		return true
	// MMU/privilege opcodes: all are block terminators to ensure they are always
	// the last instruction, so the dispatcher re-enters with updated state.
	case OP_SYSCALL, OP_ERET, OP_MTCR, OP_MFCR, OP_TLBFLUSH, OP_TLBINVAL, OP_SMODE,
		OP_SUAEN, OP_SUADIS:
		return true
	// Atomic RMW: block terminators because they can trap (alignment, MMU)
	case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		return true
	}
	return false
}

// scanBlock decodes IE64 instructions starting at startPC until a block
// terminator is found or the max block size is reached. The terminating
// instruction IS included in the block (branches need to be compiled).
func scanBlock(memory []byte, startPC uint32) []JITInstr {
	instrs := make([]JITInstr, 0, 32)
	memSize := uint32(len(memory))
	pc := startPC

	for len(instrs) < jitMaxBlockSize {
		if pc+IE64_INSTR_SIZE > memSize {
			break
		}

		instr := binary.LittleEndian.Uint64(memory[pc:])
		opcode := byte(instr)
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)

		ji := JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: pc - startPC,
		}
		instrs = append(instrs, ji)

		if isBlockTerminator(opcode) {
			break
		}
		pc += IE64_INSTR_SIZE
	}

	return instrs
}

// scanBlockWithLimit is like scanBlock but stops at maxPC (exclusive).
// Used when MMU is enabled to prevent scanning across page boundaries.
func scanBlockWithLimit(memory []byte, startPC, maxPC uint32) []JITInstr {
	instrs := make([]JITInstr, 0, 32)
	pc := startPC

	for len(instrs) < jitMaxBlockSize {
		if pc+IE64_INSTR_SIZE > maxPC {
			break
		}

		instr := binary.LittleEndian.Uint64(memory[pc:])
		opcode := byte(instr)
		byte1 := byte(instr >> 8)
		byte2 := byte(instr >> 16)
		byte3 := byte(instr >> 24)
		imm32 := uint32(instr >> 32)

		ji := JITInstr{
			opcode:   opcode,
			rd:       byte1 >> 3,
			size:     (byte1 >> 1) & 0x03,
			xbit:     byte1 & 1,
			rs:       byte2 >> 3,
			rt:       byte3 >> 3,
			imm32:    imm32,
			pcOffset: pc - startPC,
		}
		instrs = append(instrs, ji)

		if isBlockTerminator(opcode) {
			break
		}
		pc += IE64_INSTR_SIZE
	}

	return instrs
}

// needsFallback returns true if the block's first instruction requires
// the interpreter (FPU, WAIT, HALT, etc.) and can't be JIT-compiled.
func needsFallback(instrs []JITInstr) bool {
	if len(instrs) == 0 {
		return true
	}
	op := instrs[0].opcode
	// Transcendentals as sole instruction need interpreter (no native ARM64 equivalent)
	switch op {
	case OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW, OP_DMOD:
		return true
	}
	// HALT and WAIT need interpreter (they block/sleep)
	if op == OP_HALT64 || op == OP_WAIT64 {
		return true
	}
	// RTI needs interpreter (complex interrupt state management)
	if op == OP_RTI64 {
		return true
	}
	// MMU/privilege opcodes need interpreter (they mutate CPU state)
	switch op {
	case OP_SYSCALL, OP_ERET, OP_MTCR, OP_MFCR, OP_TLBFLUSH, OP_TLBINVAL, OP_SMODE,
		OP_SUAEN, OP_SUADIS:
		return true
	}
	// Atomic RMW always interpreted (infrequent sync ops)
	switch op {
	case OP_CAS, OP_XCHG, OP_FAA, OP_FAND, OP_FOR, OP_FXOR:
		return true
	}
	return false
}

// ===========================================================================
// Register Liveness Analysis
// ===========================================================================

// blockRegs holds register usage bitmasks for a JIT block.
// Bit i corresponds to IE64 register i (0-31). R0 is always cleared (XZR).
type blockRegs struct {
	read              uint32 // IE64 registers that are read by any instruction
	written           uint32 // IE64 registers that are written by any instruction
	used              uint32 // read | written (determines callee-saved pairs)
	hasFPU            bool   // true if any FPU opcode (0x60-0x7C) is in the block
	hasBackwardBranch bool   // true if any Bcc/BRA targets an earlier instruction
}

// analyzeBlockRegs scans a block's instructions and returns bitmasks of
// which IE64 registers are read and written. Used to minimize prologue/epilogue
// overhead — only load/store registers the block actually touches.
func analyzeBlockRegs(instrs []JITInstr) blockRegs {
	var read, written uint32
	hasFPU := false
	for _, ji := range instrs {
		switch ji.opcode {
		case OP_MOVE:
			if ji.xbit == 0 {
				read |= 1 << ji.rs
			}
			written |= 1 << ji.rd
		case OP_MOVT:
			read |= 1 << ji.rd // read-modify-write (preserves lower 32 bits)
			written |= 1 << ji.rd
		case OP_MOVEQ:
			written |= 1 << ji.rd
		case OP_LEA:
			read |= 1 << ji.rs
			written |= 1 << ji.rd
		case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR:
			read |= 1 << ji.rs
			if ji.xbit == 0 {
				read |= 1 << ji.rt
			}
			written |= 1 << ji.rd
		case OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS:
			read |= 1 << ji.rs
			if ji.xbit == 0 {
				read |= 1 << ji.rt
			}
			written |= 1 << ji.rd
		case OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT, OP_CTZ, OP_POPCNT, OP_BSWAP:
			read |= 1 << ji.rs
			written |= 1 << ji.rd
		case OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
			read |= 1 << ji.rs
			if ji.xbit == 0 {
				read |= 1 << ji.rt
			}
			written |= 1 << ji.rd
		case OP_LOAD:
			read |= 1 << ji.rs
			written |= 1 << ji.rd
		case OP_STORE:
			read |= 1 << ji.rs
			read |= 1 << ji.rd // rd is value to store (read)
		case OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
			read |= 1 << ji.rs
			read |= 1 << ji.rt
		case OP_JMP:
			read |= 1 << ji.rs
		case OP_JSR64:
			read |= 1 << 31
			written |= 1 << 31
		case OP_RTS64:
			read |= 1 << 31
			written |= 1 << 31
		case OP_PUSH64:
			read |= 1 << ji.rs
			read |= 1 << 31
			written |= 1 << 31
		case OP_POP64:
			written |= 1 << ji.rd
			read |= 1 << 31
			written |= 1 << 31
		case OP_JSR_IND:
			read |= 1 << ji.rs
			read |= 1 << 31
			written |= 1 << 31

		// FPU opcodes that touch integer registers
		case OP_FMOVI:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs
		case OP_FMOVO:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd
		case OP_FCMP:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd (comparison result)
		case OP_FCVTIF:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs
		case OP_FCVTFI:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd
		case OP_FMOVSR, OP_FMOVCR:
			hasFPU = true
			written |= 1 << ji.rd // writes integer rd (FPSR/FPCR value)
		case OP_FMOVSC, OP_FMOVCC:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs
		case OP_FLOAD:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs (address base)
		case OP_FSTORE:
			hasFPU = true
			read |= 1 << ji.rs // reads integer rs (address base)
		case OP_FMOV, OP_FABS, OP_FNEG, OP_FMOVECR,
			OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV, OP_FSQRT, OP_FINT,
			OP_FMOD, OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP, OP_FPOW:
			hasFPU = true
		case OP_DCVTIF:
			hasFPU = true
			read |= 1 << ji.rs
		case OP_DCMP, OP_DCVTFI:
			hasFPU = true
			written |= 1 << ji.rd
		case OP_DLOAD, OP_DSTORE:
			hasFPU = true
			read |= 1 << ji.rs
		case OP_DMOV, OP_DABS, OP_DNEG, OP_DSQRT, OP_DINT, OP_FCVTSD, OP_FCVTDS,
			OP_DADD, OP_DSUB, OP_DMUL, OP_DDIV, OP_DMOD:
			hasFPU = true

		// RTI pops return address from stack (reads & writes R31/SP)
		case OP_RTI64:
			read |= 1 << 31
			written |= 1 << 31
		}
	}
	// R0 is XZR — never loaded or stored
	read &^= 1
	written &^= 1
	return blockRegs{read: read, written: written, used: read | written, hasFPU: hasFPU}
}

// instrWrittenRegs returns a bitmask of IE64 registers written by a single
// instruction. Used to track writtenSoFar for I/O bail epilogues.
func instrWrittenRegs(ji *JITInstr) uint32 {
	var w uint32
	switch ji.opcode {
	case OP_MOVE, OP_MOVT, OP_MOVEQ, OP_LEA, OP_LOAD:
		w = 1 << ji.rd
	case OP_ADD, OP_SUB, OP_AND64, OP_OR64, OP_EOR,
		OP_MULU, OP_MULS, OP_DIVU, OP_DIVS, OP_MOD64, OP_MODS, OP_MULHU, OP_MULHS,
		OP_NEG, OP_NOT64, OP_CLZ, OP_SEXT, OP_CTZ, OP_POPCNT, OP_BSWAP,
		OP_LSL, OP_LSR, OP_ASR, OP_ROL, OP_ROR:
		w = 1 << ji.rd
	case OP_JSR64, OP_RTS64, OP_JSR_IND:
		w = 1 << 31
	case OP_PUSH64:
		w = 1 << 31
	case OP_POP64:
		w = (1 << ji.rd) | (1 << 31)
	// FPU opcodes that write integer registers
	case OP_FMOVO, OP_FCMP, OP_FCVTFI, OP_FMOVSR, OP_FMOVCR, OP_DCMP, OP_DCVTFI:
		w = 1 << ji.rd
	// RTI writes R31 (SP += 8)
	case OP_RTI64:
		w = 1 << 31
	}
	return w &^ 1 // clear R0
}

// detectBackwardBranches returns true if any conditional branch (BEQ-BLS) or
// BRA targets an earlier instruction within the same block. Used to enable
// native backward branches with budget-based timer safety.
func detectBackwardBranches(instrs []JITInstr, startPC uint32) bool {
	for _, ji := range instrs {
		var isBranch bool
		switch ji.opcode {
		case OP_BRA, OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT, OP_BLE, OP_BHI, OP_BLS:
			isBranch = true
		}
		if !isBranch {
			continue
		}
		instrPC := startPC + ji.pcOffset
		targetPC := uint32(int64(instrPC) + int64(int32(ji.imm32)))
		if targetPC >= startPC && targetPC < instrPC && (targetPC-startPC)%IE64_INSTR_SIZE == 0 {
			return true
		}
	}
	return false
}

// ===========================================================================
// CodeBuffer — Byte buffer for emitting native machine code
// ===========================================================================

type fixup struct {
	name   string
	offset int // byte offset in buf where patch is needed
	size   int // patch size in bytes (4 for ARM64, variable for x86-64)
	pcBase int // base PC for PC-relative calculations
}

type CodeBuffer struct {
	buf    []byte
	labels map[string]int // label name -> byte offset
	fixups []fixup
}

func NewCodeBuffer(capacity int) *CodeBuffer {
	return &CodeBuffer{
		buf:    make([]byte, 0, capacity),
		labels: make(map[string]int),
	}
}

// Emit32 appends a 32-bit value (little-endian). Used for ARM64 fixed-width instructions.
func (cb *CodeBuffer) Emit32(val uint32) {
	cb.buf = append(cb.buf, byte(val), byte(val>>8), byte(val>>16), byte(val>>24))
}

// EmitBytes appends raw bytes. Used for x86-64 variable-length instructions.
func (cb *CodeBuffer) EmitBytes(b ...byte) {
	cb.buf = append(cb.buf, b...)
}

// Emit16 appends a 16-bit value (little-endian).
func (cb *CodeBuffer) Emit16(val uint16) {
	cb.buf = append(cb.buf, byte(val), byte(val>>8))
}

// Emit64 appends a 64-bit value (little-endian).
func (cb *CodeBuffer) Emit64(val uint64) {
	cb.buf = append(cb.buf,
		byte(val), byte(val>>8), byte(val>>16), byte(val>>24),
		byte(val>>32), byte(val>>40), byte(val>>48), byte(val>>56))
}

// Label records the current byte offset for a named label.
func (cb *CodeBuffer) Label(name string) {
	cb.labels[name] = len(cb.buf)
}

// FixupRel32 records a 32-bit PC-relative fixup at the current position.
// pcBase is the reference point for the relative calculation.
func (cb *CodeBuffer) FixupRel32(name string, pcBase int) {
	cb.fixups = append(cb.fixups, fixup{
		name:   name,
		offset: len(cb.buf),
		size:   4,
		pcBase: pcBase,
	})
	// Emit placeholder
	cb.buf = append(cb.buf, 0, 0, 0, 0)
}

// Resolve patches all forward-reference fixups with actual label offsets.
func (cb *CodeBuffer) Resolve() {
	for _, f := range cb.fixups {
		target, ok := cb.labels[f.name]
		if !ok {
			continue
		}
		rel := int32(target - f.pcBase)
		binary.LittleEndian.PutUint32(cb.buf[f.offset:], uint32(rel))
	}
	cb.fixups = cb.fixups[:0]
}

// Len returns the current code size in bytes.
func (cb *CodeBuffer) Len() int {
	return len(cb.buf)
}

// Bytes returns the emitted code.
func (cb *CodeBuffer) Bytes() []byte {
	return cb.buf
}

// PatchUint32 overwrites 4 bytes at the given offset.
func (cb *CodeBuffer) PatchUint32(offset int, val uint32) {
	binary.LittleEndian.PutUint32(cb.buf[offset:], val)
}

// ===========================================================================
// Code Cache
// ===========================================================================

// chainSlot records a patchable chain exit point within a compiled block.
type chainSlot struct {
	targetPC  uint32  // 6502/IE64 PC this exit targets
	patchAddr uintptr // address of JMP rel32 displacement in ExecMem
}

type JITBlock struct {
	startPC        uint32
	endPC          uint32
	instrCount     int
	execAddr       uintptr
	execSize       int
	chainEntry     uintptr     // lightweight entry point for chained transitions (0 = none)
	chainSlots     []chainSlot // patchable exit points
	execCount      uint32      // execution count for hot-block detection (Tier 2)
	tier           int         // compilation tier (0=Tier 1, 1=Tier 2)
	regMap         [8]byte     // x86 JIT: guest-to-host register mapping for chain compatibility
	chainHits      uint32      // times this block was entered via chain (not Go dispatch)
	unchainedExits uint32      // times this block exited via unchained path
	ioBails        uint32      // times this block triggered I/O fallback
	lastPromoteAt  uint32      // exec count when last promoted (hysteresis)
	rIncrements    int         // Z80: total R register increments for this block
}

type CodeCache struct {
	blocks map[uint64]*JITBlock
}

func NewCodeCache() *CodeCache {
	return &CodeCache{
		blocks: make(map[uint64]*JITBlock),
	}
}

func (cc *CodeCache) Get(pc uint32) *JITBlock {
	return cc.blocks[uint64(pc)]
}

func (cc *CodeCache) Put(block *JITBlock) {
	cc.blocks[uint64(block.startPC)] = block
}

func (cc *CodeCache) GetKey(key uint64) *JITBlock {
	return cc.blocks[key]
}

func (cc *CodeCache) PutKey(key uint64, block *JITBlock) {
	cc.blocks[key] = block
}

// Invalidate clears the entire code cache.
func (cc *CodeCache) Invalidate() {
	clear(cc.blocks)
}

// InvalidateRange removes any blocks whose [startPC, endPC) overlaps [lo, hi).
func (cc *CodeCache) InvalidateRange(lo, hi uint32) {
	for key, block := range cc.blocks {
		if block.endPC > lo && block.startPC < hi {
			delete(cc.blocks, key)
		}
	}
}

// PatchChainsTo scans all cached blocks for chain slots targeting targetPC
// and patches their JMP rel32 to jump to chainEntry.
func (cc *CodeCache) PatchChainsTo(targetPC uint32, chainEntry uintptr) {
	for _, block := range cc.blocks {
		for _, slot := range block.chainSlots {
			if slot.targetPC == targetPC && slot.patchAddr != 0 {
				PatchRel32At(slot.patchAddr, chainEntry)
			}
		}
	}
}

// UnpatchChainsInRange resets chain slots that target any block whose
// [startPC, endPC) overlaps [lo, hi). This must match the same overlap
// condition used by InvalidateRange, so that every block about to be removed
// has all inbound chain jumps reset to their unchained fallback first.
// Must be called BEFORE InvalidateRange.
func (cc *CodeCache) UnpatchChainsInRange(lo, hi uint32) {
	// Collect the startPCs of all blocks that will be removed.
	var doomed []uint32
	for _, block := range cc.blocks {
		if block.endPC > lo && block.startPC < hi {
			doomed = append(doomed, block.startPC)
		}
	}
	if len(doomed) == 0 {
		return
	}

	// Build a set for O(1) lookup.
	doomedSet := make(map[uint32]struct{}, len(doomed))
	for _, pc := range doomed {
		doomedSet[pc] = struct{}{}
	}

	// Unpatch every chain slot in every surviving block that targets a doomed block.
	for _, block := range cc.blocks {
		for _, slot := range block.chainSlots {
			if slot.patchAddr == 0 {
				continue
			}
			if _, ok := doomedSet[slot.targetPC]; ok {
				PatchRel32At(slot.patchAddr, slot.patchAddr+4)
			}
		}
	}
}

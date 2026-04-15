// jit_m68k_common.go - M68020 JIT compiler infrastructure: context, block scanner, code cache integration

package main

import (
	"math/bits"
	"unsafe"
)

// ===========================================================================
// M68KJITContext — Bridge between Go and JIT-compiled native code
// ===========================================================================

// M68KJITContext is passed to every JIT-compiled M68K block as its sole argument.
// On ARM64 it arrives in X0; on x86-64 in RDI.
type M68KJITContext struct {
	DataRegsPtr       uintptr // 0:  &cpu.DataRegs[0]
	AddrRegsPtr       uintptr // 8:  &cpu.AddrRegs[0]
	MemPtr            uintptr // 16: &cpu.memory[0]
	MemSize           uint32  // 24: len(cpu.memory)
	IOThreshold       uint32  // 28: 0xA0000 (fast-path boundary)
	SRPtr             uintptr // 32: &cpu.SR
	CpuPtr            uintptr // 40: &cpu
	NeedInval         uint32  // 48: self-modification flag
	NeedIOFallback    uint32  // 52: I/O bail flag
	RetPC             uint32  // 56: next PC after block execution
	RetCount          uint32  // 60: instructions retired in block
	CodePageBitmapPtr uintptr // 64: pointer to heap-allocated code page bitmap
	ChainBudget       uint32  // 72: blocks remaining before returning to Go (init=64)
	ChainCount        uint32  // 76: accumulated instruction count during chaining
	RTSCache0PC       uint32  // 80: MRU entry 0 - M68K PC
	_pad0             uint32  // 84: padding for alignment
	RTSCache0Addr     uintptr // 88: MRU entry 0 - chain entry address
	RTSCache1PC       uint32  // 96: MRU entry 1 - M68K PC
	_pad1             uint32  // 100: padding for alignment
	RTSCache1Addr     uintptr // 104: MRU entry 1 - chain entry address
}

// M68KJITContext field offsets (must match struct layout above)
const (
	m68kCtxOffDataRegsPtr       = 0
	m68kCtxOffAddrRegsPtr       = 8
	m68kCtxOffMemPtr            = 16
	m68kCtxOffMemSize           = 24
	m68kCtxOffIOThreshold       = 28
	m68kCtxOffSRPtr             = 32
	m68kCtxOffCpuPtr            = 40
	m68kCtxOffNeedInval         = 48
	m68kCtxOffNeedIOFallback    = 52
	m68kCtxOffRetPC             = 56
	m68kCtxOffRetCount          = 60
	m68kCtxOffCodePageBitmapPtr = 64
	m68kCtxOffChainBudget       = 72
	m68kCtxOffChainCount        = 76
	m68kCtxOffRTSCache0PC       = 80
	m68kCtxOffRTSCache0Addr     = 88 // MRU entry 0
	m68kCtxOffRTSCache1PC       = 96
	m68kCtxOffRTSCache1Addr     = 104 // MRU entry 1
)

// m68kJitAvailable is set to true at init time on platforms that support JIT.
var m68kJitAvailable bool

func newM68KJITContext(cpu *M68KCPU, codePageBitmap []byte) *M68KJITContext {
	ctx := &M68KJITContext{
		DataRegsPtr: uintptr(unsafe.Pointer(&cpu.DataRegs[0])),
		AddrRegsPtr: uintptr(unsafe.Pointer(&cpu.AddrRegs[0])),
		MemPtr:      uintptr(unsafe.Pointer(&cpu.memory[0])),
		MemSize:     uint32(len(cpu.memory)),
		IOThreshold: 0xA0000,
		SRPtr:       uintptr(unsafe.Pointer(&cpu.SR)),
		CpuPtr:      uintptr(unsafe.Pointer(cpu)),
	}
	if len(codePageBitmap) > 0 {
		ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&codePageBitmap[0]))
	}
	return ctx
}

// ===========================================================================
// M68KJITInstr — Pre-decoded M68K instruction for JIT compilation
// ===========================================================================

// M68KJITInstr represents one M68K instruction scanned from a basic block.
// Fields are pre-decoded during scanning to avoid re-parsing during emission.
type M68KJITInstr struct {
	opcode   uint16 // first word of instruction
	pcOffset uint32 // byte offset from block start
	length   uint16 // total instruction length in bytes (2-10+)
	group    uint8  // opcode >> 12
}

// ===========================================================================
// Instruction Length Calculator
// ===========================================================================

// m68kEAExtBytes returns the number of additional bytes consumed by the
// effective address extension words for the given addressing mode and register.
// memory and pc are needed to peek at 68020 full-format extension words.
// pc points to the first extension word (immediately after the opcode or
// previous extension words).
func m68kEAExtBytes(mode, reg uint16, size int, memory []byte, pc uint32) int {
	switch mode {
	case 0, 1, 2, 3, 4: // Dn, An, (An), (An)+, -(An)
		return 0
	case 5: // (d16,An)
		return 2
	case 6: // (d8,An,Xn) or 68020 full format
		return m68kIndexedExtBytes(memory, pc)
	case 7:
		switch reg {
		case 0: // (xxx).W
			return 2
		case 1: // (xxx).L
			return 4
		case 2: // (d16,PC)
			return 2
		case 3: // (d8,PC,Xn) or 68020 full format
			return m68kIndexedExtBytes(memory, pc)
		case 4: // #imm
			return m68kImmediateBytes(size)
		}
	}
	return 0
}

// m68kIndexedExtBytes returns the bytes consumed by an indexed extension word
// (brief format = 2 bytes, 68020 full format = variable).
func m68kIndexedExtBytes(memory []byte, pc uint32) int {
	if pc+2 > uint32(len(memory)) {
		return 2 // assume brief if we can't read
	}
	// Read extension word in big-endian
	extWord := uint16(memory[pc])<<8 | uint16(memory[pc+1])

	if extWord&0x0100 == 0 {
		// Brief format: always 2 bytes (the extension word itself)
		return 2
	}

	// Full 68020 format
	total := 2 // the extension word itself

	// Base displacement size (bits 5-4)
	bd := (extWord >> 4) & 3
	switch bd {
	case 2:
		total += 2 // word BD
	case 3:
		total += 4 // long BD
	}

	// Outer displacement from I/IS field (bits 2-0)
	iis := extWord & 7
	switch iis {
	case 2, 6: // word outer displacement
		total += 2
	case 3, 7: // long outer displacement
		total += 4
	}

	return total
}

// m68kImmediateBytes returns the number of bytes consumed by an immediate
// operand for the given operation size.
func m68kImmediateBytes(size int) int {
	switch size {
	case M68K_SIZE_BYTE, M68K_SIZE_WORD:
		return 2 // byte/word immediates occupy one extension word
	case M68K_SIZE_LONG:
		return 4 // long immediates occupy two extension words
	}
	return 2 // default to word
}

// m68kInstrLength returns the total byte length of the M68K instruction at
// the given PC. This is the foundation of the block scanner — it must handle
// every opcode group correctly.
func m68kInstrLength(memory []byte, pc uint32) int {
	if pc+2 > uint32(len(memory)) {
		return 2
	}

	// Read opcode in big-endian
	opcode := uint16(memory[pc])<<8 | uint16(memory[pc+1])
	group := opcode >> 12

	switch group {
	case 0x0: // Bit manipulation, immediate, MOVEP, CAS, CHK2/CMP2
		return m68kGroup0Length(opcode, memory, pc)
	case 0x1: // MOVE.B
		return m68kMoveLength(opcode, M68K_SIZE_BYTE, memory, pc)
	case 0x2: // MOVE.L / MOVEA.L
		return m68kMoveLength(opcode, M68K_SIZE_LONG, memory, pc)
	case 0x3: // MOVE.W / MOVEA.W
		return m68kMoveLength(opcode, M68K_SIZE_WORD, memory, pc)
	case 0x4: // Miscellaneous
		return m68kGroup4Length(opcode, memory, pc)
	case 0x5: // ADDQ, SUBQ, Scc, DBcc, TRAPcc
		return m68kGroup5Length(opcode, memory, pc)
	case 0x6: // Bcc, BRA, BSR
		return m68kGroup6Length(opcode)
	case 0x7: // MOVEQ
		return 2
	case 0x8: // OR, DIV, SBCD, PACK, UNPK
		return m68kGroup8Length(opcode, memory, pc)
	case 0x9: // SUB, SUBA, SUBX
		return m68kGroup9DLength(opcode, memory, pc)
	case 0xA: // Line A trap
		return 2
	case 0xB: // CMP, CMPA, EOR, CMPM
		return m68kGroupBLength(opcode, memory, pc)
	case 0xC: // AND, MUL, ABCD, EXG
		return m68kGroupCLength(opcode, memory, pc)
	case 0xD: // ADD, ADDA, ADDX
		return m68kGroup9DLength(opcode, memory, pc)
	case 0xE: // Shift, Rotate, Bit Field
		return m68kGroupELength(opcode, memory, pc)
	case 0xF: // Line F trap / FPU
		return 2
	}
	return 2
}

// m68kGroup0Length handles Group 0 (0x0xxx) instruction lengths.
func m68kGroup0Length(opcode uint16, memory []byte, pc uint32) int {
	// MOVEP: opcode & 0xF138 == 0x0108
	if opcode&0xF138 == 0x0108 {
		return 4 // opcode + displacement word
	}

	// CAS2: 0x0CFC or 0x0EFC
	if opcode == 0x0CFC || opcode == 0x0EFC {
		return 6 // opcode + 2 register mask words
	}

	// CAS: opcode & 0xF9C0 == 0x08C0 and size != 0
	if opcode&0xF9C0 == 0x08C0 && (opcode&0x0600) != 0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+4) // opcode + ext word + EA
	}

	// CHK2/CMP2: opcode & 0xF9C0 == 0x00C0 and bit 11 set
	if opcode&0xF9C0 == 0x00C0 && opcode&0x0800 != 0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+4)
	}

	// Immediate bit manipulation: BTST/BCHG/BCLR/BSET with immediate bit number
	// Pattern: 0x0800, 0x0840, 0x0880, 0x08C0
	if opcode&0xFF00 == 0x0800 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		// 2 (opcode) + 2 (immediate bit number) + EA
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+4)
	}

	// Dynamic bit manipulation: BTST/BCHG/BCLR/BSET Dn,<ea>
	// Pattern: opcode & 0xF1C0 in {0x0100, 0x0140, 0x0180, 0x01C0}
	if opcode&0xF100 == 0x0100 && opcode&0x00C0 != 0x00C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+2)
	}

	// Immediate instructions: ADDI/SUBI/CMPI/ANDI/ORI/EORI
	size := int((opcode >> 6) & 3)
	immBytes := m68kImmediateBytes(size)
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	return 2 + immBytes + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+uint32(2+immBytes))
}

// m68kMoveLength handles MOVE instruction lengths (groups 1, 2, 3).
// MOVE has reversed destination encoding: mode=bits[8:6], reg=bits[11:9].
func m68kMoveLength(opcode uint16, size int, memory []byte, pc uint32) int {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7

	srcExt := m68kEAExtBytes(srcMode, srcReg, size, memory, pc+2)
	dstExt := m68kEAExtBytes(dstMode, dstReg, size, memory, pc+2+uint32(srcExt))
	return 2 + srcExt + dstExt
}

// m68kGroup4Length handles Group 4 (0x4xxx) instruction lengths.
func m68kGroup4Length(opcode uint16, memory []byte, pc uint32) int {
	// Fixed-length instructions (exact opcode matches)
	switch opcode {
	case 0x4E70: // RESET
		return 2
	case 0x4E71: // NOP
		return 2
	case 0x4E72: // STOP
		return 4 // opcode + immediate word
	case 0x4E73: // RTE
		return 2
	case 0x4E75: // RTS
		return 2
	case 0x4E76: // TRAPV
		return 2
	case 0x4E77: // RTR
		return 2
	}

	// TRAP #vector
	if opcode&0xFFF0 == 0x4E40 {
		return 2
	}

	// LINK.W
	if opcode&0xFFF8 == 0x4E50 {
		return 4 // opcode + word displacement
	}

	// UNLK
	if opcode&0xFFF8 == 0x4E58 {
		return 2
	}

	// MOVE USP
	if opcode&0xFFF0 == 0x4E60 {
		return 2
	}

	// MOVEC
	if opcode&0xFFFE == 0x4E7A {
		return 4 // opcode + control register word
	}

	// LINK.L (68020)
	if opcode&0xFFF8 == 0x4808 {
		return 6 // opcode + long displacement
	}

	// BKPT
	if opcode&0xFFF8 == 0x4848 {
		return 2
	}

	// SWAP: 0x4840-0x4847
	if opcode&0xFFF8 == 0x4840 {
		return 2
	}

	// EXT/EXTB: 0x4880-0x4887 (EXT.W), 0x48C0-0x48C7 (EXT.L), 0x49C0-0x49C7 (EXTB.L)
	if opcode&0xFFF8 == 0x4880 || opcode&0xFFF8 == 0x48C0 || opcode&0xFFF8 == 0x49C0 {
		return 2
	}

	// MOVEM: opcode & 0xFB80 == 0x4880
	if opcode&0xFB80 == 0x4880 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		// 2 (opcode) + 2 (register mask) + EA
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+4)
	}

	// MULL/DIVL (68020): opcode & 0xFF80 == 0x4C00
	if opcode&0xFF80 == 0x4C00 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		// 2 (opcode) + 2 (register pair ext word) + EA
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_LONG, memory, pc+4)
	}

	// LEA: opcode & 0xF1C0 == 0x41C0
	if opcode&0xF1C0 == 0x41C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_LONG, memory, pc+2)
	}

	// CHK: opcode & 0xF040 == 0x4000 and bits 8-7 indicate CHK
	if opcode&0xF1C0 == 0x4180 || opcode&0xF1C0 == 0x4100 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+2)
	}

	// PEA: 0x4840 but only with mode >= 2
	// (SWAP/EXT already handled above for mode=0)
	if opcode&0xFFC0 == 0x4840 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_LONG, memory, pc+2)
	}

	// JSR: 0x4E80
	if opcode&0xFFC0 == 0x4E80 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_LONG, memory, pc+2)
	}

	// JMP: 0x4EC0
	if opcode&0xFFC0 == 0x4EC0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_LONG, memory, pc+2)
	}

	// MOVE from SR: 0x40C0, MOVE from CCR: 0x42C0, MOVE to CCR: 0x44C0, MOVE to SR: 0x46C0
	if opcode&0xFDC0 == 0x40C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+2)
	}

	// TAS: 0x4AC0
	if opcode&0xFFC0 == 0x4AC0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+2)
	}

	// NBCD: 0x4800
	if opcode&0xFFC0 == 0x4800 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+2)
	}

	// CLR/NEG/NEGX/NOT/TST: 0x42xx, 0x44xx, 0x40xx, 0x46xx, 0x4Axx
	// These all have size in bits 7-6, EA in bits 5-0
	size := int((opcode >> 6) & 3)
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
}

// m68kGroup5Length handles Group 5 (0x5xxx) instruction lengths.
func m68kGroup5Length(opcode uint16, memory []byte, pc uint32) int {
	// DBcc: opcode & 0xF0F8 == 0x50C8
	if opcode&0xF0F8 == 0x50C8 {
		return 4 // opcode + word displacement
	}

	// TRAPcc (68020): opcode & 0xF0F8 == 0x50F8
	if opcode&0xF0F8 == 0x50F8 {
		switch opcode & 7 {
		case 2:
			return 4 // word operand
		case 3:
			return 6 // long operand
		case 4:
			return 2 // no operand
		}
		return 2
	}

	// Scc: opcode & 0xF0C0 == 0x50C0
	if opcode&0x00C0 == 0x00C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+2)
	}

	// ADDQ/SUBQ
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	size := int((opcode >> 6) & 3)
	return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
}

// m68kGroup6Length handles Group 6 (0x6xxx) instruction lengths.
func m68kGroup6Length(opcode uint16) int {
	disp := opcode & 0xFF
	switch disp {
	case 0x00:
		return 4 // word displacement
	case 0xFF:
		return 6 // long displacement (68020)
	default:
		return 2 // byte displacement in opcode
	}
}

// m68kGroup8Length handles Group 8 (0x8xxx) instruction lengths.
func m68kGroup8Length(opcode uint16, memory []byte, pc uint32) int {
	// PACK: opcode & 0xF1F0 == 0x8140
	if opcode&0xF1F0 == 0x8140 {
		return 4 // opcode + adjustment word
	}
	// UNPK: opcode & 0xF1F0 == 0x8180
	if opcode&0xF1F0 == 0x8180 {
		return 4 // opcode + adjustment word
	}
	// SBCD: opcode & 0xF1F0 == 0x8100
	if opcode&0xF1F0 == 0x8100 {
		return 2
	}
	// DIVU/DIVS.W: opcode & 0xF0C0 == 0x80C0
	if opcode&0xF0C0 == 0x80C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+2)
	}
	// OR
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	size := int((opcode >> 6) & 3)
	return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
}

// m68kGroup9DLength handles Group 9 (SUB) and Group D (ADD) instruction lengths.
// Both have identical encoding structure.
func m68kGroup9DLength(opcode uint16, memory []byte, pc uint32) int {
	opmode := (opcode >> 6) & 7
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7

	// SUBX/ADDX: register or memory predecrement
	if (opmode == 0 || opmode == 1 || opmode == 2 || opmode == 4 || opmode == 5 || opmode == 6) &&
		(eaMode == 0 || eaMode == 1) && (opcode&0x0130) == 0x0100 {
		return 2
	}

	// SUBA/ADDA: opmode 3 or 7
	if opmode == 3 || opmode == 7 {
		size := M68K_SIZE_WORD
		if opmode == 7 {
			size = M68K_SIZE_LONG
		}
		return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
	}

	// SUB/ADD: standard EA operand
	size := int(opmode & 3)
	return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
}

// m68kGroupBLength handles Group B (0xBxxx) instruction lengths.
func m68kGroupBLength(opcode uint16, memory []byte, pc uint32) int {
	opmode := (opcode >> 6) & 7

	// CMPM: opcode & 0xF138 == 0xB108
	if opcode&0xF138 == 0xB108 {
		return 2
	}

	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7

	// CMPA: opmode 3 or 7
	if opmode == 3 || opmode == 7 {
		size := M68K_SIZE_WORD
		if opmode == 7 {
			size = M68K_SIZE_LONG
		}
		return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
	}

	// EOR: opmode 4-6 (direction bit set)
	// CMP: opmode 0-2
	size := int(opmode & 3)
	return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
}

// m68kGroupCLength handles Group C (0xCxxx) instruction lengths.
func m68kGroupCLength(opcode uint16, memory []byte, pc uint32) int {
	// ABCD: opcode & 0xF1F0 == 0xC100
	if opcode&0xF1F0 == 0xC100 {
		return 2
	}
	// EXG: opcode & 0xF130 == 0xC100 (different patterns)
	if opcode&0xF100 == 0xC100 {
		opmode := (opcode >> 3) & 0x1F
		if opmode == 0x08 || opmode == 0x09 || opmode == 0x11 {
			return 2 // EXG
		}
	}
	// MULU/MULS.W: opcode & 0xF0C0 == 0xC0C0
	if opcode&0xF0C0 == 0xC0C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+2)
	}
	// AND
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	size := int((opcode >> 6) & 3)
	return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
}

// m68kGroupELength handles Group E (0xExxx) instruction lengths.
func m68kGroupELength(opcode uint16, memory []byte, pc uint32) int {
	// Bit field operations: 0xE8C0-0xEFC0
	if opcode&0xF8C0 == 0xE8C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		// 2 (opcode) + 2 (bit field extension word) + EA
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+4)
	}

	// Memory shift/rotate: opcode & 0xFEC0 == 0xE0C0 (size field == 3)
	if (opcode>>6)&3 == 3 && (opcode&0xF800) == 0xE000 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+2)
	}

	// Register shift/rotate: always 2 bytes
	return 2
}

// ===========================================================================
// Block Scanner
// ===========================================================================

const m68kJitMaxBlockSize = 256

// m68kIsBlockTerminator returns true if the opcode unconditionally transfers
// control (no fallthrough). Conditional branches (Bcc, DBcc) are NOT
// terminators — they are compiled as multi-exit internal branches.
func m68kIsBlockTerminator(opcode uint16) bool {
	group := opcode >> 12

	switch group {
	case 0x6: // Branch group
		cond := (opcode >> 8) & 0xF
		// BRA (cond=0) and BSR (cond=1) are terminators.
		// Bcc (cond>=2) are NOT — they're conditional branches compiled inline.
		return cond <= 1

	case 0x4: // Miscellaneous
		switch {
		case opcode == 0x4E75: // RTS
			return true
		case opcode == 0x4E73: // RTE
			return true
		case opcode == 0x4E77: // RTR
			return true
		case opcode == 0x4E72: // STOP
			return true
		case opcode == 0x4E76: // TRAPV
			return true
		case opcode == 0x4E70: // RESET
			return true
		case opcode&0xFFF0 == 0x4E40: // TRAP #n
			return true
		case opcode&0xFFC0 == 0x4E80: // JSR
			return true
		case opcode&0xFFC0 == 0x4EC0: // JMP
			return true
		}

	case 0xA: // Line A trap
		return true

	case 0xF: // Line F trap
		return true
	}

	return false
}

// m68kNeedsFallback returns true if the block's first instruction requires
// the interpreter and can't be JIT-compiled.
func m68kNeedsFallback(instrs []M68KJITInstr) bool {
	if len(instrs) == 0 {
		return true
	}
	opcode := instrs[0].opcode
	group := opcode >> 12

	switch group {
	case 0xA: // Line A trap
		return true
	case 0xF: // Line F / FPU
		return true
	}

	// System instructions that need interpreter
	switch {
	case opcode == 0x4E72: // STOP
		return true
	case opcode == 0x4E73: // RTE
		return true
	case opcode == 0x4E77: // RTR
		return true
	case opcode == 0x4E70: // RESET
		return true
	case opcode&0xFFF0 == 0x4E40: // TRAP #n
		return true
	case opcode == 0x4E76: // TRAPV
		return true
	case opcode&0xFFFE == 0x4E7A: // MOVEC
		return true
	}

	// Exception-control blocks are correctness-sensitive. Do not partially JIT a
	// handler prologue only to bail at a trailing RTE/RTR.
	for _, ji := range instrs {
		switch ji.opcode {
		case 0x4E73, 0x4E77: // RTE, RTR
			return true
		}
	}

	return false
}

func m68kModeIsIndexed(mode, reg uint16) bool {
	return mode == 6 || (mode == 7 && reg == 3)
}

func m68kModeTouchesSP(mode, reg uint16) bool {
	if reg != 7 {
		return false
	}
	switch mode {
	case 1, 2, 3, 4, 5, 6:
		return true
	}
	return false
}

// m68kNeedsConservativeFallback rejects blocks that contain opcode families or
// EA forms that are still known-bad in the JIT. This intentionally favors
// correctness over coverage: the interpreter remains the source of truth until
// focused JIT regressions exist for a shape.
func m68kNeedsConservativeFallback(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	// Correctness-first temporary policy: keep runtime execution in the
	// interpreter until native M68K block coverage is rebuilt behind the full
	// test matrix. The native emitter remains covered by its direct unit tests.
	_ = memory
	_ = startPC
	_ = instrs
	return true

	for _, ji := range instrs {
		opcode := ji.opcode
		group := opcode >> 12

		// CHK2/CMP2
		if opcode&0xF9C0 == 0x00C0 && opcode&0x0800 != 0 {
			return true
		}

		// Branch/control-transfer correctness is still incomplete. Keep the
		// entire branch family interpreted until the suite is green, then
		// re-enable forms behind focused regressions.
		if group == 0x6 {
			return true
		}
		if opcode&0xF0F8 == 0x50C8 { // DBcc
			return true
		}
		if opcode&0xF0F8 == 0x50F8 { // TRAPcc
			return true
		}
		if opcode&0xF0C0 == 0x50C0 && opcode&0xF0F8 != 0x50F8 { // Scc
			return true
		}

		// Shift/rotate coverage is still selective; only bitfields are trusted.
		if group == 0xE && opcode&0xF8C0 != 0xE8C0 {
			return true
		}

		// MOVE.W / MOVEA.W still has multiple sign/flag gaps.
		if group == 0x3 {
			return true
		}

		// Group-4 remains correctness-sensitive. Keep the whole family
		// interpreted for now, including RTS/JSR/JMP control flow, and only
		// re-enable specific forms once they are covered end-to-end.
		if group == 0x4 {
			return true
		}

		// Conservative indexed-EA bailout for the families currently exercised by
		// the failing suite backlog. This includes both brief and full-format 68020
		// indexed forms, plus PC-indexed.
		switch group {
		case 0x2, 0x3: // MOVE.L / MOVE.W / MOVEA
			srcMode := (opcode >> 3) & 7
			srcReg := opcode & 7
			dstMode := (opcode >> 6) & 7
			dstReg := (opcode >> 9) & 7
			if m68kModeIsIndexed(srcMode, srcReg) || m68kModeIsIndexed(dstMode, dstReg) ||
				m68kModeTouchesSP(srcMode, srcReg) || m68kModeTouchesSP(dstMode, dstReg) {
				return true
			}

		case 0x0, 0x1, 0x5, 0x9, 0xB, 0xC, 0xD:
			eaMode := (opcode >> 3) & 7
			eaReg := opcode & 7
			if m68kModeIsIndexed(eaMode, eaReg) || m68kModeTouchesSP(eaMode, eaReg) {
				return true
			}

		case 0x4:
			eaMode := (opcode >> 3) & 7
			eaReg := opcode & 7
			if m68kModeIsIndexed(eaMode, eaReg) {
				return true
			}
		}

		// Direct stack-register destination updates are still safer in the
		// interpreter until the nested-frame tests are green.
		if group == 0x5 {
			eaMode := (opcode >> 3) & 7
			eaReg := opcode & 7
			if eaMode == 1 && eaReg == 7 { // ADDQ/SUBQ An, specifically A7
				return true
			}
		}
		if group == 0x9 || group == 0xD {
			opmode := (opcode >> 6) & 7
			dstReg := (opcode >> 9) & 7
			if (opmode == 3 || opmode == 7) && dstReg == 7 { // SUBA/ADDA ...,A7
				return true
			}
		}

		_ = memory
		_ = startPC
	}

	return false
}

// m68kScanBlock decodes M68K instructions starting at startPC until a block
// terminator is found or the max block size is reached. The terminating
// instruction IS included in the block (it needs to be compiled).
func m68kScanBlock(memory []byte, startPC uint32) []M68KJITInstr {
	instrs := make([]M68KJITInstr, 0, 32)
	memSize := uint32(len(memory))
	pc := startPC

	for len(instrs) < m68kJitMaxBlockSize {
		if pc+2 > memSize {
			break
		}

		opcode := uint16(memory[pc])<<8 | uint16(memory[pc+1])
		length := m68kInstrLength(memory, pc)

		ji := M68KJITInstr{
			opcode:   opcode,
			pcOffset: pc - startPC,
			length:   uint16(length),
			group:    uint8(opcode >> 12),
		}
		instrs = append(instrs, ji)

		if m68kIsBlockTerminator(opcode) {
			break
		}

		pc += uint32(length)
	}

	return instrs
}

// ===========================================================================
// Register Liveness Analysis
// ===========================================================================

// m68kBlockRegs holds register usage bitmasks for a JIT block.
// Bit i corresponds to data register Di or address register Ai.
type m68kBlockRegs struct {
	dataRead          uint8 // D0-D7 read bitmask
	dataWritten       uint8 // D0-D7 write bitmask
	addrRead          uint8 // A0-A7 read bitmask
	addrWritten       uint8 // A0-A7 write bitmask
	readsCCR          bool  // true if any instruction reads CCR (Bcc, Scc, DBcc, ADDX, etc.)
	writesCCR         bool  // true if any instruction writes CCR
	hasBackwardBranch bool
}

// m68kAnalyzeBlockRegs scans a block's instructions and returns bitmasks of
// which M68K registers are read and written.
func m68kAnalyzeBlockRegs(instrs []M68KJITInstr) m68kBlockRegs {
	var br m68kBlockRegs

	for _, ji := range instrs {
		group := ji.opcode >> 12
		srcMode := (ji.opcode >> 3) & 7
		srcReg := ji.opcode & 7
		dstMode := (ji.opcode >> 6) & 7
		dstReg := (ji.opcode >> 9) & 7

		switch group {
		case 0x1, 0x2, 0x3: // MOVE
			m68kMarkEARead(&br, srcMode, srcReg)
			m68kMarkEAWritten(&br, dstMode, dstReg)
			br.writesCCR = true

		case 0x7: // MOVEQ
			br.dataWritten |= 1 << ((ji.opcode >> 9) & 7)
			br.writesCCR = true

		case 0x6: // Bcc/BRA/BSR
			cond := (ji.opcode >> 8) & 0xF
			if cond >= 2 {
				br.readsCCR = true // Bcc reads flags
			}
			if cond == 1 { // BSR writes A7
				br.addrRead |= 1 << 7
				br.addrWritten |= 1 << 7
			}

		case 0x5: // ADDQ/SUBQ/Scc/DBcc
			if ji.opcode&0xF0F8 == 0x50C8 { // DBcc
				reg := ji.opcode & 7
				br.dataRead |= 1 << reg
				br.dataWritten |= 1 << reg
				br.readsCCR = true
			} else if ji.opcode&0x00C0 == 0x00C0 { // Scc
				br.readsCCR = true
				m68kMarkEAWritten(&br, srcMode, srcReg)
			} else { // ADDQ/SUBQ
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
				br.writesCCR = true
			}

		case 0x9, 0xD: // SUB/ADD
			m68kMarkEARead(&br, srcMode, srcReg)
			reg := (ji.opcode >> 9) & 7
			opmode := (ji.opcode >> 6) & 7
			if opmode == 3 || opmode == 7 { // SUBA/ADDA
				br.addrRead |= 1 << reg
				br.addrWritten |= 1 << reg
			} else if opmode >= 4 { // Dn → EA
				br.dataRead |= 1 << reg
				m68kMarkEAWritten(&br, srcMode, srcReg)
			} else { // EA → Dn
				br.dataRead |= 1 << reg
				br.dataWritten |= 1 << reg
			}
			br.writesCCR = true

		case 0x8, 0xC: // OR/AND, DIV/MUL
			m68kMarkEARead(&br, srcMode, srcReg)
			reg := (ji.opcode >> 9) & 7
			br.dataRead |= 1 << reg
			br.dataWritten |= 1 << reg
			br.writesCCR = true

		case 0xB: // CMP/EOR/CMPM
			opmode := (ji.opcode >> 6) & 7
			reg := (ji.opcode >> 9) & 7
			br.dataRead |= 1 << reg
			if opmode >= 4 && opmode <= 6 { // EOR: Dn → EA (writes to EA)
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
			} else { // CMP: EA → Dn (only reads)
				m68kMarkEARead(&br, srcMode, srcReg)
			}
			br.writesCCR = true

		case 0x4: // Misc
			m68kAnalyzeGroup4Regs(&br, ji.opcode)

		case 0xE: // Shifts
			if ji.opcode&0xF8C0 == 0xE8C0 { // Bit field
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
			} else if (ji.opcode>>6)&3 == 3 { // Memory shift
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
			} else { // Register shift
				reg := ji.opcode & 7
				br.dataRead |= 1 << reg
				br.dataWritten |= 1 << reg
				if (ji.opcode>>5)&1 == 1 { // Count in register
					countReg := (ji.opcode >> 9) & 7
					br.dataRead |= 1 << countReg
				}
			}
			br.writesCCR = true
		}
	}

	return br
}

func m68kAnalyzeGroup4Regs(br *m68kBlockRegs, opcode uint16) {
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7

	switch {
	case opcode&0xFFC0 == 0x4E80: // JSR
		br.addrRead |= 1 << 7
		br.addrWritten |= 1 << 7
		m68kMarkEARead(br, eaMode, eaReg)
	case opcode&0xFFC0 == 0x4EC0: // JMP
		m68kMarkEARead(br, eaMode, eaReg)
	case opcode == 0x4E75: // RTS
		br.addrRead |= 1 << 7
		br.addrWritten |= 1 << 7
	case opcode&0xF1C0 == 0x41C0 && eaMode >= 2: // LEA (mode must be >= 2; mode 0 is EXT/EXTB/SWAP)
		m68kMarkEARead(br, eaMode, eaReg)
		reg := (opcode >> 9) & 7
		br.addrWritten |= 1 << reg
	case opcode&0xFFC0 == 0x4840 && eaMode >= 2: // PEA
		m68kMarkEARead(br, eaMode, eaReg)
		br.addrRead |= 1 << 7
		br.addrWritten |= 1 << 7
	case opcode&0xFFF8 == 0x4E50: // LINK
		reg := opcode & 7
		br.addrRead |= (1 << reg) | (1 << 7)
		br.addrWritten |= (1 << reg) | (1 << 7)
	case opcode&0xFFF8 == 0x4E58: // UNLK
		reg := opcode & 7
		br.addrRead |= 1 << reg
		br.addrWritten |= (1 << reg) | (1 << 7)
	case opcode&0xFB80 == 0x4880: // MOVEM
		// MOVEM reads/writes multiple registers — conservatively mark all
		br.dataRead |= 0xFF
		br.dataWritten |= 0xFF
		br.addrRead |= 0xFF
		br.addrWritten |= 0xFF
	default:
		// CLR/NEG/NOT/TST/SWAP/EXT etc.
		m68kMarkEAReadWrite(br, eaMode, eaReg)
		br.writesCCR = true
	}
}

// m68kMarkEARead marks registers read by the given EA mode.
func m68kMarkEARead(br *m68kBlockRegs, mode, reg uint16) {
	switch mode {
	case 0: // Dn
		br.dataRead |= 1 << reg
	case 1: // An
		br.addrRead |= 1 << reg
	case 2, 3, 4, 5, 6: // (An), (An)+, -(An), (d16,An), (d8,An,Xn)
		br.addrRead |= 1 << reg
		if mode == 3 || mode == 4 { // post-inc/pre-dec also write
			br.addrWritten |= 1 << reg
		}
	}
}

// m68kMarkEAWritten marks registers written by the given EA mode.
func m68kMarkEAWritten(br *m68kBlockRegs, mode, reg uint16) {
	switch mode {
	case 0: // Dn
		br.dataWritten |= 1 << reg
	case 1: // An
		br.addrWritten |= 1 << reg
	case 2, 5, 6: // (An), (d16,An), (d8,An,Xn) — write to memory, read An
		br.addrRead |= 1 << reg
	case 3: // (An)+ — writes to memory, reads and writes An
		br.addrRead |= 1 << reg
		br.addrWritten |= 1 << reg
	case 4: // -(An) — writes to memory, reads and writes An
		br.addrRead |= 1 << reg
		br.addrWritten |= 1 << reg
	}
}

// m68kMarkEAReadWrite marks registers both read and written by the given EA mode.
func m68kMarkEAReadWrite(br *m68kBlockRegs, mode, reg uint16) {
	m68kMarkEARead(br, mode, reg)
	m68kMarkEAWritten(br, mode, reg)
}

// ===========================================================================
// Backward Branch Detection
// ===========================================================================

// m68kDetectBackwardBranches returns true if any Bcc or DBcc in the block
// targets an earlier instruction within the same block. Used to enable
// native backward branches with budget-based timer safety.
func m68kDetectBackwardBranches(instrs []M68KJITInstr, startPC uint32) bool {
	return m68kDetectBackwardBranchesWithMem(instrs, startPC, nil)
}

func m68kDetectBackwardBranchesWithMem(instrs []M68KJITInstr, startPC uint32, memory []byte) bool {
	for _, ji := range instrs {
		group := ji.opcode >> 12

		var targetPC uint32
		var isBranch bool

		if group == 0x6 {
			cond := (ji.opcode >> 8) & 0xF
			if cond >= 2 { // Bcc (not BRA/BSR)
				instrPC := startPC + ji.pcOffset
				disp := ji.opcode & 0xFF
				switch disp {
				case 0x00: // word displacement
					if memory != nil && instrPC+4 <= uint32(len(memory)) {
						w := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
						targetPC = uint32(int64(instrPC) + 2 + int64(w))
						isBranch = true
					}
				case 0xFF: // long displacement
					if memory != nil && instrPC+6 <= uint32(len(memory)) {
						l := int32(uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
							uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5]))
						targetPC = uint32(int64(instrPC) + 2 + int64(l))
						isBranch = true
					}
				default: // byte displacement
					targetPC = uint32(int64(startPC+ji.pcOffset) + 2 + int64(int8(disp)))
					isBranch = true
				}
			}
		} else if ji.opcode&0xF0F8 == 0x50C8 { // DBcc
			instrPC := startPC + ji.pcOffset
			if memory != nil && instrPC+4 <= uint32(len(memory)) {
				w := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
				targetPC = uint32(int64(instrPC) + 2 + int64(w))
				isBranch = true
			}
		}

		if isBranch && targetPC >= startPC && targetPC < startPC+ji.pcOffset {
			return true
		}
	}
	return false
}

// Suppress unused import warnings
var _ = bits.Len
var _ = unsafe.Pointer(nil)

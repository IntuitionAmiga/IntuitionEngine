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
	DataRegsPtr         uintptr // 0:  &cpu.DataRegs[0]
	AddrRegsPtr         uintptr // 8:  &cpu.AddrRegs[0]
	MemPtr              uintptr // 16: &cpu.memory[0]
	MemSize             uint32  // 24: len(cpu.memory)
	IOThreshold         uint32  // 28: 0xA0000 (fast-path boundary)
	SRPtr               uintptr // 32: &cpu.SR
	CpuPtr              uintptr // 40: &cpu
	NeedInval           uint32  // 48: self-modification flag
	NeedIOFallback      uint32  // 52: I/O bail flag
	RetPC               uint32  // 56: next PC after block execution
	RetCount            uint32  // 60: instructions retired in block
	CodePageBitmapPtr   uintptr // 64: pointer to heap-allocated code page bitmap
	ChainBudget         uint32  // 72: blocks remaining before returning to Go (init=64)
	ChainCount          uint32  // 76: accumulated instruction count during chaining
	RTSCache0PC         uint32  // 80: MRU entry 0 - M68K PC
	_pad0               uint32  // 84: padding for alignment
	RTSCache0Addr       uintptr // 88: MRU entry 0 - chain entry address
	RTSCache1PC         uint32  // 96: MRU entry 1 - M68K PC
	_pad1               uint32  // 100: padding for alignment
	RTSCache1Addr       uintptr // 104: MRU entry 1 - chain entry address
	RTSCache2PC         uint32  // 112: MRU entry 2 - M68K PC
	_pad2               uint32  // 116: padding for alignment
	RTSCache2Addr       uintptr // 120: MRU entry 2 - chain entry address
	RTSCache3PC         uint32  // 128: MRU entry 3 - M68K PC
	_pad3               uint32  // 132: padding for alignment
	RTSCache3Addr       uintptr // 136: MRU entry 3 - chain entry address
	RTSCache4PC         uint32  // 144: MRU entry 4 - M68K PC
	_pad4               uint32  // 148: padding for alignment
	RTSCache4Addr       uintptr // 152: MRU entry 4 - chain entry address
	RTSCache5PC         uint32  // 160: MRU entry 5 - M68K PC
	_pad5               uint32  // 164: padding for alignment
	RTSCache5Addr       uintptr // 168: MRU entry 5 - chain entry address
	RTSCache6PC         uint32  // 176: MRU entry 6 - M68K PC
	_pad6               uint32  // 180: padding for alignment
	RTSCache6Addr       uintptr // 184: MRU entry 6 - chain entry address
	RTSCache7PC         uint32  // 192: MRU entry 7 - M68K PC
	_pad7               uint32  // 196: padding for alignment
	RTSCache7Addr       uintptr // 200: MRU entry 7 - chain entry address
	IOPageBitmapPtr     uintptr // 208: pointer to MachineBus ioPageBitmap
	IOPageBitmapLen     uint32  // 216: len(ioPageBitmap)
	_pad8               uint32  // 220: padding
	USPPtr              uintptr // 224: &cpu.USP
	SSPPtr              uintptr // 232: &cpu.SSP
	VBRPtr              uintptr // 240: &cpu.VBR
	SFCPtr              uintptr // 248: &cpu.SFC
	DFCPtr              uintptr // 256: &cpu.DFC
	CACRPtr             uintptr // 264: &cpu.CACR
	CAARPtr             uintptr // 272: &cpu.CAAR
	MSPPtr              uintptr // 280: &cpu.MSP
	ISPPtr              uintptr // 288: &cpu.ISP
	Use68000Frame       uint32  // 296: cpu.use68000ExceptionFrame snapshot
	_pad9               uint32  // 300: padding
	NativeException     uint32  // 304: exception vector requested by native code
	NativeExceptionPC   uint32  // 308: faulting instruction PC for native exception
	NativeExceptionIR   uint32  // 312: faulting opcode for diagnostics
	_pad10              uint32  // 316: padding
	NeedHelper          uint32  // 320: M68K helper opcode requested by native code
	HelperPC            uint32  // 324: PC of helper instruction
	CodePageMinPtr      uintptr // 328: per-page minimum compiled byte offset
	CodePageMaxPtr      uintptr // 336: per-page exclusive maximum compiled byte offset
	CodePageBoundsLen   uint32  // 344: len(code page bounds)
	_pad11              uint32  // 348: padding
	InvalAddr           uint32  // 352: guest address that triggered native SMC invalidation
	InvalSize           uint32  // 356: byte count for native SMC invalidation
	InvalGenPtr         uintptr // 360: &cpu.m68kJitInvalGen storage
	InvalGenSnapshot    uint64  // 368: dispatcher snapshot before native entry
	InExceptionPtr      uintptr // 376: &cpu.inException storage
	RTECountPtr         uintptr // 384: &cpu.rteCount storage
	PendingExceptionPtr uintptr // 392: &cpu.pendingException storage
	PendingInterruptPtr uintptr // 400: &cpu.pendingInterrupt storage
	FPRegsPtr           uintptr // 408: &cpu.FPU.fp[0] (float64 FP register file)
	FPSRPtr             uintptr // 416: &cpu.FPU.FPSR
	FPCRPtr             uintptr // 424: &cpu.FPU.FPCR
	FPIARPtr            uintptr // 432: &cpu.FPU.FPIAR
}

// M68KJITContext field offsets (must match struct layout above)
const (
	m68kCtxOffDataRegsPtr         = 0
	m68kCtxOffAddrRegsPtr         = 8
	m68kCtxOffMemPtr              = 16
	m68kCtxOffMemSize             = 24
	m68kCtxOffIOThreshold         = 28
	m68kCtxOffSRPtr               = 32
	m68kCtxOffCpuPtr              = 40
	m68kCtxOffNeedInval           = 48
	m68kCtxOffNeedIOFallback      = 52
	m68kCtxOffRetPC               = 56
	m68kCtxOffRetCount            = 60
	m68kCtxOffCodePageBitmapPtr   = 64
	m68kCtxOffChainBudget         = 72
	m68kCtxOffChainCount          = 76
	m68kCtxOffRTSCache0PC         = 80
	m68kCtxOffRTSCache0Addr       = 88 // MRU entry 0
	m68kCtxOffRTSCache1PC         = 96
	m68kCtxOffRTSCache1Addr       = 104 // MRU entry 1
	m68kCtxOffRTSCache2PC         = 112
	m68kCtxOffRTSCache2Addr       = 120 // MRU entry 2
	m68kCtxOffRTSCache3PC         = 128
	m68kCtxOffRTSCache3Addr       = 136 // MRU entry 3
	m68kCtxOffRTSCache4PC         = 144
	m68kCtxOffRTSCache4Addr       = 152 // MRU entry 4
	m68kCtxOffRTSCache5PC         = 160
	m68kCtxOffRTSCache5Addr       = 168 // MRU entry 5
	m68kCtxOffRTSCache6PC         = 176
	m68kCtxOffRTSCache6Addr       = 184 // MRU entry 6
	m68kCtxOffRTSCache7PC         = 192
	m68kCtxOffRTSCache7Addr       = 200 // MRU entry 7
	m68kCtxOffIOPageBitmapPtr     = 208
	m68kCtxOffIOPageBitmapLen     = 216
	m68kCtxOffUSPPtr              = 224
	m68kCtxOffSSPPtr              = 232
	m68kCtxOffVBRPtr              = 240
	m68kCtxOffSFCPtr              = 248
	m68kCtxOffDFCPtr              = 256
	m68kCtxOffCACRPtr             = 264
	m68kCtxOffCAARPtr             = 272
	m68kCtxOffMSPPtr              = 280
	m68kCtxOffISPPtr              = 288
	m68kCtxOffUse68000Frame       = 296
	m68kCtxOffNativeException     = 304
	m68kCtxOffNativeExceptionPC   = 308
	m68kCtxOffNativeExceptionIR   = 312
	m68kCtxOffNeedHelper          = 320
	m68kCtxOffHelperPC            = 324
	m68kCtxOffCodePageMinPtr      = 328
	m68kCtxOffCodePageMaxPtr      = 336
	m68kCtxOffCodePageBoundsLen   = 344
	m68kCtxOffInvalAddr           = 352
	m68kCtxOffInvalSize           = 356
	m68kCtxOffInvalGenPtr         = 360
	m68kCtxOffInvalGenSnapshot    = 368
	m68kCtxOffInExceptionPtr      = 376
	m68kCtxOffRTECountPtr         = 384
	m68kCtxOffPendingExceptionPtr = 392
	m68kCtxOffPendingInterruptPtr = 400
	m68kCtxOffFPRegsPtr           = 408
	m68kCtxOffFPSRPtr             = 416
	m68kCtxOffFPCRPtr             = 424
	m68kCtxOffFPIARPtr            = 432
)

const (
	m68kJITHelperNone     uint32 = 0
	m68kJITHelperFPU      uint32 = 1
	m68kJITHelperMMIOMOVE uint32 = 2
	m68kJITHelperMMIOCLR  uint32 = 3
	m68kJITHelperCHK2CMP2 uint32 = 4
	m68kJITHelperCASCAS2  uint32 = 5
	m68kJITHelperMOVES    uint32 = 6
	m68kJITHelperTRAPcc   uint32 = 7
	m68kJITHelperBKPT     uint32 = 8
	m68kJITHelperCALLM    uint32 = 9
	m68kJITHelperRTM      uint32 = 10
)

// m68kStepSize returns the address increment for post-increment/pre-decrement.
func m68kStepSize(size int, reg uint16) uint32 {
	switch size {
	case M68K_SIZE_BYTE:
		if reg == 7 {
			return 2 // A7 always steps by 2 for alignment.
		}
		return 1
	case M68K_SIZE_WORD:
		return 2
	case M68K_SIZE_LONG:
		return 4
	}
	return 1
}

func m68kEAMayUseMemHelper(mode, reg uint16, isWrite bool) bool {
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		if isWrite {
			return reg <= 1 // abs.W / abs.L
		}
		return reg <= 3 // abs.W / abs.L / (d16,PC) / (d8,PC,Xn)
	}
	return false
}

// m68kJitAvailable is set to true at init time on platforms that support JIT.
var m68kJitAvailable bool

func newM68KJITContext(cpu *M68KCPU, codePageBitmap []byte, codePageMin []uint16, codePageMax []uint16) *M68KJITContext {
	ctx := &M68KJITContext{
		DataRegsPtr:         uintptr(unsafe.Pointer(&cpu.DataRegs[0])),
		AddrRegsPtr:         uintptr(unsafe.Pointer(&cpu.AddrRegs[0])),
		MemPtr:              uintptr(unsafe.Pointer(&cpu.memory[0])),
		MemSize:             uint32(len(cpu.memory)),
		IOThreshold:         0xA0000,
		SRPtr:               uintptr(unsafe.Pointer(&cpu.SR)),
		CpuPtr:              uintptr(unsafe.Pointer(cpu)),
		USPPtr:              uintptr(unsafe.Pointer(&cpu.USP)),
		SSPPtr:              uintptr(unsafe.Pointer(&cpu.SSP)),
		VBRPtr:              uintptr(unsafe.Pointer(&cpu.VBR)),
		SFCPtr:              uintptr(unsafe.Pointer(&cpu.SFC)),
		DFCPtr:              uintptr(unsafe.Pointer(&cpu.DFC)),
		CACRPtr:             uintptr(unsafe.Pointer(&cpu.CACR)),
		CAARPtr:             uintptr(unsafe.Pointer(&cpu.CAAR)),
		MSPPtr:              uintptr(unsafe.Pointer(&cpu.MSP)),
		ISPPtr:              uintptr(unsafe.Pointer(&cpu.ISP)),
		InvalGenPtr:         uintptr(unsafe.Pointer(&cpu.m68kJitInvalGen)),
		InExceptionPtr:      uintptr(unsafe.Pointer(&cpu.inException)),
		RTECountPtr:         uintptr(unsafe.Pointer(&cpu.rteCount)),
		PendingExceptionPtr: uintptr(unsafe.Pointer(&cpu.pendingException)),
		PendingInterruptPtr: uintptr(unsafe.Pointer(&cpu.pendingInterrupt)),
	}
	// The FPU is optional (cpu.FPU may be nil). Only wire the FP register/status
	// pointers when present; native FPU emission is gated on the same nil check,
	// and a nil-FPU CPU raises Line-F for FPU opcodes instead.
	if cpu.FPU != nil {
		ctx.FPRegsPtr = uintptr(unsafe.Pointer(&cpu.FPU.fp[0]))
		ctx.FPSRPtr = uintptr(unsafe.Pointer(&cpu.FPU.FPSR))
		ctx.FPCRPtr = uintptr(unsafe.Pointer(&cpu.FPU.FPCR))
		ctx.FPIARPtr = uintptr(unsafe.Pointer(&cpu.FPU.FPIAR))
	}
	if cpu.use68000ExceptionFrame {
		ctx.Use68000Frame = 1
	}
	if len(codePageBitmap) > 0 {
		ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&codePageBitmap[0]))
	}
	if len(codePageMin) > 0 && len(codePageMax) == len(codePageMin) {
		ctx.CodePageMinPtr = uintptr(unsafe.Pointer(&codePageMin[0]))
		ctx.CodePageMaxPtr = uintptr(unsafe.Pointer(&codePageMax[0]))
		ctx.CodePageBoundsLen = uint32(len(codePageMin))
	}
	if len(cpu.m68kJitIOPageBitmap) > 0 {
		ctx.IOPageBitmapPtr = uintptr(unsafe.Pointer(&cpu.m68kJitIOPageBitmap[0]))
		ctx.IOPageBitmapLen = uint32(len(cpu.m68kJitIOPageBitmap))
	}
	return ctx
}

// ===========================================================================
// M68KJITInstr — Pre-decoded M68K instruction for JIT compilation
// ===========================================================================

// M68KJITInstr represents one M68K instruction scanned from a basic block.
// Fields are pre-decoded during scanning to avoid re-parsing during emission.
type M68KJITInstr struct {
	opcode    uint16 // first word of instruction
	pcOffset  uint32 // byte offset from block start
	length    uint16 // total instruction length in bytes (2-10+)
	group     uint8  // opcode >> 12
	fusedFlag uint8  // see m68kFused* constants
}

// Fusion flags for M68KJITInstr.fusedFlag.
const (
	m68kFusedJSRLeafCall   uint8 = 1 << 0 // JSR replaced by inlined leaf body — emit nothing
	m68kFusedRTSLeafReturn uint8 = 1 << 1 // synthetic RTS marker for fused leaf — emit nothing
)

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
		return m68kGroupFLength(opcode, memory, pc)
	}
	return 2
}

// m68kGroup0Length handles Group 0 (0x0xxx) instruction lengths.
func m68kGroup0Length(opcode uint16, memory []byte, pc uint32) int {
	// MOVEP: opcode & 0xF138 == 0x0108
	if opcode&0xF138 == 0x0108 {
		return 4 // opcode + displacement word
	}

	// RTM: opcode only.
	if opcode&0xFFF0 == 0x06C0 {
		return 2
	}

	// CALLM: opcode + argument count word + descriptor EA extension.
	if opcode&0xFFC0 == 0x06C0 && ((opcode>>3)&7) >= M68K_AM_AR_IND {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+4)
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

	// CHK2/CMP2: opcode & 0xF9C0 == 0x00C0
	if opcode&0xF9C0 == 0x00C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 4 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_WORD, memory, pc+4)
	}

	// MOVES: opcode & 0xFF00 == 0x0E00
	if opcode&0xFF00 == 0x0E00 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		size := int((opcode >> 6) & 3)
		return 4 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+4)
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
	if bitOp := opcode & 0xF1C0; bitOp == 0x0100 || bitOp == 0x0140 || bitOp == 0x0180 || bitOp == 0x01C0 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		return 2 + m68kEAExtBytes(eaMode, eaReg, M68K_SIZE_BYTE, memory, pc+2)
	}

	if opcode == 0x023C || opcode == 0x027C ||
		opcode == 0x003C || opcode == 0x007C ||
		opcode == 0x0A3C || opcode == 0x0A7C {
		return 4
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

	// CHK.W / CHK.L. CHK.L is 68020+ and its immediate source is long-sized.
	if opcode&0xF1C0 == 0x4180 || opcode&0xF1C0 == 0x4100 {
		eaMode := (opcode >> 3) & 7
		eaReg := opcode & 7
		size := M68K_SIZE_WORD
		if opcode&0xF1C0 == 0x4100 {
			size = M68K_SIZE_LONG
		}
		return 2 + m68kEAExtBytes(eaMode, eaReg, size, memory, pc+2)
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

	// TRAPcc (68020): mode 7, reg 2/3/4. Reg 0/1 are Scc abs.W/abs.L.
	if opcode&0xF0F8 == 0x50F8 && opcode&7 >= 2 {
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

	// CMPM: base/memory-postincrement bits match and size field is valid.
	if m68kIsCMPMOpcode(opcode) {
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

func m68kFPUFormatImmediateBytes(format uint16) int {
	switch format {
	case 0, 1: // long integer, single precision
		return 4
	case 2: // extended precision
		return 12
	case 4, 6: // word integer, byte integer
		return 2
	case 5: // double precision
		return 8
	default:
		return 0
	}
}

func m68kFPUFormatEAExtBytes(mode, reg, format uint16, memory []byte, pc uint32) int {
	if mode == 7 && reg == 4 {
		return m68kFPUFormatImmediateBytes(format)
	}
	return m68kEAExtBytes(mode, reg, M68K_SIZE_LONG, memory, pc)
}

func m68kGroupFLength(opcode uint16, memory []byte, pc uint32) int {
	typeField := (opcode >> 6) & 0x7
	mode := (opcode >> 3) & 0x7
	reg := opcode & 0x7

	switch typeField {
	case 0: // General FPU operations: opcode + command word + optional EA/immediate
		if pc+4 > uint32(len(memory)) {
			return 4
		}
		cmdWord := uint16(memory[pc+2])<<8 | uint16(memory[pc+3])
		if (cmdWord & 0x8000) == 0 {
			rm := (cmdWord >> 14) & 1
			if rm == 0 {
				return 4 // FP register to FP register
			}
			dir := (cmdWord >> 13) & 1
			if dir == 0 {
				if (cmdWord & 0xFC00) == 0x5C00 {
					return 4 // FMOVECR
				}
				format := (cmdWord >> 10) & 0x7
				return 4 + m68kFPUFormatEAExtBytes(mode, reg, format, memory, pc+4)
			}
			format := (cmdWord >> 10) & 0x7
			return 4 + m68kFPUFormatEAExtBytes(mode, reg, format, memory, pc+4)
		}

		// Control-register and FMOVEM forms keep their register mask in the
		// command word; only the primary EA can add extension words.
		return 4 + m68kEAExtBytes(mode, reg, M68K_SIZE_LONG, memory, pc+4)

	case 1: // FScc / FDBcc / FTRAPcc: opcode + condition word + optional operand/EA
		switch {
		case mode == 1: // FDBcc Dn,<disp16>
			return 6
		case mode == 7 && reg == 2: // FTRAPcc.W
			return 6
		case mode == 7 && reg == 3: // FTRAPcc.L
			return 8
		case mode == 7 && reg == 4: // FTRAPcc
			return 4
		default: // FScc <ea>
			return 4 + m68kEAExtBytes(mode, reg, M68K_SIZE_BYTE, memory, pc+4)
		}

	case 2: // FBcc.W
		return 4
	case 3: // FBcc.L
		return 6
	case 4: // FSAVE <ea>
		return 2 + m68kEAExtBytes(mode, reg, M68K_SIZE_LONG, memory, pc+2)
	case 5: // FRESTORE <ea>
		return 2 + m68kEAExtBytes(mode, reg, M68K_SIZE_LONG, memory, pc+2)
	default:
		return 2
	}
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

	case 0xF:
		return m68kGroupFTerminatesBlock(opcode)
	}

	return false
}

func m68kGroupFTerminatesBlock(opcode uint16) bool {
	typeField := (opcode >> 6) & 0x7
	return typeField == 6 || typeField == 7
}

func m68kIsJITHelperSupportedFPU(opcode uint16) bool {
	if opcode>>12 != 0xF {
		return false
	}
	return !m68kGroupFTerminatesBlock(opcode)
}

// m68kFPUNativeOp identifies a register-to-register 68881 arithmetic operation
// the JIT can emit natively in SSE2 scalar double. The FP register file is
// stored as float64 (fpu_m68881.go), and Go's float64 arithmetic lowers to the
// same SSE2 scalar-double instructions, so native emission is bit-identical to
// the interpreter — provided FMA fusion is never used (it would change rounding).
// Transcendentals, EA-operand forms, control/FMOVEM and non-general Line-F
// instructions are excluded and stay on the FPU helper path.
type m68kFPUNativeOp uint8

const (
	m68kFPUNativeNone m68kFPUNativeOp = iota
	m68kFPUNativeFMOVE
	m68kFPUNativeFADD
	m68kFPUNativeFSUB
	m68kFPUNativeFMUL
	m68kFPUNativeFDIV
	m68kFPUNativeFABS
	m68kFPUNativeFNEG
	m68kFPUNativeFSQRT
	m68kFPUNativeFCMP    // compare, no result store; custom CC
	m68kFPUNativeFTST    // test source, no result store
	m68kFPUNativeFSGLDIV // single-precision divide (operands rounded to float32 first)
	m68kFPUNativeFSGLMUL // single-precision multiply
)

// m68kFPUConditionBits computes the FPSR condition-code bits (N/Z/I/NAN) for a
// float64 result, exactly mirroring (*M68881FPU).setCC64. It is the reference
// the native FPU path must replicate after each arithmetic op; keeping it as a
// pure function lets the emitter be validated against the interpreter without
// executing code. Order matters: a zero (incl. -0.0) is Zero, never Negative.
func m68kFPUConditionBits(bits uint64) uint32 {
	exp := bits & 0x7FF0000000000000
	frac := bits & 0x000FFFFFFFFFFFFF
	sign := bits >> 63
	if exp == 0x7FF0000000000000 {
		if frac != 0 {
			return FPU_CC_NAN
		}
		cc := FPU_CC_I
		if sign != 0 {
			cc |= FPU_CC_N
		}
		return cc
	}
	if exp|frac == 0 {
		return FPU_CC_Z
	}
	if sign != 0 {
		return FPU_CC_N
	}
	return 0
}

// m68kDecodeNativeFPURegToReg decodes a Line-F general FPU instruction (opcode +
// command word) and reports whether it is a register-to-register op the JIT can
// emit natively, along with the source/destination FP registers and the result
// rounding precision. It mirrors execFPUGeneral → execFPURegToReg and
// m68kFPUDecodePrecisionOpmode exactly so the native path and the interpreter
// agree on operands and precision; any unsupported encoding returns ok=false so
// the caller falls back to the FPU helper.
func m68kDecodeNativeFPURegToReg(opcode, cmdWord uint16) (op m68kFPUNativeOp, src, dst, precision int, ok bool) {
	if (opcode>>6)&0x7 != 0 {
		return // not a general FPU instruction (FDBcc/FScc/FBcc/FSAVE/FRESTORE)
	}
	if cmdWord&0x8000 != 0 {
		return // control register / FMOVEM
	}
	if (cmdWord>>14)&1 != 0 {
		return // R/M=1: EA source, not register-to-register
	}
	src = int((cmdWord >> 10) & 0x7)
	dst = int((cmdWord >> 7) & 0x7)
	baseOp, prec := m68kFPUDecodePrecisionOpmode(cmdWord & 0x7F)
	precision = prec
	switch baseOp {
	case FPU_OP_FMOVE:
		op = m68kFPUNativeFMOVE
	case FPU_OP_FADD:
		op = m68kFPUNativeFADD
	case FPU_OP_FSUB:
		op = m68kFPUNativeFSUB
	case FPU_OP_FMUL:
		op = m68kFPUNativeFMUL
	case FPU_OP_FDIV:
		op = m68kFPUNativeFDIV
	case FPU_OP_FABS:
		op = m68kFPUNativeFABS
	case FPU_OP_FNEG:
		op = m68kFPUNativeFNEG
	case FPU_OP_FSQRT:
		op = m68kFPUNativeFSQRT
	case FPU_OP_FCMP:
		// FCMP/FTST write no result, but the interpreter still applies
		// applyFPUResultPrecision(dst, precision) for the single/double opmodes,
		// which rounds fp[dst] and refreshes the CC. The native handlers don't
		// model that, so only the plain (extended) form is native; precision-
		// qualified forms fall back to the helper.
		if precision != m68kFPURoundExtended {
			return
		}
		op = m68kFPUNativeFCMP
	case FPU_OP_FTST:
		if precision != m68kFPURoundExtended {
			return
		}
		op = m68kFPUNativeFTST
	case FPU_OP_FSGLDIV:
		op = m68kFPUNativeFSGLDIV
	case FPU_OP_FSGLMUL:
		op = m68kFPUNativeFSGLMUL
	default:
		return // transcendental / unsupported reg-to-reg op → helper
	}
	ok = true
	return
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
	case 0xF: // Line F trap / FPU
		return !m68kIsJITHelperSupportedFPU(opcode)
	}

	// System instructions that need interpreter
	switch {
	case opcode == 0x4E72: // STOP
		return true
	case opcode == 0x4E73: // RTE
		// Native RTE owns supervisor 68020 format-0 frames and self-bails
		// for unsupported frame shapes.
	case opcode == 0x4E77: // RTR
		return true
	case opcode == 0x4E70: // RESET
		return true
	case opcode&0xFFF0 == 0x4E40: // TRAP #n
		// Native TRAP requests the same exception path the dispatcher uses for
		// native privilege faults.
	case opcode == 0x4E76: // TRAPV
		return true
	case opcode&0xFFFE == 0x4E7A: // MOVEC
		// Supervisor-mode valid-register cases emit natively; user-mode or
		// invalid-register cases self-bail in the emitter for interpreter
		// exception handling.
	case opcode&0xFF80 == 0x4C00: // MULL/DIVL (68020)
		return !m68kIsNativeSupportedMULLDIVL(opcode)
	case opcode&0xFFF0 == 0x4E60: // MOVE USP
		// Audited privileged register transfer; user-mode cases self-bail.
	case opcode&0xFFC0 == 0x46C0: // MOVE to SR
		// Audited source forms are handled by the production-native gate.
	case opcode&0xFF00 == 0x4600: // NOT
		if !m68kIsNativeSupportedNOT(opcode) {
			return true
		}
	}

	// Frame-return instructions need the interpreter unless they have a fully
	// audited native path. RTE owns 68020 format-0 frames natively; RTR still
	// needs interpreter frame handling.
	for _, ji := range instrs {
		switch ji.opcode {
		case 0x4E77: // RTR
			return true
		}
	}

	return false
}

func m68kModeIsIndexed(mode, reg uint16) bool {
	return mode == 6 || (mode == 7 && reg == 3)
}

func m68kBriefIndexedEAAllowed(memory []byte, extPC uint32, mode, reg uint16) bool {
	if mode != 6 && !(mode == 7 && reg == 3) {
		return false
	}
	if extPC+2 > uint32(len(memory)) {
		return false
	}
	extWord := uint16(memory[extPC])<<8 | uint16(memory[extPC+1])
	return extWord&0x0100 == 0
}

func m68kIndexedEAAllowed(memory []byte, extPC uint32, mode, reg uint16) bool {
	if mode != 6 && !(mode == 7 && reg == 3) {
		return false
	}
	if extPC+2 > uint32(len(memory)) {
		return false
	}
	return extPC+uint32(m68kIndexedExtBytes(memory, extPC)) <= uint32(len(memory))
}

func m68kIsBriefIndexedLEA(memory []byte, startPC uint32, ji *M68KJITInstr) bool {
	if ji == nil || ji.opcode&0xF1C0 != 0x41C0 {
		return false
	}
	mode := (ji.opcode >> 3) & 7
	reg := ji.opcode & 7
	return m68kModeIsIndexed(mode, reg) && m68kBriefIndexedEAAllowed(memory, startPC+ji.pcOffset+2, mode, reg)
}

func m68kMoveSizeForGroup(group uint16) int {
	switch group {
	case 0x1:
		return M68K_SIZE_BYTE
	case 0x3:
		return M68K_SIZE_WORD
	default:
		return M68K_SIZE_LONG
	}
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

func m68kIsMoveLongStackDispToReg(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	return m68kIsMoveLongAnDispToReg(opcode) && (opcode&7) == 7
}

func m68kIsMoveLongStackDispToAddressIndirect(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	return srcMode == 5 && srcReg == 7 && dstMode == 2
}

func m68kIsMoveLongAnDispToReg(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	dstMode := (opcode >> 6) & 7
	return srcMode == 5 && (dstMode == 0 || dstMode == 1)
}

func m68kIsMoveLongRegToStackDisp(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	return (srcMode == 0 || srcMode == 1) && dstMode == 5 && dstReg == 7
}

func m68kIsMoveLongRegToStackPredec(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	return (srcMode == 0 || srcMode == 1) && dstMode == 4 && dstReg == 7
}

func m68kIsMoveLongStackDispToStackPredec(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	return srcMode == 5 && srcReg == 7 && dstMode == 4 && dstReg == 7
}

func m68kIsMoveLongStackPostincToStackPredec(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	return srcMode == 3 && srcReg == 7 && dstMode == 4 && dstReg == 7
}

func m68kIsMoveLongStackIndirectToStackPostinc(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	return srcMode == 2 && srcReg == 7 && dstMode == 3 && dstReg == 7
}

func m68kIsMoveLongAuditedStackMemory(opcode uint16) bool {
	if opcode>>12 != 0x2 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	if dstReg != 7 {
		return false
	}
	switch {
	case srcMode == 5 && srcReg == 7 && dstMode == 4:
		return true // MOVE.L d16(A7),-(A7)
	case srcMode == 3 && srcReg == 7 && dstMode == 4:
		return true // MOVE.L (A7)+,-(A7)
	case srcMode == 2 && srcReg == 7 && dstMode == 3:
		return true // MOVE.L (A7),(A7)+
	case srcMode == 4 && srcReg == 7 && dstMode == 4:
		return true // MOVE.L -(A7),-(A7)
	case srcMode == 3 && srcReg == 7 && (dstMode == 2 || dstMode == 3 || dstMode == 5):
		return true // MOVE.L (A7)+,(A7)/(A7)+/d16(A7)
	case srcMode == 7 && srcReg == 0 && (dstMode == 2 || dstMode == 3):
		return true // MOVE.L abs.W,(A7)/(A7)+
	case srcMode == 7 && srcReg == 2 && dstMode == 3:
		return true // MOVE.L d16(PC),(A7)+
	default:
		return false
	}
}

func m68kIsMovePostincPostinc(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x1 && group != 0x2 && group != 0x3 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	return srcMode == 3 && dstMode == 3 && srcReg != dstReg
}

func m68kIsNativeSupportedMOVEA(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x2 && group != 0x3 {
		return false
	}
	dstMode := (opcode >> 6) & 7
	if dstMode != 1 {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	switch srcMode {
	case 0, 1, 2, 3, 4, 5, 6:
		return true
	case 7:
		return srcReg <= 4
	default:
		return false
	}
}

func m68kIsNativeSupportedMOVEGuarded(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x1 && group != 0x2 && group != 0x3 {
		return false
	}
	if m68kIsNativeSupportedMOVEA(opcode) {
		return false
	}
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	dstMode := (opcode >> 6) & 7
	dstReg := (opcode >> 9) & 7
	srcRegOrImm := srcMode == 0 || (group != 0x1 && srcMode == 1) || (srcMode == 7 && srcReg == 4)
	srcMem := srcMode == 2 || srcMode == 3 || srcMode == 4 || srcMode == 5 || srcMode == 6 ||
		(srcMode == 7 && srcReg <= 3)
	dstMem := dstMode == 2 || dstMode == 3 || dstMode == 4 || dstMode == 5 || dstMode == 6 ||
		(dstMode == 7 && dstReg <= 1)
	if srcMem && dstMode == 0 {
		return true
	}
	if srcRegOrImm && dstMem {
		return true
	}
	return false
}

func m68kIsNativeSupportedMOVEMemToMemGuarded(opcode uint16) bool {
	group := opcode >> 12
	switch group {
	case 0x1, 0x2, 0x3:
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		return m68kMoveMemToMemEASupported(srcMode, srcReg, true) &&
			m68kMoveMemToMemEASupported(dstMode, dstReg, false)
	default:
		return false
	}
}

func m68kIsNativeSupportedMemShiftRotateWord(opcode uint16) bool {
	if opcode>>12 != 0xE || (opcode>>6)&3 != 3 || opcode&0xF800 != 0xE000 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kMoveMemToMemEASupported(mode, reg uint16, source bool) bool {
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		if source {
			return reg <= 3
		}
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedScc(opcode uint16) bool {
	if opcode&0xF0C0 != 0x50C0 || opcode&0xF0F8 == 0x50C8 {
		return false
	}
	mode := (opcode >> 3) & 7
	if mode == 0 {
		return true
	}
	reg := opcode & 7
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsCLRLongStackPredec(opcode uint16) bool {
	return opcode == 0x42A7
}

func m68kIsNativeSupportedDBcc(opcode uint16) bool {
	return opcode&0xF0F8 == 0x50C8
}

func m68kIsNativeSupportedBSR(opcode uint16) bool {
	return opcode>>12 == 0x6 && (opcode>>8)&0xF == 1
}

func m68kIsNativeSupportedMOVEToSR(opcode uint16) bool {
	if opcode&0xFFC0 != 0x46C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg <= 4
	default:
		return false
	}
}

func m68kIsNativeSupportedMOVEToCCR(opcode uint16) bool {
	if opcode&0xFFC0 != 0x44C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg <= 4
	default:
		return false
	}
}

func m68kIsNativeSupportedMOVEFromStatus(opcode uint16) bool {
	if opcode&0xFFC0 != 0x40C0 && opcode&0xFFC0 != 0x42C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg <= 1
	default:
		return false
	}
}

func m68kIsNativeSupportedPEA(opcode uint16) bool {
	if opcode&0xFFC0 != 0x4840 || ((opcode>>3)&7) < 2 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 5, 6:
		return true
	case 7:
		return reg <= 3
	default:
		return false
	}
}

func m68kIsNativeSupportedCHK(opcode uint16) bool {
	chkMasked := opcode & 0xF1C0
	if chkMasked != 0x4180 && chkMasked != 0x4100 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedMULW(opcode uint16) bool {
	if opcode&0xF0C0 != 0xC0C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedMULLDIVL(opcode uint16) bool {
	if opcode&0xFF80 != 0x4C00 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg <= 4
	default:
		return false
	}
}

func m68kIsNativeSupportedDIVW(opcode uint16) bool {
	if opcode&0xF0C0 != 0x80C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedCMPA(opcode uint16) bool {
	if opcode>>12 != 0xB {
		return false
	}
	opmode := (opcode >> 6) & 7
	if opmode != 3 && opmode != 7 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	if opmode == 0 && mode == 1 {
		return false
	}
	switch mode {
	case 0, 1, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedAddrArithA(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x9 && group != 0xD {
		return false
	}
	opmode := (opcode >> 6) & 7
	if opmode != 3 && opmode != 7 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 1, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedLogicEAToDn(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x8 && group != 0xC {
		return false
	}
	opmode := (opcode >> 6) & 7
	if opmode > 2 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedLogicDnToEA(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x8 && group != 0xB && group != 0xC {
		return false
	}
	opmode := (opcode >> 6) & 7
	if opmode < 4 || opmode > 6 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedArithDnToEA(opcode uint16) bool {
	group := opcode >> 12
	if group != 0x9 && group != 0xD {
		return false
	}
	opmode := (opcode >> 6) & 7
	if opmode < 4 || opmode > 6 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kEAToDnALUSourceFallbackUnsafe(opcode uint16) bool {
	srcMode := (opcode >> 3) & 7
	srcReg := opcode & 7
	if !m68kEAMayUseMemHelper(srcMode, srcReg, false) {
		return false
	}
	switch srcMode {
	case 2, 3, 4, 5, 6:
		return false
	case 7:
		return srcReg != 0 && srcReg != 1 && srcReg != 2 && srcReg != 3 && srcReg != 4
	default:
		return true
	}
}

func m68kIsBTSTImmDn(opcode uint16) bool {
	return opcode&0xFFC0 == 0x0800 && ((opcode>>3)&7) == 0
}

func m68kIsBTSTImmAnDisp(opcode uint16) bool {
	return opcode&0xFFC0 == 0x0800 && ((opcode>>3)&7) == 5
}

func m68kIsBTSTDynamic(opcode uint16) bool {
	return opcode&0xF1C0 == 0x0100
}

func m68kIsBTSTImmediate(opcode uint16) bool {
	return opcode&0xFFC0 == 0x0800
}

func m68kIsBitModifyDynamic(opcode uint16) bool {
	switch opcode & 0xF1C0 {
	case 0x0140, 0x0180, 0x01C0:
		return true
	default:
		return false
	}
}

func m68kIsBitModifyImmediate(opcode uint16) bool {
	return opcode&0xFF00 == 0x0800 && opcode&0x00C0 != 0
}

func m68kIsNativeSupportedBTST(opcode uint16) bool {
	if !m68kIsBTSTDynamic(opcode) && !m68kIsBTSTImmediate(opcode) {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3
	default:
		return false
	}
}

func m68kIsNativeSupportedBitModify(opcode uint16) bool {
	if !m68kIsBitModifyDynamic(opcode) && !m68kIsBitModifyImmediate(opcode) {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedTST(opcode uint16) bool {
	if opcode&0xFF00 != 0x4A00 || ((opcode>>6)&3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 1, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3
	default:
		return false
	}
}

func m68kIsNativeSupportedCLR(opcode uint16) bool {
	if opcode&0xFF00 != 0x4200 || ((opcode>>6)&3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedNOT(opcode uint16) bool {
	if opcode&0xFF00 != 0x4600 || ((opcode>>6)&3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedNEG(opcode uint16) bool {
	if opcode&0xFF00 != 0x4400 || ((opcode>>6)&3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedNEGX(opcode uint16) bool {
	if opcode&0xFF00 != 0x4000 || ((opcode>>6)&3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedTAS(opcode uint16) bool {
	if opcode&0xFFC0 != 0x4AC0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedBFTSTRegister(opcode uint16) bool {
	return opcode&0xFFC0 == 0xE8C0 && ((opcode>>3)&7) == 0
}

func m68kIsNativeSupportedBFTSTMemoryImmediate(opcode uint16) bool {
	if opcode&0xFFC0 != 0xE8C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 5:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedBFEXTURegister(opcode uint16) bool {
	return opcode&0xFFC0 == 0xE9C0 && ((opcode>>3)&7) == 0
}

func m68kIsNativeSupportedBFEXTSRegister(opcode uint16) bool {
	return opcode&0xFFC0 == 0xEBC0 && ((opcode>>3)&7) == 0
}

func m68kIsNativeSupportedBFFFORegister(opcode uint16) bool {
	return opcode&0xFFC0 == 0xEDC0 && ((opcode>>3)&7) == 0
}

func m68kIsNativeSupportedBFEXTUMemoryImmediate(opcode uint16) bool {
	if opcode&0xFFC0 != 0xE9C0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 5:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedBFEXTSMemoryImmediate(opcode uint16) bool {
	if opcode&0xFFC0 != 0xEBC0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 5:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedBFFFOMemoryImmediate(opcode uint16) bool {
	if opcode&0xFFC0 != 0xEDC0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 5:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedBFWriteRegisterImmediate(opcode uint16) bool {
	switch opcode & 0xFFC0 {
	case 0xEAC0, 0xECC0, 0xEEC0, 0xEFC0:
	default:
		return false
	}
	return ((opcode >> 3) & 7) == 0
}

func m68kIsNativeSupportedPACKUNPK(opcode uint16) bool {
	if opcode&0xF1F0 != 0x8140 && opcode&0xF1F0 != 0x8180 {
		return false
	}
	return ((opcode >> 3) & 1) <= 1
}

func m68kIsNativeSupportedNBCD(opcode uint16) bool {
	if opcode&0xFFC0 != 0x4800 || opcode&0xFFF8 == 0x4808 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsTSTAnDisp(opcode uint16) bool {
	return opcode&0xFF00 == 0x4A00 && ((opcode>>6)&3) != 3 && ((opcode>>3)&7) == 5
}

func m68kIsImmediateLogicDn(opcode uint16) bool {
	op := opcode & 0xFF00
	if op != 0x0000 && op != 0x0200 && op != 0x0A00 {
		return false
	}
	return ((opcode>>6)&3) != 3 && ((opcode>>3)&7) == 0
}

func m68kIsNativeSupportedImmediateLogicEA(opcode uint16) bool {
	op := opcode & 0xFF00
	if op != 0x0000 && op != 0x0200 && op != 0x0A00 {
		return false
	}
	if ((opcode >> 6) & 3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedCMPI(opcode uint16) bool {
	if opcode&0xFF00 != 0x0C00 || ((opcode>>6)&3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 3
	default:
		return false
	}
}

func m68kIsNativeSupportedCMPToDn(opcode uint16) bool {
	if opcode>>12 != 0xB {
		return false
	}
	opmode := (opcode >> 6) & 7
	if opmode > 2 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 1, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1 || reg == 2 || reg == 3 || reg == 4
	default:
		return false
	}
}

func m68kIsNativeSupportedCMPM(opcode uint16) bool {
	return m68kIsCMPMOpcode(opcode)
}

func m68kIsImmediateArithmeticDn(opcode uint16) bool {
	op := opcode & 0xFF00
	if op != 0x0400 && op != 0x0600 {
		return false
	}
	return ((opcode>>6)&3) != 3 && ((opcode>>3)&7) == 0
}

func m68kIsNativeSupportedImmediateArithmeticEA(opcode uint16) bool {
	op := opcode & 0xFF00
	if op != 0x0400 && op != 0x0600 {
		return false
	}
	if ((opcode >> 6) & 3) == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsImmediateStatusOp(opcode uint16) bool {
	return opcode == 0x023C || opcode == 0x027C ||
		opcode == 0x003C || opcode == 0x007C ||
		opcode == 0x0A3C || opcode == 0x0A7C
}

func m68kIsNativeSupportedADDQSUBQ(opcode uint16) bool {
	if opcode>>12 != 0x5 || opcode&0x00C0 == 0x00C0 {
		return false
	}
	size := (opcode >> 6) & 3
	if size == 3 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 0, 1, 2, 3, 4, 5, 6:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsNativeSupportedMOVEM(opcode uint16) bool {
	if opcode&0xFB80 != 0x4880 {
		return false
	}
	eaMode := (opcode >> 3) & 7
	eaReg := opcode & 7
	dirMemToReg := (opcode>>10)&1 == 1
	return (eaMode == 4 && !dirMemToReg) ||
		(eaMode == 3 && dirMemToReg) ||
		(eaMode == 2 && !dirMemToReg) ||
		(eaMode == 2 && dirMemToReg) ||
		(eaMode == 5 && !dirMemToReg) ||
		(eaMode == 5 && dirMemToReg) ||
		(eaMode == 6 && !dirMemToReg) ||
		(eaMode == 6 && dirMemToReg) ||
		(eaMode == 7 && (eaReg == 0 || (dirMemToReg && (eaReg == 2 || eaReg == 3))))
}

func m68kIsNativeSupportedJMP(opcode uint16) bool {
	if opcode&0xFFC0 != 0x4EC0 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	return mode == 2 || mode == 5 || mode == 6 || (mode == 7 && (reg == 0 || reg == 1 || reg == 2 || reg == 3))
}

func m68kIsNativeSupportedJSR(opcode uint16) bool {
	if opcode&0xFFC0 != 0x4E80 {
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	return mode == 2 || mode == 5 || mode == 6 || (mode == 7 && (reg == 0 || reg == 1 || reg == 2 || reg == 3))
}

func m68kIsEXG(opcode uint16) bool {
	if opcode&0xF130 != 0xC100 {
		return false
	}
	opmode := (opcode >> 3) & 0x1F
	return opmode == 0x08 || opmode == 0x09 || opmode == 0x11
}

func m68kIsPEARTSTrampoline(instrs []M68KJITInstr) bool {
	for i := 0; i+2 < len(instrs); i++ {
		if instrs[i].opcode != 0x487A { // PEA d16(PC)
			continue
		}
		if !m68kIsMoveLongRegToStackPredec(instrs[i+1].opcode) {
			continue
		}
		if instrs[i+2].opcode == 0x4E75 {
			return true
		}
	}
	return false
}

func m68kIsNativeSupportedBFWriteMemoryImmediate(opcode uint16) bool {
	switch opcode & 0xFFC0 {
	case 0xEAC0, 0xECC0, 0xEEC0, 0xEFC0:
	default:
		return false
	}
	mode := (opcode >> 3) & 7
	reg := opcode & 7
	switch mode {
	case 2, 5:
		return true
	case 7:
		return reg == 0 || reg == 1
	default:
		return false
	}
}

func m68kIsAROSConstructorDispatchBlock(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	if len(instrs) < 4 || memory == nil {
		return false
	}
	for i := 0; i+3 < len(instrs); i++ {
		if instrs[i].opcode != 0x274A || instrs[i+1].opcode != 0x674A ||
			instrs[i+2].opcode != 0x2C4A || instrs[i+3].opcode != 0x4EAE {
			continue
		}
		moveDispPC := startPC + instrs[i].pcOffset + 2
		jsrDispPC := startPC + instrs[i+3].pcOffset + 2
		if moveDispPC+2 > uint32(len(memory)) || jsrDispPC+2 > uint32(len(memory)) {
			return false
		}
		moveDisp := uint16(memory[moveDispPC])<<8 | uint16(memory[moveDispPC+1])
		jsrDisp := uint16(memory[jsrDispPC])<<8 | uint16(memory[jsrDispPC+1])
		if moveDisp == 0x00AA && jsrDisp == 0xFFE2 {
			return true
		}
	}
	return false
}

func m68kIsAROSMemmoveStackArgSetupPrefix(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	if len(instrs) < 4 || memory == nil {
		return false
	}
	if instrs[0].opcode != 0x202F || instrs[1].opcode != 0x222F || instrs[2].opcode != 0x242F {
		return false
	}
	if instrs[3].opcode>>12 != 0x6 {
		return false
	}
	dispPC0 := startPC + instrs[0].pcOffset + 2
	dispPC1 := startPC + instrs[1].pcOffset + 2
	dispPC2 := startPC + instrs[2].pcOffset + 2
	if dispPC2+2 > uint32(len(memory)) {
		return false
	}
	disp0 := uint16(memory[dispPC0])<<8 | uint16(memory[dispPC0+1])
	disp1 := uint16(memory[dispPC1])<<8 | uint16(memory[dispPC1+1])
	disp2 := uint16(memory[dispPC2])<<8 | uint16(memory[dispPC2+1])
	return disp0 == 0x0010 && disp1 == 0x0014 && disp2 == 0x0018
}

func m68kBlockOverlapsGuestRange(startPC uint32, instrs []M68KJITInstr, rangeStart, rangeEnd uint32) bool {
	if len(instrs) == 0 || rangeEnd <= rangeStart {
		return false
	}
	last := instrs[len(instrs)-1]
	endPC := startPC + last.pcOffset + uint32(last.length)
	return startPC < rangeEnd && endPC > rangeStart
}

func m68kInstrProductionNativeSafe(ji *M68KJITInstr) bool {
	opcode := ji.opcode
	group := opcode >> 12
	switch group {
	case 0x0:
		if opcode&0xFFF0 == 0x06C0 { // RTM
			return true
		}
		if opcode&0xFFC0 == 0x06C0 && ((opcode>>3)&7) >= M68K_AM_AR_IND { // CALLM
			return true
		}
		if opcode == 0x0CFC || opcode == 0x0EFC { // CAS2
			return true
		}
		if opcode&0xF9C0 == 0x08C0 && opcode&0x0600 != 0 { // CAS
			return true
		}
		if opcode&0xFF00 == 0x0E00 { // MOVES
			return true
		}
		if opcode&0xF9C0 == 0x00C0 { // CHK2/CMP2
			return true
		}
		if opcode&0xF138 == 0x0108 { // MOVEP
			return true
		}
		if m68kIsBTSTDynamic(opcode) || m68kIsBTSTImmediate(opcode) {
			return m68kIsNativeSupportedBTST(opcode)
		}
		if m68kIsBitModifyDynamic(opcode) || m68kIsBitModifyImmediate(opcode) {
			return m68kIsNativeSupportedBitModify(opcode)
		}
		if m68kIsImmediateStatusOp(opcode) {
			return true
		}
		if m68kIsBTSTImmDn(opcode) || m68kIsBTSTImmAnDisp(opcode) ||
			m68kIsImmediateLogicDn(opcode) || m68kIsImmediateArithmeticDn(opcode) {
			return true
		}
		if m68kIsNativeSupportedImmediateLogicEA(opcode) {
			return true
		}
		if m68kIsNativeSupportedImmediateArithmeticEA(opcode) {
			return true
		}
		return m68kIsNativeSupportedCMPI(opcode)
	case 0x7: // MOVEQ #imm,Dn
		return true
	case 0x2: // MOVE.L Dn,Dn
		if m68kIsNativeSupportedMOVEA(opcode) {
			return true
		}
		if m68kIsNativeSupportedMOVEGuarded(opcode) {
			return true
		}
		if m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
			return true
		}
		if m68kIsMoveLongStackDispToReg(opcode) || m68kIsMoveLongRegToStackDisp(opcode) ||
			m68kIsMoveLongRegToStackPredec(opcode) || m68kIsMoveLongStackDispToStackPredec(opcode) ||
			m68kIsMoveLongStackPostincToStackPredec(opcode) || m68kIsMoveLongStackIndirectToStackPostinc(opcode) ||
			m68kIsMoveLongStackDispToAddressIndirect(opcode) || m68kIsMoveLongAuditedStackMemory(opcode) {
			return true
		}
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		srcRegisterOrImmediate := srcMode == 0 || srcMode == 1 || (srcMode == 7 && srcReg == 4)
		srcLoadSafe := srcMode == 0 || srcMode == 1 || srcMode == 2 || srcMode == 3 || srcMode == 5 ||
			(srcMode == 7 && (srcReg == 0 || srcReg == 1 || srcReg == 4))
		dstRegisterSafe := dstMode == 0 || dstMode == 1
		dstReg := (opcode >> 9) & 7
		dstMemSafe := dstMode == 2 || dstMode == 3 || dstMode == 5 || (dstMode == 7 && dstReg <= 1)
		return (dstRegisterSafe && srcLoadSafe) ||
			(srcRegisterOrImmediate && dstMemSafe) ||
			(srcMode == 3 && dstMode == 3 && srcReg != dstReg)
	case 0x1, 0x3: // MOVE.B / MOVE.W, excluding MOVEA.W
		if m68kIsNativeSupportedMOVEA(opcode) {
			return true
		}
		if m68kIsNativeSupportedMOVEGuarded(opcode) {
			return true
		}
		if m68kIsNativeSupportedMOVEMemToMemGuarded(opcode) {
			return true
		}
		if m68kIsMovePostincPostinc(opcode) {
			return true
		}
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		srcLoadSafe := srcMode == 0 || (group == 0x3 && srcMode == 1) || srcMode == 2 || srcMode == 3 || srcMode == 5 ||
			(srcMode == 7 && (srcReg == 0 || srcReg == 1 || srcReg == 4))
		if group == 0x3 && dstMode == 1 {
			return srcLoadSafe
		}
		dstSafe := dstMode == 0 || dstMode == 2 || dstMode == 3 || dstMode == 5 ||
			dstMode == 6 || (dstMode == 7 && dstReg <= 1)
		return srcLoadSafe && dstSafe
	case 0x4:
		mode := (opcode >> 3) & 7
		reg := opcode & 7
		switch {
		case opcode&0xFFF0 == 0x4E40: // TRAP #n
			return true
		case opcode == 0x4E71: // NOP
			return true
		case opcode&0xFFF8 == 0x4880 || opcode&0xFFF8 == 0x48C0 || opcode&0xFFF8 == 0x49C0: // EXT.W / EXT.L / EXTB.L Dn
			return true
		case opcode&0xF1C0 == 0x4180 || opcode&0xF1C0 == 0x4100: // CHK.W/L <ea>,Dn
			return m68kIsNativeSupportedCHK(opcode)
		case opcode&0xF1C0 == 0x41C0: // LEA <ea>,An
			return mode == 2 || mode == 5 || mode == 6 || (mode == 7 && reg <= 3)
		case opcode&0xFF80 == 0x4C00: // MULL/DIVL
			return m68kIsNativeSupportedMULLDIVL(opcode)
		case opcode == 0x4E75: // RTS
			return true
		case opcode == 0x4E73: // RTE
			return true
		case opcode&0xFFC0 == 0x4E80: // JSR
			return m68kIsNativeSupportedJSR(opcode)
		case opcode&0xFFC0 == 0x4EC0: // JMP
			return m68kIsNativeSupportedJMP(opcode)
		case opcode&0xFFC0 == 0x4840 && mode >= 2: // PEA <ea>
			return m68kIsNativeSupportedPEA(opcode)
		case opcode&0xFFF8 == 0x4E50: // LINK.W An,#disp
			return reg != 7
		case opcode&0xFFF8 == 0x4808: // LINK.L An,#disp
			return reg != 7
		case opcode&0xFFF8 == 0x4848: // BKPT
			return true
		case opcode&0xFFF8 == 0x4E58: // UNLK An
			return true
		case opcode&0xFFF0 == 0x4E60: // MOVE USP
			return true
		case opcode&0xFFFE == 0x4E7A: // MOVEC
			return true
		case opcode&0xFB80 == 0x4880: // MOVEM.{W,L}
			return m68kIsNativeSupportedMOVEM(opcode)
		case opcode&0xFFC0 == 0x4800: // NBCD
			return m68kIsNativeSupportedNBCD(opcode)
		case opcode&0xFFC0 == 0x40C0 || opcode&0xFFC0 == 0x42C0: // MOVE from SR/CCR
			return m68kIsNativeSupportedMOVEFromStatus(opcode)
		case opcode&0xFFC0 == 0x44C0: // MOVE Dn/#imm,CCR
			return m68kIsNativeSupportedMOVEToCCR(opcode)
		case opcode&0xFFC0 == 0x46C0: // MOVE <ea>,SR
			return m68kIsNativeSupportedMOVEToSR(opcode)
		case opcode&0xFF00 == 0x4000: // NEGX
			return m68kIsNativeSupportedNEGX(opcode)
		case opcode&0xFF00 == 0x4200: // CLR
			return m68kIsNativeSupportedCLR(opcode)
		case opcode&0xFF00 == 0x4600: // NOT
			return m68kIsNativeSupportedNOT(opcode)
		case opcode&0xFF00 == 0x4400: // NEG
			return m68kIsNativeSupportedNEG(opcode)
		case opcode&0xFFC0 == 0x4AC0: // TAS
			return m68kIsNativeSupportedTAS(opcode)
		case opcode&0xFF00 == 0x4A00: // TST
			return m68kIsNativeSupportedTST(opcode)
		case opcode&0xFFF8 == 0x4840: // SWAP Dn
			return true
		}
	case 0x5: // ADDQ.L/SUBQ.L Dn or An, DBF, Scc Dn
		if opcode&0xF0F8 == 0x50F8 && opcode&7 >= 2 && opcode&7 <= 4 {
			return true
		}
		if opcode&0xF0F8 == 0x50C8 {
			return m68kIsNativeSupportedDBcc(opcode)
		}
		if opcode&0xF0C0 == 0x50C0 {
			return m68kIsNativeSupportedScc(opcode)
		}
		return m68kIsNativeSupportedADDQSUBQ(opcode)
	case 0x6: // BRA/Bcc/BSR.
		cond := (opcode >> 8) & 0xF
		return cond != 1 || m68kIsNativeSupportedBSR(opcode)
	case 0xF: // FPU helper exit; Line-F trap classes remain unsupported.
		return m68kIsJITHelperSupportedFPU(opcode)
	case 0x8: // OR.B/W/L <ea>,Dn
		if opcode&0xF1F0 == 0x8100 { // SBCD Dx,Dy
			regMode := (opcode >> 3) & 1
			return regMode == 0 || regMode == 1
		}
		if opcode&0xF1F0 == 0x8140 || opcode&0xF1F0 == 0x8180 { // PACK/UNPK
			return m68kIsNativeSupportedPACKUNPK(opcode)
		}
		if opcode&0xF0C0 == 0x80C0 { // DIVU.W/DIVS.W <ea>,Dn
			return m68kIsNativeSupportedDIVW(opcode)
		}
		return m68kIsNativeSupportedLogicEAToDn(opcode) || m68kIsNativeSupportedLogicDnToEA(opcode)
	case 0x9: // SUB.B/W/L <ea>,Dn / SUBA.L Dn|An,An
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		if opcode&0xF130 == 0x9100 && opcode&0x00C0 != 0x00C0 {
			size := (opcode >> 6) & 3
			regMode := (opcode >> 3) & 1
			return size != 3 && (regMode == 0 || regMode == 1)
		}
		return (opmode <= 2 && (srcMode == 0 || srcMode == 1 || srcMode == 2 || srcMode == 3 || srcMode == 4 || srcMode == 5 ||
			srcMode == 6 || (srcMode == 7 && (srcReg == 0 || srcReg == 1 || srcReg == 2 || srcReg == 3 || srcReg == 4)))) ||
			m68kIsNativeSupportedArithDnToEA(opcode) ||
			m68kIsNativeSupportedAddrArithA(opcode)
	case 0xB: // CMP.B/W/L <ea>,Dn / EOR.B/W/L Dn,<ea>
		if m68kIsNativeSupportedCMPM(opcode) {
			return true
		}
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		if opmode == 3 || opmode == 7 {
			return m68kIsNativeSupportedCMPA(opcode)
		}
		if opmode == 0 && srcMode == 1 {
			return false
		}
		return m68kIsNativeSupportedCMPToDn(opcode) ||
			(opmode >= 4 && opmode <= 6 && (srcMode == 0 || m68kIsNativeSupportedLogicDnToEA(opcode)))
	case 0xC: // AND.B/W/L <ea>,Dn
		if opcode&0xF1F0 == 0xC100 { // ABCD Dx,Dy
			regMode := (opcode >> 3) & 1
			return regMode == 0 || regMode == 1
		}
		if m68kIsEXG(opcode) {
			return true
		}
		if opcode&0xF0C0 == 0xC0C0 {
			return m68kIsNativeSupportedMULW(opcode)
		}
		return m68kIsNativeSupportedLogicEAToDn(opcode) || m68kIsNativeSupportedLogicDnToEA(opcode)
	case 0xD: // ADD.B/W/L <ea>,Dn / ADDA.L Dn|An,An
		opmode := (opcode >> 6) & 7
		srcMode := (opcode >> 3) & 7
		srcReg := opcode & 7
		if opcode&0xF130 == 0xD100 && opcode&0x00C0 != 0x00C0 {
			size := (opcode >> 6) & 3
			regMode := (opcode >> 3) & 1
			return size != 3 && (regMode == 0 || regMode == 1)
		}
		return (opmode <= 2 && (srcMode == 0 || srcMode == 1 || srcMode == 2 || srcMode == 3 || srcMode == 4 || srcMode == 5 ||
			srcMode == 6 || (srcMode == 7 && (srcReg == 0 || srcReg == 1 || srcReg == 2 || srcReg == 3 || srcReg == 4)))) ||
			m68kIsNativeSupportedArithDnToEA(opcode) ||
			m68kIsNativeSupportedAddrArithA(opcode)
	case 0xE: // RO[LR].B/W/L #n,Dn
		if m68kIsNativeSupportedBFTSTRegister(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFTSTMemoryImmediate(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFEXTURegister(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFEXTUMemoryImmediate(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFEXTSRegister(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFEXTSMemoryImmediate(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFFFORegister(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFFFOMemoryImmediate(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFWriteRegisterImmediate(opcode) {
			return true
		}
		if m68kIsNativeSupportedBFWriteMemoryImmediate(opcode) {
			return true
		}
		if m68kIsNativeSupportedMemShiftRotateWord(opcode) {
			return true
		}
		size := (opcode >> 6) & 3
		isRegCount := (opcode >> 5) & 1
		shiftType := (opcode >> 3) & 3
		if size != 3 && isRegCount == 0 && shiftType == 0 {
			return true
		}
		if size != 3 && isRegCount == 1 && shiftType == 0 {
			return true
		}
		if size != 3 && isRegCount == 0 && shiftType == 2 {
			return true
		}
		if size != 3 && isRegCount == 1 && shiftType == 2 {
			// ROXL/ROXR register-count. The native loop uses RCX as the
			// iteration counter. The earlier fear that the rotate target could
			// alias RCX is unfounded for the M68K JIT's static register map:
			// only D0->RBX and D1->RBP are ever allocated to host registers
			// (m68kDataRegToAMD64); every other data register resolves into the
			// RAX scratch. So the rotate target is always one of {RAX,RBX,RBP},
			// never RCX, and the LOOP counter is safe. Count 0 leaves all CCR
			// unchanged in the interpreter (ExecShiftRotate count==0 early
			// return) — which the native count-0 path matches by jumping to the
			// epilogue with R14 holding the pre-materialized incoming CCR. Both
			// premises for deferring were wrong; run it natively.
			return true
		}
		if size != M68K_SIZE_LONG && isRegCount == 0 && shiftType == 1 {
			return true
		}
		if size != 3 && isRegCount == 1 && shiftType == 1 {
			return true
		}
		if size == M68K_SIZE_LONG && isRegCount == 0 && shiftType <= 1 {
			return true
		}
		if size != 3 && isRegCount == 1 && shiftType == 3 {
			return true
		}
		return size != 3 && isRegCount == 0 && shiftType == 3
	}
	return false
}

// m68kNeedsConservativeFallback rejects blocks that contain opcode families or
// EA forms that are still known-bad in the JIT. This intentionally favors
// correctness over coverage: the interpreter remains the source of truth until
// focused JIT regressions exist for a shape.
func m68kNeedsConservativeFallback(memory []byte, startPC uint32, instrs []M68KJITInstr) bool {
	for _, ji := range instrs {
		opcode := ji.opcode
		group := opcode >> 12

		// Branch/control-transfer correctness is still incomplete. Only
		// audited simple BRA blocks may pass to the production native path.
		if group == 0x6 && !m68kInstrProductionNativeSafe(&ji) {
			return true
		}
		if opcode&0xF0F8 == 0x50C8 && !m68kIsNativeSupportedDBcc(opcode) { // DBcc
			return true
		}
		if opcode&0xF0C0 == 0x50C0 && opcode&0xF0F8 != 0x50C8 &&
			!m68kInstrProductionNativeSafe(&ji) { // Scc
			return true
		}

		// Shift/rotate coverage is still selective.
		if group == 0xE && opcode&0xF8C0 != 0xE8C0 && !m68kInstrProductionNativeSafe(&ji) {
			return true
		}

		// MOVE.W / MOVEA.W remains sensitive; only audited MOVE.W
		// non-MOVEA forms may pass to the production native path.
		if group == 0x3 && !m68kInstrProductionNativeSafe(&ji) {
			return true
		}

		// Group-4 remains correctness-sensitive. Keep control flow and
		// frame/memory forms interpreted; only audited register-long
		// operations may pass to the production native path.
		if group == 0x4 && !m68kInstrProductionNativeSafe(&ji) {
			return true
		}

		// Conservative indexed-EA bailout for the families currently exercised by
		// the failing suite backlog. This includes both brief and full-format 68020
		// indexed forms, plus PC-indexed.
		switch group {
		case 0x1, 0x2, 0x3: // MOVE.B / MOVE.L / MOVE.W / MOVEA
			if m68kIsNativeSupportedMOVEA(opcode) {
				continue
			}
			if m68kIsNativeSupportedMOVEGuarded(opcode) {
				continue
			}
			srcMode := (opcode >> 3) & 7
			srcReg := opcode & 7
			dstMode := (opcode >> 6) & 7
			dstReg := (opcode >> 9) & 7
			srcExtPC := startPC + ji.pcOffset + 2
			srcExtBytes := m68kEAExtBytes(srcMode, srcReg, m68kMoveSizeForGroup(group), memory, srcExtPC)
			dstExtPC := srcExtPC + uint32(srcExtBytes)
			srcIndexedUnsupported := m68kModeIsIndexed(srcMode, srcReg) &&
				!m68kIndexedEAAllowed(memory, srcExtPC, srcMode, srcReg)
			dstIndexedUnsupported := m68kModeIsIndexed(dstMode, dstReg) &&
				!m68kIndexedEAAllowed(memory, dstExtPC, dstMode, dstReg)
			stackSafeMove := group == 0x2 &&
				(m68kIsMoveLongStackDispToReg(opcode) || m68kIsMoveLongRegToStackDisp(opcode) ||
					m68kIsMoveLongRegToStackPredec(opcode) || m68kIsMoveLongStackDispToStackPredec(opcode) ||
					m68kIsMoveLongStackPostincToStackPredec(opcode) || m68kIsMoveLongStackIndirectToStackPostinc(opcode) ||
					m68kIsMoveLongStackDispToAddressIndirect(opcode) || m68kIsMoveLongAuditedStackMemory(opcode))
			stackSafeMove = stackSafeMove || m68kIsNativeSupportedMOVEGuarded(opcode) ||
				m68kIsNativeSupportedMOVEMemToMemGuarded(opcode)
			srcTouchesStackMemory := m68kModeTouchesSP(srcMode, srcReg) && srcMode != 1
			dstTouchesStackMemory := m68kModeTouchesSP(dstMode, dstReg) && dstMode != 1
			if srcIndexedUnsupported || dstIndexedUnsupported ||
				(!stackSafeMove && (srcTouchesStackMemory || dstTouchesStackMemory)) {
				return true
			}

		case 0x0, 0x5, 0x9, 0xB, 0xC, 0xD:
			if group == 0x0 && m68kIsNativeSupportedCMPI(opcode) {
				mode := (opcode >> 3) & 7
				reg := opcode & 7
				immBytes := m68kImmediateBytes(int((opcode >> 6) & 3))
				eaExtPC := startPC + ji.pcOffset + 2 + uint32(immBytes)
				if m68kModeIsIndexed(mode, reg) && !m68kIndexedEAAllowed(memory, eaExtPC, mode, reg) {
					return true
				}
				continue
			}
			if group == 0x0 && m68kIsNativeSupportedImmediateLogicEA(opcode) {
				mode := (opcode >> 3) & 7
				reg := opcode & 7
				immBytes := m68kImmediateBytes(int((opcode >> 6) & 3))
				eaExtPC := startPC + ji.pcOffset + 2 + uint32(immBytes)
				if m68kModeIsIndexed(mode, reg) && !m68kIndexedEAAllowed(memory, eaExtPC, mode, reg) {
					return true
				}
				continue
			}
			if group == 0x0 && m68kIsNativeSupportedImmediateArithmeticEA(opcode) {
				mode := (opcode >> 3) & 7
				reg := opcode & 7
				immBytes := m68kImmediateBytes(int((opcode >> 6) & 3))
				eaExtPC := startPC + ji.pcOffset + 2 + uint32(immBytes)
				if mode == 6 && !m68kBriefIndexedEAAllowed(memory, eaExtPC, mode, reg) {
					return true
				}
				continue
			}
			if group == 0x5 && opcode&0x00C0 != 0x00C0 {
				mode := (opcode >> 3) & 7
				reg := opcode & 7
				if mode == 0 || mode == 1 || mode == 2 || mode == 3 || mode == 4 || mode == 5 ||
					(mode == 6 && m68kBriefIndexedEAAllowed(memory, startPC+ji.pcOffset+2, mode, reg)) ||
					(mode == 7 && (reg == 0 || reg == 1)) {
					continue
				}
			}
			if group == 0xB {
				if m68kIsNativeSupportedCMPM(opcode) {
					continue
				}
				opmode := (opcode >> 6) & 7
				if opmode <= 2 && m68kIsNativeSupportedCMPToDn(opcode) {
					continue
				}
			}
			if group == 0x9 || group == 0xD {
				if (opcode&0xF130 == 0x9100 || opcode&0xF130 == 0xD100) && opcode&0x00C0 != 0x00C0 &&
					((opcode>>3)&1) == 1 && ((opcode>>6)&3) != 3 {
					continue
				}
				if m68kIsNativeSupportedArithDnToEA(opcode) {
					eaMode := (opcode >> 3) & 7
					eaReg := opcode & 7
					if m68kModeIsIndexed(eaMode, eaReg) &&
						!m68kBriefIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
						return true
					}
					continue
				}
				opmode := (opcode >> 6) & 7
				mode := (opcode >> 3) & 7
				if opmode == 2 && mode == 1 {
					continue
				}
			}
			if group == 0x0 && opcode&0xF138 == 0x0108 {
				// MOVEP's encoded bits are not a normal EA mode field; its
				// real EA is always d16(An). It never mutates An, and the
				// emitter prechecks the sparse byte range before access.
				continue
			}
			if group == 0x0 && (m68kIsBitModifyDynamic(opcode) || m68kIsBitModifyImmediate(opcode)) &&
				m68kIsNativeSupportedBitModify(opcode) {
				continue
			}
			if group == 0x0 && (m68kIsBTSTDynamic(opcode) || m68kIsBTSTImmediate(opcode)) &&
				m68kIsNativeSupportedBTST(opcode) {
				continue
			}
			if group == 0x8 || group == 0xB || group == 0xC {
				if group == 0xC && m68kIsEXG(opcode) {
					continue
				}
				if group == 0x8 && opcode&0xF0C0 == 0x80C0 && m68kIsNativeSupportedDIVW(opcode) {
					eaMode := (opcode >> 3) & 7
					eaReg := opcode & 7
					if m68kModeIsIndexed(eaMode, eaReg) &&
						!m68kIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
						return true
					}
					continue
				}
				if group == 0xC && m68kIsNativeSupportedMULW(opcode) {
					eaMode := (opcode >> 3) & 7
					eaReg := opcode & 7
					if m68kModeIsIndexed(eaMode, eaReg) &&
						!m68kIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
						return true
					}
					continue
				}
				if m68kIsNativeSupportedLogicDnToEA(opcode) {
					continue
				}
				if m68kIsNativeSupportedLogicEAToDn(opcode) {
					eaMode := (opcode >> 3) & 7
					eaReg := opcode & 7
					if m68kModeIsIndexed(eaMode, eaReg) &&
						!m68kIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
						return true
					}
					continue
				}
			}
			if group == 0x9 || group == 0xD {
				opmode := (opcode >> 6) & 7
				if ((opmode <= 2) || opmode == 3 || opmode == 7) && m68kInstrProductionNativeSafe(&ji) {
					eaMode := (opcode >> 3) & 7
					eaReg := opcode & 7
					if m68kModeIsIndexed(eaMode, eaReg) &&
						!m68kIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
						return true
					}
					continue
				}
			}
			if group == 0xB {
				opmode := (opcode >> 6) & 7
				if (opmode == 3 || opmode == 7) && m68kIsNativeSupportedCMPA(opcode) {
					eaMode := (opcode >> 3) & 7
					eaReg := opcode & 7
					if m68kModeIsIndexed(eaMode, eaReg) &&
						!m68kIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
						return true
					}
					continue
				}
			}
			if group == 0x5 && opcode&0xF0F8 == 0x50C8 {
				continue // DBcc uses low bits as Dn, not an EA touching A7.
			}
			if group == 0x5 && m68kIsNativeSupportedScc(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) && !m68kBriefIndexedEAAllowed(memory, startPC+ji.pcOffset+2, eaMode, eaReg) {
					return true
				}
				continue
			}
			eaMode := (opcode >> 3) & 7
			eaReg := opcode & 7
			if m68kModeIsIndexed(eaMode, eaReg) || m68kModeTouchesSP(eaMode, eaReg) {
				return true
			}

		case 0x4:
			if opcode&0xFFF0 == 0x4E40 {
				continue
			}
			if opcode == 0x4E73 {
				continue
			}
			if opcode == 0x4E75 {
				continue
			}
			if opcode == 0x4E71 {
				continue
			}
			if m68kIsNativeSupportedCLR(opcode) {
				continue
			}
			if m68kIsNativeSupportedNOT(opcode) {
				continue
			}
			if m68kIsNativeSupportedNEG(opcode) {
				continue
			}
			if m68kIsNativeSupportedNEGX(opcode) {
				continue
			}
			if m68kIsNativeSupportedNBCD(opcode) {
				continue
			}
			if m68kIsNativeSupportedTAS(opcode) {
				continue
			}
			if m68kIsNativeSupportedTST(opcode) {
				continue
			}
			if opcode&0xFFFE == 0x4E7A {
				continue
			}
			if m68kIsNativeSupportedPEA(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 2
					if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedMOVEM(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if eaMode == 6 {
					extPC := startPC + ji.pcOffset + 4
					if !m68kBriefIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedMULLDIVL(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 4
					if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedCHK(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 2
					if !m68kBriefIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedMOVEToSR(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 2
					if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedMOVEToCCR(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 2
					if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedMOVEFromStatus(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 2
					if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			if m68kIsNativeSupportedJSR(opcode) {
				eaMode := (opcode >> 3) & 7
				eaReg := opcode & 7
				if m68kModeIsIndexed(eaMode, eaReg) {
					extPC := startPC + ji.pcOffset + 2
					if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
						return true
					}
				}
				continue
			}
			eaMode := (opcode >> 3) & 7
			eaReg := opcode & 7
			if opcode&0xFFC0 == 0x4E80 && m68kModeIsIndexed(eaMode, eaReg) {
				extPC := startPC + ji.pcOffset + 2
				if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
					return true
				}
				continue
			}
			if opcode&0xFFC0 == 0x4EC0 && m68kModeIsIndexed(eaMode, eaReg) {
				extPC := startPC + ji.pcOffset + 2
				if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
					return true
				}
				continue
			}
			if opcode&0xF1C0 == 0x41C0 && m68kModeIsIndexed(eaMode, eaReg) {
				extPC := startPC + ji.pcOffset + 2
				if !m68kIndexedEAAllowed(memory, extPC, eaMode, eaReg) {
					return true
				}
				continue
			}
			if m68kModeIsIndexed(eaMode, eaReg) {
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

// m68kAnalyzeJSRLeafFusion validates whether a JSR target is a register-only
// leaf (≤ 4 body instructions terminated by RTS, no memory access, no A7
// manipulation, no embedded control flow). On success it returns the leaf's
// body instructions (excluding the trailing RTS), each with pcOffset = 0 and
// length = the source instruction length. Caller is responsible for adopting
// the JSR's pcOffset on each body instr.
//
// Restricting to register-only ops keeps bail semantics simple: none of the
// emitted instructions can fail mid-block, so the dispatcher never has to
// re-execute the leaf — only the JSR (which never executed in JIT) gets
// re-issued by the interpreter on full-block restart.
func m68kAnalyzeJSRLeafFusion(memory []byte, targetPC uint32) ([]M68KJITInstr, bool) {
	const maxBodyInstrs = 4
	memSize := uint32(len(memory))
	pc := targetPC
	body := make([]M68KJITInstr, 0, maxBodyInstrs)

	for i := 0; i < maxBodyInstrs+1; i++ {
		if pc+2 > memSize {
			return nil, false
		}
		opcode := uint16(memory[pc])<<8 | uint16(memory[pc+1])
		if opcode == 0x4E75 { // RTS — leaf complete
			return body, true
		}
		if !m68kIsLeafFusionSafe(opcode) {
			return nil, false
		}
		length := m68kInstrLength(memory, pc)
		body = append(body, M68KJITInstr{
			opcode:   opcode,
			pcOffset: 0,
			length:   uint16(length),
			group:    uint8(opcode >> 12),
		})
		pc += uint32(length)
	}
	return nil, false
}

// m68kIsLeafFusionSafe returns true iff the opcode is a register-only
// instruction safe to inline at a JSR site: no memory access, no A7
// manipulation, no control flow, no exception side effects.
func m68kIsLeafFusionSafe(opcode uint16) bool {
	if opcode == 0x4E71 { // NOP
		return true
	}
	group := opcode >> 12
	switch group {
	case 0x0: // Immediate logic to Dn, e.g. ANDI.L #imm,D0
		return m68kIsImmediateLogicDn(opcode)
	case 0x7: // MOVEQ #imm,Dn
		return opcode&0x0100 == 0
	case 0x1, 0x2, 0x3: // MOVE
		srcMode := (opcode >> 3) & 7
		dstMode := (opcode >> 6) & 7
		dstReg := (opcode >> 9) & 7
		// Allow Dn → Dn / Dn → An / An → Dn / An → An. Any An destination
		// of A7 is rejected (writes A7).
		if srcMode > 1 || dstMode > 1 {
			return false
		}
		if dstMode == 1 && dstReg == 7 {
			return false
		}
		return true
	case 0x5: // ADDQ/SUBQ/Scc/DBcc
		// Only ADDQ/SUBQ to Dn/An, size != byte/word/long-on-mem.
		if opcode&0x00C0 == 0x00C0 { // Scc/DBcc
			return false
		}
		// destination EA: bits 5..3 = mode, 2..0 = reg
		if mode := (opcode >> 3) & 7; mode != 0 && mode != 1 {
			return false
		}
		return true
	case 0x4: // misc — allow SWAP, EXT only
		switch {
		case opcode&0xFF38 == 0x4000 && (opcode>>6)&3 != 3: // NEGX.{B,W,L} Dn
			return true
		case opcode&0xFFF8 == 0x4840: // SWAP Dn
			return true
		case opcode&0xFFF8 == 0x4880: // EXT.W Dn
			return true
		case opcode&0xFFF8 == 0x48C0: // EXT.L Dn
			return true
		case opcode&0xFFF8 == 0x49C0: // EXTB.L Dn (68020)
			return true
		}
		return false
	case 0x8, 0x9, 0xB, 0xC, 0xD: // OR/SUB/CMP/EOR/AND/MUL/ADD
		// Allow only register-direct source AND register destination forms.
		srcMode := (opcode >> 3) & 7
		opmode := (opcode >> 6) & 7
		if group == 0xC && m68kIsEXG(opcode) {
			return true
		}
		if group == 0x8 && opcode&0xF0C0 == 0x80C0 && m68kIsNativeSupportedDIVW(opcode) {
			return true
		}
		if srcMode != 0 { // need Dn source
			return false
		}
		// Disallow EA-destination forms (opmode 4,5,6 = Dn → EA mem).
		// Disallow ADDA/SUBA/CMPA (opmode 3 or 7) — writes An, including
		// possibly A7. Restrict to Dn ← op(Dn, Dn).
		if opmode == 3 || opmode == 7 || opmode >= 4 {
			return false
		}
		return true
	}
	return false
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
		case 0x0: // CMPI / status-immediate
			if ji.opcode&0xF138 == 0x0108 {
				reg := (ji.opcode >> 9) & 7
				areg := ji.opcode & 7
				opmode := (ji.opcode >> 6) & 7
				br.addrRead |= 1 << areg
				if opmode >= 6 {
					br.dataRead |= 1 << reg
				} else {
					br.dataWritten |= 1 << reg
				}
			} else if m68kIsBTSTDynamic(ji.opcode) {
				src := (ji.opcode >> 9) & 7
				br.dataRead |= 1 << src
				m68kMarkEARead(&br, srcMode, srcReg)
				br.writesCCR = true
			} else if m68kIsBTSTImmediate(ji.opcode) {
				m68kMarkEARead(&br, srcMode, srcReg)
				br.writesCCR = true
			} else if m68kIsBitModifyDynamic(ji.opcode) {
				src := (ji.opcode >> 9) & 7
				br.dataRead |= 1 << src
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
				br.writesCCR = true
			} else if m68kIsBitModifyImmediate(ji.opcode) {
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
				br.writesCCR = true
			} else if m68kIsImmediateStatusOp(ji.opcode) {
				br.writesCCR = true
			} else if ji.opcode&0xFF00 == 0x0C00 && (ji.opcode>>6)&3 != 3 {
				m68kMarkEARead(&br, srcMode, srcReg)
				br.writesCCR = true
			}

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
				if srcMode != 1 {
					br.writesCCR = true
				}
			}

		case 0x9, 0xD: // SUB/ADD
			reg := (ji.opcode >> 9) & 7
			opmode := (ji.opcode >> 6) & 7
			if (ji.opcode&0xF130 == 0x9100 || ji.opcode&0xF130 == 0xD100) && ji.opcode&0x00C0 != 0x00C0 {
				rx := ji.opcode & 7
				ry := (ji.opcode >> 9) & 7
				br.dataRead |= 1 << rx
				br.dataRead |= 1 << ry
				br.dataWritten |= 1 << ry
				br.readsCCR = true
				br.writesCCR = true
			} else if opmode == 3 || opmode == 7 { // SUBA/ADDA
				m68kMarkEARead(&br, srcMode, srcReg)
				br.addrRead |= 1 << reg
				br.addrWritten |= 1 << reg
			} else if opmode >= 4 { // Dn → EA
				m68kMarkEARead(&br, srcMode, srcReg)
				br.dataRead |= 1 << reg
				m68kMarkEAWritten(&br, srcMode, srcReg)
			} else { // EA → Dn
				m68kMarkEARead(&br, srcMode, srcReg)
				br.dataRead |= 1 << reg
				br.dataWritten |= 1 << reg
				br.writesCCR = true
			}

		case 0x8, 0xC: // OR/AND, DIV/MUL
			reg := (ji.opcode >> 9) & 7
			if group == 0xC && m68kIsEXG(ji.opcode) {
				opmode := (ji.opcode >> 3) & 0x1F
				switch opmode {
				case 0x08:
					br.dataRead |= (1 << reg) | (1 << srcReg)
					br.dataWritten |= (1 << reg) | (1 << srcReg)
				case 0x09:
					br.addrRead |= (1 << reg) | (1 << srcReg)
					br.addrWritten |= (1 << reg) | (1 << srcReg)
				case 0x11:
					br.dataRead |= 1 << reg
					br.dataWritten |= 1 << reg
					br.addrRead |= 1 << srcReg
					br.addrWritten |= 1 << srcReg
				}
			} else if (group == 0x8 && ji.opcode&0xF1F0 == 0x8100) ||
				(group == 0xC && ji.opcode&0xF1F0 == 0xC100) {
				rx := ji.opcode & 7
				ry := (ji.opcode >> 9) & 7
				regMode := (ji.opcode >> 3) & 1
				if regMode == 0 {
					br.dataRead |= (1 << rx) | (1 << ry)
					br.dataWritten |= 1 << ry
				} else {
					br.addrRead |= (1 << rx) | (1 << ry)
					br.addrWritten |= (1 << rx) | (1 << ry)
				}
				br.readsCCR = true
				br.writesCCR = true
			} else {
				m68kMarkEARead(&br, srcMode, srcReg)
				br.dataRead |= 1 << reg
				br.dataWritten |= 1 << reg
				br.writesCCR = true
			}

		case 0xB: // CMP/EOR/CMPM
			if m68kIsNativeSupportedCMPM(ji.opcode) {
				rx := ji.opcode & 7
				ry := (ji.opcode >> 9) & 7
				br.addrRead |= (1 << rx) | (1 << ry)
				br.addrWritten |= (1 << rx) | (1 << ry)
				br.writesCCR = true
				break
			}
			opmode := (ji.opcode >> 6) & 7
			reg := (ji.opcode >> 9) & 7
			if opmode == 3 || opmode == 7 { // CMPA: EA -> An
				m68kMarkEARead(&br, srcMode, srcReg)
				br.addrRead |= 1 << reg
			} else if opmode >= 4 && opmode <= 6 { // EOR: Dn → EA (writes to EA)
				br.dataRead |= 1 << reg
				m68kMarkEAReadWrite(&br, srcMode, srcReg)
			} else { // CMP: EA → Dn (only reads)
				br.dataRead |= 1 << reg
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
	case opcode&0xFFF8 == 0x4E50 || opcode&0xFFF8 == 0x4808: // LINK
		reg := opcode & 7
		br.addrRead |= (1 << reg) | (1 << 7)
		br.addrWritten |= (1 << reg) | (1 << 7)
	case opcode&0xFFF8 == 0x4E58: // UNLK
		reg := opcode & 7
		br.addrRead |= 1 << reg
		br.addrWritten |= (1 << reg) | (1 << 7)
	case opcode&0xF1C0 == 0x4180 || opcode&0xF1C0 == 0x4100: // CHK.W/L
		reg := (opcode >> 9) & 7
		br.dataRead |= 1 << reg
		m68kMarkEARead(br, eaMode, eaReg)
		br.writesCCR = true
	case opcode&0xFFF0 == 0x4E60: // MOVE USP
		reg := opcode & 7
		if ((opcode >> 3) & 1) == 1 {
			br.addrWritten |= 1 << reg
		} else {
			br.addrRead |= 1 << reg
		}
	case opcode&0xFB80 == 0x4880: // MOVEM
		// MOVEM reads/writes multiple registers — conservatively mark all
		br.dataRead |= 0xFF
		br.dataWritten |= 0xFF
		br.addrRead |= 0xFF
		br.addrWritten |= 0xFF
	case opcode&0xFFC0 == 0x4800: // NBCD
		m68kMarkEAReadWrite(br, eaMode, eaReg)
		br.readsCCR = true
		br.writesCCR = true
	case opcode&0xFFC0 == 0x40C0 || opcode&0xFFC0 == 0x42C0: // MOVE from SR/CCR
		if eaMode == 0 {
			br.dataWritten |= 1 << eaReg
		} else {
			m68kMarkEAWritten(br, eaMode, eaReg)
		}
	case opcode&0xFFC0 == 0x44C0: // MOVE to CCR
		m68kMarkEARead(br, eaMode, eaReg)
		br.writesCCR = true
	case opcode&0xFFC0 == 0x46C0: // MOVE to SR
		m68kMarkEARead(br, eaMode, eaReg)
		br.writesCCR = true
	case opcode&0xFF00 == 0x4000 && (opcode>>6)&3 != 3: // NEGX
		m68kMarkEAReadWrite(br, eaMode, eaReg)
		br.readsCCR = true
		br.writesCCR = true
	case opcode&0xFFC0 == 0x4AC0: // TAS
		m68kMarkEAReadWrite(br, eaMode, eaReg)
		br.writesCCR = true
	case opcode&0xFF00 == 0x4A00 && (opcode>>6)&3 != 3: // TST
		m68kMarkEARead(br, eaMode, eaReg)
		br.writesCCR = true
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

// m68kResolveTerminatorTarget computes the static branch target for a
// region-eligible terminator. Returns (targetPC, true) for BRA (8/16/32-bit
// displacement), JMP <abs.W>, JMP <abs.L>, and JMP (PC,d16). Calls (BSR,
// JSR) and indirect transfers (RTS, RTE, RTR, JMP via Dn/An, JMP indexed,
// TRAP, Line A/F) return (0, false) — region formation does not follow them.
//
// instrPC is the PC of the terminating instruction itself. memory must
// span the full instruction including any displacement / EA extension words.
func m68kResolveTerminatorTarget(opcode uint16, instrPC uint32, memory []byte) (uint32, bool) {
	memSize := uint32(len(memory))
	group := opcode >> 12

	if group == 0x6 { // Branch group
		cond := (opcode >> 8) & 0xF
		if cond > 1 {
			return 0, false // Bcc — handled inline by emitter, not a region terminator
		}
		if cond == 1 {
			return 0, false // BSR — call, do not follow into region
		}
		// BRA
		disp := opcode & 0xFF
		switch disp {
		case 0x00: // 16-bit displacement
			if instrPC+4 > memSize {
				return 0, false
			}
			w := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
			return uint32(int64(instrPC) + 2 + int64(w)), true
		case 0xFF: // 32-bit displacement (68020+)
			if instrPC+6 > memSize {
				return 0, false
			}
			l := int32(uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
				uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5]))
			return uint32(int64(instrPC) + 2 + int64(l)), true
		default: // 8-bit displacement embedded in opcode low byte
			return uint32(int64(instrPC) + 2 + int64(int8(disp))), true
		}
	}

	// JMP — opcode 0100 1110 11_mmmreg, where mmmreg encodes the EA mode.
	if opcode&0xFFC0 == 0x4EC0 {
		mode := (opcode >> 3) & 0x7
		reg := opcode & 0x7
		switch {
		case mode == 7 && reg == 0: // <abs.W>
			if instrPC+4 > memSize {
				return 0, false
			}
			w := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
			return uint32(int32(w)), true
		case mode == 7 && reg == 1: // <abs.L>
			if instrPC+6 > memSize {
				return 0, false
			}
			return uint32(memory[instrPC+2])<<24 | uint32(memory[instrPC+3])<<16 |
				uint32(memory[instrPC+4])<<8 | uint32(memory[instrPC+5]), true
		case mode == 7 && reg == 2: // (d16,PC)
			if instrPC+4 > memSize {
				return 0, false
			}
			w := int16(uint16(memory[instrPC+2])<<8 | uint16(memory[instrPC+3]))
			return uint32(int64(instrPC) + 2 + int64(w)), true
		}
		// (An), (PC,d8,Xn), other indirect — not statically resolvable.
		return 0, false
	}

	return 0, false
}

// Suppress unused import warnings
var _ = bits.Len
var _ = unsafe.Pointer(nil)

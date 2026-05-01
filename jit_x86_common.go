// jit_x86_common.go - x86 JIT compiler infrastructure: context, scanner, length calculator
//
// (c) 2024-2026 Zayn Otley - GPLv3 or later

package main

import (
	"unsafe"
)

// ===========================================================================
// X86JITContext — Bridge between Go and JIT-compiled native code
// ===========================================================================

// X86JITContext is passed to every JIT-compiled x86 block as its sole argument.
// On ARM64 it arrives in X0; on x86-64 in RDI.
type X86JITContext struct {
	JITRegsPtr        uintptr // 0:   &cpu.jitRegs[0]
	MemPtr            uintptr // 8:   &cpu.memory[0]
	MemSize           uint32  // 16:  len(memory)
	_pad0             uint32  // 20:  alignment padding
	FlagsPtr          uintptr // 24:  &cpu.Flags
	EIPPtr            uintptr // 32:  &cpu.EIP
	CpuPtr            uintptr // 40:  &cpu
	NeedInval         uint32  // 48:  self-modification detected
	NeedIOFallback    uint32  // 52:  I/O bail flag
	RetPC             uint32  // 56:  next EIP after block
	RetCount          uint32  // 60:  instructions retired
	CodePageBitmapPtr uintptr // 64:  &codePageBitmap[0]
	IOBitmapPtr       uintptr // 72:  &x86IOBitmap[0] (256-byte page granularity)
	FPUPtr            uintptr // 80:  unsafe.Pointer(cpu.FPU) -- FPU_X87 struct, not Go pointer field
	SegRegsPtr        uintptr // 88:  &cpu.jitSegRegs[0] (ES,CS,SS,DS,FS,GS)
	ChainBudget       uint32  // 96:  blocks remaining before mandatory Go return
	ChainCount        uint32  // 100: accumulated instruction count across chain
	RTSCache0PC       uint32  // 104: MRU RET target cache entry 0 - guest PC
	_pad1             uint32  // 108: alignment
	RTSCache0Addr     uintptr // 112: MRU entry 0 - chain entry address
	RTSCache0RegMap   uint64  // 120: MRU entry 0 - target block's regMap (8 bytes packed as uint64)
	RTSCache1PC       uint32  // 128: MRU entry 1 - guest PC
	_pad2             uint32  // 132: alignment
	RTSCache1Addr     uintptr // 136: MRU entry 1 - chain entry address
	RTSCache1RegMap   uint64  // 144: MRU entry 1 - target block's regMap
}

// X86JITContext field offsets (must match struct layout above)
const (
	x86CtxOffJITRegsPtr        = 0
	x86CtxOffMemPtr            = 8
	x86CtxOffMemSize           = 16
	x86CtxOffFlagsPtr          = 24
	x86CtxOffEIPPtr            = 32
	x86CtxOffCpuPtr            = 40
	x86CtxOffNeedInval         = 48
	x86CtxOffNeedIOFallback    = 52
	x86CtxOffRetPC             = 56
	x86CtxOffRetCount          = 60
	x86CtxOffCodePageBitmapPtr = 64
	x86CtxOffIOBitmapPtr       = 72
	x86CtxOffFPUPtr            = 80
	x86CtxOffSegRegsPtr        = 88
	x86CtxOffChainBudget       = 96
	x86CtxOffChainCount        = 100
	x86CtxOffRTSCache0PC       = 104
	x86CtxOffRTSCache0Addr     = 112
	x86CtxOffRTSCache0RegMap   = 120
	x86CtxOffRTSCache1PC       = 128
	x86CtxOffRTSCache1Addr     = 136
	x86CtxOffRTSCache1RegMap   = 144
)

// x86JitAvailable is set to true at init time on platforms that support x86 JIT.
var x86JitAvailable bool

// x86JitMaxBlockSize is the maximum number of instructions in a single JIT block.
const x86JitMaxBlockSize = 256

func newX86JITContext(cpu *CPU_X86, codePageBitmap []byte, ioBitmap []byte) *X86JITContext {
	ctx := &X86JITContext{
		JITRegsPtr: uintptr(unsafe.Pointer(&cpu.jitRegs[0])),
		MemPtr:     uintptr(unsafe.Pointer(&cpu.memory[0])),
		MemSize:    uint32(len(cpu.memory)),
		FlagsPtr:   uintptr(unsafe.Pointer(&cpu.Flags)),
		EIPPtr:     uintptr(unsafe.Pointer(&cpu.EIP)),
		CpuPtr:     uintptr(unsafe.Pointer(cpu)),
		SegRegsPtr: uintptr(unsafe.Pointer(&cpu.jitSegRegs[0])),
	}
	if cpu.FPU != nil {
		ctx.FPUPtr = uintptr(unsafe.Pointer(cpu.FPU))
	}
	if len(codePageBitmap) > 0 {
		ctx.CodePageBitmapPtr = uintptr(unsafe.Pointer(&codePageBitmap[0]))
	}
	if len(ioBitmap) > 0 {
		ctx.IOBitmapPtr = uintptr(unsafe.Pointer(&ioBitmap[0]))
	}
	return ctx
}

// ===========================================================================
// X86JITInstr — Pre-decoded x86 instruction for JIT compilation
// ===========================================================================

// X86JITInstr represents one x86 instruction scanned from a basic block.
type X86JITInstr struct {
	opcodePC uint32 // absolute PC of first byte (including prefixes)
	pcOffset uint32 // byte offset from block start
	length   uint16 // total instruction length including prefixes
	opcode   uint16 // opcode: low byte for 1-byte, 0x0Fxx for 2-byte
	modrm    byte   // ModR/M byte (0 if none)
	hasModRM bool   // whether ModR/M is present
	prefixes byte   // packed prefix flags
	grpOp    byte   // group sub-opcode from ModR/M reg field (for Grp1-5)
}

// Prefix flag bits packed into X86JITInstr.prefixes
const (
	x86PrefSeg      = 0x01 // segment override present
	x86PrefOpSize   = 0x02 // 0x66 operand size
	x86PrefAddrSize = 0x04 // 0x67 address size
	x86PrefRep      = 0x08 // 0xF3 REP/REPE
	x86PrefRepNE    = 0x10 // 0xF2 REPNE
	x86PrefLock     = 0x20 // 0xF0 LOCK
)

// ===========================================================================
// Instruction Length Calculator
// ===========================================================================

// x86InstrLength returns the total length in bytes of the x86 instruction
// at memory[pc], including all prefixes, opcode, ModR/M, SIB, displacement,
// and immediate bytes.
func x86InstrLength(memory []byte, pc uint32) int {
	start := pc
	memSize := uint32(len(memory))

	if pc >= memSize {
		return 1
	}

	// Track operand size override for immediate size changes
	opSizePrefix := false

	// Skip prefixes
	for pc < memSize {
		b := memory[pc]
		switch b {
		case 0x26, 0x2E, 0x36, 0x3E, 0x64, 0x65: // segment overrides
			pc++
		case 0x66: // operand size
			opSizePrefix = true
			pc++
		case 0x67: // address size
			pc++
		case 0xF0: // LOCK
			pc++
		case 0xF2, 0xF3: // REPNE, REP
			pc++
		default:
			goto donePrefix
		}
	}
donePrefix:

	if pc >= memSize {
		return int(pc - start)
	}

	opcode := memory[pc]
	pc++

	// Two-byte opcode (0x0F prefix)
	if opcode == 0x0F {
		if pc >= memSize {
			return int(pc - start)
		}
		opcode2 := memory[pc]
		pc++
		return int(pc-start) + x86ExtendedOpcodeExtra(memory, pc, opcode2, opSizePrefix)
	}

	// Single-byte opcode
	return int(pc-start) + x86BaseOpcodeExtra(memory, pc, opcode, opSizePrefix)
}

// x86BaseOpcodeExtra returns the number of additional bytes after the opcode byte
// for a single-byte opcode. pc points to the byte after the opcode.
func x86BaseOpcodeExtra(memory []byte, pc uint32, opcode byte, opSize bool) int {
	switch opcode {
	// ---- No extra bytes ----
	case 0x06, 0x07, 0x0E, 0x16, 0x17, 0x1E, 0x1F: // PUSH/POP seg
		return 0
	case 0x27, 0x2F, 0x37, 0x3F: // DAA, DAS, AAA, AAS
		return 0
	case 0x90: // NOP
		return 0
	case 0x98, 0x99: // CBW/CWDE, CWD/CDQ
		return 0
	case 0x9B: // WAIT
		return 0
	case 0x9C, 0x9D: // PUSHF, POPF
		return 0
	case 0x9E, 0x9F: // SAHF, LAHF
		return 0
	case 0xC3, 0xCB: // RET, RETF
		return 0
	case 0xC9: // LEAVE
		return 0
	case 0xCC: // INT3
		return 0
	case 0xCE: // INTO
		return 0
	case 0xCF: // IRET
		return 0
	case 0xD6: // SALC
		return 0
	case 0xD7: // XLAT
		return 0
	case 0xF4: // HLT
		return 0
	case 0xF5: // CMC
		return 0
	case 0xF8, 0xF9, 0xFA, 0xFB, 0xFC, 0xFD: // CLC/STC/CLI/STI/CLD/STD
		return 0

	// ---- INC/DEC/PUSH/POP r32 (0x40-0x5F) ----
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47: // INC r32
		return 0
	case 0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F: // DEC r32
		return 0
	case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57: // PUSH r32
		return 0
	case 0x58, 0x59, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F: // POP r32
		return 0
	case 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97: // XCHG EAX, r32
		return 0

	// ---- String ops (1 byte) ----
	case 0xA4, 0xA5, 0xA6, 0xA7: // MOVS/CMPS
		return 0
	case 0xAA, 0xAB, 0xAC, 0xAD, 0xAE, 0xAF: // STOS/LODS/SCAS
		return 0

	// ---- imm8 only ----
	case 0x04, 0x0C, 0x14, 0x1C, 0x24, 0x2C, 0x34, 0x3C: // ALU AL, imm8
		return 1
	case 0x6A: // PUSH imm8
		return 1
	case 0xA8: // TEST AL, imm8
		return 1
	case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7: // MOV r8, imm8
		return 1
	case 0xCD: // INT imm8
		return 1
	case 0xD4, 0xD5: // AAM/AAD imm8
		return 1
	case 0xE0, 0xE1, 0xE2, 0xE3: // LOOP/LOOPE/LOOPNE/JCXZ rel8
		return 1
	case 0xE4, 0xE5, 0xE6, 0xE7: // IN/OUT imm8
		return 1
	case 0xEB: // JMP rel8
		return 1
	case 0x6C, 0x6D, 0x6E, 0x6F: // INS/OUTS
		return 0

	// ---- Jcc rel8 (0x70-0x7F) ----
	case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77,
		0x78, 0x79, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F:
		return 1

	// ---- imm16/imm32 (depends on operand size) ----
	case 0x05, 0x0D, 0x15, 0x1D, 0x25, 0x2D, 0x35, 0x3D: // ALU EAX, imm32
		if opSize {
			return 2
		}
		return 4
	case 0x68: // PUSH imm32
		if opSize {
			return 2
		}
		return 4
	case 0xA9: // TEST EAX, imm32
		if opSize {
			return 2
		}
		return 4
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF: // MOV r32, imm32
		if opSize {
			return 2
		}
		return 4

	// ---- moffs (direct address) ----
	case 0xA0, 0xA1, 0xA2, 0xA3: // MOV AL/AX, moffs
		return 4 // 32-bit address in 32-bit mode

	// ---- rel16/rel32 ----
	case 0xE8, 0xE9: // CALL rel32, JMP rel32
		if opSize {
			return 2
		}
		return 4

	// ---- Special multi-byte ----
	case 0xC2, 0xCA: // RET imm16, RETF imm16
		return 2
	case 0xC8: // ENTER imm16, imm8
		return 3
	case 0x9A: // CALL far (ptr16:32)
		if opSize {
			return 4 // ptr16:16
		}
		return 6 // ptr16:32
	case 0xEA: // JMP far (ptr16:32)
		if opSize {
			return 4
		}
		return 6

	// ---- ModR/M opcodes ----
	case 0x00, 0x01, 0x02, 0x03: // ADD Eb/Ev, Gb/Gv and reverse
		return x86ModRMExtra(memory, pc, 0)
	case 0x08, 0x09, 0x0A, 0x0B: // OR
		return x86ModRMExtra(memory, pc, 0)
	case 0x10, 0x11, 0x12, 0x13: // ADC
		return x86ModRMExtra(memory, pc, 0)
	case 0x18, 0x19, 0x1A, 0x1B: // SBB
		return x86ModRMExtra(memory, pc, 0)
	case 0x20, 0x21, 0x22, 0x23: // AND
		return x86ModRMExtra(memory, pc, 0)
	case 0x28, 0x29, 0x2A, 0x2B: // SUB
		return x86ModRMExtra(memory, pc, 0)
	case 0x30, 0x31, 0x32, 0x33: // XOR
		return x86ModRMExtra(memory, pc, 0)
	case 0x38, 0x39, 0x3A, 0x3B: // CMP
		return x86ModRMExtra(memory, pc, 0)
	case 0x62: // BOUND (unused but for completeness)
		return x86ModRMExtra(memory, pc, 0)
	case 0x63: // ARPL
		return x86ModRMExtra(memory, pc, 0)
	case 0x84, 0x85: // TEST Eb/Ev, Gb/Gv
		return x86ModRMExtra(memory, pc, 0)
	case 0x86, 0x87: // XCHG
		return x86ModRMExtra(memory, pc, 0)
	case 0x88, 0x89, 0x8A, 0x8B: // MOV
		return x86ModRMExtra(memory, pc, 0)
	case 0x8C: // MOV Ev, Sreg
		return x86ModRMExtra(memory, pc, 0)
	case 0x8D: // LEA
		return x86ModRMExtra(memory, pc, 0)
	case 0x8E: // MOV Sreg, Ew
		return x86ModRMExtra(memory, pc, 0)
	case 0x8F: // POP Ev
		return x86ModRMExtra(memory, pc, 0)

	// ---- ModR/M + imm8 ----
	case 0x80, 0x82: // Grp1 Eb, Ib
		return x86ModRMExtra(memory, pc, 1)
	case 0x83: // Grp1 Ev, Ib
		return x86ModRMExtra(memory, pc, 1)
	case 0x6B: // IMUL Gv, Ev, Ib
		return x86ModRMExtra(memory, pc, 1)
	case 0xC0: // Grp2 Eb, Ib
		return x86ModRMExtra(memory, pc, 1)
	case 0xC1: // Grp2 Ev, Ib
		return x86ModRMExtra(memory, pc, 1)
	case 0xC6: // MOV Eb, Ib
		return x86ModRMExtra(memory, pc, 1)

	// ---- ModR/M + imm16/imm32 ----
	case 0x81: // Grp1 Ev, Iv
		immSize := 4
		if opSize {
			immSize = 2
		}
		return x86ModRMExtra(memory, pc, immSize)
	case 0xC7: // MOV Ev, Iv
		immSize := 4
		if opSize {
			immSize = 2
		}
		return x86ModRMExtra(memory, pc, immSize)
	case 0x69: // IMUL Gv, Ev, Iv
		immSize := 4
		if opSize {
			immSize = 2
		}
		return x86ModRMExtra(memory, pc, immSize)

	// ---- ModR/M only (no immediate), shifts, etc. ----
	case 0xD0, 0xD1, 0xD2, 0xD3: // Grp2 shifts
		return x86ModRMExtra(memory, pc, 0)
	case 0xFE: // Grp4 Eb
		return x86ModRMExtra(memory, pc, 0)
	case 0xFF: // Grp5 Ev
		return x86ModRMExtra(memory, pc, 0)

	// ---- Grp3: TEST has immediate, others don't ----
	case 0xF6: // Grp3 Eb
		return x86Grp3Extra(memory, pc, 1)
	case 0xF7: // Grp3 Ev
		immSize := 4
		if opSize {
			immSize = 2
		}
		return x86Grp3Extra(memory, pc, immSize)

	// ---- x87 FPU escapes (D8-DF): all have ModR/M ----
	case 0xD8, 0xD9, 0xDA, 0xDB, 0xDC, 0xDD, 0xDE, 0xDF:
		return x86ModRMExtra(memory, pc, 0)

	// ---- IN/OUT with DX ----
	case 0xEC, 0xED, 0xEE, 0xEF:
		return 0

	// ---- PUSHA/POPA ----
	case 0x60, 0x61:
		return 0
	}

	// Unknown opcode: assume 1 byte total (already consumed opcode byte)
	return 0
}

// x86ExtendedOpcodeExtra returns the number of additional bytes after the
// second opcode byte for a two-byte (0x0F xx) opcode.
func x86ExtendedOpcodeExtra(memory []byte, pc uint32, opcode2 byte, opSize bool) int {
	switch opcode2 {
	// Jcc rel32 (0x0F 80 - 0x0F 8F)
	case 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87,
		0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8D, 0x8E, 0x8F:
		if opSize {
			return 2
		}
		return 4

	// SETcc (0x0F 90 - 0x0F 9F) - ModR/M
	case 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
		0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F:
		return x86ModRMExtra(memory, pc, 0)

	// PUSH/POP FS/GS
	case 0xA0, 0xA1, 0xA8, 0xA9:
		return 0

	// BT/BTS/BTR/BTC Ev, Gv - ModR/M
	case 0xA3, 0xAB, 0xB3, 0xBB:
		return x86ModRMExtra(memory, pc, 0)

	// SHLD/SHRD Ev, Gv, imm8
	case 0xA4, 0xAC:
		return x86ModRMExtra(memory, pc, 1)

	// SHLD/SHRD Ev, Gv, CL
	case 0xA5, 0xAD:
		return x86ModRMExtra(memory, pc, 0)

	// IMUL Gv, Ev - ModR/M
	case 0xAF:
		return x86ModRMExtra(memory, pc, 0)

	// MOVZX/MOVSX - ModR/M
	case 0xB6, 0xB7, 0xBE, 0xBF:
		return x86ModRMExtra(memory, pc, 0)

	// Grp8 Ev, Ib - ModR/M + imm8
	case 0xBA:
		return x86ModRMExtra(memory, pc, 1)

	// BSF/BSR - ModR/M
	case 0xBC, 0xBD:
		return x86ModRMExtra(memory, pc, 0)

	// CMOVcc (0x0F 40 - 0x0F 4F) - ModR/M
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47,
		0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
		return x86ModRMExtra(memory, pc, 0)

	// LFS/LGS/LSS - ModR/M
	case 0xB2, 0xB4, 0xB5:
		return x86ModRMExtra(memory, pc, 0)
	}

	// Unknown extended opcode
	return 0
}

// x86ModRMExtra returns the number of additional bytes consumed by the ModR/M
// byte (and optional SIB and displacement) plus any trailing immediate bytes.
// pc points to the ModR/M byte.
func x86ModRMExtra(memory []byte, pc uint32, immSize int) int {
	if pc >= uint32(len(memory)) {
		return 1 + immSize
	}

	modrm := memory[pc]
	mod := modrm >> 6
	rm := modrm & 7
	extra := 1 // ModR/M byte itself

	switch mod {
	case 0:
		if rm == 5 {
			// disp32, no base
			extra += 4
		} else if rm == 4 {
			// SIB byte follows
			extra++
			if pc+1 < uint32(len(memory)) {
				sib := memory[pc+1]
				sibBase := sib & 7
				if sibBase == 5 {
					// SIB base=5 with mod=0 means disp32
					extra += 4
				}
			}
		}
	case 1:
		// disp8
		extra++
		if rm == 4 {
			extra++ // SIB
		}
	case 2:
		// disp32
		extra += 4
		if rm == 4 {
			extra++ // SIB
		}
	case 3:
		// register direct, no extra
	}

	return extra + immSize
}

// x86Grp3Extra handles the special case of Grp3 opcodes (0xF6/0xF7) where
// the TEST sub-opcode (/0 and /1) has an immediate but other sub-opcodes don't.
func x86Grp3Extra(memory []byte, pc uint32, testImmSize int) int {
	if pc >= uint32(len(memory)) {
		return 1
	}

	modrm := memory[pc]
	regOp := (modrm >> 3) & 7

	// TEST (reg=0 or reg=1) has an immediate operand
	immSize := 0
	if regOp == 0 || regOp == 1 {
		immSize = testImmSize
	}

	return x86ModRMExtra(memory, pc, immSize)
}

// ===========================================================================
// Block Scanner
// ===========================================================================

// x86ScanBlock decodes x86 instructions starting at startPC until a block
// terminator is found or the max block size is reached.
func x86ScanBlock(memory []byte, startPC uint32) []X86JITInstr {
	instrs := make([]X86JITInstr, 0, 32)
	memSize := uint32(len(memory))
	pc := startPC

	for len(instrs) < x86JitMaxBlockSize {
		if pc >= memSize {
			break
		}

		length := x86InstrLength(memory, pc)
		if length <= 0 {
			break
		}

		// Decode the instruction for the JIT
		ji := x86DecodeInstr(memory, pc, uint16(length))
		ji.pcOffset = pc - startPC
		instrs = append(instrs, ji)

		if x86IsBlockTerminator(ji.opcode) {
			break
		}

		pc += uint32(length)
	}

	return instrs
}

// x86DecodeInstr pre-decodes an instruction at memory[pc] with the given length.
func x86DecodeInstr(memory []byte, pc uint32, length uint16) X86JITInstr {
	ji := X86JITInstr{
		opcodePC: pc,
		length:   length,
	}

	cur := pc
	memSize := uint32(len(memory))

	// Parse prefixes
	for cur < memSize {
		b := memory[cur]
		switch b {
		case 0x26, 0x2E, 0x36, 0x3E, 0x64, 0x65:
			ji.prefixes |= x86PrefSeg
			cur++
		case 0x66:
			ji.prefixes |= x86PrefOpSize
			cur++
		case 0x67:
			ji.prefixes |= x86PrefAddrSize
			cur++
		case 0xF0:
			ji.prefixes |= x86PrefLock
			cur++
		case 0xF2:
			ji.prefixes |= x86PrefRepNE
			cur++
		case 0xF3:
			ji.prefixes |= x86PrefRep
			cur++
		default:
			goto donePfx
		}
	}
donePfx:

	if cur >= memSize {
		return ji
	}

	opcode := memory[cur]
	cur++

	if opcode == 0x0F {
		// Two-byte opcode
		if cur < memSize {
			ji.opcode = 0x0F00 | uint16(memory[cur])
			cur++
		}
	} else {
		ji.opcode = uint16(opcode)
	}

	// Parse ModR/M if present
	if cur < memSize && x86OpcodeHasModRM(ji.opcode) {
		ji.modrm = memory[cur]
		ji.hasModRM = true
		ji.grpOp = (ji.modrm >> 3) & 7
	}

	return ji
}

// x86OpcodeHasModRM returns true if the opcode requires a ModR/M byte.
func x86OpcodeHasModRM(opcode uint16) bool {
	if opcode >= 0x0F00 {
		// Two-byte opcodes: most have ModR/M except Jcc and PUSH/POP FS/GS
		op2 := byte(opcode)
		switch {
		case op2 >= 0x80 && op2 <= 0x8F: // Jcc rel32
			return false
		case op2 == 0xA0 || op2 == 0xA1 || op2 == 0xA8 || op2 == 0xA9: // PUSH/POP FS/GS
			return false
		}
		return true
	}

	op := byte(opcode)
	switch {
	// ModR/M opcodes
	case op <= 0x03: // ADD variants
		return true
	case op >= 0x08 && op <= 0x0B: // OR
		return true
	case op >= 0x10 && op <= 0x13: // ADC
		return true
	case op >= 0x18 && op <= 0x1B: // SBB
		return true
	case op >= 0x20 && op <= 0x23: // AND
		return true
	case op >= 0x28 && op <= 0x2B: // SUB
		return true
	case op >= 0x30 && op <= 0x33: // XOR
		return true
	case op >= 0x38 && op <= 0x3B: // CMP
		return true
	case op == 0x62, op == 0x63: // BOUND, ARPL
		return true
	case op == 0x69, op == 0x6B: // IMUL variants
		return true
	case op >= 0x80 && op <= 0x8F: // Grp1, TEST, XCHG, MOV, LEA, POP
		return true
	case op >= 0xC0 && op <= 0xC1: // Grp2
		return true
	case op == 0xC4, op == 0xC5: // LES, LDS
		return true
	case op == 0xC6, op == 0xC7: // MOV Eb/Ev, Ib/Iv
		return true
	case op >= 0xD0 && op <= 0xD3: // Grp2 shifts
		return true
	case op >= 0xD8 && op <= 0xDF: // FPU escapes
		return true
	case op == 0xF6, op == 0xF7: // Grp3
		return true
	case op == 0xFE, op == 0xFF: // Grp4, Grp5
		return true
	}
	return false
}

// ===========================================================================
// Block Terminators and Fallback Detection
// ===========================================================================

// x86IsBlockTerminator returns true if the opcode ends a basic block.
func x86IsBlockTerminator(opcode uint16) bool {
	switch opcode {
	case 0xC3, 0xCB: // RET, RETF
		return true
	case 0xC2, 0xCA: // RET imm16, RETF imm16
		return true
	case 0xE8: // CALL rel
		return true
	case 0xE9, 0xEB: // JMP rel32, JMP rel8
		return true
	case 0xEA: // JMP far
		return true
	case 0x9A: // CALL far
		return true
	case 0xCF: // IRET
		return true
	case 0xCC, 0xCD, 0xCE: // INT3, INT, INTO
		return true
	case 0xF4: // HLT
		return true
	case 0xFF: // Grp5 (indirect CALL/JMP/PUSH)
		return true
	}
	return false
}

// x86NeedsFallback returns true if the block's first instruction requires
// the interpreter and cannot be JIT-compiled.
func x86NeedsFallback(instrs []X86JITInstr) bool {
	if len(instrs) == 0 {
		return true
	}

	// Segment-override-prefixed first instruction: the compile loop bails
	// at the segment-prefix gate without emitting anything, which post
	// Phase-8 surfaces as a "no instructions compiled" panic. Route to
	// single-instruction Step bail instead so the fast-path stays correct
	// without growing per-segment native-emit support.
	if instrs[0].prefixes&x86PrefSeg != 0 {
		return true
	}

	opcode := instrs[0].opcode

	switch opcode {
	// Segment register writes
	case 0x8E: // MOV Sreg, Ew
		return true

	// Far control flow
	case 0x9A: // CALL far
		return true
	case 0xEA: // JMP far
		return true
	case 0xCB, 0xCA: // RETF, RETF imm16
		return true

	// Interrupts
	case 0xCC, 0xCD, 0xCE: // INT3, INT, INTO
		return true
	case 0xCF: // IRET
		return true

	// Load segment + pointer
	case 0xC4: // LES
		return true
	case 0xC5: // LDS
		return true

	// PUSH/POP segment registers
	case 0x06, 0x07: // PUSH/POP ES
		return true
	case 0x0E: // PUSH CS
		return true
	case 0x16, 0x17: // PUSH/POP SS
		return true
	case 0x1E, 0x1F: // PUSH/POP DS
		return true

	// I/O port instructions
	case 0x6C, 0x6D, 0x6E, 0x6F: // INS/OUTS
		return true
	case 0xE4, 0xE5, 0xE6, 0xE7: // IN/OUT imm8
		return true
	case 0xEC, 0xED, 0xEE, 0xEF: // IN/OUT DX
		return true

	// HLT — bail to single-instruction Step so cpu.Halted is set
	// through the canonical interpreter path. The exec loop's Halted
	// check then exits cleanly. Native HLT emit would need its own
	// epilogue + Halted-set, while the per-instruction bail-and-resume
	// protocol already exists for MMIO / unsupported-op cases; reusing
	// it costs nothing on real workloads (HLT runs once per program).
	case 0xF4: // HLT
		return true
	}

	// Two-byte opcodes
	if opcode >= 0x0F00 {
		op2 := byte(opcode)
		switch op2 {
		case 0xA0, 0xA1: // PUSH/POP FS
			return true
		case 0xA8, 0xA9: // PUSH/POP GS
			return true
		case 0xB2: // LSS
			return true
		case 0xB4: // LFS
			return true
		case 0xB5: // LGS
			return true
		}
	}

	return false
}

// ===========================================================================
// Register Liveness Analysis
// ===========================================================================

// x86BlockRegs holds register usage bitmasks for a JIT block.
// Bit i corresponds to x86 register index i (EAX=0, ECX=1, EDX=2, EBX=3,
// ESP=4, EBP=5, ESI=6, EDI=7).
type x86BlockRegs struct {
	read    uint8     // registers read by block
	written uint8     // registers written by block
	freq    [8]uint16 // access count per register
}

// x86AnalyzeBlockRegs computes register read/written bitmasks for the block.
func x86AnalyzeBlockRegs(instrs []X86JITInstr, memory []byte, startPC uint32) x86BlockRegs {
	var regs x86BlockRegs

	for _, ji := range instrs {
		opcode := ji.opcode

		// Handle two-byte opcodes (0x0F xx)
		if opcode >= 0x0F00 {
			if ji.hasModRM {
				mod := ji.modrm >> 6
				reg := (ji.modrm >> 3) & 7
				rm := ji.modrm & 7
				op2 := byte(opcode)

				switch {
				case op2 == 0xAF: // IMUL Gv, Ev
					regs.read |= 1 << reg
					regs.written |= 1 << reg
					regs.freq[reg] += 2
					if mod == 3 {
						regs.read |= 1 << rm
						regs.freq[rm]++
					}
				case op2 == 0xB6 || op2 == 0xB7 || op2 == 0xBE || op2 == 0xBF: // MOVZX/MOVSX
					regs.written |= 1 << reg
					regs.freq[reg]++
					if mod == 3 {
						regs.read |= 1 << (rm & 3)
						regs.freq[rm&3]++
					}
				case op2 >= 0x40 && op2 <= 0x4F: // CMOVcc
					regs.read |= 1 << reg
					regs.written |= 1 << reg
					regs.freq[reg] += 2
					if mod == 3 {
						regs.read |= 1 << rm
						regs.freq[rm]++
					}
				case op2 == 0xBC || op2 == 0xBD: // BSF/BSR
					regs.written |= 1 << reg
					regs.freq[reg]++
					if mod == 3 {
						regs.read |= 1 << rm
						regs.freq[rm]++
					}
				}
			}
			continue
		}

		op := byte(opcode)

		// MOV r32, imm32 (0xB8-0xBF)
		if op >= 0xB8 && op <= 0xBF {
			reg := op - 0xB8
			regs.written |= 1 << reg
			regs.freq[reg]++
			continue
		}

		// MOV r8, imm8 (0xB0-0xB7)
		if op >= 0xB0 && op <= 0xB7 {
			// r8 encoding: 0-3 = AL/CL/DL/BL, 4-7 = AH/CH/DH/BH
			r8 := op - 0xB0
			reg := r8 & 3 // maps to EAX/ECX/EDX/EBX
			regs.written |= 1 << reg
			regs.freq[reg]++
			continue
		}

		// INC/DEC r32 (0x40-0x4F)
		if op >= 0x40 && op <= 0x4F {
			reg := (op - 0x40) & 7
			regs.read |= 1 << reg
			regs.written |= 1 << reg
			regs.freq[reg] += 2
			continue
		}

		// PUSH r32 (0x50-0x57) - reads reg + ESP
		if op >= 0x50 && op <= 0x57 {
			reg := op - 0x50
			regs.read |= 1 << reg
			regs.read |= 1 << 4 // ESP
			regs.written |= 1 << 4
			regs.freq[reg]++
			regs.freq[4]++
			continue
		}

		// POP r32 (0x58-0x5F) - writes reg, reads/writes ESP
		if op >= 0x58 && op <= 0x5F {
			reg := op - 0x58
			regs.written |= 1 << reg
			regs.read |= 1 << 4
			regs.written |= 1 << 4
			regs.freq[reg]++
			regs.freq[4]++
			continue
		}

		// ALU with ModR/M: extract reg and rm fields
		if ji.hasModRM {
			mod := ji.modrm >> 6
			reg := (ji.modrm >> 3) & 7
			rm := ji.modrm & 7

			// Determine direction from opcode
			switch {
			case op >= 0x00 && op <= 0x03,
				op >= 0x08 && op <= 0x0B,
				op >= 0x10 && op <= 0x13,
				op >= 0x18 && op <= 0x1B,
				op >= 0x20 && op <= 0x23,
				op >= 0x28 && op <= 0x2B,
				op >= 0x30 && op <= 0x33,
				op >= 0x38 && op <= 0x3B:
				// ALU ops: even opcodes = rm<-reg, odd = rm<-reg (Ev,Gv)
				// reg field is source, rm is destination (for even) or vice versa
				if op&1 == 0 { // Eb/Ev, Gb/Gv (rm = dest, reg = src)
					regs.read |= 1 << reg
					regs.freq[reg]++
					if mod == 3 {
						regs.read |= 1 << rm
						regs.written |= 1 << rm
						regs.freq[rm] += 2
					}
				} else if op&3 == 1 { // Ev, Gv (rm = dest, reg = src)
					regs.read |= 1 << reg
					regs.freq[reg]++
					if mod == 3 {
						regs.read |= 1 << rm
						regs.written |= 1 << rm
						regs.freq[rm] += 2
					}
				} else { // Gb/Gv, Eb/Ev (reg = dest, rm = src)
					regs.written |= 1 << reg
					regs.read |= 1 << reg
					regs.freq[reg] += 2
					if mod == 3 {
						regs.read |= 1 << rm
						regs.freq[rm]++
					}
				}

			case op == 0x89: // MOV Ev, Gv
				regs.read |= 1 << reg
				regs.freq[reg]++
				if mod == 3 {
					regs.written |= 1 << rm
					regs.freq[rm]++
				}

			case op == 0x8B: // MOV Gv, Ev
				regs.written |= 1 << reg
				regs.freq[reg]++
				if mod == 3 {
					regs.read |= 1 << rm
					regs.freq[rm]++
				}
			}

			// Memory addressing reads base/index registers
			if mod != 3 {
				if rm == 4 { // SIB
					// Would need to decode SIB for exact registers
				} else if rm != 5 || mod != 0 {
					regs.read |= 1 << rm
					regs.freq[rm]++
				}
			}
		}

		// ALU AL/EAX, imm
		switch op {
		case 0x04, 0x0C, 0x14, 0x1C, 0x24, 0x2C, 0x34, 0x3C,
			0x05, 0x0D, 0x15, 0x1D, 0x25, 0x2D, 0x35, 0x3D:
			regs.read |= 1 << 0 // EAX
			if op < 0x38 {      // CMP doesn't write
				regs.written |= 1 << 0
			}
			regs.freq[0] += 2
		case 0xA8, 0xA9: // TEST AL/EAX, imm
			regs.read |= 1 << 0
			regs.freq[0]++
		}
	}

	return regs
}

// ===========================================================================
// Tier 2: Per-Block Register Allocation
// ===========================================================================

const x86Tier2Threshold = 64 // execution count before recompilation

// x86TierController declaration lives in jit_x86_tier_amd64.go so it
// stays gated behind the same `amd64 && (linux||windows||darwin)`
// build tag as NewTierController / X86RegProfile (jit_tier_common.go).
// jit_x86_common.go itself is untagged and must compile on
// arm64/non-Linux test builds, where those symbols are absent.

// x86Tier2RegAlloc computes an optimal register mapping for a specific block.
// Returns a mapping: guestReg -> hostReg (0 if spilled).
// The 5 available callee-saved host slots are assigned to the most frequently
// accessed guest registers in the block.
func x86Tier2RegAlloc(instrs []X86JITInstr, memory []byte, startPC uint32) [8]byte {
	br := x86AnalyzeBlockRegs(instrs, memory, startPC)

	type regFreq struct {
		guest byte
		freq  uint16
	}
	freqs := make([]regFreq, 8)
	for i := byte(0); i < 8; i++ {
		freqs[i] = regFreq{guest: i, freq: br.freq[i]}
	}

	// Sort by frequency (descending) -- simple insertion sort for 8 elements
	for i := 1; i < 8; i++ {
		key := freqs[i]
		j := i - 1
		for j >= 0 && freqs[j].freq < key.freq {
			freqs[j+1] = freqs[j]
			j--
		}
		freqs[j+1] = key
	}

	// Available host registers (callee-saved)
	hostSlots := [5]byte{amd64RBX, amd64RBP, amd64R12, amd64R13, amd64R14}

	var mapping [8]byte // 0 = spilled
	slotIdx := 0

	// ESP (guest reg 4) always gets a slot if used
	for i := 0; i < 8; i++ {
		if freqs[i].guest == 4 && freqs[i].freq > 0 && slotIdx < 5 {
			mapping[4] = hostSlots[slotIdx]
			slotIdx++
			freqs[i].freq = 0 // mark as allocated
			break
		}
	}

	// Assign remaining slots to highest-frequency registers
	for i := 0; i < 8 && slotIdx < 5; i++ {
		if freqs[i].freq > 0 {
			mapping[freqs[i].guest] = hostSlots[slotIdx]
			slotIdx++
		}
	}

	return mapping
}

// ===========================================================================
// Multi-Block Region Formation
// ===========================================================================

const (
	x86RegionMaxBlocks = 8   // max blocks in a region
	x86RegionMaxInstrs = 512 // max total instructions in a region
)

// x86Region represents a multi-block hot region to be compiled as a single unit.
type x86Region struct {
	blocks   [][]X86JITInstr // instructions per block
	blockPCs []uint32        // start PC of each block
	entryPC  uint32          // region entry PC
	// backEdge: targetBlockIdx for backward jumps (loop back-edges)
	backEdges map[int]int // blockIdx -> target blockIdx
}

// x86FormRegion builds a region starting from a hot block by following
// direct successor chains. Only follows statically-known JMP/Jcc targets
// that are already compiled in the cache.
func x86FormRegion(hotPC uint32, cache *CodeCache, memory []byte) *x86Region {
	region := &x86Region{
		entryPC:   hotPC,
		backEdges: make(map[int]int),
	}

	visited := make(map[uint32]int) // PC -> block index in region
	totalInstrs := 0

	pc := hotPC
	for len(region.blocks) < x86RegionMaxBlocks && totalInstrs < x86RegionMaxInstrs {
		if _, seen := visited[pc]; seen {
			break // cycle detected, stop
		}
		if pc >= uint32(len(memory)) {
			break
		}

		instrs := x86ScanBlock(memory, pc)
		if len(instrs) == 0 || x86NeedsFallback(instrs) {
			break
		}

		blockIdx := len(region.blocks)
		visited[pc] = blockIdx
		region.blocks = append(region.blocks, instrs)
		region.blockPCs = append(region.blockPCs, pc)
		totalInstrs += len(instrs)

		// Find successor: look at last instruction
		last := &instrs[len(instrs)-1]
		if !x86IsBlockTerminator(last.opcode) {
			// Fall-through to next instruction
			nextPC := last.opcodePC + uint32(last.length)
			pc = nextPC
			continue
		}

		// For JMP/CALL with known target, follow the chain
		targetPC, hasTarget := x86ResolveTerminatorTarget(last, memory, region.blockPCs[blockIdx])
		if !hasTarget {
			break // indirect control flow, stop
		}

		// Check for back-edge (loop)
		if targetIdx, isBackEdge := visited[targetPC]; isBackEdge {
			region.backEdges[blockIdx] = targetIdx
			break // loop found, region complete
		}

		// Follow forward edge
		pc = targetPC
	}

	if len(region.blocks) < 2 {
		return nil // single block, not worth a region
	}

	return region
}

// ===========================================================================
// Peephole Optimizer
// ===========================================================================

// x86PeepholeFlags annotates the instruction slice with flag optimization hints.
// Returns a parallel slice of booleans: true means this instruction's flag output
// is consumed by a later instruction (i.e., flags matter). If false, the emitter
// can skip flag materialization.
func x86PeepholeFlags(instrs []X86JITInstr) []bool {
	n := len(instrs)
	flagsNeeded := make([]bool, n)

	// Walk backward: if we need flags, mark the closest flag-producing instruction
	needFlags := false

	for i := n - 1; i >= 0; i-- {
		ji := &instrs[i]
		op := byte(ji.opcode)

		// Does this instruction READ flags?
		readsFlags := false
		if ji.opcode < 0x0F00 {
			switch {
			case op >= 0x70 && op <= 0x7F: // Jcc
				readsFlags = true
			case op == 0x9C: // PUSHF
				readsFlags = true
			case op == 0x9E: // SAHF
				readsFlags = false // writes, not reads
			case op == 0x9F: // LAHF
				readsFlags = true
			case op == 0x10 || op == 0x11 || op == 0x12 || op == 0x13: // ADC
				readsFlags = true
			case op == 0x14 || op == 0x15: // ADC AL/EAX, imm
				readsFlags = true
			case op == 0x18 || op == 0x19 || op == 0x1A || op == 0x1B: // SBB
				readsFlags = true
			case op == 0x1C || op == 0x1D: // SBB AL/EAX, imm
				readsFlags = true
			case op == 0xD0 || op == 0xD1 || op == 0xD2 || op == 0xD3: // Shifts (RCL/RCR read CF)
				if ji.hasModRM {
					shiftOp := (ji.modrm >> 3) & 7
					if shiftOp == 2 || shiftOp == 3 { // RCL, RCR
						readsFlags = true
					}
				}
			}
		}

		if readsFlags {
			needFlags = true
		}

		// Does this instruction WRITE flags?
		writesFlags := x86InstrWritesFlags(ji)

		if writesFlags {
			flagsNeeded[i] = needFlags
			needFlags = false // this instruction produces flags; earlier ones don't need to
		}
	}

	return flagsNeeded
}

// x86InstrWritesFlags returns true if the instruction modifies EFLAGS.
func x86InstrWritesFlags(ji *X86JITInstr) bool {
	op := byte(ji.opcode)
	if ji.opcode >= 0x0F00 {
		return false // Most 0x0F opcodes that we compile don't change flags meaningfully
	}
	switch {
	case op >= 0x00 && op <= 0x05:
		return true // ADD
	case op >= 0x08 && op <= 0x0D:
		return true // OR
	case op >= 0x10 && op <= 0x15:
		return true // ADC
	case op >= 0x18 && op <= 0x1D:
		return true // SBB
	case op >= 0x20 && op <= 0x25:
		return true // AND
	case op >= 0x28 && op <= 0x2D:
		return true // SUB
	case op >= 0x30 && op <= 0x35:
		return true // XOR
	case op >= 0x38 && op <= 0x3D:
		return true // CMP
	case op >= 0x40 && op <= 0x4F:
		return true // INC/DEC
	case op == 0x80 || op == 0x81 || op == 0x82 || op == 0x83:
		return true // Grp1
	case op == 0x84 || op == 0x85:
		return true // TEST
	case op == 0xA8 || op == 0xA9:
		return true // TEST AL/EAX
	case op >= 0xC0 && op <= 0xC1:
		return true // Grp2 shifts
	case op >= 0xD0 && op <= 0xD3:
		return true // Grp2 shifts
	case op == 0xF6 || op == 0xF7:
		return true // Grp3
	}
	return false
}

// ===========================================================================
// I/O Bitmap Builder
// ===========================================================================

// buildX86IOBitmap creates the JIT I/O page bitmap (256-byte page granularity,
// indexed by addr >> 8). Merges MachineBus.ioPageBitmap with adapter-specific regions.
func buildX86IOBitmap(adapter *X86BusAdapter, bus *MachineBus) []byte {
	// Bitmap size: total address space / 256 bytes per page. PLAN_MAX_RAM
	// slice 10g: bus-driven, replaces the retired 32 MiB constant.
	bitmapSize := len(bus.GetMemory()) >> 8
	if bitmapSize == 0 {
		bitmapSize = 1
	}
	bitmap := make([]byte, bitmapSize)

	// 1. Copy MachineBus ioPageBitmap ([]bool -> []byte)
	for i, isIO := range bus.ioPageBitmap {
		if isIO && i < len(bitmap) {
			bitmap[i] = 1
		}
	}

	// 2. Mark translateIO region: 0xF000-0xFFFF
	// The adapter remaps these addresses to 0xF0000+, which has bus I/O mappings
	for addr := uint32(0xF000); addr < 0x10000; addr += 0x100 {
		page := addr >> 8
		if page < uint32(len(bitmap)) {
			bitmap[page] = 1
		}
	}

	// 3. Mark bank control register pages: 0xF700-0xF7F1
	// (already covered by translateIO range above, but explicit for clarity)

	// 4. Mark VGA VRAM: 0xA0000-0xAFFFF
	for addr := uint32(0xA0000); addr < 0xB0000; addr += 0x100 {
		page := addr >> 8
		if page < uint32(len(bitmap)) {
			bitmap[page] = 1
		}
	}

	// 5. Mark bank windows when banking is enabled
	if adapter.bank1Enable {
		for addr := uint32(X86_BANK1_WINDOW_BASE); addr < uint32(X86_BANK1_WINDOW_BASE+X86_BANK_WINDOW_SIZE); addr += 0x100 {
			page := addr >> 8
			if page < uint32(len(bitmap)) {
				bitmap[page] = 1
			}
		}
	}
	if adapter.bank2Enable {
		for addr := uint32(X86_BANK2_WINDOW_BASE); addr < uint32(X86_BANK2_WINDOW_BASE+X86_BANK_WINDOW_SIZE); addr += 0x100 {
			page := addr >> 8
			if page < uint32(len(bitmap)) {
				bitmap[page] = 1
			}
		}
	}
	if adapter.bank3Enable {
		for addr := uint32(X86_BANK3_WINDOW_BASE); addr < uint32(X86_BANK3_WINDOW_BASE+X86_BANK_WINDOW_SIZE); addr += 0x100 {
			page := addr >> 8
			if page < uint32(len(bitmap)) {
				bitmap[page] = 1
			}
		}
	}
	if adapter.vramEnabled {
		for addr := uint32(X86_VRAM_BANK_WINDOW_BASE); addr < uint32(X86_VRAM_BANK_WINDOW_BASE+X86_VRAM_BANK_WINDOW_SIZE); addr += 0x100 {
			page := addr >> 8
			if page < uint32(len(bitmap)) {
				bitmap[page] = 1
			}
		}
	}

	return bitmap
}

// jit_z80_common.go - Z80 JIT compiler infrastructure: context, scanner, lookup tables

//go:build (amd64 && (linux || windows || darwin)) || (arm64 && linux)

package main

import (
	"unsafe"
)

// ===========================================================================
// Z80JITContext — Bridge between Go and JIT-compiled native code
// ===========================================================================

// Z80JITContext is passed to every JIT-compiled Z80 block as its sole argument.
// On ARM64 it arrives in X0; on x86-64 in RDI.
type Z80JITContext struct {
	MemPtr              uintptr // 0:   MachineBus.GetMemory() base
	CpuPtr              uintptr // 8:   &cpu (CPU_Z80 pointer)
	DirectPageBitmapPtr uintptr // 16:  &cpu.directPageBitmap[0]
	CodePageBitmapPtr   uintptr // 24:  &cpu.codePageBitmap[0]
	RetCycles           uint64  // 32:  accumulated T-states
	NeedBail            uint32  // 40:  re-execute current instruction via interpreter
	NeedInval           uint32  // 44:  self-modification detected
	RetPC               uint32  // 48:  PC to resume at (bail=current instr, normal/inval=next)
	RetCount            uint32  // 52:  instructions retired before bail/exit
	ChainBudget         uint32  // 56:  blocks remaining before returning to Go
	ChainCount          uint32  // 60:  accumulated instruction count during chaining
	InvalPage           uint32  // 64:  page number that triggered NeedInval
	RTSCache0PC         uint32  // 68:  MRU RET target cache 0 — Z80 PC
	_pad0               uint32  // 72:  alignment padding
	RTSCache0Addr       uintptr // 80:  MRU RET target cache 0 — chain entry address
	RTSCache1PC         uint32  // 88:  MRU RET target cache 1 — Z80 PC
	_pad1               uint32  // 92:  alignment padding
	RTSCache1Addr       uintptr // 96:  MRU RET target cache 1 — chain entry address
	ParityTablePtr      uintptr // 104: &z80ParityTable[0] (256 bytes)
	DAATablePtr         uintptr // 112: &z80DAATable[0] ([2048]uint16 = 4096 bytes)
	ChainCycles         uint64  // 120: accumulated T-states across chained blocks
	ChainRIncrements    uint32  // 128: accumulated R register increments across chain
	CycleBudget         uint32  // 132: max cycles before forced Go return (interrupt budget)
}

// Z80JITContext field offsets (must match struct layout above).
// These are used by native code emitters to access context fields.
const (
	jzCtxOffMemPtr              = 0
	jzCtxOffCpuPtr              = 8
	jzCtxOffDirectPageBitmapPtr = 16
	jzCtxOffCodePageBitmapPtr   = 24
	jzCtxOffRetCycles           = 32
	jzCtxOffNeedBail            = 40
	jzCtxOffNeedInval           = 44
	jzCtxOffRetPC               = 48
	jzCtxOffRetCount            = 52
	jzCtxOffChainBudget         = 56
	jzCtxOffChainCount          = 60
	jzCtxOffInvalPage           = 64
	jzCtxOffRTSCache0PC         = 68
	jzCtxOffRTSCache0Addr       = 80
	jzCtxOffRTSCache1PC         = 88
	jzCtxOffRTSCache1Addr       = 96
	jzCtxOffParityTablePtr      = 104
	jzCtxOffDAATablePtr         = 112
	jzCtxOffChainCycles         = 120
	jzCtxOffChainRIncrements    = 128
	jzCtxOffCycleBudget         = 132
)

// CPU_Z80 struct field offsets (from CpuPtr). Must match cpu_z80.go layout.
// Validated by TestZ80JIT_CPUStructFieldOffsets.
var (
	cpuZ80OffA        = uintptr(unsafe.Offsetof(CPU_Z80{}.A))
	cpuZ80OffF        = uintptr(unsafe.Offsetof(CPU_Z80{}.F))
	cpuZ80OffB        = uintptr(unsafe.Offsetof(CPU_Z80{}.B))
	cpuZ80OffC        = uintptr(unsafe.Offsetof(CPU_Z80{}.C))
	cpuZ80OffD        = uintptr(unsafe.Offsetof(CPU_Z80{}.D))
	cpuZ80OffE        = uintptr(unsafe.Offsetof(CPU_Z80{}.E))
	cpuZ80OffH        = uintptr(unsafe.Offsetof(CPU_Z80{}.H))
	cpuZ80OffL        = uintptr(unsafe.Offsetof(CPU_Z80{}.L))
	cpuZ80OffA2       = uintptr(unsafe.Offsetof(CPU_Z80{}.A2))
	cpuZ80OffF2       = uintptr(unsafe.Offsetof(CPU_Z80{}.F2))
	cpuZ80OffB2       = uintptr(unsafe.Offsetof(CPU_Z80{}.B2))
	cpuZ80OffC2       = uintptr(unsafe.Offsetof(CPU_Z80{}.C2))
	cpuZ80OffD2       = uintptr(unsafe.Offsetof(CPU_Z80{}.D2))
	cpuZ80OffE2       = uintptr(unsafe.Offsetof(CPU_Z80{}.E2))
	cpuZ80OffH2       = uintptr(unsafe.Offsetof(CPU_Z80{}.H2))
	cpuZ80OffL2       = uintptr(unsafe.Offsetof(CPU_Z80{}.L2))
	cpuZ80OffIX       = uintptr(unsafe.Offsetof(CPU_Z80{}.IX))
	cpuZ80OffIY       = uintptr(unsafe.Offsetof(CPU_Z80{}.IY))
	cpuZ80OffSP       = uintptr(unsafe.Offsetof(CPU_Z80{}.SP))
	cpuZ80OffPC       = uintptr(unsafe.Offsetof(CPU_Z80{}.PC))
	cpuZ80OffI        = uintptr(unsafe.Offsetof(CPU_Z80{}.I))
	cpuZ80OffR        = uintptr(unsafe.Offsetof(CPU_Z80{}.R))
	cpuZ80OffIM       = uintptr(unsafe.Offsetof(CPU_Z80{}.IM))
	cpuZ80OffWZ       = uintptr(unsafe.Offsetof(CPU_Z80{}.WZ))
	cpuZ80OffIFF1     = uintptr(unsafe.Offsetof(CPU_Z80{}.IFF1))
	cpuZ80OffIFF2     = uintptr(unsafe.Offsetof(CPU_Z80{}.IFF2))
	cpuZ80OffHalted   = uintptr(unsafe.Offsetof(CPU_Z80{}.Halted))
	cpuZ80OffCycles   = uintptr(unsafe.Offsetof(CPU_Z80{}.Cycles))
	cpuZ80OffIffDelay = uintptr(unsafe.Offsetof(CPU_Z80{}.iffDelay))
)

// z80JitAvailable is set to true at init time on platforms that support Z80 JIT.
var z80JitAvailable bool

// ===========================================================================
// Precomputed Tables
// ===========================================================================

// z80ParityTable stores even-parity flag for each byte value.
// z80ParityTable[v] = 0x04 (P/V flag bit) if v has even parity, 0 otherwise.
var z80ParityTable [256]byte

func init() {
	for i := 0; i < 256; i++ {
		bits := 0
		v := i
		for v != 0 {
			bits += v & 1
			v >>= 1
		}
		if bits%2 == 0 {
			z80ParityTable[i] = 0x04 // P/V flag bit position
		}
	}
}

// z80DAATable stores precomputed DAA results.
// Index = (A << 3) | (C << 2) | (H << 1) | N
// Value = uint16((resultA << 8) | resultF)
var z80DAATable [2048]uint16

func init() {
	for a := 0; a < 256; a++ {
		for flags := 0; flags < 8; flags++ {
			c := (flags >> 2) & 1
			h := (flags >> 1) & 1
			n := flags & 1

			result := byte(a)
			newC := c

			if n == 0 {
				// After ADD/ADC
				if c == 0 && h == 0 && (a&0x0F) <= 9 && a <= 0x99 {
					// No adjustment needed
				} else if c == 0 && h == 0 && (a&0x0F) >= 0x0A && a <= 0x99 {
					result += 0x06
				} else if c == 0 && h == 1 && (a&0x0F) <= 3 && a <= 0x99 {
					result += 0x06
				} else if c == 0 && h == 0 && (a&0x0F) <= 9 && a >= 0xA0 {
					result += 0x60
					newC = 1
				} else if c == 0 && h == 0 && (a&0x0F) >= 0x0A && a >= 0x9A {
					result += 0x66
					newC = 1
				} else if c == 0 && h == 1 && (a&0x0F) <= 3 && a >= 0xA0 {
					result += 0x66
					newC = 1
				} else if c == 1 && h == 0 && (a&0x0F) <= 9 {
					result += 0x60
					newC = 1
				} else if c == 1 && h == 0 && (a&0x0F) >= 0x0A {
					result += 0x66
					newC = 1
				} else if c == 1 && h == 1 && (a&0x0F) <= 3 {
					result += 0x66
					newC = 1
				}
			} else {
				// After SUB/SBC
				if c == 0 && h == 0 {
					// No adjustment
				} else if c == 0 && h == 1 && (a&0x0F) >= 6 {
					result -= 0x06
				} else if c == 1 && h == 0 {
					result -= 0x60
					newC = 1
				} else if c == 1 && h == 1 {
					result -= 0x66
					newC = 1
				}
			}

			// Build flags
			var f byte
			if result&0x80 != 0 {
				f |= 0x80 // S flag
			}
			if result == 0 {
				f |= 0x40 // Z flag
			}
			f |= result & 0x28 // Y (bit 5) and X (bit 3) from result
			if z80ParityTable[result] != 0 {
				f |= 0x04 // P/V = parity
			}
			if n != 0 {
				f |= 0x02 // N preserved
			}
			if newC != 0 {
				f |= 0x01 // C flag
			}
			// H flag for DAA is complex - simplified: set if lower nibble adjustment occurred
			if n == 0 {
				if h == 1 || (a&0x0F) >= 0x0A {
					f |= 0x10 // H
				}
			} else {
				if h == 1 && (a&0x0F) < 6 {
					f |= 0x10 // H
				}
			}

			idx := (a << 3) | flags
			z80DAATable[idx] = uint16(result)<<8 | uint16(f)
		}
	}
}

// ===========================================================================
// JITZ80Instr — Pre-decoded Z80 instruction for JIT compilation
// ===========================================================================

type JITZ80Instr struct {
	opcode       byte   // opcode byte (after prefix)
	prefix       byte   // 0=none, 0xCB, 0xDD, 0xFD, 0xED
	displacement int8   // signed offset for DD/FD indexed addressing
	operand      uint16 // immediate value or address
	hasOperand   bool   // true if operand field is valid
	length       byte   // total instruction length (1-4)
	pcOffset     uint16 // byte offset from block startPC
	cycles       byte   // base T-state cost
	cbSubOp      byte   // for DDCB/FDCB: the actual operation byte
	rIncrements  byte   // R register increments (1=unprefixed, 2=CB/DD/FD/ED, 3=DDCB/FDCB)
}

// Z80 prefix byte values for JIT scanner (distinct from cpu_z80.go's iota-based mode flags)
const (
	z80JITPrefixNone byte = 0x00
	z80JITPrefixCB   byte = 0xCB
	z80JITPrefixDD   byte = 0xDD
	z80JITPrefixFD   byte = 0xFD
	z80JITPrefixED   byte = 0xED
)

// Maximum block size in instructions
const z80JITMaxBlockSize = 128

// ===========================================================================
// Block Scanner
// ===========================================================================

// z80JITScanBlock scans Z80 instructions from raw memory starting at startPC,
// building a slice of pre-decoded instructions. Scanning stops at block
// terminators, fallback instructions, or the maximum block size.
// The caller must ensure startPC is on a direct page.
func z80JITScanBlock(mem []byte, startPC uint16, memSize int, directPageBitmap *[256]byte) []JITZ80Instr {
	instrs := make([]JITZ80Instr, 0, 32)
	pc := startPC
	startPage := startPC >> 8

	for len(instrs) < z80JITMaxBlockSize {
		if int(pc) >= memSize {
			break
		}

		opcode := mem[pc]
		var instr JITZ80Instr
		instr.pcOffset = pc - startPC
		instr.rIncrements = 1 // default: 1 fetchOpcode in Step

		switch opcode {
		case 0xCB:
			// CB prefix: 2 bytes total
			if int(pc)+1 >= memSize {
				return instrs
			}
			instr.prefix = z80JITPrefixCB
			instr.opcode = mem[pc+1]
			instr.length = 2
			instr.rIncrements = 2
			instr.cycles = 8 // most CB ops are 8 T-states (register), 15 for (HL)
			if instr.opcode&0x07 == 6 {
				instr.cycles = 15 // (HL) operand
				if instr.opcode >= 0x40 && instr.opcode <= 0x7F {
					instr.cycles = 12 // BIT b,(HL)
				}
			}

		case 0xDD, 0xFD:
			instr.prefix = opcode
			instr.rIncrements = 2
			if int(pc)+1 >= memSize {
				return instrs
			}
			nextOp := mem[pc+1]
			instr.opcode = nextOp

			if nextOp == 0xCB {
				// DDCB/FDCB: 4 bytes: prefix + CB + displacement + opcode
				if int(pc)+3 >= memSize {
					return instrs
				}
				instr.displacement = int8(mem[pc+2])
				instr.cbSubOp = mem[pc+3]
				instr.opcode = 0xCB // mark as CB sub-prefix
				instr.length = 4
				instr.rIncrements = 3
				instr.cycles = 23 // most DDCB ops
				if instr.cbSubOp >= 0x40 && instr.cbSubOp <= 0x7F {
					instr.cycles = 20 // BIT b,(IX+d)
				}
			} else {
				// DD/FD + opcode [+ displacement] [+ operand]
				instr.length = 2
				instr.cycles = 8 // default

				// Instructions that take a displacement byte (IX/IY+d)
				if z80DDFDHasDisplacement(nextOp) {
					if int(pc)+2 >= memSize {
						return instrs
					}
					instr.displacement = int8(mem[pc+2])
					instr.length = 3
					instr.cycles = 19 // most indexed operations

					// Instructions that also take an immediate after displacement
					if nextOp == 0x36 { // LD (IX+d),n
						if int(pc)+3 >= memSize {
							return instrs
						}
						instr.operand = uint16(mem[pc+3])
						instr.hasOperand = true
						instr.length = 4
						instr.cycles = 19
					}
				} else if z80DDFDHasImm16(nextOp) {
					if int(pc)+3 >= memSize {
						return instrs
					}
					instr.operand = uint16(mem[pc+2]) | uint16(mem[pc+3])<<8
					instr.hasOperand = true
					instr.length = 4
					instr.cycles = 14 // LD IX,nn
				} else if nextOp == 0xE9 {
					// JP (IX) / JP (IY) - just 2 bytes
					instr.cycles = 8
				} else if nextOp == 0xE5 || nextOp == 0xE1 {
					// PUSH IX / POP IX
					instr.cycles = 15
					if nextOp == 0xE1 {
						instr.cycles = 14
					}
				} else if nextOp == 0xF9 {
					// LD SP,IX
					instr.cycles = 10
				} else if nextOp == 0x09 || nextOp == 0x19 || nextOp == 0x29 || nextOp == 0x39 {
					// ADD IX,rp
					instr.cycles = 15
				}
			}

		case 0xED:
			instr.prefix = z80JITPrefixED
			instr.rIncrements = 2
			if int(pc)+1 >= memSize {
				return instrs
			}
			nextOp := mem[pc+1]
			instr.opcode = nextOp
			instr.length = 2
			instr.cycles = 8 // default

			// ED instructions with 16-bit operands
			if z80EDHasImm16(nextOp) {
				if int(pc)+3 >= memSize {
					return instrs
				}
				instr.operand = uint16(mem[pc+2]) | uint16(mem[pc+3])<<8
				instr.hasOperand = true
				instr.length = 4
				instr.cycles = 20
			} else {
				// Set cycles for common ED instructions
				switch nextOp {
				case 0x44: // NEG
					instr.cycles = 8
				case 0x46, 0x56, 0x5E: // IM 0/1/2
					instr.cycles = 8
				case 0x47, 0x4F: // LD I,A / LD R,A
					instr.cycles = 9
				case 0x57, 0x5F: // LD A,I / LD A,R
					instr.cycles = 9
				case 0x42, 0x52, 0x62, 0x72: // SBC HL,rp
					instr.cycles = 15
				case 0x4A, 0x5A, 0x6A, 0x7A: // ADC HL,rp
					instr.cycles = 15
				case 0x4D: // RETI
					instr.cycles = 14
				case 0x45, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D: // RETN
					instr.cycles = 14
				case 0xA0, 0xA8: // LDI / LDD
					instr.cycles = 16
				case 0xA1, 0xA9: // CPI / CPD
					instr.cycles = 16
				case 0xB0, 0xB8: // LDIR / LDDR
					instr.cycles = 21 // 21 when BC!=0, 16 when BC==0
				case 0xB1, 0xB9: // CPIR / CPDR
					instr.cycles = 21
				}
			}

		default:
			// Unprefixed instruction
			instr.prefix = z80JITPrefixNone
			instr.opcode = opcode
			instr.length = 1
			instr.cycles = z80BaseInstrCycles[opcode]

			// Handle 1-byte and 2-byte immediates / addresses
			switch {
			case z80BaseHasImm8(opcode):
				if int(pc)+1 >= memSize {
					return instrs
				}
				instr.operand = uint16(mem[pc+1])
				instr.hasOperand = true
				instr.length = 2
			case z80BaseHasImm16(opcode):
				if int(pc)+2 >= memSize {
					return instrs
				}
				instr.operand = uint16(mem[pc+1]) | uint16(mem[pc+2])<<8
				instr.hasOperand = true
				instr.length = 3
			case z80BaseHasRelJump(opcode):
				if int(pc)+1 >= memSize {
					return instrs
				}
				instr.operand = uint16(mem[pc+1]) // signed displacement stored as byte
				instr.hasOperand = true
				instr.length = 2
			}
		}

		// Page boundary safety: check if instruction crosses into a non-direct page
		endByte := pc + uint16(instr.length) - 1
		endPage := endByte >> 8
		if endPage != startPage && directPageBitmap[endPage] != 0 {
			break // stop before this instruction
		}

		// Check if this instruction needs fallback to interpreter
		if z80JITNeedsFallback(&instr) || !z80JITCanEmit(&instr) {
			if len(instrs) == 0 {
				// First instruction is fallback — return empty block
				// (exec loop will use interpretZ80One)
				return instrs
			}
			break // stop before this instruction
		}

		instrs = append(instrs, instr)

		pc += uint16(instr.length)

		// Check if this instruction is a block terminator
		if z80JITIsTerminator(&instr) {
			break
		}
	}

	return instrs
}

// ===========================================================================
// Instruction Classification Helpers
// ===========================================================================

// z80JITIsTerminator returns true if the instruction ends a basic block.
func z80JITIsTerminator(instr *JITZ80Instr) bool {
	switch instr.prefix {
	case z80JITPrefixNone:
		switch instr.opcode {
		case 0xC3: // JP nn
			return true
		case 0x18: // JR e
			return true
		case 0xE9: // JP (HL)
			return true
		case 0xC9: // RET
			return true
		case 0x76: // HALT
			return true
		case 0xCD: // CALL nn
			return true
		case 0xFB: // EI — block terminator for iffDelay correctness
			return true
		case 0xF3: // DI — block terminator for safety
			return true
		}
		// Conditional jumps: JP cc,nn
		if instr.opcode&0xC7 == 0xC2 {
			return true
		}
		// Conditional calls: CALL cc,nn
		if instr.opcode&0xC7 == 0xC4 {
			return true
		}
		// Conditional returns: RET cc
		if instr.opcode&0xC7 == 0xC0 {
			return true
		}
		// RST n
		if instr.opcode&0xC7 == 0xC7 {
			return true
		}
		// JR cc,e (0x20, 0x28, 0x30, 0x38)
		if instr.opcode == 0x20 || instr.opcode == 0x28 || instr.opcode == 0x30 || instr.opcode == 0x38 {
			return true
		}
		// DJNZ
		if instr.opcode == 0x10 {
			return true
		}

	case z80JITPrefixDD, z80JITPrefixFD:
		if instr.opcode == 0xE9 { // JP (IX) / JP (IY)
			return true
		}

	case z80JITPrefixED:
		switch instr.opcode {
		case 0x4D: // RETI
			return true
		case 0x45, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D: // RETN variants
			return true
		case 0xB0, 0xB8: // LDIR / LDDR — terminators (loop, must yield)
			return true
		case 0xB1, 0xB9: // CPIR / CPDR — terminators
			return true
		}
	}

	return false
}

// z80ResolveTerminatorTarget returns the static branch target for a
// region-eligible Z80 terminator. Only unconditional, statically-known
// transfers qualify:
//   - JP nn (0xC3, unprefixed): target = operand (16-bit absolute)
//   - JR e  (0x18, unprefixed): target = instrPC + 2 + sign_extend(disp)
//
// Conditional jumps (JP cc/JR cc/DJNZ) have two successors and are not
// followed by region formation. CALL/RET/RST + indirect transfers
// (JP (HL), JP (IX), JP (IY)) are unresolvable. EI/DI/HALT and the
// ED-prefixed RETI/RETN/LDIR/LDDR/CPIR/CPDR terminators all return
// (0, false). instrPC is the PC of the terminating instruction itself.
func z80ResolveTerminatorTarget(instr *JITZ80Instr, instrPC uint16) (uint16, bool) {
	if instr.prefix != z80JITPrefixNone {
		return 0, false
	}
	switch instr.opcode {
	case 0xC3: // JP nn
		return instr.operand, true
	case 0x18: // JR e
		disp := int8(instr.operand & 0xFF)
		return instrPC + 2 + uint16(int16(disp)), true
	}
	return 0, false
}

// z80JITNeedsFallback returns true if the instruction cannot be JIT-compiled.
func z80JITNeedsFallback(instr *JITZ80Instr) bool {
	switch instr.prefix {
	case z80JITPrefixNone:
		switch instr.opcode {
		case 0x76: // HALT
			return true
		case 0xDB: // IN A,(n)
			return true
		case 0xD3: // OUT (n),A
			return true
		case 0xE3: // EX (SP),HL
			return true
			// DAA now handled via lookup table
		}

	case z80JITPrefixDD, z80JITPrefixFD:
		if instr.opcode == 0xE3 { // EX (SP),IX / EX (SP),IY
			return true
		}

	case z80JITPrefixED:
		switch instr.opcode {
		case 0x67, 0x6F: // RLD / RRD
			return true
		case 0x57: // LD A,R — needs exact R value
			return true
		// Block I/O operations
		case 0xA2, 0xAA, 0xB2, 0xBA: // INI/IND/INIR/INDR
			return true
		case 0xA3, 0xAB, 0xB3, 0xBB: // OUTI/OUTD/OTIR/OTDR
			return true
		// Port I/O
		case 0x40, 0x48, 0x50, 0x58, 0x60, 0x68, 0x78: // IN r,(C)
			return true
		case 0x41, 0x49, 0x51, 0x59, 0x61, 0x69, 0x79: // OUT (C),r
			return true
		case 0x70: // IN F,(C) undocumented
			return true
		case 0x71: // OUT (C),0 undocumented
			return true
		}
	}

	return false
}

// z80JITCanEmit returns true if the emitter has native code generation for this instruction.
// Instructions that return false will end the block before them (interpreter fallback).
func z80JITCanEmit(instr *JITZ80Instr) bool {
	switch instr.prefix {
	case z80JITPrefixNone:
		op := instr.opcode
		switch {
		case op == 0x00: // NOP
			return true
		case op >= 0x40 && op <= 0x7F && op != 0x76: // LD r,r / LD r,(HL) / LD (HL),r
			return true
		case op&0xC7 == 0x06: // LD r,n / LD (HL),n
			return true
		case op >= 0x80 && op <= 0xBF: // ALU A,r including (HL)
			return true
		case op&0xC7 == 0x04 || op&0xC7 == 0x05: // INC/DEC r and INC/DEC (HL)
			return true
		case op&0xCF == 0x01: // LD rp,nn
			return true
		case op&0xCF == 0x03 || op&0xCF == 0x0B: // INC/DEC rp
			return true
		case op&0xCF == 0xC5 || op&0xCF == 0xC1: // PUSH/POP
			return true
		case op == 0x08 || op == 0xD9 || op == 0xEB: // EX AF/EXX/EX DE,HL
			return true
		case op&0xCF == 0x09: // ADD HL,rp
			return true
		case op == 0x0A || op == 0x1A || op == 0x02 || op == 0x12: // LD A,(BC)/(DE) / LD (BC)/(DE),A
			return true
		case op == 0x3A || op == 0x32: // LD A,(nn) / LD (nn),A
			return true
		case op == 0x2A: // LD HL,(nn)
			return true
		case op == 0x22: // LD (nn),HL
			return false
		case op == 0x37 || op == 0x3F || op == 0x2F: // SCF/CCF/CPL
			return true
		case op&0xC7 == 0xC6: // ALU A,n
			return true
		case op == 0x07 || op == 0x0F || op == 0x17 || op == 0x1F: // RLCA/RRCA/RLA/RRA
			return true
		case op == 0x27: // DAA
			return true
		case op == 0xF9: // LD SP,HL
			return true
		// Terminators: JP/JR/CALL/RET/RST/DJNZ/DI/EI/JP(HL)
		case op == 0xC3 || op == 0x18 || op == 0xE9: // JP nn / JR e / JP (HL)
			return true
		case op&0xC7 == 0xC2 || op&0xC7 == 0xC4 || op&0xC7 == 0xC0: // JP cc / CALL cc / RET cc
			return true
		case op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38: // JR cc
			return true
		case op == 0x10 || op == 0xCD || op == 0xC9 || op == 0xFB || op == 0xF3: // DJNZ/CALL/RET/EI/DI
			return true
		case op&0xC7 == 0xC7: // RST n
			return true
		}

	case z80JITPrefixCB:
		// All CB instructions handled: register operands + (HL) operand
		return true

	case z80JITPrefixDD, z80JITPrefixFD:
		op := instr.opcode
		switch {
		case op == 0x21: // LD IX,nn
			return true
		case op == 0x09 || op == 0x19 || op == 0x29 || op == 0x39: // ADD IX,rp
			return true
		case op == 0xF9: // LD SP,IX
			return true
		case op == 0xE5 || op == 0xE1: // PUSH/POP IX/IY
			return true
		case op == 0x23 || op == 0x2B: // INC/DEC IX
			return true
		case op&0xC7 == 0x46 && op != 0x76: // LD r,(IX+d)
			return true
		case op >= 0x70 && op <= 0x77 && op != 0x76: // LD (IX+d),r
			return true
		case op == 0x36: // LD (IX+d),n
			return true
		case op&0xC7 == 0x86: // ALU A,(IX+d)
			return true
		case op == 0x34 || op == 0x35: // INC/DEC (IX+d)
			return true
		case op == 0xE9: // JP (IX)
			return true
		case op == 0xCB: // DDCB/FDCB indexed bit operations
			return true
		}

	case z80JITPrefixED:
		op := instr.opcode
		switch op {
		case 0x44, 0x46, 0x56, 0x5E, 0x47, 0x4F, 0x5F: // NEG/IM/LD I,A/LD R,A/LD A,I
			return true
		case 0x42, 0x52, 0x62, 0x72: // SBC HL,rp
			return true
		case 0x4A, 0x5A, 0x6A, 0x7A: // ADC HL,rp
			return true
		case 0x43, 0x53, 0x63, 0x73: // LD (nn),rp
			return false
		case 0x4B, 0x5B, 0x6B, 0x7B: // LD rp,(nn)
			return true
		case 0xA0, 0xA8: // LDI / LDD
			return true
		case 0xA1, 0xA9: // CPI / CPD
			return true
		case 0xB0, 0xB8: // LDIR / LDDR (terminators)
			return true
		case 0xB1, 0xB9: // CPIR / CPDR (terminators)
			return true
		case 0x4D, 0x45, 0x55, 0x5D, 0x65, 0x6D, 0x75, 0x7D: // RETI/RETN (terminators)
			return true
		}
	}

	return false
}

// ===========================================================================
// Lazy Flag Peephole Pass
// ===========================================================================

// z80FlagAll is used by the peephole pass when all flags may be needed.
// Individual flag bit constants (z80FlagC, z80FlagZ, etc.) are in cpu_z80.go.
const z80FlagAll uint8 = 0xFF

// z80PeepholeFlags performs a backward scan over the instruction list and
// returns a parallel []uint8. flagsNeeded[i] is a bitmask of which Z80 flag
// bits are consumed by subsequent instructions before the next flag producer.
// 0x00 = no flags needed (skip materialization entirely).
// 0xFF = all flags needed (full materialization).
// Intermediate values enable partial materialization.
func z80PeepholeFlags(instrs []JITZ80Instr) []uint8 {
	n := len(instrs)
	flagsNeeded := make([]uint8, n)

	// F must be valid at every block exit (chain target might consume it).
	// Conservative: assume all flags needed at exit.
	needFlags := z80FlagAll

	// Scan backward: track which flags are needed by future consumers.
	for i := n - 1; i >= 0; i-- {
		instr := &instrs[i]

		consumed := z80InstrConsumedFlagMask(instr)
		if consumed != 0 {
			needFlags |= consumed
		}

		producedMask := z80InstrProducedFlagMask(instr)
		if producedMask != 0 {
			flagsNeeded[i] = needFlags
			// Only clear demand for bits this instruction actually writes.
			// Partial producers (INC/DEC r preserve C; CPL preserves S/Z/PV/C;
			// SCF/CCF preserve S/Z/PV; rotate accumulator preserves S/Z/PV;
			// DAA preserves N; ADD HL,rp preserves S/Z/PV; LDI/LDD/CPI/CPD/
			// LD A,I/R preserve C; CB BIT preserves C) keep upstream demand
			// for the preserved bits.
			needFlags &^= producedMask
		}
	}

	return flagsNeeded
}

// z80InstrProducedFlagMask returns a bitmask of which Z80 flag bits the
// instruction writes. 0 = no flag effect; 0xFF = all flags clobbered;
// partial values mark partial producers that preserve the unset bits.
// Used by z80PeepholeFlags so demand for preserved bits propagates
// upstream past partial producers.
func z80InstrProducedFlagMask(instr *JITZ80Instr) uint8 {
	// Full producers also overwrite undocumented Y/X copies of result bits.
	const fullMask uint8 = 0xFF
	// Partial: writes S/Z/H/PV/N, preserves C.
	const partialNoCarry = z80FlagS | z80FlagZ | z80FlagH | z80FlagPV | z80FlagN
	// Partial: writes H/N/C, preserves S/Z/PV (rotates, SCF, CCF, ADD HL,rp).
	const partialCarryOnly = z80FlagH | z80FlagN | z80FlagC
	// Partial: writes H/N, preserves S/Z/PV/C (CPL).
	const partialHN = z80FlagH | z80FlagN
	// Partial: writes S/Z/H/PV/C, preserves N (DAA).
	const partialNoN = z80FlagS | z80FlagZ | z80FlagH | z80FlagPV | z80FlagC
	// Partial: writes H/PV/N, preserves S/Z/C (LDI/LDD).
	const partialBlockMove = z80FlagH | z80FlagPV | z80FlagN

	switch instr.prefix {
	case z80JITPrefixNone:
		op := instr.opcode
		// 8-bit ALU A,r block (full).
		if op >= 0x80 && op <= 0xBF {
			return fullMask
		}
		if op&0xC7 == 0xC6 { // ALU A,n (full)
			return fullMask
		}
		// INC/DEC r — preserve C.
		if op&0xC7 == 0x04 || op&0xC7 == 0x05 {
			return partialNoCarry
		}
		switch op {
		case 0x07, 0x0F, 0x17, 0x1F: // RLCA/RRCA/RLA/RRA
			return partialCarryOnly
		case 0x27: // DAA — preserves N
			return partialNoN
		case 0x2F: // CPL — H/N
			return partialHN
		case 0x37, 0x3F: // SCF, CCF
			return partialCarryOnly
		case 0x09, 0x19, 0x29, 0x39: // ADD HL,rp
			return partialCarryOnly
		}
	case z80JITPrefixCB:
		op := instr.opcode
		// 00..3F: rotate/shift — full update.
		if op <= 0x3F {
			return fullMask
		}
		// 40..7F: BIT n,r — writes S/Z/H/PV/N, preserves C.
		if op <= 0x7F {
			return partialNoCarry
		}
		// 80..FF: RES/SET — no flag effect.
		return 0
	case z80JITPrefixED:
		op := instr.opcode
		switch {
		case op == 0x44: // NEG (full)
			return fullMask
		case op == 0x42 || op == 0x52 || op == 0x62 || op == 0x72: // SBC HL,ss
			return fullMask
		case op == 0x4A || op == 0x5A || op == 0x6A || op == 0x7A: // ADC HL,ss
			return fullMask
		case op == 0xA0 || op == 0xA8 || op == 0xB0 || op == 0xB8: // LDI/LDD/LDIR/LDDR
			return partialBlockMove
		case op == 0xA1 || op == 0xA9 || op == 0xB1 || op == 0xB9: // CPI/CPD/CPIR/CPDR
			return partialNoCarry
		case op == 0x57 || op == 0x5F: // LD A,I / LD A,R
			return partialNoCarry
		case op == 0x67 || op == 0x6F: // RRD / RLD
			return partialNoCarry
		}
	case z80JITPrefixDD, z80JITPrefixFD:
		op := instr.opcode
		if op&0xC7 == 0x86 { // ALU A,(IX+d)
			return fullMask
		}
		if op == 0x34 || op == 0x35 { // INC/DEC (IX+d)
			return partialNoCarry
		}
		if op == 0x09 || op == 0x19 || op == 0x29 || op == 0x39 { // ADD IX,rp
			return partialCarryOnly
		}
	}
	return 0
}

// z80InstrConsumedFlagMask returns a bitmask of which Z80 flag bits the
// instruction reads. Returns 0 if the instruction doesn't consume any flags.
func z80InstrConsumedFlagMask(instr *JITZ80Instr) uint8 {
	switch instr.prefix {
	case z80JITPrefixNone:
		op := instr.opcode
		// Conditional jumps/calls/returns read specific condition flags
		if op&0xC7 == 0xC2 || op&0xC7 == 0xC4 || op&0xC7 == 0xC0 { // JP cc / CALL cc / RET cc
			return z80ConditionFlagMask((op >> 3) & 0x07)
		}
		if op == 0x20 || op == 0x28 { // JR NZ / JR Z
			return z80FlagZ
		}
		if op == 0x30 || op == 0x38 { // JR NC / JR C
			return z80FlagC
		}
		// ADC/SBC read carry
		if (op >= 0x88 && op <= 0x8F) || (op >= 0x98 && op <= 0x9F) {
			return z80FlagC
		}
		if op == 0xCE || op == 0xDE { // ADC A,n / SBC A,n
			return z80FlagC
		}
		switch op {
		case 0x17, 0x1F: // RLA/RRA (read C)
			return z80FlagC
		case 0x3F: // CCF (reads C)
			return z80FlagC
		case 0x27: // DAA (reads N, H, C)
			return z80FlagN | z80FlagH | z80FlagC
		}
	case z80JITPrefixCB:
		if instr.opcode >= 0x10 && instr.opcode <= 0x1F { // RL/RR read carry
			return z80FlagC
		}
	case z80JITPrefixED:
		op := instr.opcode
		if op == 0x42 || op == 0x52 || op == 0x62 || op == 0x72 { // SBC HL
			return z80FlagC
		}
		if op == 0x4A || op == 0x5A || op == 0x6A || op == 0x7A { // ADC HL
			return z80FlagC
		}
	case z80JITPrefixDD, z80JITPrefixFD:
		if instr.opcode&0xC7 == 0x86 {
			aluOp := (instr.opcode >> 3) & 0x07
			if aluOp == 1 || aluOp == 3 { // ADC/SBC A,(IX+d)
				return z80FlagC
			}
		}
	}
	return 0
}

// z80ConditionFlagMask returns the flag bits needed for a Z80 condition code.
func z80ConditionFlagMask(cc uint8) uint8 {
	switch cc {
	case 0, 1: // NZ, Z
		return z80FlagZ
	case 2, 3: // NC, C
		return z80FlagC
	case 4, 5: // PO, PE
		return z80FlagPV
	case 6, 7: // P, M
		return z80FlagS
	}
	return z80FlagAll
}

// z80InstrProducesFlags returns true if the instruction modifies the F register.
func z80InstrProducesFlags(instr *JITZ80Instr) bool {
	switch instr.prefix {
	case z80JITPrefixNone:
		op := instr.opcode
		// ALU operations (0x80-0xBF, 0xC6-0xFE with &C7==C6)
		if op >= 0x80 && op <= 0xBF {
			return true
		}
		if op&0xC7 == 0xC6 { // ALU A,n
			return true
		}
		// INC/DEC r
		if op&0xC7 == 0x04 || op&0xC7 == 0x05 {
			return true
		}
		switch op {
		case 0x07, 0x0F, 0x17, 0x1F: // RLCA/RRCA/RLA/RRA
			return true
		case 0x27: // DAA
			return true
		case 0x2F: // CPL
			return true
		case 0x37, 0x3F: // SCF, CCF
			return true
		case 0x09, 0x19, 0x29, 0x39: // ADD HL,rp
			return true
		}
	case z80JITPrefixCB:
		return true // all CB ops affect flags
	case z80JITPrefixED:
		op := instr.opcode
		switch {
		case op == 0x44: // NEG
			return true
		case op == 0x42 || op == 0x52 || op == 0x62 || op == 0x72: // SBC HL
			return true
		case op == 0x4A || op == 0x5A || op == 0x6A || op == 0x7A: // ADC HL
			return true
		case op == 0xA0 || op == 0xA8 || op == 0xA1 || op == 0xA9: // LDI/LDD/CPI/CPD
			return true
		case op == 0x5F: // LD A,I
			return true
		}
	case z80JITPrefixDD, z80JITPrefixFD:
		op := instr.opcode
		if op&0xC7 == 0x86 { // ALU A,(IX+d)
			return true
		}
		if op == 0x34 || op == 0x35 { // INC/DEC (IX+d)
			return true
		}
		if op == 0x09 || op == 0x19 || op == 0x29 || op == 0x39 { // ADD IX,rp
			return true
		}
	}
	return false
}

// z80InstrConsumesFlags returns true if the instruction reads the F register.
func z80InstrConsumesFlags(instr *JITZ80Instr) bool {
	switch instr.prefix {
	case z80JITPrefixNone:
		op := instr.opcode
		// Conditional jumps/calls/returns
		if op&0xC7 == 0xC2 || op&0xC7 == 0xC4 || op&0xC7 == 0xC0 { // JP cc / CALL cc / RET cc
			return true
		}
		if op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38 { // JR cc
			return true
		}
		// ADC/SBC read carry
		if op >= 0x88 && op <= 0x8F { // ADC A,r
			return true
		}
		if op >= 0x98 && op <= 0x9F { // SBC A,r
			return true
		}
		if op == 0xCE || op == 0xDE { // ADC A,n / SBC A,n
			return true
		}
		switch op {
		case 0x17, 0x1F: // RLA/RRA (read C)
			return true
		case 0x3F: // CCF (reads C)
			return true
		case 0x27: // DAA (reads N, H, C)
			return true
		}
	case z80JITPrefixCB:
		if instr.opcode >= 0x10 && instr.opcode <= 0x1F { // RL/RR read carry
			return true
		}
	case z80JITPrefixED:
		op := instr.opcode
		if op == 0x42 || op == 0x52 || op == 0x62 || op == 0x72 { // SBC HL (reads C)
			return true
		}
		if op == 0x4A || op == 0x5A || op == 0x6A || op == 0x7A { // ADC HL (reads C)
			return true
		}
	case z80JITPrefixDD, z80JITPrefixFD:
		if instr.opcode&0xC7 == 0x86 {
			aluOp := (instr.opcode >> 3) & 0x07
			if aluOp == 1 || aluOp == 3 { // ADC/SBC A,(IX+d)
				return true
			}
		}
	}
	return false
}

// ===========================================================================
// DJNZ Loop Analysis (Phase C: memory specialization)
// ===========================================================================

// z80LoopInfo describes a DJNZ-style counted loop suitable for memory
// access optimization (page-check hoisting).
type z80LoopInfo struct {
	loopStart int  // index of first instruction in loop body
	loopEnd   int  // index of DJNZ (inclusive)
	readHL    bool // loop reads via (HL) — LD r,(HL), ALU A,(HL)
	writeHL   bool // loop writes via (HL) — LD (HL),r, LD (HL),n, INC/DEC (HL)
	readDE    bool // loop reads via (DE) — LD A,(DE)
	writeDE   bool // loop writes via (DE) — LD (DE),A
	readBC    bool // loop reads via (BC) — LD A,(BC)
	writeBC   bool // loop writes via (BC) — LD (BC),A
}

// z80AnalyzeDJNZLoop inspects a scanned instruction list for a qualifying
// DJNZ loop. Returns non-nil if the loop is safe for page-check hoisting:
//   - DJNZ is the last instruction and branches backward into the block
//   - Address registers used for memory access are only modified by INC rp
//     (monotonic increment only — DEC rp rejected, see plan C4)
//   - All memory ops use (HL), (DE), or (BC) addressing (no indexed IX/IY)
//   - No memory ops use absolute addressing (LD A,(nn), etc.)
func z80AnalyzeDJNZLoop(instrs []JITZ80Instr, startPC uint16) *z80LoopInfo {
	n := len(instrs)
	if n < 2 {
		return nil
	}

	// Last instruction must be DJNZ
	last := &instrs[n-1]
	if last.prefix != z80JITPrefixNone || last.opcode != 0x10 {
		return nil
	}

	// DJNZ target must be within this block (backward branch)
	target := int32(startPC) + int32(last.pcOffset) + 2 + int32(int8(last.operand&0xFF))
	if target < int32(startPC) {
		return nil // target before block start
	}

	// Find the loop start index
	loopStartIdx := -1
	targetOff := uint16(target) - startPC
	for i := range instrs {
		if instrs[i].pcOffset == targetOff {
			loopStartIdx = i
			break
		}
	}
	if loopStartIdx < 0 {
		return nil // target not found in block
	}

	info := &z80LoopInfo{
		loopStart: loopStartIdx,
		loopEnd:   n - 1,
	}

	// Track which pair registers are used for memory access and how they're modified
	hlModified := false // set if HL is used as memory pointer
	deModified := false
	bcModified := false
	hlInc := false // set if HL is INC'd (valid)
	deInc := false
	bcInc := false
	hlDec := false // set if HL is DEC'd (invalid — rejects loop)
	deDec := false
	bcDec := false
	hlOtherMod := false // set if HL is modified by something other than INC/DEC
	deOtherMod := false
	bcOtherMod := false

	for i := loopStartIdx; i < n-1; i++ { // exclude DJNZ itself
		instr := &instrs[i]
		if instr.prefix != z80JITPrefixNone {
			// DD/FD/ED/CB prefix instructions — reject if they access memory
			// through indexed addressing (IX+d, IY+d) since we can't hoist those checks
			if instr.prefix == z80JITPrefixDD || instr.prefix == z80JITPrefixFD {
				return nil // indexed memory ops not supported
			}
			// ED prefix: LDI/LDD/CPI/CPD use HL/DE/BC but are handled as fallback
			if instr.prefix == z80JITPrefixED {
				return nil // too complex
			}
			// CB prefix: BIT/SET/RES (HL) — we could support but skip for now
			if instr.prefix == z80JITPrefixCB && (instr.opcode&0x07 == 6) {
				info.readHL = true
				if instr.opcode >= 0xC0 { // SET/RES (HL) write back
					info.writeHL = true
				}
				hlModified = true
			}
			continue
		}

		op := instr.opcode

		// Check for memory-accessing instructions via (HL), (DE), (BC)
		switch {
		// LD r,(HL) — 0x46,0x4E,0x56,0x5E,0x66,0x6E,0x7E
		case op&0xC7 == 0x46 && op != 0x76:
			info.readHL = true
		// LD (HL),r — 0x70-0x75,0x77
		case op >= 0x70 && op <= 0x77 && op != 0x76:
			info.writeHL = true
		// LD (HL),n — 0x36
		case op == 0x36:
			info.writeHL = true
		// ALU A,(HL) — 0x86,0x8E,0x96,0x9E,0xA6,0xAE,0xB6,0xBE
		case op&0xC7 == 0x86:
			info.readHL = true
		// INC (HL) — 0x34
		case op == 0x34:
			info.readHL = true
			info.writeHL = true
		// DEC (HL) — 0x35
		case op == 0x35:
			info.readHL = true
			info.writeHL = true
		// LD A,(BC) — 0x0A
		case op == 0x0A:
			info.readBC = true
		// LD A,(DE) — 0x1A
		case op == 0x1A:
			info.readDE = true
		// LD (BC),A — 0x02
		case op == 0x02:
			info.writeBC = true
		// LD (DE),A — 0x12
		case op == 0x12:
			info.writeDE = true
		// LD A,(nn), LD (nn),A — absolute addressing, reject
		case op == 0x3A || op == 0x32:
			return nil
		}

		// Check for register pair modifications
		switch {
		// INC HL (0x23)
		case op == 0x23:
			hlInc = true
		// DEC HL (0x2B)
		case op == 0x2B:
			hlDec = true
		// INC DE (0x13)
		case op == 0x13:
			deInc = true
		// DEC DE (0x1B)
		case op == 0x1B:
			deDec = true
		// INC BC (0x03)
		case op == 0x03:
			bcInc = true
		// DEC BC (0x0B)
		case op == 0x0B:
			bcDec = true
		// LD HL,nn / LD DE,nn / LD BC,nn — reject (overwrites pointer)
		case op&0xCF == 0x01:
			pair := (op >> 4) & 0x03
			switch pair {
			case 0:
				bcOtherMod = true
			case 1:
				deOtherMod = true
			case 2:
				hlOtherMod = true
			}
		// EX DE,HL — swaps pointers, reject
		case op == 0xEB:
			return nil
		// PUSH/POP HL, PUSH/POP DE, PUSH/POP BC — don't modify the pair value for address purposes
		// (POP loads new value, which would invalidate the page proof)
		case op == 0xC1: // POP BC
			bcOtherMod = true
		case op == 0xD1: // POP DE
			deOtherMod = true
		case op == 0xE1: // POP HL
			hlOtherMod = true
		}
	}

	// Validate: address registers used for memory access must be
	// monotonic increment only (no DEC, no other modification).
	_ = hlModified
	_ = deModified
	_ = bcModified
	if (info.readHL || info.writeHL) && (hlDec || hlOtherMod) {
		return nil
	}
	if (info.readDE || info.writeDE) && (deDec || deOtherMod) {
		return nil
	}
	if (info.readBC || info.writeBC) && (bcDec || bcOtherMod) {
		return nil
	}

	// Must have at least one memory access to be worth optimizing
	if !info.readHL && !info.writeHL && !info.readDE && !info.writeDE && !info.readBC && !info.writeBC {
		return nil
	}

	// INC is optional (the register might be used as a fixed pointer)
	// but page-crossing guards are only needed for registers that INC
	_ = hlInc
	_ = deInc
	_ = bcInc

	return info
}

// ===========================================================================
// Instruction Length / Operand Helpers (unprefixed opcodes)
// ===========================================================================

// z80BaseHasImm8 returns true if the unprefixed opcode takes an 8-bit immediate.
func z80BaseHasImm8(op byte) bool {
	switch op {
	case 0x06, 0x0E, 0x16, 0x1E, 0x26, 0x2E, 0x36, 0x3E: // LD r,n / LD (HL),n
		return true
	case 0xC6, 0xCE, 0xD6, 0xDE, 0xE6, 0xEE, 0xF6, 0xFE: // ALU A,n
		return true
	case 0xDB, 0xD3: // IN A,(n) / OUT (n),A
		return true
	case 0x27: // DAA (no operand, but listed for completeness — actually no imm)
		return false
	}
	return false
}

// z80BaseHasImm16 returns true if the unprefixed opcode takes a 16-bit immediate/address.
func z80BaseHasImm16(op byte) bool {
	switch op {
	case 0x01, 0x11, 0x21, 0x31: // LD rp,nn
		return true
	case 0xC3: // JP nn
		return true
	case 0xCD: // CALL nn
		return true
	case 0x22, 0x2A: // LD (nn),HL / LD HL,(nn)
		return true
	case 0x32, 0x3A: // LD (nn),A / LD A,(nn)
		return true
	}
	// JP cc,nn (C2, CA, D2, DA, E2, EA, F2, FA)
	if op&0xC7 == 0xC2 {
		return true
	}
	// CALL cc,nn (C4, CC, D4, DC, E4, EC, F4, FC)
	if op&0xC7 == 0xC4 {
		return true
	}
	return false
}

// z80BaseHasRelJump returns true if the unprefixed opcode has a relative jump displacement.
func z80BaseHasRelJump(op byte) bool {
	switch op {
	case 0x10: // DJNZ
		return true
	case 0x18: // JR e
		return true
	case 0x20, 0x28, 0x30, 0x38: // JR cc,e
		return true
	}
	return false
}

// z80DDFDHasDisplacement returns true if the DD/FD prefixed opcode uses (IX/IY+d).
func z80DDFDHasDisplacement(op byte) bool {
	// LD r,(IX+d): 0x46, 0x4E, 0x56, 0x5E, 0x66, 0x6E, 0x7E
	if op&0xC7 == 0x46 && op != 0x76 {
		return true
	}
	// LD (IX+d),r: 0x70-0x75, 0x77
	if op >= 0x70 && op <= 0x77 && op != 0x76 {
		return true
	}
	// LD (IX+d),n: 0x36
	if op == 0x36 {
		return true
	}
	// ALU A,(IX+d): 0x86, 0x8E, 0x96, 0x9E, 0xA6, 0xAE, 0xB6, 0xBE
	if op&0xC7 == 0x86 {
		return true
	}
	// INC/DEC (IX+d): 0x34, 0x35
	if op == 0x34 || op == 0x35 {
		return true
	}
	return false
}

// z80DDFDHasImm16 returns true if the DD/FD prefixed opcode takes a 16-bit immediate.
func z80DDFDHasImm16(op byte) bool {
	switch op {
	case 0x21: // LD IX,nn
		return true
	case 0x22: // LD (nn),IX
		return true
	case 0x2A: // LD IX,(nn)
		return true
	}
	return false
}

// z80EDHasImm16 returns true if the ED prefixed opcode takes a 16-bit immediate.
func z80EDHasImm16(op byte) bool {
	switch op {
	case 0x43, 0x4B: // LD (nn),BC / LD BC,(nn)
		return true
	case 0x53, 0x5B: // LD (nn),DE / LD DE,(nn)
		return true
	case 0x63, 0x6B: // LD (nn),HL / LD HL,(nn)
		return true
	case 0x73, 0x7B: // LD (nn),SP / LD SP,(nn)
		return true
	}
	return false
}

// ===========================================================================
// Base Instruction Cycle Table
// ===========================================================================

// z80BaseInstrCycles stores the T-state cost for each unprefixed opcode.
// Conditional instructions use the "not taken" cycle count; the emitter
// adds the difference for the "taken" path.
var z80BaseInstrCycles = [256]byte{
	// 0x00-0x0F: NOP, LD BC,nn, LD (BC),A, INC BC, INC B, DEC B, LD B,n, RLCA,
	//            EX AF,AF', ADD HL,BC, LD A,(BC), DEC BC, INC C, DEC C, LD C,n, RRCA
	4, 10, 7, 6, 4, 4, 7, 4, 4, 11, 7, 6, 4, 4, 7, 4,
	// 0x10-0x1F: DJNZ, LD DE,nn, LD (DE),A, INC DE, INC D, DEC D, LD D,n, RLA,
	//            JR, ADD HL,DE, LD A,(DE), DEC DE, INC E, DEC E, LD E,n, RRA
	13, 10, 7, 6, 4, 4, 7, 4, 12, 11, 7, 6, 4, 4, 7, 4,
	// 0x20-0x2F: JR NZ, LD HL,nn, LD (nn),HL, INC HL, INC H, DEC H, LD H,n, DAA,
	//            JR Z, ADD HL,HL, LD HL,(nn), DEC HL, INC L, DEC L, LD L,n, CPL
	12, 10, 16, 6, 4, 4, 7, 4, 12, 11, 16, 6, 4, 4, 7, 4,
	// 0x30-0x3F: JR NC, LD SP,nn, LD (nn),A, INC SP, INC (HL), DEC (HL), LD (HL),n, SCF,
	//            JR C, ADD HL,SP, LD A,(nn), DEC SP, INC A, DEC A, LD A,n, CCF
	12, 10, 13, 6, 11, 11, 10, 4, 12, 11, 13, 6, 4, 4, 7, 4,
	// 0x40-0x4F: LD B,B through LD C,A (4 cycles each, except LD r,(HL) = 7)
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0x50-0x5F: LD D,B through LD E,A
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0x60-0x6F: LD H,B through LD L,A
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0x70-0x7F: LD (HL),B through LD A,A (0x76 = HALT = 4)
	7, 7, 7, 7, 7, 7, 4, 7, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0x80-0x8F: ADD A,r / ADC A,r (4 each, (HL) = 7)
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0x90-0x9F: SUB r / SBC A,r
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0xA0-0xAF: AND r / XOR r
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0xB0-0xBF: OR r / CP r
	4, 4, 4, 4, 4, 4, 7, 4, 4, 4, 4, 4, 4, 4, 7, 4,
	// 0xC0-0xCF: RET NZ, POP BC, JP NZ, JP nn, CALL NZ, PUSH BC, ADD A,n, RST 0,
	//            RET Z, RET, JP Z, (CB prefix), CALL Z, CALL nn, ADC A,n, RST 8
	11, 10, 10, 10, 10, 11, 7, 11, 11, 10, 10, 4, 10, 17, 7, 11,
	// 0xD0-0xDF: RET NC, POP DE, JP NC, OUT (n),A, CALL NC, PUSH DE, SUB n, RST 10,
	//            RET C, EXX, JP C, IN A,(n), CALL C, (DD prefix), SBC A,n, RST 18
	11, 10, 10, 11, 10, 11, 7, 11, 11, 4, 10, 11, 10, 4, 7, 11,
	// 0xE0-0xEF: RET PO, POP HL, JP PO, EX (SP),HL, CALL PO, PUSH HL, AND n, RST 20,
	//            RET PE, JP (HL), JP PE, EX DE,HL, CALL PE, (ED prefix), XOR n, RST 28
	11, 10, 10, 19, 10, 11, 7, 11, 11, 4, 10, 4, 10, 4, 7, 11,
	// 0xF0-0xFF: RET P, POP AF, JP P, DI, CALL P, PUSH AF, OR n, RST 30,
	//            RET M, LD SP,HL, JP M, EI, CALL M, (FD prefix), CP n, RST 38
	11, 10, 10, 4, 10, 11, 7, 11, 11, 6, 10, 4, 10, 4, 7, 11,
}

// ===========================================================================
// Context Construction
// ===========================================================================

// newZ80JITContext creates a new Z80JITContext for the given CPU.
// The adapter must have a valid MachineBus reference.
func newZ80JITContext(cpu *CPU_Z80, adapter *Z80BusAdapter) *Z80JITContext {
	if adapter == nil || adapter.bus == nil {
		return nil
	}
	mem := adapter.bus.GetMemory()
	if len(mem) == 0 {
		return nil
	}

	cpu.initDirectPageBitmapZ80(adapter)

	ctx := &Z80JITContext{
		MemPtr:              uintptr(unsafe.Pointer(&mem[0])),
		CpuPtr:              uintptr(unsafe.Pointer(cpu)),
		DirectPageBitmapPtr: uintptr(unsafe.Pointer(&cpu.directPageBitmap[0])),
		CodePageBitmapPtr:   uintptr(unsafe.Pointer(&cpu.codePageBitmap[0])),
		ParityTablePtr:      uintptr(unsafe.Pointer(&z80ParityTable[0])),
		DAATablePtr:         uintptr(unsafe.Pointer(&z80DAATable[0])),
	}
	return ctx
}

// initDirectPageBitmapZ80 computes which Z80 pages can use direct MachineBus
// memory access without going through Z80BusAdapter translation.
// Must be called AFTER MachineBus.SealMappings().
func (cpu *CPU_Z80) initDirectPageBitmapZ80(adapter *Z80BusAdapter) {
	bus := adapter.bus
	for page := 0; page < 256; page++ {
		addr := uint16(page) << 8
		direct := true

		// Z80-level non-direct ranges
		if page >= 0x20 && page <= 0x7F {
			direct = false // bank windows ($2000-$7FFF)
		}
		if page >= 0x80 && page <= 0xBF {
			direct = false // VRAM window ($8000-$BFFF)
		}
		if page >= 0xF0 {
			direct = false // I/O translation ($F000-$FFFF)
		}

		// MachineBus-level I/O check (catches MMIO handlers)
		if direct {
			mbPage := int(translateIO8Bit(addr) >> 8)
			if mbPage < len(bus.ioPageBitmap) && bus.ioPageBitmap[mbPage] {
				direct = false
			}
		}

		if direct {
			cpu.directPageBitmap[page] = 0
		} else {
			cpu.directPageBitmap[page] = 1
		}
	}
}

// ===========================================================================
// Block Compilation Stub (replaced by jit_z80_emit_amd64.go / arm64.go)
// ===========================================================================

// compileBlockZ80 compiles a scanned Z80 instruction block into native code.
// This is a placeholder that will be replaced by the architecture-specific emitter.
func compileBlockZ80(instrs []JITZ80Instr, startPC uint16, execMem *ExecMem, codePageBitmap *[256]byte) (*JITBlock, error) {
	// Calculate total R increments for the block
	totalR := 0
	for _, instr := range instrs {
		totalR += int(instr.rIncrements)
	}

	// Calculate end PC
	lastInstr := instrs[len(instrs)-1]
	endPC := startPC + lastInstr.pcOffset + uint16(lastInstr.length)

	// Mark code pages in bitmap
	for page := startPC >> 8; page <= (endPC-1)>>8; page++ {
		codePageBitmap[page] = 1
	}

	return compileBlockZ80Stub(instrs, startPC, endPC, execMem, totalR)
}

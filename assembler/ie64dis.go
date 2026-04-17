// ie64dis.go - IE64 Disassembler

//go:build ie64dis

/*
 ██▓ ███▄    █ ▄▄▄█████▓ █    ██  ██▓▄▄▄█████▓ ██▓ ▒█████   ███▄    █    ▓█████  ███▄    █   ▄████  ██▓ ███▄    █ ▓█████
▓██▒ ██ ▀█   █ ▓  ██▒ ▓▒ ██  ▓██▒▓██▒▓  ██▒ ▓▒▓██▒▒██▒  ██▒ ██ ▀█   █    ▓█   ▀  ██ ▀█   █  ██▒ ▀█▒▓██▒ ██ ▀█   █ ▓█   ▀
▒██▒▓██  ▀█ ██▒▒ ▓██░ ▒░▓██  ▒██░▒██▒▒ ▓██░ ▒░▒██▒▒██░  ██▒▓██  ▀█ ██▒   ▒███   ▓██  ▀█ ██▒▒██░▄▄▄░▒██▒▓██  ▀█ ██▒▒███
░██░▓██▒  ▐▌██▒░ ▓██▓ ░ ▓▓█  ░██░░██░░ ▓██▓ ░ ░██░▒██   ██░▓██▒  ▐▌██▒   ▒▓█  ▄ ▓██▒  ▐▌██▒░▓█  ██▓░██░▓██▒  ▐▌██▒▒▓█  ▄
░██░▒██░   ▓██░  ▒██▒ ░ ▒▒█████▓ ░██░  ▒██▒ ░ ░██░░ ████▓▒░▒██░   ▓██░   ░▒████▒▒██░   ▓██░░▒▓███▀▒░██░▒██░   ▓██░░▒████▒
░▓  ░ ▒░   ▒ ▒   ▒ ░░   ░▒▓▒ ▒ ▒ ░▓    ▒ ░░   ░▓  ░ ▒░▒░▒░ ░ ▒░   ▒ ▒    ░░ ▒░ ░░ ▒░   ▒ ▒  ░▒   ▒ ░▓  ░ ▒░   ▒ ▒ ░░ ▒░ ░
 ▒ ░░ ░░   ░ ▒░    ░    ░░▒░ ░ ░  ▒ ░    ░     ▒ ░  ░ ▒ ▒░ ░ ░░   ░ ▒░    ░ ░  ░░ ░░   ░ ▒░  ░   ░  ▒ ░░ ░░   ░ ▒░ ░ ░  ░
 ▒ ░   ░   ░ ░   ░       ░░░ ░ ░  ▒ ░  ░       ▒ ░░ ░ ░ ▒     ░   ░ ░       ░      ░   ░ ░ ░ ░   ░  ▒ ░   ░   ░ ░    ░
 ░           ░             ░      ░            ░      ░ ░           ░       ░  ░         ░       ░  ░           ░    ░  ░

IE64 Disassembler - 64-bit RISC CPU disassembler for the Intuition Engine
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import (
	"encoding/binary"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------
// Opcode constants (local to avoid cross-build-tag dependency)
// ---------------------------------------------------------------------
const (
	dis64_MOVE     = 0x01
	dis64_MOVT     = 0x02
	dis64_MOVEQ    = 0x03
	dis64_LEA      = 0x04
	dis64_LOAD     = 0x10
	dis64_STORE    = 0x11
	dis64_ADD      = 0x20
	dis64_SUB      = 0x21
	dis64_MULU     = 0x22
	dis64_MULS     = 0x23
	dis64_DIVU     = 0x24
	dis64_DIVS     = 0x25
	dis64_MOD      = 0x26
	dis64_NEG      = 0x27
	dis64_MODS     = 0x28
	dis64_MULHU    = 0x29
	dis64_MULHS    = 0x2A
	dis64_AND      = 0x30
	dis64_OR       = 0x31
	dis64_EOR      = 0x32
	dis64_NOT      = 0x33
	dis64_LSL      = 0x34
	dis64_LSR      = 0x35
	dis64_ASR      = 0x36
	dis64_CLZ      = 0x37
	dis64_SEXT     = 0x38
	dis64_ROL      = 0x39
	dis64_ROR      = 0x3A
	dis64_CTZ      = 0x3B
	dis64_POPCNT   = 0x3C
	dis64_BSWAP    = 0x3D
	dis64_BRA      = 0x40
	dis64_BEQ      = 0x41
	dis64_BNE      = 0x42
	dis64_BLT      = 0x43
	dis64_BGE      = 0x44
	dis64_BGT      = 0x45
	dis64_BLE      = 0x46
	dis64_BHI      = 0x47
	dis64_BLS      = 0x48
	dis64_JMP      = 0x49
	dis64_JSR      = 0x50
	dis64_RTS      = 0x51
	dis64_PUSH     = 0x52
	dis64_POP      = 0x53
	dis64_JSRI     = 0x54
	dis64_DMOV     = 0x80
	dis64_DLOAD    = 0x81
	dis64_DSTORE   = 0x82
	dis64_DADD     = 0x83
	dis64_DSUB     = 0x84
	dis64_DMUL     = 0x85
	dis64_DDIV     = 0x86
	dis64_DMOD     = 0x87
	dis64_DABS     = 0x88
	dis64_DNEG     = 0x89
	dis64_DSQRT    = 0x8A
	dis64_DINT     = 0x8B
	dis64_DCMP     = 0x8C
	dis64_DCVTIF   = 0x8D
	dis64_DCVTFI   = 0x8E
	dis64_FCVTSD   = 0x8F
	dis64_FCVTDS   = 0x90
	dis64_NOP      = 0xE0
	dis64_HALT     = 0xE1
	dis64_SEI      = 0xE2
	dis64_CLI      = 0xE3
	dis64_RTI      = 0xE4
	dis64_WAIT     = 0xE5
	dis64_MTCR     = 0xE6
	dis64_MFCR     = 0xE7
	dis64_ERET     = 0xE8
	dis64_TLBFLUSH = 0xE9
	dis64_TLBINVAL = 0xEA
	dis64_SYSCALL  = 0xEB
	dis64_SMODE    = 0xEC
	dis64_CAS      = 0xED
	dis64_XCHG     = 0xEE
	dis64_FAA      = 0xEF
	dis64_FAND     = 0xF0
	dis64_FOR      = 0xF1
	dis64_FXOR     = 0xF2
	dis64_SUAEN    = 0xF3
	dis64_SUADIS   = 0xF4
)

// Instruction size in bytes
const dis64InstrSize = 8

// ---------------------------------------------------------------------
// Opcode name table
// ---------------------------------------------------------------------

var opcodeNames = map[byte]string{
	dis64_MOVE:     "move",
	dis64_MOVT:     "movt",
	dis64_MOVEQ:    "moveq",
	dis64_LEA:      "lea",
	dis64_LOAD:     "load",
	dis64_STORE:    "store",
	dis64_ADD:      "add",
	dis64_SUB:      "sub",
	dis64_MULU:     "mulu",
	dis64_MULS:     "muls",
	dis64_DIVU:     "divu",
	dis64_DIVS:     "divs",
	dis64_MOD:      "mod",
	dis64_NEG:      "neg",
	dis64_MODS:     "mods",
	dis64_MULHU:    "mulhu",
	dis64_MULHS:    "mulhs",
	dis64_AND:      "and",
	dis64_OR:       "or",
	dis64_EOR:      "eor",
	dis64_NOT:      "not",
	dis64_LSL:      "lsl",
	dis64_LSR:      "lsr",
	dis64_ASR:      "asr",
	dis64_CLZ:      "clz",
	dis64_SEXT:     "sext",
	dis64_ROL:      "rol",
	dis64_ROR:      "ror",
	dis64_CTZ:      "ctz",
	dis64_POPCNT:   "popcnt",
	dis64_BSWAP:    "bswap",
	dis64_BRA:      "bra",
	dis64_BEQ:      "beq",
	dis64_BNE:      "bne",
	dis64_BLT:      "blt",
	dis64_BGE:      "bge",
	dis64_BGT:      "bgt",
	dis64_BLE:      "ble",
	dis64_BHI:      "bhi",
	dis64_BLS:      "bls",
	dis64_JMP:      "jmp",
	dis64_JSR:      "jsr",
	dis64_RTS:      "rts",
	dis64_PUSH:     "push",
	dis64_POP:      "pop",
	dis64_JSRI:     "jsr",
	dis64_DMOV:     "dmov",
	dis64_DLOAD:    "dload",
	dis64_DSTORE:   "dstore",
	dis64_DADD:     "dadd",
	dis64_DSUB:     "dsub",
	dis64_DMUL:     "dmul",
	dis64_DDIV:     "ddiv",
	dis64_DMOD:     "dmod",
	dis64_DABS:     "dabs",
	dis64_DNEG:     "dneg",
	dis64_DSQRT:    "dsqrt",
	dis64_DINT:     "dint",
	dis64_DCMP:     "dcmp",
	dis64_DCVTIF:   "dcvtif",
	dis64_DCVTFI:   "dcvtfi",
	dis64_FCVTSD:   "fcvtsd",
	dis64_FCVTDS:   "fcvtds",
	dis64_NOP:      "nop",
	dis64_HALT:     "halt",
	dis64_SEI:      "sei",
	dis64_CLI:      "cli",
	dis64_RTI:      "rti",
	dis64_WAIT:     "wait",
	dis64_MTCR:     "mtcr",
	dis64_MFCR:     "mfcr",
	dis64_ERET:     "eret",
	dis64_TLBFLUSH: "tlbflush",
	dis64_TLBINVAL: "tlbinval",
	dis64_SYSCALL:  "syscall",
	dis64_SMODE:    "smode",
	dis64_CAS:      "cas",
	dis64_XCHG:     "xchg",
	dis64_FAA:      "faa",
	dis64_FAND:     "fand",
	dis64_FOR:      "for",
	dis64_FXOR:     "fxor",
	dis64_SUAEN:    "suaen",
	dis64_SUADIS:   "suadis",
}

// ---------------------------------------------------------------------
// Size suffix table
// ---------------------------------------------------------------------

var sizeSuffix = [4]string{".b", ".w", ".l", ".q"}

// ---------------------------------------------------------------------
// Register name helper
// ---------------------------------------------------------------------

func regName(r byte) string {
	if r == 31 {
		return "sp"
	}
	return fmt.Sprintf("r%d", r)
}

// crName returns the symbolic name of a control register.
func crName(cr byte) string {
	switch cr {
	case 0:
		return "cr0"
	case 1:
		return "cr1"
	case 2:
		return "cr2"
	case 3:
		return "cr3"
	case 4:
		return "cr4"
	case 5:
		return "cr5"
	case 6:
		return "cr6"
	case 7:
		return "cr7"
	case 8:
		return "cr8"
	case 9:
		return "cr9"
	case 10:
		return "cr10"
	case 11:
		return "cr11"
	case 12:
		return "cr12"
	case 13:
		return "cr13"
	default:
		return fmt.Sprintf("cr%d", cr)
	}
}

// ---------------------------------------------------------------------
// DecodedInstruction holds the decoded fields of a single instruction
// ---------------------------------------------------------------------

type DecodedInstruction struct {
	PC     uint32
	Raw    [8]byte
	Opcode byte
	Rd     byte
	Size   byte
	Xbit   byte
	Rs     byte
	Rt     byte
	Imm32  uint32
}

// Decode decodes an 8-byte instruction at the given PC.
func Decode(data []byte, pc uint32) DecodedInstruction {
	var d DecodedInstruction
	d.PC = pc
	copy(d.Raw[:], data[:8])
	d.Opcode = data[0]
	byte1 := data[1]
	d.Rd = byte1 >> 3
	d.Size = (byte1 >> 1) & 0x03
	d.Xbit = byte1 & 1
	d.Rs = data[2] >> 3
	d.Rt = data[3] >> 3
	d.Imm32 = binary.LittleEndian.Uint32(data[4:8])
	return d
}

// ---------------------------------------------------------------------
// Instruction classification helpers
// ---------------------------------------------------------------------

func isBranch(op byte) bool {
	return op >= dis64_BRA && op <= dis64_JMP
}

func isConditionalBranch(op byte) bool {
	return op >= dis64_BEQ && op <= dis64_BLS
}

func isSized(op byte) bool {
	switch op {
	case dis64_NOP, dis64_HALT, dis64_SEI, dis64_CLI, dis64_RTI, dis64_WAIT,
		dis64_BRA, dis64_BEQ, dis64_BNE, dis64_BLT, dis64_BGE, dis64_BGT,
		dis64_BLE, dis64_BHI, dis64_BLS, dis64_JMP, dis64_JSR, dis64_RTS,
		dis64_MOVT, dis64_MOVEQ, dis64_LEA, dis64_PUSH, dis64_POP, dis64_JSRI,
		dis64_MULHU, dis64_MULHS,
		dis64_MTCR, dis64_MFCR, dis64_ERET, dis64_TLBFLUSH, dis64_TLBINVAL,
		dis64_SYSCALL, dis64_SMODE, dis64_SUAEN, dis64_SUADIS,
		dis64_CAS, dis64_XCHG, dis64_FAA, dis64_FAND, dis64_FOR, dis64_FXOR:
		return false
	}
	if op >= dis64_DMOV && op <= dis64_FCVTDS {
		return false
	}
	return true
}

// isALU3 returns true for three-register/immediate ALU operations.
func isALU3(op byte) bool {
	switch op {
	case dis64_ADD, dis64_SUB, dis64_MULU, dis64_MULS,
		dis64_DIVU, dis64_DIVS, dis64_MOD, dis64_MODS, dis64_MULHU, dis64_MULHS,
		dis64_AND, dis64_OR, dis64_EOR,
		dis64_LSL, dis64_LSR, dis64_ASR, dis64_ROL, dis64_ROR:
		return true
	}
	return false
}

// isUnaryALU returns true for two-operand ALU ops (Rd, Rs).
func isUnaryALU(op byte) bool {
	switch op {
	case dis64_NEG, dis64_NOT, dis64_CLZ, dis64_SEXT, dis64_CTZ, dis64_POPCNT, dis64_BSWAP:
		return true
	}
	return false
}

// ---------------------------------------------------------------------
// FormatInstruction formats a single decoded instruction as a string.
// It does NOT handle multi-instruction pseudo-ops (li 64-bit).
// Returns (hex bytes, mnemonic+operands).
// ---------------------------------------------------------------------

func FormatInstruction(d DecodedInstruction) (string, string) {
	hexBytes := fmt.Sprintf("%02X %02X %02X %02X %02X %02X %02X %02X",
		d.Raw[0], d.Raw[1], d.Raw[2], d.Raw[3],
		d.Raw[4], d.Raw[5], d.Raw[6], d.Raw[7])

	name, ok := opcodeNames[d.Opcode]
	if !ok {
		return hexBytes, fmt.Sprintf("dc.b $%02X  ; unknown opcode", d.Opcode)
	}

	suffix := ""
	if isSized(d.Opcode) {
		suffix = sizeSuffix[d.Size]
	}

	mnemonic := name + suffix

	switch {
	// System instructions with no operands
	case d.Opcode == dis64_NOP || d.Opcode == dis64_HALT ||
		d.Opcode == dis64_SEI || d.Opcode == dis64_CLI ||
		d.Opcode == dis64_RTI ||
		d.Opcode == dis64_ERET || d.Opcode == dis64_TLBFLUSH ||
		d.Opcode == dis64_SUAEN || d.Opcode == dis64_SUADIS:
		return hexBytes, mnemonic

	// RTS: no operands
	case d.Opcode == dis64_RTS:
		return hexBytes, mnemonic

	// WAIT: imm32 operand
	case d.Opcode == dis64_WAIT:
		return hexBytes, fmt.Sprintf("%s #%d", mnemonic, d.Imm32)

	// MTCR: cr#, Rs
	case d.Opcode == dis64_MTCR:
		return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, crName(d.Rd), regName(d.Rs))

	// MFCR: Rd, cr#
	case d.Opcode == dis64_MFCR:
		return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), crName(d.Rs))

	// TLBINVAL: Rs
	case d.Opcode == dis64_TLBINVAL:
		return hexBytes, fmt.Sprintf("%s %s", mnemonic, regName(d.Rs))

	// SYSCALL: #imm32
	case d.Opcode == dis64_SYSCALL:
		return hexBytes, fmt.Sprintf("%s #%d", mnemonic, d.Imm32)

	// SMODE: Rd
	case d.Opcode == dis64_SMODE:
		return hexBytes, fmt.Sprintf("%s %s", mnemonic, regName(d.Rd))

	// Atomic RMW: rd, disp(rs), rt
	case d.Opcode == dis64_CAS || d.Opcode == dis64_XCHG ||
		d.Opcode == dis64_FAA || d.Opcode == dis64_FAND ||
		d.Opcode == dis64_FOR || d.Opcode == dis64_FXOR:
		if d.Imm32 != 0 {
			return hexBytes, fmt.Sprintf("%s %s, %d(%s), %s", mnemonic,
				regName(d.Rd), int32(d.Imm32), regName(d.Rs), regName(d.Rt))
		}
		return hexBytes, fmt.Sprintf("%s %s, (%s), %s", mnemonic,
			regName(d.Rd), regName(d.Rs), regName(d.Rt))

	// MOVE: Rd, Rs or Rd, #imm
	case d.Opcode == dis64_MOVE:
		if d.Xbit == 1 {
			return hexBytes, fmt.Sprintf("%s %s, #$%X", mnemonic, regName(d.Rd), d.Imm32)
		}
		return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), regName(d.Rs))

	// MOVT: Rd, #imm32 (upper 32 bits)
	case d.Opcode == dis64_MOVT:
		return hexBytes, fmt.Sprintf("%s %s, #$%X", mnemonic, regName(d.Rd), d.Imm32)

	// MOVEQ: Rd, #imm32 (sign-extend)
	case d.Opcode == dis64_MOVEQ:
		return hexBytes, fmt.Sprintf("%s %s, #$%X", mnemonic, regName(d.Rd), d.Imm32)

	// LEA: Rd, disp(Rs)
	case d.Opcode == dis64_LEA:
		disp := int32(d.Imm32)
		if d.Rs == 0 {
			// Pseudo-op: la Rd, $addr
			return hexBytes, fmt.Sprintf("la %s, $%X", regName(d.Rd), d.Imm32)
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s %s, -%d(%s)", mnemonic, regName(d.Rd), -disp, regName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, regName(d.Rd), disp, regName(d.Rs))

	// LOAD: Rd, disp(Rs)
	case d.Opcode == dis64_LOAD:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s %s, (%s)", mnemonic, regName(d.Rd), regName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s %s, -%d(%s)", mnemonic, regName(d.Rd), -disp, regName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, regName(d.Rd), disp, regName(d.Rs))

	// STORE: Rd, disp(Rs)
	case d.Opcode == dis64_STORE:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s %s, (%s)", mnemonic, regName(d.Rd), regName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s %s, -%d(%s)", mnemonic, regName(d.Rd), -disp, regName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, regName(d.Rd), disp, regName(d.Rs))

	// Three-operand ALU: Rd, Rs, Rt/#imm
	case isALU3(d.Opcode):
		if d.Xbit == 1 {
			return hexBytes, fmt.Sprintf("%s %s, %s, #$%X", mnemonic, regName(d.Rd), regName(d.Rs), d.Imm32)
		}
		return hexBytes, fmt.Sprintf("%s %s, %s, %s", mnemonic, regName(d.Rd), regName(d.Rs), regName(d.Rt))

	// Unary ALU: Rd, Rs
	case isUnaryALU(d.Opcode):
		return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), regName(d.Rs))

	// BRA: unconditional branch
	case d.Opcode == dis64_BRA:
		target := uint32(int32(d.PC) + int32(d.Imm32))
		return hexBytes, fmt.Sprintf("%s $%06X", mnemonic, target)

	// Conditional branches: Rs, Rt, offset
	case isConditionalBranch(d.Opcode):
		target := uint32(int32(d.PC) + int32(d.Imm32))
		// Check for pseudo-ops: compare against r0
		if d.Rt == 0 {
			switch d.Opcode {
			case dis64_BEQ:
				return hexBytes, fmt.Sprintf("beqz %s, $%06X", regName(d.Rs), target)
			case dis64_BNE:
				return hexBytes, fmt.Sprintf("bnez %s, $%06X", regName(d.Rs), target)
			case dis64_BLT:
				return hexBytes, fmt.Sprintf("bltz %s, $%06X", regName(d.Rs), target)
			case dis64_BGE:
				return hexBytes, fmt.Sprintf("bgez %s, $%06X", regName(d.Rs), target)
			case dis64_BGT:
				return hexBytes, fmt.Sprintf("bgtz %s, $%06X", regName(d.Rs), target)
			case dis64_BLE:
				return hexBytes, fmt.Sprintf("blez %s, $%06X", regName(d.Rs), target)
			}
		}
		return hexBytes, fmt.Sprintf("%s %s, %s, $%06X", mnemonic, regName(d.Rs), regName(d.Rt), target)

	// JSR: PC-relative
	case d.Opcode == dis64_JSR:
		target := uint32(int32(d.PC) + int32(d.Imm32))
		return hexBytes, fmt.Sprintf("%s $%06X", mnemonic, target)

	// JMP: register-indirect
	case d.Opcode == dis64_JMP:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s (%s)", mnemonic, regName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s -%d(%s)", mnemonic, -disp, regName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %d(%s)", mnemonic, disp, regName(d.Rs))

	// JSR indirect: register-indirect
	case d.Opcode == dis64_JSRI:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s (%s)", mnemonic, regName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s -%d(%s)", mnemonic, -disp, regName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %d(%s)", mnemonic, disp, regName(d.Rs))

	// PUSH: Rs
	case d.Opcode == dis64_PUSH:
		return hexBytes, fmt.Sprintf("%s %s", mnemonic, regName(d.Rs))

	// POP: Rd
	case d.Opcode == dis64_POP:
		return hexBytes, fmt.Sprintf("%s %s", mnemonic, regName(d.Rd))

	case d.Opcode >= dis64_DMOV && d.Opcode <= dis64_FCVTDS:
		fr := func(r byte) string { return fmt.Sprintf("f%d", r) }
		switch d.Opcode {
		case dis64_DMOV:
			return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
		case dis64_DLOAD, dis64_DSTORE:
			disp := int32(d.Imm32)
			if disp == 0 {
				return hexBytes, fmt.Sprintf("%s %s, (%s)", mnemonic, fr(d.Rd), regName(d.Rs))
			}
			return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, fr(d.Rd), disp, regName(d.Rs))
		case dis64_DADD, dis64_DSUB, dis64_DMUL, dis64_DDIV, dis64_DMOD:
			return hexBytes, fmt.Sprintf("%s %s, %s, %s", mnemonic, fr(d.Rd), fr(d.Rs), fr(d.Rt))
		case dis64_DABS, dis64_DNEG, dis64_DSQRT, dis64_DINT:
			return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
		case dis64_DCMP:
			return hexBytes, fmt.Sprintf("%s %s, %s, %s", mnemonic, regName(d.Rd), fr(d.Rs), fr(d.Rt))
		case dis64_DCVTIF:
			return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), regName(d.Rs))
		case dis64_DCVTFI:
			return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), fr(d.Rs))
		case dis64_FCVTSD, dis64_FCVTDS:
			return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s ???", mnemonic)

	default:
		return hexBytes, fmt.Sprintf("%s ???", mnemonic)
	}
}

// ---------------------------------------------------------------------
// Disassemble processes an entire binary and returns formatted lines.
// It recognizes multi-instruction pseudo-ops like li (move.l + movt).
// ---------------------------------------------------------------------

func Disassemble(data []byte, baseAddr uint32) []string {
	var lines []string
	offset := 0
	for offset+dis64InstrSize <= len(data) {
		pc := baseAddr + uint32(offset)
		d := Decode(data[offset:], pc)

		// Check for li pseudo-op: move.l Rd, #lo32 followed by movt Rd, #hi32
		if d.Opcode == dis64_MOVE && d.Xbit == 1 && d.Size == 2 &&
			offset+2*dis64InstrSize <= len(data) {
			next := Decode(data[offset+dis64InstrSize:], pc+dis64InstrSize)
			if next.Opcode == dis64_MOVT && next.Rd == d.Rd {
				lo := uint64(d.Imm32)
				hi := uint64(next.Imm32) << 32
				combined := hi | lo

				hexBytes1 := fmt.Sprintf("%02X %02X %02X %02X %02X %02X %02X %02X",
					d.Raw[0], d.Raw[1], d.Raw[2], d.Raw[3],
					d.Raw[4], d.Raw[5], d.Raw[6], d.Raw[7])
				hexBytes2 := fmt.Sprintf("%02X %02X %02X %02X %02X %02X %02X %02X",
					next.Raw[0], next.Raw[1], next.Raw[2], next.Raw[3],
					next.Raw[4], next.Raw[5], next.Raw[6], next.Raw[7])

				lines = append(lines,
					fmt.Sprintf("$%06X: %s    li %s, #$%X", pc, hexBytes1, regName(d.Rd), combined))
				lines = append(lines,
					fmt.Sprintf("$%06X: %s     ; (movt %s, #$%X)", pc+dis64InstrSize, hexBytes2, regName(next.Rd), next.Imm32))
				offset += 2 * dis64InstrSize
				continue
			}
		}

		hexBytes, asm := FormatInstruction(d)
		lines = append(lines, fmt.Sprintf("$%06X: %s    %s", pc, hexBytes, asm))
		offset += dis64InstrSize
	}

	// Handle trailing bytes that don't form a complete instruction
	if offset < len(data) {
		remaining := len(data) - offset
		pc := baseAddr + uint32(offset)
		var hexParts []string
		for i := 0; i < remaining; i++ {
			hexParts = append(hexParts, fmt.Sprintf("%02X", data[offset+i]))
		}
		lines = append(lines, fmt.Sprintf("$%06X: %-23s    dc.b %s  ; trailing bytes",
			pc, strings.Join(hexParts, " "),
			strings.Join(hexParts, ", $")))
	}

	return lines
}

// ---------------------------------------------------------------------
// CLI entry point
// ---------------------------------------------------------------------

func main() {
	baseAddr := uint32(0x1000)
	args := os.Args[1:]

	if len(args) == 0 {
		fmt.Fprintf(os.Stderr, "IE64 Disassembler\n")
		fmt.Fprintf(os.Stderr, "Usage: ie64dis [-base $ADDR] file.ie64\n")
		os.Exit(1)
	}

	// Parse arguments
	var filename string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-base":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "Error: -base requires an address argument\n")
				os.Exit(1)
			}
			i++
			addrStr := args[i]
			// Strip leading $ for hex
			if strings.HasPrefix(addrStr, "$") {
				addrStr = addrStr[1:]
			} else if strings.HasPrefix(addrStr, "0x") || strings.HasPrefix(addrStr, "0X") {
				addrStr = addrStr[2:]
			}
			val, err := strconv.ParseUint(addrStr, 16, 32)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: invalid base address '%s': %v\n", args[i], err)
				os.Exit(1)
			}
			baseAddr = uint32(val)
		default:
			filename = args[i]
		}
	}

	if filename == "" {
		fmt.Fprintf(os.Stderr, "Error: no input file specified\n")
		os.Exit(1)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading '%s': %v\n", filename, err)
		os.Exit(1)
	}

	if len(data) == 0 {
		fmt.Fprintf(os.Stderr, "Warning: empty file '%s'\n", filename)
		os.Exit(0)
	}

	lines := Disassemble(data, baseAddr)
	for _, line := range lines {
		fmt.Println(line)
	}
}

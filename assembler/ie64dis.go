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
// Opcode constants are generated from internal/ie64meta.
// ---------------------------------------------------------------------

// Instruction size in bytes
const dis64InstrSize = 8

// ---------------------------------------------------------------------
// Opcode names are generated from internal/ie64meta.
// ---------------------------------------------------------------------

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
	return fmt.Sprintf("cr%d", cr)
}

// ---------------------------------------------------------------------
// DecodedInstruction holds the decoded fields of a single instruction
// ---------------------------------------------------------------------

type DecodedInstruction struct {
	PC     uint64
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
func Decode(data []byte, pc uint64) DecodedInstruction {
	var d DecodedInstruction
	d.PC = pc
	if len(data) < dis64InstrSize {
		return d
	}
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
	if op >= dis64_FMOV && op <= dis64_FMOVCC {
		return false
	}
	if op >= dis64_DMOV && op <= dis64_DPOW {
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
// Returns (hex bytes, mnemonic+operands).
// ---------------------------------------------------------------------

func FormatInstruction(d DecodedInstruction) (string, string) {
	hexBytes := fmt.Sprintf("%02X %02X %02X %02X %02X %02X %02X %02X",
		d.Raw[0], d.Raw[1], d.Raw[2], d.Raw[3],
		d.Raw[4], d.Raw[5], d.Raw[6], d.Raw[7])

	name, ok := opcodeNames[d.Opcode]
	if !ok {
		return hexBytes, fmt.Sprintf("dc.b $%02X, $%02X, $%02X, $%02X, $%02X, $%02X, $%02X, $%02X  ; unknown opcode",
			d.Raw[0], d.Raw[1], d.Raw[2], d.Raw[3], d.Raw[4], d.Raw[5], d.Raw[6], d.Raw[7])
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
		if int32(d.Imm32) < 0 {
			return hexBytes, fmt.Sprintf("%s %s, #%d", mnemonic, regName(d.Rd), int32(d.Imm32))
		}
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
		target := uint64(int64(d.PC) + int64(int32(d.Imm32)))
		return hexBytes, fmt.Sprintf("%s $%06X", mnemonic, target)

	// Conditional branches: Rs, Rt, offset
	case isConditionalBranch(d.Opcode):
		target := uint64(int64(d.PC) + int64(int32(d.Imm32)))
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
		target := uint64(int64(d.PC) + int64(int32(d.Imm32)))
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

	case d.Opcode >= dis64_FMOV && d.Opcode <= dis64_FMOVCC:
		return hexBytes, formatFPU(d, mnemonic)

	case d.Opcode >= dis64_DMOV && d.Opcode <= dis64_DPOW:
		return hexBytes, formatFPU(d, mnemonic)

	default:
		return hexBytes, fmt.Sprintf("%s ???", mnemonic)
	}
}

func formatFPU(d DecodedInstruction, mnemonic string) string {
	fr := func(r byte) string { return fmt.Sprintf("f%d", r) }
	switch d.Opcode {
	case dis64_FMOV:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	case dis64_FLOAD, dis64_FSTORE:
		disp := int32(d.Imm32)
		if disp == 0 {
			return fmt.Sprintf("%s %s, (%s)", mnemonic, fr(d.Rd), regName(d.Rs))
		}
		return fmt.Sprintf("%s %s, %d(%s)", mnemonic, fr(d.Rd), disp, regName(d.Rs))
	case dis64_FADD, dis64_FSUB, dis64_FMUL, dis64_FDIV, dis64_FMOD, dis64_FPOW:
		return fmt.Sprintf("%s %s, %s, %s", mnemonic, fr(d.Rd), fr(d.Rs), fr(d.Rt))
	case dis64_FABS, dis64_FNEG, dis64_FSQRT, dis64_FINT,
		dis64_FSIN, dis64_FCOS, dis64_FTAN, dis64_FATAN, dis64_FLOG, dis64_FEXP:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	case dis64_FCMP:
		return fmt.Sprintf("%s %s, %s, %s", mnemonic, regName(d.Rd), fr(d.Rs), fr(d.Rt))
	case dis64_FCVTIF:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), regName(d.Rs))
	case dis64_FCVTFI:
		return fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), fr(d.Rs))
	case dis64_FMOVI:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), regName(d.Rs))
	case dis64_FMOVO:
		return fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), fr(d.Rs))
	case dis64_FMOVECR:
		return fmt.Sprintf("%s %s, #%d", mnemonic, fr(d.Rd), d.Imm32)
	case dis64_FMOVSR, dis64_FMOVCR:
		return fmt.Sprintf("%s %s", mnemonic, regName(d.Rd))
	case dis64_FMOVSC, dis64_FMOVCC:
		return fmt.Sprintf("%s %s", mnemonic, regName(d.Rs))
	case dis64_DMOV:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	case dis64_DLOAD, dis64_DSTORE:
		disp := int32(d.Imm32)
		if disp == 0 {
			return fmt.Sprintf("%s %s, (%s)", mnemonic, fr(d.Rd), regName(d.Rs))
		}
		return fmt.Sprintf("%s %s, %d(%s)", mnemonic, fr(d.Rd), disp, regName(d.Rs))
	case dis64_DADD, dis64_DSUB, dis64_DMUL, dis64_DDIV, dis64_DMOD, dis64_DPOW:
		return fmt.Sprintf("%s %s, %s, %s", mnemonic, fr(d.Rd), fr(d.Rs), fr(d.Rt))
	case dis64_DABS, dis64_DNEG, dis64_DSQRT, dis64_DINT,
		dis64_DSIN, dis64_DCOS, dis64_DTAN, dis64_DATAN, dis64_DLOG, dis64_DEXP:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	case dis64_DCMP:
		return fmt.Sprintf("%s %s, %s, %s", mnemonic, regName(d.Rd), fr(d.Rs), fr(d.Rt))
	case dis64_DCVTIF:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), regName(d.Rs))
	case dis64_DCVTFI:
		return fmt.Sprintf("%s %s, %s", mnemonic, regName(d.Rd), fr(d.Rs))
	case dis64_FCVTSD, dis64_FCVTDS:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	default:
		return fmt.Sprintf("%s ???", mnemonic)
	}
}

// ---------------------------------------------------------------------
// Disassemble processes an entire binary and returns formatted lines.
// It recognizes multi-instruction pseudo-ops like li (move.l + movt).
// ---------------------------------------------------------------------

func Disassemble(data []byte, baseAddr uint64) []string {
	var lines []string
	offset := 0
	for offset+dis64InstrSize <= len(data) {
		pc := baseAddr + uint64(offset)
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
		pc := baseAddr + uint64(offset)
		var hexParts, dcParts []string
		for i := 0; i < remaining; i++ {
			hexParts = append(hexParts, fmt.Sprintf("%02X", data[offset+i]))
			dcParts = append(dcParts, fmt.Sprintf("$%02X", data[offset+i]))
		}
		lines = append(lines, fmt.Sprintf("$%06X: %-23s    dc.b %s  ; trailing bytes",
			pc, strings.Join(hexParts, " "),
			strings.Join(dcParts, ", ")))
	}

	return lines
}

// ---------------------------------------------------------------------
// CLI entry point
// ---------------------------------------------------------------------

func main() {
	filename, baseAddr, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "IE64 Disassembler\n")
		fmt.Fprintf(os.Stderr, "Usage: ie64dis [-base $ADDR] file.ie64\n")
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
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

func parseArgs(args []string) (string, uint64, error) {
	baseAddr := uint64(0x1000)
	var filename string

	if len(args) == 0 {
		return "", 0, fmt.Errorf("no input file specified")
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-base":
			if i+1 >= len(args) {
				return "", 0, fmt.Errorf("-base requires an address argument")
			}
			i++
			addrStr := args[i]
			if strings.HasPrefix(addrStr, "$") {
				addrStr = addrStr[1:]
			} else if strings.HasPrefix(addrStr, "0x") || strings.HasPrefix(addrStr, "0X") {
				addrStr = addrStr[2:]
			}
			val, err := strconv.ParseUint(addrStr, 16, 64)
			if err != nil {
				return "", 0, fmt.Errorf("invalid base address %q: %w", args[i], err)
			}
			baseAddr = val
		default:
			if filename != "" {
				return "", 0, fmt.Errorf("multiple input files specified: %q and %q", filename, args[i])
			}
			filename = args[i]
		}
	}

	if filename == "" {
		return "", 0, fmt.Errorf("no input file specified")
	}
	return filename, baseAddr, nil
}

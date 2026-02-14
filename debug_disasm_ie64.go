// debug_disasm_ie64.go - IE64 disassembler for Machine Monitor

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

(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later
*/

package main

import "fmt"

var ie64OpcodeNames = map[byte]string{
	OP_MOVE: "move", OP_MOVT: "movt", OP_MOVEQ: "moveq", OP_LEA: "lea",
	OP_LOAD: "load", OP_STORE: "store",
	OP_ADD: "add", OP_SUB: "sub", OP_MULU: "mulu", OP_MULS: "muls",
	OP_DIVU: "divu", OP_DIVS: "divs", OP_MOD64: "mod", OP_NEG: "neg",
	OP_AND64: "and", OP_OR64: "or", OP_EOR: "eor", OP_NOT64: "not",
	OP_LSL: "lsl", OP_LSR: "lsr", OP_ASR: "asr", OP_CLZ: "clz",
	OP_BRA: "bra", OP_BEQ: "beq", OP_BNE: "bne", OP_BLT: "blt",
	OP_BGE: "bge", OP_BGT: "bgt", OP_BLE: "ble", OP_BHI: "bhi",
	OP_BLS: "bls", OP_JMP: "jmp",
	OP_JSR64: "jsr", OP_RTS64: "rts", OP_PUSH64: "push", OP_POP64: "pop",
	OP_JSR_IND: "jsr",
	OP_FMOV:    "fmov", OP_FLOAD: "fload", OP_FSTORE: "fstore",
	OP_FADD: "fadd", OP_FSUB: "fsub", OP_FMUL: "fmul", OP_FDIV: "fdiv",
	OP_FMOD: "fmod", OP_FABS: "fabs", OP_FNEG: "fneg", OP_FSQRT: "fsqrt",
	OP_FINT: "fint", OP_FCMP: "fcmp", OP_FCVTIF: "fcvtif", OP_FCVTFI: "fcvtfi",
	OP_FMOVI: "fmovi", OP_FMOVO: "fmovo",
	OP_FSIN: "fsin", OP_FCOS: "fcos", OP_FTAN: "ftan", OP_FATAN: "fatan",
	OP_FLOG: "flog", OP_FEXP: "fexp", OP_FPOW: "fpow",
	OP_FMOVECR: "fmovecr", OP_FMOVSR: "fmovsr", OP_FMOVCR: "fmovcr",
	OP_FMOVSC: "fmovsc", OP_FMOVCC: "fmovcc",
	OP_NOP64: "nop", OP_HALT64: "halt", OP_SEI64: "sei", OP_CLI64: "cli",
	OP_RTI64: "rti", OP_WAIT64: "wait",
}

var ie64SizeSuffix = [4]string{".b", ".w", ".l", ".q"}

func ie64RegName(r byte) string {
	if r == 31 {
		return "sp"
	}
	return fmt.Sprintf("r%d", r)
}

func ie64IsBranch(op byte) bool {
	return op >= OP_BRA && op <= OP_JMP
}

func ie64IsConditionalBranch(op byte) bool {
	return op >= OP_BEQ && op <= OP_BLS
}

func ie64IsSized(op byte) bool {
	switch op {
	case OP_NOP64, OP_HALT64, OP_SEI64, OP_CLI64, OP_RTI64, OP_WAIT64,
		OP_BRA, OP_BEQ, OP_BNE, OP_BLT, OP_BGE, OP_BGT,
		OP_BLE, OP_BHI, OP_BLS, OP_JMP, OP_JSR64, OP_RTS64,
		OP_MOVT, OP_MOVEQ, OP_LEA, OP_PUSH64, OP_POP64, OP_JSR_IND:
		return false
	}
	// FPU instructions are not sized in the same way
	if op >= OP_FMOV && op <= OP_FMOVCC {
		return false
	}
	return true
}

func ie64IsALU3(op byte) bool {
	switch op {
	case OP_ADD, OP_SUB, OP_MULU, OP_MULS,
		OP_DIVU, OP_DIVS, OP_MOD64,
		OP_AND64, OP_OR64, OP_EOR,
		OP_LSL, OP_LSR, OP_ASR:
		return true
	}
	return false
}

func ie64IsUnaryALU(op byte) bool {
	return op == OP_NEG || op == OP_NOT64 || op == OP_CLZ
}

type ie64Decoded struct {
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

func ie64Decode(data []byte, pc uint64) ie64Decoded {
	var d ie64Decoded
	d.PC = pc
	copy(d.Raw[:], data[:8])
	d.Opcode = data[0]
	byte1 := data[1]
	d.Rd = byte1 >> 3
	d.Size = (byte1 >> 1) & 0x03
	d.Xbit = byte1 & 1
	d.Rs = data[2] >> 3
	d.Rt = data[3] >> 3
	d.Imm32 = uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16 | uint32(data[7])<<24
	return d
}

func ie64FormatInstruction(d ie64Decoded) (string, string) {
	hexBytes := fmt.Sprintf("%02X %02X %02X %02X %02X %02X %02X %02X",
		d.Raw[0], d.Raw[1], d.Raw[2], d.Raw[3],
		d.Raw[4], d.Raw[5], d.Raw[6], d.Raw[7])

	name, ok := ie64OpcodeNames[d.Opcode]
	if !ok {
		return hexBytes, fmt.Sprintf("dc.b $%02X", d.Opcode)
	}

	suffix := ""
	if ie64IsSized(d.Opcode) {
		suffix = ie64SizeSuffix[d.Size]
	}
	mnemonic := name + suffix

	switch {
	case d.Opcode == OP_NOP64 || d.Opcode == OP_HALT64 ||
		d.Opcode == OP_SEI64 || d.Opcode == OP_CLI64 ||
		d.Opcode == OP_RTI64:
		return hexBytes, mnemonic

	case d.Opcode == OP_RTS64:
		return hexBytes, mnemonic

	case d.Opcode == OP_WAIT64:
		return hexBytes, fmt.Sprintf("%s #%d", mnemonic, d.Imm32)

	case d.Opcode == OP_MOVE:
		if d.Xbit == 1 {
			return hexBytes, fmt.Sprintf("%s %s, #$%X", mnemonic, ie64RegName(d.Rd), d.Imm32)
		}
		return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, ie64RegName(d.Rd), ie64RegName(d.Rs))

	case d.Opcode == OP_MOVT:
		return hexBytes, fmt.Sprintf("%s %s, #$%X", mnemonic, ie64RegName(d.Rd), d.Imm32)

	case d.Opcode == OP_MOVEQ:
		return hexBytes, fmt.Sprintf("%s %s, #$%X", mnemonic, ie64RegName(d.Rd), d.Imm32)

	case d.Opcode == OP_LEA:
		disp := int32(d.Imm32)
		if d.Rs == 0 {
			return hexBytes, fmt.Sprintf("la %s, $%X", ie64RegName(d.Rd), d.Imm32)
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s %s, -%d(%s)", mnemonic, ie64RegName(d.Rd), -disp, ie64RegName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, ie64RegName(d.Rd), disp, ie64RegName(d.Rs))

	case d.Opcode == OP_LOAD:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s %s, (%s)", mnemonic, ie64RegName(d.Rd), ie64RegName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s %s, -%d(%s)", mnemonic, ie64RegName(d.Rd), -disp, ie64RegName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, ie64RegName(d.Rd), disp, ie64RegName(d.Rs))

	case d.Opcode == OP_STORE:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s %s, (%s)", mnemonic, ie64RegName(d.Rd), ie64RegName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s %s, -%d(%s)", mnemonic, ie64RegName(d.Rd), -disp, ie64RegName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %s, %d(%s)", mnemonic, ie64RegName(d.Rd), disp, ie64RegName(d.Rs))

	case ie64IsALU3(d.Opcode):
		if d.Xbit == 1 {
			return hexBytes, fmt.Sprintf("%s %s, %s, #$%X", mnemonic, ie64RegName(d.Rd), ie64RegName(d.Rs), d.Imm32)
		}
		return hexBytes, fmt.Sprintf("%s %s, %s, %s", mnemonic, ie64RegName(d.Rd), ie64RegName(d.Rs), ie64RegName(d.Rt))

	case ie64IsUnaryALU(d.Opcode):
		return hexBytes, fmt.Sprintf("%s %s, %s", mnemonic, ie64RegName(d.Rd), ie64RegName(d.Rs))

	case d.Opcode == OP_BRA:
		target := uint64(int64(int32(d.PC)) + int64(int32(d.Imm32)))
		return hexBytes, fmt.Sprintf("%s $%06X", mnemonic, target)

	case ie64IsConditionalBranch(d.Opcode):
		target := uint64(int64(int32(d.PC)) + int64(int32(d.Imm32)))
		if d.Rt == 0 {
			switch d.Opcode {
			case OP_BEQ:
				return hexBytes, fmt.Sprintf("beqz %s, $%06X", ie64RegName(d.Rs), target)
			case OP_BNE:
				return hexBytes, fmt.Sprintf("bnez %s, $%06X", ie64RegName(d.Rs), target)
			case OP_BLT:
				return hexBytes, fmt.Sprintf("bltz %s, $%06X", ie64RegName(d.Rs), target)
			case OP_BGE:
				return hexBytes, fmt.Sprintf("bgez %s, $%06X", ie64RegName(d.Rs), target)
			case OP_BGT:
				return hexBytes, fmt.Sprintf("bgtz %s, $%06X", ie64RegName(d.Rs), target)
			case OP_BLE:
				return hexBytes, fmt.Sprintf("blez %s, $%06X", ie64RegName(d.Rs), target)
			}
		}
		return hexBytes, fmt.Sprintf("%s %s, %s, $%06X", mnemonic, ie64RegName(d.Rs), ie64RegName(d.Rt), target)

	case d.Opcode == OP_JSR64:
		target := uint64(int64(int32(d.PC)) + int64(int32(d.Imm32)))
		return hexBytes, fmt.Sprintf("%s $%06X", mnemonic, target)

	case d.Opcode == OP_JMP:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s (%s)", mnemonic, ie64RegName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s -%d(%s)", mnemonic, -disp, ie64RegName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %d(%s)", mnemonic, disp, ie64RegName(d.Rs))

	case d.Opcode == OP_JSR_IND:
		disp := int32(d.Imm32)
		if disp == 0 {
			return hexBytes, fmt.Sprintf("%s (%s)", mnemonic, ie64RegName(d.Rs))
		}
		if disp < 0 {
			return hexBytes, fmt.Sprintf("%s -%d(%s)", mnemonic, -disp, ie64RegName(d.Rs))
		}
		return hexBytes, fmt.Sprintf("%s %d(%s)", mnemonic, disp, ie64RegName(d.Rs))

	case d.Opcode == OP_PUSH64:
		return hexBytes, fmt.Sprintf("%s %s", mnemonic, ie64RegName(d.Rs))

	case d.Opcode == OP_POP64:
		return hexBytes, fmt.Sprintf("%s %s", mnemonic, ie64RegName(d.Rd))

	case d.Opcode >= OP_FMOV && d.Opcode <= OP_FMOVCC:
		return hexBytes, ie64FormatFPU(d, mnemonic)

	default:
		return hexBytes, fmt.Sprintf("%s ???", mnemonic)
	}
}

func ie64FormatFPU(d ie64Decoded, mnemonic string) string {
	fr := func(r byte) string { return fmt.Sprintf("f%d", r) }
	switch d.Opcode {
	case OP_FMOV:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	case OP_FLOAD:
		disp := int32(d.Imm32)
		if disp == 0 {
			return fmt.Sprintf("%s %s, (%s)", mnemonic, fr(d.Rd), ie64RegName(d.Rs))
		}
		return fmt.Sprintf("%s %s, %d(%s)", mnemonic, fr(d.Rd), disp, ie64RegName(d.Rs))
	case OP_FSTORE:
		disp := int32(d.Imm32)
		if disp == 0 {
			return fmt.Sprintf("%s %s, (%s)", mnemonic, fr(d.Rd), ie64RegName(d.Rs))
		}
		return fmt.Sprintf("%s %s, %d(%s)", mnemonic, fr(d.Rd), disp, ie64RegName(d.Rs))
	case OP_FADD, OP_FSUB, OP_FMUL, OP_FDIV, OP_FMOD, OP_FPOW:
		return fmt.Sprintf("%s %s, %s, %s", mnemonic, fr(d.Rd), fr(d.Rs), fr(d.Rt))
	case OP_FABS, OP_FNEG, OP_FSQRT, OP_FINT,
		OP_FSIN, OP_FCOS, OP_FTAN, OP_FATAN, OP_FLOG, OP_FEXP:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), fr(d.Rs))
	case OP_FCMP:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rs), fr(d.Rt))
	case OP_FCVTIF:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), ie64RegName(d.Rs))
	case OP_FCVTFI:
		return fmt.Sprintf("%s %s, %s", mnemonic, ie64RegName(d.Rd), fr(d.Rs))
	case OP_FMOVI:
		return fmt.Sprintf("%s %s, %s", mnemonic, fr(d.Rd), ie64RegName(d.Rs))
	case OP_FMOVO:
		return fmt.Sprintf("%s %s, %s", mnemonic, ie64RegName(d.Rd), fr(d.Rs))
	case OP_FMOVECR:
		return fmt.Sprintf("%s %s, #%d", mnemonic, fr(d.Rd), d.Imm32)
	case OP_FMOVSR, OP_FMOVCR:
		return fmt.Sprintf("%s %s", mnemonic, ie64RegName(d.Rd))
	case OP_FMOVSC, OP_FMOVCC:
		return fmt.Sprintf("%s %s", mnemonic, ie64RegName(d.Rs))
	default:
		return mnemonic
	}
}

// disassembleIE64 disassembles count instructions starting from addr,
// reading memory via the provided function.
func disassembleIE64(readMem func(addr uint64, size int) []byte, addr uint64, count int) []DisassembledLine {
	var lines []DisassembledLine
	for range count {
		data := readMem(addr, 8)
		if len(data) < 8 {
			break
		}
		d := ie64Decode(data, addr)
		hexBytes, mnemonic := ie64FormatInstruction(d)
		lines = append(lines, DisassembledLine{
			Address:  addr,
			HexBytes: hexBytes,
			Mnemonic: mnemonic,
			Size:     8,
		})
		addr += 8
	}
	return lines
}

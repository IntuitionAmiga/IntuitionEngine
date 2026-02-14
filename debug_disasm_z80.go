// debug_disasm_z80.go - Z80 disassembler for Machine Monitor

package main

import (
	"fmt"
	"strings"
)

func disassembleZ80(readMem func(addr uint64, size int) []byte, addr uint64, count int) []DisassembledLine {
	var lines []DisassembledLine
	for range count {
		data := readMem(addr, 4) // max Z80 instruction is 4 bytes
		if len(data) < 1 {
			break
		}
		size, mnemonic := decodeZ80Instruction(data, uint16(addr))
		var hexParts []string
		for j := 0; j < size && j < len(data); j++ {
			hexParts = append(hexParts, fmt.Sprintf("%02X", data[j]))
		}
		line := DisassembledLine{
			Address:  addr,
			HexBytes: strings.Join(hexParts, " "),
			Mnemonic: mnemonic,
			Size:     size,
		}

		// Detect branches by opcode
		op := data[0]
		switch {
		case op == 0xC3 || op == 0xC2 || op == 0xCA || op == 0xD2 || op == 0xDA || op == 0xE2 || op == 0xEA || op == 0xF2 || op == 0xFA: // JP nn / JP cc,nn
			line.IsBranch = true
			if len(data) >= 3 {
				line.BranchTarget = uint64(uint16(data[1]) | uint16(data[2])<<8)
			}
		case op == 0xCD || op == 0xC4 || op == 0xCC || op == 0xD4 || op == 0xDC || op == 0xE4 || op == 0xEC || op == 0xF4 || op == 0xFC: // CALL nn / CALL cc,nn
			line.IsBranch = true
			if len(data) >= 3 {
				line.BranchTarget = uint64(uint16(data[1]) | uint16(data[2])<<8)
			}
		case op == 0x18 || op == 0x20 || op == 0x28 || op == 0x30 || op == 0x38: // JR / JR cc
			line.IsBranch = true
			if len(data) >= 2 {
				line.BranchTarget = uint64(uint16(addr) + 2 + uint16(int8(data[1])))
			}
		case op == 0x10: // DJNZ
			line.IsBranch = true
			if len(data) >= 2 {
				line.BranchTarget = uint64(uint16(addr) + 2 + uint16(int8(data[1])))
			}
		}

		lines = append(lines, line)
		addr += uint64(size)
	}
	return lines
}

func decodeZ80Instruction(data []byte, pc uint16) (int, string) {
	op := data[0]

	// Handle prefix bytes
	switch op {
	case 0xCB:
		if len(data) < 2 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		return 2, decodeZ80CB(data[1])
	case 0xED:
		if len(data) < 2 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		return decodeZ80ED(data[1:], pc)
	case 0xDD:
		if len(data) < 2 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		return decodeZ80DDFD(data[1:], pc, "IX")
	case 0xFD:
		if len(data) < 2 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		return decodeZ80DDFD(data[1:], pc, "IY")
	}

	return decodeZ80Base(data, pc)
}

var z80Reg8 = [8]string{"B", "C", "D", "E", "H", "L", "(HL)", "A"}
var z80Reg16 = [4]string{"BC", "DE", "HL", "SP"}
var z80Reg16Push = [4]string{"BC", "DE", "HL", "AF"}
var z80Cond = [8]string{"NZ", "Z", "NC", "C", "PO", "PE", "P", "M"}
var z80ALU = [8]string{"ADD A,", "ADC A,", "SUB", "SBC A,", "AND", "XOR", "OR", "CP"}

func decodeZ80Base(data []byte, pc uint16) (int, string) {
	op := data[0]

	// NOP
	if op == 0x00 {
		return 1, "NOP"
	}
	// HALT
	if op == 0x76 {
		return 1, "HALT"
	}

	// LD r, r' (01rrrsss) excluding HALT
	if op&0xC0 == 0x40 {
		dst := z80Reg8[(op>>3)&7]
		src := z80Reg8[op&7]
		return 1, fmt.Sprintf("LD %s, %s", dst, src)
	}

	// ALU r (10aaasss)
	if op&0xC0 == 0x80 {
		alu := z80ALU[(op>>3)&7]
		src := z80Reg8[op&7]
		return 1, fmt.Sprintf("%s %s", alu, src)
	}

	switch op {
	case 0x01, 0x11, 0x21, 0x31: // LD rr, nn
		if len(data) < 3 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("LD %s, $%04X", z80Reg16[(op>>4)&3], nn)
	case 0x06, 0x0E, 0x16, 0x1E, 0x26, 0x2E, 0x36, 0x3E: // LD r, n
		if len(data) < 2 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		return 2, fmt.Sprintf("LD %s, $%02X", z80Reg8[(op>>3)&7], data[1])
	case 0xC6, 0xCE, 0xD6, 0xDE, 0xE6, 0xEE, 0xF6, 0xFE: // ALU n
		if len(data) < 2 {
			return 1, fmt.Sprintf("db $%02X", op)
		}
		return 2, fmt.Sprintf("%s $%02X", z80ALU[(op>>3)&7], data[1])
	case 0xC3: // JP nn
		if len(data) < 3 {
			return 1, "JP ???"
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("JP $%04X", nn)
	case 0xCD: // CALL nn
		if len(data) < 3 {
			return 1, "CALL ???"
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("CALL $%04X", nn)
	case 0xC9: // RET
		return 1, "RET"
	case 0x18: // JR e
		if len(data) < 2 {
			return 1, "JR ???"
		}
		target := pc + 2 + uint16(int8(data[1]))
		return 2, fmt.Sprintf("JR $%04X", target)
	case 0x20, 0x28, 0x30, 0x38: // JR cc, e
		if len(data) < 2 {
			return 1, fmt.Sprintf("JR %s, ???", z80Cond[(op>>3)&3])
		}
		target := pc + 2 + uint16(int8(data[1]))
		return 2, fmt.Sprintf("JR %s, $%04X", z80Cond[(op>>3)&3], target)
	case 0xC2, 0xCA, 0xD2, 0xDA, 0xE2, 0xEA, 0xF2, 0xFA: // JP cc, nn
		if len(data) < 3 {
			return 1, fmt.Sprintf("JP %s, ???", z80Cond[(op>>3)&7])
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("JP %s, $%04X", z80Cond[(op>>3)&7], nn)
	case 0xC4, 0xCC, 0xD4, 0xDC, 0xE4, 0xEC, 0xF4, 0xFC: // CALL cc, nn
		if len(data) < 3 {
			return 1, fmt.Sprintf("CALL %s, ???", z80Cond[(op>>3)&7])
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("CALL %s, $%04X", z80Cond[(op>>3)&7], nn)
	case 0xC0, 0xC8, 0xD0, 0xD8, 0xE0, 0xE8, 0xF0, 0xF8: // RET cc
		return 1, fmt.Sprintf("RET %s", z80Cond[(op>>3)&7])
	case 0xC5, 0xD5, 0xE5, 0xF5: // PUSH rr
		return 1, fmt.Sprintf("PUSH %s", z80Reg16Push[(op>>4)&3])
	case 0xC1, 0xD1, 0xE1, 0xF1: // POP rr
		return 1, fmt.Sprintf("POP %s", z80Reg16Push[(op>>4)&3])
	case 0x03, 0x13, 0x23, 0x33: // INC rr
		return 1, fmt.Sprintf("INC %s", z80Reg16[(op>>4)&3])
	case 0x0B, 0x1B, 0x2B, 0x3B: // DEC rr
		return 1, fmt.Sprintf("DEC %s", z80Reg16[(op>>4)&3])
	case 0x04, 0x0C, 0x14, 0x1C, 0x24, 0x2C, 0x34, 0x3C: // INC r
		return 1, fmt.Sprintf("INC %s", z80Reg8[(op>>3)&7])
	case 0x05, 0x0D, 0x15, 0x1D, 0x25, 0x2D, 0x35, 0x3D: // DEC r
		return 1, fmt.Sprintf("DEC %s", z80Reg8[(op>>3)&7])
	case 0x09, 0x19, 0x29, 0x39: // ADD HL, rr
		return 1, fmt.Sprintf("ADD HL, %s", z80Reg16[(op>>4)&3])
	case 0x0A: // LD A, (BC)
		return 1, "LD A, (BC)"
	case 0x1A: // LD A, (DE)
		return 1, "LD A, (DE)"
	case 0x02: // LD (BC), A
		return 1, "LD (BC), A"
	case 0x12: // LD (DE), A
		return 1, "LD (DE), A"
	case 0x22: // LD (nn), HL
		if len(data) < 3 {
			return 1, "LD (nn), HL"
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("LD ($%04X), HL", nn)
	case 0x2A: // LD HL, (nn)
		if len(data) < 3 {
			return 1, "LD HL, (nn)"
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("LD HL, ($%04X)", nn)
	case 0x32: // LD (nn), A
		if len(data) < 3 {
			return 1, "LD (nn), A"
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("LD ($%04X), A", nn)
	case 0x3A: // LD A, (nn)
		if len(data) < 3 {
			return 1, "LD A, (nn)"
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 3, fmt.Sprintf("LD A, ($%04X)", nn)
	case 0xE9: // JP (HL)
		return 1, "JP (HL)"
	case 0xF9: // LD SP, HL
		return 1, "LD SP, HL"
	case 0xEB: // EX DE, HL
		return 1, "EX DE, HL"
	case 0xD9: // EXX
		return 1, "EXX"
	case 0x08: // EX AF, AF'
		return 1, "EX AF, AF'"
	case 0xF3: // DI
		return 1, "DI"
	case 0xFB: // EI
		return 1, "EI"
	case 0xDB: // IN A, (n)
		if len(data) < 2 {
			return 1, "IN A, (n)"
		}
		return 2, fmt.Sprintf("IN A, ($%02X)", data[1])
	case 0xD3: // OUT (n), A
		if len(data) < 2 {
			return 1, "OUT (n), A"
		}
		return 2, fmt.Sprintf("OUT ($%02X), A", data[1])
	case 0xC7, 0xCF, 0xD7, 0xDF, 0xE7, 0xEF, 0xF7, 0xFF: // RST n
		return 1, fmt.Sprintf("RST $%02X", op&0x38)
	case 0x07:
		return 1, "RLCA"
	case 0x0F:
		return 1, "RRCA"
	case 0x17:
		return 1, "RLA"
	case 0x1F:
		return 1, "RRA"
	case 0x27:
		return 1, "DAA"
	case 0x2F:
		return 1, "CPL"
	case 0x37:
		return 1, "SCF"
	case 0x3F:
		return 1, "CCF"
	case 0x10: // DJNZ
		if len(data) < 2 {
			return 1, "DJNZ ???"
		}
		target := pc + 2 + uint16(int8(data[1]))
		return 2, fmt.Sprintf("DJNZ $%04X", target)
	case 0xE3:
		return 1, "EX (SP), HL"
	}
	return 1, fmt.Sprintf("db $%02X", op)
}

var z80CBOps = [8]string{"RLC", "RRC", "RL", "RR", "SLA", "SRA", "SLL", "SRL"}

func decodeZ80CB(op byte) string {
	if op >= 0x40 && op <= 0x7F {
		bit := (op >> 3) & 7
		return fmt.Sprintf("BIT %d, %s", bit, z80Reg8[op&7])
	}
	if op >= 0x80 && op <= 0xBF {
		bit := (op >> 3) & 7
		return fmt.Sprintf("RES %d, %s", bit, z80Reg8[op&7])
	}
	if op >= 0xC0 {
		bit := (op >> 3) & 7
		return fmt.Sprintf("SET %d, %s", bit, z80Reg8[op&7])
	}
	return fmt.Sprintf("%s %s", z80CBOps[(op>>3)&7], z80Reg8[op&7])
}

func decodeZ80ED(data []byte, pc uint16) (int, string) {
	op := data[0]
	switch {
	case op >= 0x40 && op <= 0x7F:
		switch op {
		case 0x40, 0x48, 0x50, 0x58, 0x60, 0x68, 0x78:
			return 2, fmt.Sprintf("IN %s, (C)", z80Reg8[(op>>3)&7])
		case 0x41, 0x49, 0x51, 0x59, 0x61, 0x69, 0x79:
			return 2, fmt.Sprintf("OUT (C), %s", z80Reg8[(op>>3)&7])
		case 0x42, 0x52, 0x62, 0x72:
			return 2, fmt.Sprintf("SBC HL, %s", z80Reg16[(op>>4)&3])
		case 0x4A, 0x5A, 0x6A, 0x7A:
			return 2, fmt.Sprintf("ADC HL, %s", z80Reg16[(op>>4)&3])
		case 0x43, 0x53, 0x63, 0x73:
			if len(data) < 3 {
				return 2, "LD (nn), rr"
			}
			nn := uint16(data[1]) | uint16(data[2])<<8
			return 4, fmt.Sprintf("LD ($%04X), %s", nn, z80Reg16[(op>>4)&3])
		case 0x4B, 0x5B, 0x6B, 0x7B:
			if len(data) < 3 {
				return 2, "LD rr, (nn)"
			}
			nn := uint16(data[1]) | uint16(data[2])<<8
			return 4, fmt.Sprintf("LD %s, ($%04X)", z80Reg16[(op>>4)&3], nn)
		case 0x44:
			return 2, "NEG"
		case 0x45:
			return 2, "RETN"
		case 0x4D:
			return 2, "RETI"
		case 0x46:
			return 2, "IM 0"
		case 0x56:
			return 2, "IM 1"
		case 0x5E:
			return 2, "IM 2"
		case 0x47:
			return 2, "LD I, A"
		case 0x4F:
			return 2, "LD R, A"
		case 0x57:
			return 2, "LD A, I"
		case 0x5F:
			return 2, "LD A, R"
		case 0x67:
			return 2, "RRD"
		case 0x6F:
			return 2, "RLD"
		}
	case op == 0xA0:
		return 2, "LDI"
	case op == 0xA8:
		return 2, "LDD"
	case op == 0xB0:
		return 2, "LDIR"
	case op == 0xB8:
		return 2, "LDDR"
	case op == 0xA1:
		return 2, "CPI"
	case op == 0xA9:
		return 2, "CPD"
	case op == 0xB1:
		return 2, "CPIR"
	case op == 0xB9:
		return 2, "CPDR"
	case op == 0xA2:
		return 2, "INI"
	case op == 0xAA:
		return 2, "IND"
	case op == 0xB2:
		return 2, "INIR"
	case op == 0xBA:
		return 2, "INDR"
	case op == 0xA3:
		return 2, "OUTI"
	case op == 0xAB:
		return 2, "OUTD"
	case op == 0xB3:
		return 2, "OTIR"
	case op == 0xBB:
		return 2, "OTDR"
	}
	return 2, fmt.Sprintf("db $ED, $%02X", op)
}

func decodeZ80DDFD(data []byte, pc uint16, idx string) (int, string) {
	op := data[0]
	if op == 0xCB {
		if len(data) < 3 {
			return 2, fmt.Sprintf("db $%s, $CB", idx[:1])
		}
		d := int8(data[1])
		op2 := data[2]
		if op2 >= 0x40 && op2 <= 0x7F {
			bit := (op2 >> 3) & 7
			return 4, fmt.Sprintf("BIT %d, (%s%+d)", bit, idx, d)
		}
		if op2 >= 0x80 && op2 <= 0xBF {
			bit := (op2 >> 3) & 7
			return 4, fmt.Sprintf("RES %d, (%s%+d)", bit, idx, d)
		}
		if op2 >= 0xC0 {
			bit := (op2 >> 3) & 7
			return 4, fmt.Sprintf("SET %d, (%s%+d)", bit, idx, d)
		}
		return 4, fmt.Sprintf("%s (%s%+d)", z80CBOps[(op2>>3)&7], idx, d)
	}

	switch op {
	case 0x21:
		if len(data) < 3 {
			return 2, fmt.Sprintf("LD %s, nn", idx)
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 4, fmt.Sprintf("LD %s, $%04X", idx, nn)
	case 0x22:
		if len(data) < 3 {
			return 2, fmt.Sprintf("LD (nn), %s", idx)
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 4, fmt.Sprintf("LD ($%04X), %s", nn, idx)
	case 0x2A:
		if len(data) < 3 {
			return 2, fmt.Sprintf("LD %s, (nn)", idx)
		}
		nn := uint16(data[1]) | uint16(data[2])<<8
		return 4, fmt.Sprintf("LD %s, ($%04X)", idx, nn)
	case 0x23:
		return 2, fmt.Sprintf("INC %s", idx)
	case 0x2B:
		return 2, fmt.Sprintf("DEC %s", idx)
	case 0x36:
		if len(data) < 3 {
			return 2, fmt.Sprintf("LD (%s+d), n", idx)
		}
		return 4, fmt.Sprintf("LD (%s%+d), $%02X", idx, int8(data[1]), data[2])
	case 0x34:
		if len(data) < 2 {
			return 2, fmt.Sprintf("INC (%s+d)", idx)
		}
		return 3, fmt.Sprintf("INC (%s%+d)", idx, int8(data[1]))
	case 0x35:
		if len(data) < 2 {
			return 2, fmt.Sprintf("DEC (%s+d)", idx)
		}
		return 3, fmt.Sprintf("DEC (%s%+d)", idx, int8(data[1]))
	case 0xE1:
		return 2, fmt.Sprintf("POP %s", idx)
	case 0xE5:
		return 2, fmt.Sprintf("PUSH %s", idx)
	case 0xE9:
		return 2, fmt.Sprintf("JP (%s)", idx)
	case 0xF9:
		return 2, fmt.Sprintf("LD SP, %s", idx)
	case 0xE3:
		return 2, fmt.Sprintf("EX (SP), %s", idx)
	case 0x09, 0x19, 0x29, 0x39:
		return 2, fmt.Sprintf("ADD %s, %s", idx, z80Reg16[(op>>4)&3])
	}

	// LD r, (IX+d) / LD (IX+d), r
	if op&0xC0 == 0x40 {
		dst := (op >> 3) & 7
		src := op & 7
		if src == 6 { // LD r, (IX+d)
			if len(data) < 2 {
				return 2, fmt.Sprintf("LD %s, (%s+d)", z80Reg8[dst], idx)
			}
			return 3, fmt.Sprintf("LD %s, (%s%+d)", z80Reg8[dst], idx, int8(data[1]))
		}
		if dst == 6 { // LD (IX+d), r
			if len(data) < 2 {
				return 2, fmt.Sprintf("LD (%s+d), %s", idx, z80Reg8[src])
			}
			return 3, fmt.Sprintf("LD (%s%+d), %s", idx, int8(data[1]), z80Reg8[src])
		}
	}

	// ALU (IX+d)
	if op&0xC0 == 0x80 && op&7 == 6 {
		if len(data) < 2 {
			return 2, fmt.Sprintf("%s (%s+d)", z80ALU[(op>>3)&7], idx)
		}
		return 3, fmt.Sprintf("%s (%s%+d)", z80ALU[(op>>3)&7], idx, int8(data[1]))
	}

	return 2, fmt.Sprintf("db $%s, $%02X", idx[:1], op)
}

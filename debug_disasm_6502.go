// debug_disasm_6502.go - 6502 disassembler for Machine Monitor

package main

import (
	"fmt"
	"strings"
)

type opInfo6502 struct {
	name string
	mode int // addressing mode
	size int // instruction size in bytes
}

const (
	am6502Imp  = iota // Implied
	am6502Acc         // Accumulator
	am6502Imm         // #nn
	am6502Zp          // nn
	am6502ZpX         // nn,X
	am6502ZpY         // nn,Y
	am6502Abs         // nnnn
	am6502AbsX        // nnnn,X
	am6502AbsY        // nnnn,Y
	am6502Ind         // (nnnn)
	am6502IndX        // (nn,X)
	am6502IndY        // (nn),Y
	am6502Rel         // relative
)

var opcodes6502 = [256]opInfo6502{
	// 0x00-0x0F
	0x00: {"BRK", am6502Imp, 1}, 0x01: {"ORA", am6502IndX, 2},
	0x05: {"ORA", am6502Zp, 2}, 0x06: {"ASL", am6502Zp, 2},
	0x08: {"PHP", am6502Imp, 1}, 0x09: {"ORA", am6502Imm, 2},
	0x0A: {"ASL", am6502Acc, 1}, 0x0D: {"ORA", am6502Abs, 3},
	0x0E: {"ASL", am6502Abs, 3},
	// 0x10-0x1F
	0x10: {"BPL", am6502Rel, 2}, 0x11: {"ORA", am6502IndY, 2},
	0x15: {"ORA", am6502ZpX, 2}, 0x16: {"ASL", am6502ZpX, 2},
	0x18: {"CLC", am6502Imp, 1}, 0x19: {"ORA", am6502AbsY, 3},
	0x1D: {"ORA", am6502AbsX, 3}, 0x1E: {"ASL", am6502AbsX, 3},
	// 0x20-0x2F
	0x20: {"JSR", am6502Abs, 3}, 0x21: {"AND", am6502IndX, 2},
	0x24: {"BIT", am6502Zp, 2}, 0x25: {"AND", am6502Zp, 2},
	0x26: {"ROL", am6502Zp, 2}, 0x28: {"PLP", am6502Imp, 1},
	0x29: {"AND", am6502Imm, 2}, 0x2A: {"ROL", am6502Acc, 1},
	0x2C: {"BIT", am6502Abs, 3}, 0x2D: {"AND", am6502Abs, 3},
	0x2E: {"ROL", am6502Abs, 3},
	// 0x30-0x3F
	0x30: {"BMI", am6502Rel, 2}, 0x31: {"AND", am6502IndY, 2},
	0x35: {"AND", am6502ZpX, 2}, 0x36: {"ROL", am6502ZpX, 2},
	0x38: {"SEC", am6502Imp, 1}, 0x39: {"AND", am6502AbsY, 3},
	0x3D: {"AND", am6502AbsX, 3}, 0x3E: {"ROL", am6502AbsX, 3},
	// 0x40-0x4F
	0x40: {"RTI", am6502Imp, 1}, 0x41: {"EOR", am6502IndX, 2},
	0x45: {"EOR", am6502Zp, 2}, 0x46: {"LSR", am6502Zp, 2},
	0x48: {"PHA", am6502Imp, 1}, 0x49: {"EOR", am6502Imm, 2},
	0x4A: {"LSR", am6502Acc, 1}, 0x4C: {"JMP", am6502Abs, 3},
	0x4D: {"EOR", am6502Abs, 3}, 0x4E: {"LSR", am6502Abs, 3},
	// 0x50-0x5F
	0x50: {"BVC", am6502Rel, 2}, 0x51: {"EOR", am6502IndY, 2},
	0x55: {"EOR", am6502ZpX, 2}, 0x56: {"LSR", am6502ZpX, 2},
	0x58: {"CLI", am6502Imp, 1}, 0x59: {"EOR", am6502AbsY, 3},
	0x5D: {"EOR", am6502AbsX, 3}, 0x5E: {"LSR", am6502AbsX, 3},
	// 0x60-0x6F
	0x60: {"RTS", am6502Imp, 1}, 0x61: {"ADC", am6502IndX, 2},
	0x65: {"ADC", am6502Zp, 2}, 0x66: {"ROR", am6502Zp, 2},
	0x68: {"PLA", am6502Imp, 1}, 0x69: {"ADC", am6502Imm, 2},
	0x6A: {"ROR", am6502Acc, 1}, 0x6C: {"JMP", am6502Ind, 3},
	0x6D: {"ADC", am6502Abs, 3}, 0x6E: {"ROR", am6502Abs, 3},
	// 0x70-0x7F
	0x70: {"BVS", am6502Rel, 2}, 0x71: {"ADC", am6502IndY, 2},
	0x75: {"ADC", am6502ZpX, 2}, 0x76: {"ROR", am6502ZpX, 2},
	0x78: {"SEI", am6502Imp, 1}, 0x79: {"ADC", am6502AbsY, 3},
	0x7D: {"ADC", am6502AbsX, 3}, 0x7E: {"ROR", am6502AbsX, 3},
	// 0x80-0x8F
	0x81: {"STA", am6502IndX, 2}, 0x84: {"STY", am6502Zp, 2},
	0x85: {"STA", am6502Zp, 2}, 0x86: {"STX", am6502Zp, 2},
	0x88: {"DEY", am6502Imp, 1}, 0x8A: {"TXA", am6502Imp, 1},
	0x8C: {"STY", am6502Abs, 3}, 0x8D: {"STA", am6502Abs, 3},
	0x8E: {"STX", am6502Abs, 3},
	// 0x90-0x9F
	0x90: {"BCC", am6502Rel, 2}, 0x91: {"STA", am6502IndY, 2},
	0x94: {"STY", am6502ZpX, 2}, 0x95: {"STA", am6502ZpX, 2},
	0x96: {"STX", am6502ZpY, 2}, 0x98: {"TYA", am6502Imp, 1},
	0x99: {"STA", am6502AbsY, 3}, 0x9A: {"TXS", am6502Imp, 1},
	0x9D: {"STA", am6502AbsX, 3},
	// 0xA0-0xAF
	0xA0: {"LDY", am6502Imm, 2}, 0xA1: {"LDA", am6502IndX, 2},
	0xA2: {"LDX", am6502Imm, 2}, 0xA4: {"LDY", am6502Zp, 2},
	0xA5: {"LDA", am6502Zp, 2}, 0xA6: {"LDX", am6502Zp, 2},
	0xA8: {"TAY", am6502Imp, 1}, 0xA9: {"LDA", am6502Imm, 2},
	0xAA: {"TAX", am6502Imp, 1}, 0xAC: {"LDY", am6502Abs, 3},
	0xAD: {"LDA", am6502Abs, 3}, 0xAE: {"LDX", am6502Abs, 3},
	// 0xB0-0xBF
	0xB0: {"BCS", am6502Rel, 2}, 0xB1: {"LDA", am6502IndY, 2},
	0xB4: {"LDY", am6502ZpX, 2}, 0xB5: {"LDA", am6502ZpX, 2},
	0xB6: {"LDX", am6502ZpY, 2}, 0xB8: {"CLV", am6502Imp, 1},
	0xB9: {"LDA", am6502AbsY, 3}, 0xBA: {"TSX", am6502Imp, 1},
	0xBC: {"LDY", am6502AbsX, 3}, 0xBD: {"LDA", am6502AbsX, 3},
	0xBE: {"LDX", am6502AbsY, 3},
	// 0xC0-0xCF
	0xC0: {"CPY", am6502Imm, 2}, 0xC1: {"CMP", am6502IndX, 2},
	0xC4: {"CPY", am6502Zp, 2}, 0xC5: {"CMP", am6502Zp, 2},
	0xC6: {"DEC", am6502Zp, 2}, 0xC8: {"INY", am6502Imp, 1},
	0xC9: {"CMP", am6502Imm, 2}, 0xCA: {"DEX", am6502Imp, 1},
	0xCC: {"CPY", am6502Abs, 3}, 0xCD: {"CMP", am6502Abs, 3},
	0xCE: {"DEC", am6502Abs, 3},
	// 0xD0-0xDF
	0xD0: {"BNE", am6502Rel, 2}, 0xD1: {"CMP", am6502IndY, 2},
	0xD5: {"CMP", am6502ZpX, 2}, 0xD6: {"DEC", am6502ZpX, 2},
	0xD8: {"CLD", am6502Imp, 1}, 0xD9: {"CMP", am6502AbsY, 3},
	0xDD: {"CMP", am6502AbsX, 3}, 0xDE: {"DEC", am6502AbsX, 3},
	// 0xE0-0xEF
	0xE0: {"CPX", am6502Imm, 2}, 0xE1: {"SBC", am6502IndX, 2},
	0xE4: {"CPX", am6502Zp, 2}, 0xE5: {"SBC", am6502Zp, 2},
	0xE6: {"INC", am6502Zp, 2}, 0xE8: {"INX", am6502Imp, 1},
	0xE9: {"SBC", am6502Imm, 2}, 0xEA: {"NOP", am6502Imp, 1},
	0xEC: {"CPX", am6502Abs, 3}, 0xED: {"SBC", am6502Abs, 3},
	0xEE: {"INC", am6502Abs, 3},
	// 0xF0-0xFF
	0xF0: {"BEQ", am6502Rel, 2}, 0xF1: {"SBC", am6502IndY, 2},
	0xF5: {"SBC", am6502ZpX, 2}, 0xF6: {"INC", am6502ZpX, 2},
	0xF8: {"SED", am6502Imp, 1}, 0xF9: {"SBC", am6502AbsY, 3},
	0xFD: {"SBC", am6502AbsX, 3}, 0xFE: {"INC", am6502AbsX, 3},
}

func disassemble6502(readMem func(addr uint64, size int) []byte, addr uint64, count int) []DisassembledLine {
	var lines []DisassembledLine
	for i := 0; i < count; i++ {
		data := readMem(addr, 3)
		if len(data) < 1 {
			break
		}
		op := data[0]
		info := opcodes6502[op]
		size := info.size
		if size == 0 {
			size = 1
		}
		if len(data) < size {
			size = len(data)
		}

		var hexParts []string
		for j := 0; j < size; j++ {
			hexParts = append(hexParts, fmt.Sprintf("%02X", data[j]))
		}

		var mnemonic string
		if info.name == "" {
			mnemonic = fmt.Sprintf("db $%02X", op)
		} else {
			switch info.mode {
			case am6502Imp:
				mnemonic = info.name
			case am6502Acc:
				mnemonic = info.name + " A"
			case am6502Imm:
				if size >= 2 {
					mnemonic = fmt.Sprintf("%s #$%02X", info.name, data[1])
				} else {
					mnemonic = info.name + " #?"
				}
			case am6502Zp:
				if size >= 2 {
					mnemonic = fmt.Sprintf("%s $%02X", info.name, data[1])
				} else {
					mnemonic = info.name + " ?"
				}
			case am6502ZpX:
				if size >= 2 {
					mnemonic = fmt.Sprintf("%s $%02X,X", info.name, data[1])
				} else {
					mnemonic = info.name + " ?,X"
				}
			case am6502ZpY:
				if size >= 2 {
					mnemonic = fmt.Sprintf("%s $%02X,Y", info.name, data[1])
				} else {
					mnemonic = info.name + " ?,Y"
				}
			case am6502Abs:
				if size >= 3 {
					nn := uint16(data[1]) | uint16(data[2])<<8
					mnemonic = fmt.Sprintf("%s $%04X", info.name, nn)
				} else {
					mnemonic = info.name + " ???"
				}
			case am6502AbsX:
				if size >= 3 {
					nn := uint16(data[1]) | uint16(data[2])<<8
					mnemonic = fmt.Sprintf("%s $%04X,X", info.name, nn)
				} else {
					mnemonic = info.name + " ???,X"
				}
			case am6502AbsY:
				if size >= 3 {
					nn := uint16(data[1]) | uint16(data[2])<<8
					mnemonic = fmt.Sprintf("%s $%04X,Y", info.name, nn)
				} else {
					mnemonic = info.name + " ???,Y"
				}
			case am6502Ind:
				if size >= 3 {
					nn := uint16(data[1]) | uint16(data[2])<<8
					mnemonic = fmt.Sprintf("%s ($%04X)", info.name, nn)
				} else {
					mnemonic = info.name + " (???)"
				}
			case am6502IndX:
				if size >= 2 {
					mnemonic = fmt.Sprintf("%s ($%02X,X)", info.name, data[1])
				} else {
					mnemonic = info.name + " (?,X)"
				}
			case am6502IndY:
				if size >= 2 {
					mnemonic = fmt.Sprintf("%s ($%02X),Y", info.name, data[1])
				} else {
					mnemonic = info.name + " (?),Y"
				}
			case am6502Rel:
				if size >= 2 {
					target := uint16(addr) + 2 + uint16(int8(data[1]))
					mnemonic = fmt.Sprintf("%s $%04X", info.name, target)
				} else {
					mnemonic = info.name + " ???"
				}
			default:
				mnemonic = info.name
			}
		}

		lines = append(lines, DisassembledLine{
			Address:  addr,
			HexBytes: strings.Join(hexParts, " "),
			Mnemonic: mnemonic,
			Size:     size,
		})
		addr += uint64(size)
	}
	return lines
}

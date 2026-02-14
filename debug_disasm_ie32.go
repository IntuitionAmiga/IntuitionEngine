// debug_disasm_ie32.go - IE32 disassembler for Machine Monitor

package main

import "fmt"

var ie32RegNames = []string{"A", "X", "Y", "Z", "B", "C", "D", "E", "F", "G", "H", "S", "T", "U", "V", "W"}

var ie32OpcodeTable = map[byte]string{
	LOAD: "LOAD", STORE: "STORE",
	ADD: "ADD", SUB: "SUB", MUL: "MUL", DIV: "DIV", MOD: "MOD",
	AND: "AND", OR: "OR", XOR: "XOR", NOT: "NOT", SHL: "SHL", SHR: "SHR",
	JMP: "JMP", JNZ: "JNZ", JZ: "JZ", JGT: "JGT", JGE: "JGE", JLT: "JLT", JLE: "JLE",
	PUSH: "PUSH", POP: "POP", JSR: "JSR", RTS: "RTS",
	SEI: "SEI", CLI: "CLI", RTI: "RTI", WAIT: "WAIT",
	INC: "INC", DEC: "DEC", NOP: "NOP", HALT: "HALT",
	LDA: "LDA", LDX: "LDX", LDY: "LDY", LDZ: "LDZ",
	STA: "STA", STX: "STX", STY: "STY", STZ: "STZ",
	LDB: "LDB", LDC: "LDC", LDD: "LDD", LDE: "LDE",
	LDF: "LDF", LDG: "LDG", LDH: "LDH", LDS: "LDS",
	LDT: "LDT", LDU: "LDU", LDV: "LDV", LDW: "LDW",
	STB: "STB", STC: "STC", STD: "STD", STE: "STE",
	STF: "STF", STG: "STG", STH: "STH", STS: "STS",
	STT: "STT", STU: "STU", STV: "STV", STW: "STW",
}

var ie32AddrModes = [5]string{"#", "R", "(R)", "[M]", "M:"}

func disassembleIE32(readMem func(addr uint64, size int) []byte, addr uint64, count int) []DisassembledLine {
	var lines []DisassembledLine
	for i := 0; i < count; i++ {
		data := readMem(addr, 8)
		if len(data) < 8 {
			break
		}
		opcode := data[0]
		reg := data[1] & REG_INDEX_MASK
		addrMode := data[2]
		operand := uint32(data[4]) | uint32(data[5])<<8 | uint32(data[6])<<16 | uint32(data[7])<<24

		hexBytes := fmt.Sprintf("%02X %02X %02X %02X %02X %02X %02X %02X",
			data[0], data[1], data[2], data[3], data[4], data[5], data[6], data[7])

		name, ok := ie32OpcodeTable[opcode]
		if !ok {
			name = fmt.Sprintf("db $%02X", opcode)
		}

		var mnemonic string
		regName := ie32RegNames[reg]
		modeStr := ""
		if int(addrMode) < len(ie32AddrModes) {
			modeStr = ie32AddrModes[addrMode]
		}

		switch opcode {
		case NOP, RTS, RTI, HALT:
			mnemonic = name
		case JMP, JSR:
			mnemonic = fmt.Sprintf("%s $%08X", name, operand)
		default:
			mnemonic = fmt.Sprintf("%s %s, %s$%08X", name, regName, modeStr, operand)
		}

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

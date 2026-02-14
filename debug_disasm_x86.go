// debug_disasm_x86.go - X86 disassembler for Machine Monitor

package main

import (
	"fmt"
	"strings"
)

var x86Reg32 = [8]string{"EAX", "ECX", "EDX", "EBX", "ESP", "EBP", "ESI", "EDI"}
var x86Reg16 = [8]string{"AX", "CX", "DX", "BX", "SP", "BP", "SI", "DI"}
var x86Reg8 = [8]string{"AL", "CL", "DL", "BL", "AH", "CH", "DH", "BH"}
var x86SegRegs = [6]string{"ES", "CS", "SS", "DS", "FS", "GS"}
var x86Cond = [16]string{
	"O", "NO", "B", "NB", "Z", "NZ", "BE", "A",
	"S", "NS", "P", "NP", "L", "GE", "LE", "G",
}

type x86Disasm struct {
	readMem func(addr uint64, size int) []byte
	pos     uint64
}

func (d *x86Disasm) readByte() (byte, bool) {
	data := d.readMem(d.pos, 1)
	if len(data) < 1 {
		return 0, false
	}
	d.pos++
	return data[0], true
}

func (d *x86Disasm) readWord() (uint16, bool) {
	data := d.readMem(d.pos, 2)
	if len(data) < 2 {
		return 0, false
	}
	d.pos += 2
	return uint16(data[0]) | uint16(data[1])<<8, true
}

func (d *x86Disasm) readDword() (uint32, bool) {
	data := d.readMem(d.pos, 4)
	if len(data) < 4 {
		return 0, false
	}
	d.pos += 4
	return uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24, true
}

func (d *x86Disasm) decodeModRM(wide bool) (string, bool) {
	b, ok := d.readByte()
	if !ok {
		return "???", false
	}
	mod := (b >> 6) & 3
	rm := b & 7
	reg := (b >> 3) & 7
	_ = reg

	if mod == 3 {
		if wide {
			return x86Reg32[rm], true
		}
		return x86Reg8[rm], true
	}

	var base string
	var disp string

	if mod == 0 && rm == 5 {
		// Direct address
		dw, ok := d.readDword()
		if !ok {
			return "[???]", false
		}
		return fmt.Sprintf("[0x%08X]", dw), true
	}

	if rm == 4 {
		// SIB byte
		sib, ok := d.readByte()
		if !ok {
			return "[???]", false
		}
		sibBase := sib & 7
		sibIdx := (sib >> 3) & 7
		sibScale := (sib >> 6) & 3

		if mod == 0 && sibBase == 5 {
			dw, ok := d.readDword()
			if !ok {
				return "[???]", false
			}
			if sibIdx == 4 {
				return fmt.Sprintf("[0x%08X]", dw), true
			}
			return fmt.Sprintf("[%s*%d+0x%08X]", x86Reg32[sibIdx], 1<<sibScale, dw), true
		}

		base = x86Reg32[sibBase]
		if sibIdx != 4 {
			base = fmt.Sprintf("%s+%s*%d", base, x86Reg32[sibIdx], 1<<sibScale)
		}
	} else {
		base = x86Reg32[rm]
	}

	switch mod {
	case 0:
		return fmt.Sprintf("[%s]", base), true
	case 1:
		db, ok := d.readByte()
		if !ok {
			return fmt.Sprintf("[%s+?]", base), false
		}
		off := int8(db)
		if off >= 0 {
			disp = fmt.Sprintf("+0x%02X", off)
		} else {
			disp = fmt.Sprintf("-0x%02X", -off)
		}
		return fmt.Sprintf("[%s%s]", base, disp), true
	case 2:
		dw, ok := d.readDword()
		if !ok {
			return fmt.Sprintf("[%s+?]", base), false
		}
		return fmt.Sprintf("[%s+0x%08X]", base, dw), true
	}
	return "???", false
}

func disassembleX86(readMem func(addr uint64, size int) []byte, startAddr uint64, count int) []DisassembledLine {
	var lines []DisassembledLine
	addr := startAddr

	for range count {
		instrAddr := addr
		dis := &x86Disasm{readMem: readMem, pos: addr}

		mnemonic := decodeX86Instruction(dis, instrAddr)
		addr = dis.pos

		instrSize := int(addr - instrAddr)
		if instrSize == 0 {
			instrSize = 1
			addr++
		}
		hexData := readMem(instrAddr, instrSize)
		var hexParts []string
		for _, b := range hexData {
			hexParts = append(hexParts, fmt.Sprintf("%02X", b))
		}

		line := DisassembledLine{
			Address:  instrAddr,
			HexBytes: strings.Join(hexParts, " "),
			Mnemonic: mnemonic,
			Size:     instrSize,
		}

		// Branch annotation by opcode
		if len(hexData) >= 1 {
			op := hexData[0]
			switch {
			case op == 0xE8 && instrSize >= 5: // CALL rel32
				line.IsBranch = true
				off := int32(uint32(hexData[1]) | uint32(hexData[2])<<8 | uint32(hexData[3])<<16 | uint32(hexData[4])<<24)
				line.BranchTarget = uint64(int64(instrAddr) + int64(instrSize) + int64(off))
			case op == 0xE9 && instrSize >= 5: // JMP rel32
				line.IsBranch = true
				off := int32(uint32(hexData[1]) | uint32(hexData[2])<<8 | uint32(hexData[3])<<16 | uint32(hexData[4])<<24)
				line.BranchTarget = uint64(int64(instrAddr) + int64(instrSize) + int64(off))
			case op == 0xEB && instrSize >= 2: // JMP rel8
				line.IsBranch = true
				line.BranchTarget = uint64(int64(instrAddr) + 2 + int64(int8(hexData[1])))
			case op >= 0x70 && op <= 0x7F && instrSize >= 2: // Jcc rel8
				line.IsBranch = true
				line.BranchTarget = uint64(int64(instrAddr) + 2 + int64(int8(hexData[1])))
			case op == 0x0F && instrSize >= 6 && len(hexData) >= 2 && hexData[1] >= 0x80 && hexData[1] <= 0x8F: // Jcc rel32
				line.IsBranch = true
				off := int32(uint32(hexData[2]) | uint32(hexData[3])<<8 | uint32(hexData[4])<<16 | uint32(hexData[5])<<24)
				line.BranchTarget = uint64(int64(instrAddr) + int64(instrSize) + int64(off))
			}
		}

		lines = append(lines, line)
	}
	return lines
}

func decodeX86Instruction(d *x86Disasm, instrAddr uint64) string {
	// Handle prefixes
	var segOverride string
	hasOpSize := false

	for {
		b, ok := d.readByte()
		if !ok {
			return "db ??"
		}

		switch b {
		case 0x26:
			segOverride = "ES:"
			continue
		case 0x2E:
			segOverride = "CS:"
			continue
		case 0x36:
			segOverride = "SS:"
			continue
		case 0x3E:
			segOverride = "DS:"
			continue
		case 0x64:
			segOverride = "FS:"
			continue
		case 0x65:
			segOverride = "GS:"
			continue
		case 0x66:
			hasOpSize = true
			continue
		case 0xF0: // LOCK
			rest := decodeX86Instruction(d, instrAddr)
			return "LOCK " + rest
		case 0xF2: // REPNE
			rest := decodeX86Instruction(d, instrAddr)
			return "REPNE " + rest
		case 0xF3: // REP
			rest := decodeX86Instruction(d, instrAddr)
			return "REP " + rest
		}

		// Not a prefix, decode the opcode
		return decodeX86Opcode(d, b, instrAddr, segOverride, hasOpSize)
	}
}

func decodeX86Opcode(d *x86Disasm, op byte, instrAddr uint64, segOverride string, hasOpSize bool) string {
	_ = segOverride

	regStr := x86Reg32
	if hasOpSize {
		regStr = x86Reg16
	}

	switch op {
	// NOP
	case 0x90:
		return "NOP"
	// HLT
	case 0xF4:
		return "HLT"
	// RET
	case 0xC3:
		return "RET"
	case 0xC2:
		imm, _ := d.readWord()
		return fmt.Sprintf("RET 0x%04X", imm)
	// INT
	case 0xCC:
		return "INT 3"
	case 0xCD:
		imm, _ := d.readByte()
		return fmt.Sprintf("INT 0x%02X", imm)
	// IRET
	case 0xCF:
		return "IRET"

	// PUSH/POP reg
	case 0x50, 0x51, 0x52, 0x53, 0x54, 0x55, 0x56, 0x57:
		return fmt.Sprintf("PUSH %s", regStr[op-0x50])
	case 0x58, 0x59, 0x5A, 0x5B, 0x5C, 0x5D, 0x5E, 0x5F:
		return fmt.Sprintf("POP %s", regStr[op-0x58])

	// INC/DEC reg
	case 0x40, 0x41, 0x42, 0x43, 0x44, 0x45, 0x46, 0x47:
		return fmt.Sprintf("INC %s", regStr[op-0x40])
	case 0x48, 0x49, 0x4A, 0x4B, 0x4C, 0x4D, 0x4E, 0x4F:
		return fmt.Sprintf("DEC %s", regStr[op-0x48])

	// XCHG EAX, reg
	case 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97:
		return fmt.Sprintf("XCHG %s, %s", regStr[0], regStr[op-0x90])

	// MOV reg, imm32
	case 0xB8, 0xB9, 0xBA, 0xBB, 0xBC, 0xBD, 0xBE, 0xBF:
		if hasOpSize {
			imm, _ := d.readWord()
			return fmt.Sprintf("MOV %s, 0x%04X", regStr[op-0xB8], imm)
		}
		imm, _ := d.readDword()
		return fmt.Sprintf("MOV %s, 0x%08X", regStr[op-0xB8], imm)

	// MOV reg8, imm8
	case 0xB0, 0xB1, 0xB2, 0xB3, 0xB4, 0xB5, 0xB6, 0xB7:
		imm, _ := d.readByte()
		return fmt.Sprintf("MOV %s, 0x%02X", x86Reg8[op-0xB0], imm)

	// MOV EAX, [moffs]
	case 0xA1:
		if hasOpSize {
			addr, _ := d.readDword()
			return fmt.Sprintf("MOV AX, [0x%08X]", addr)
		}
		addr, _ := d.readDword()
		return fmt.Sprintf("MOV EAX, [0x%08X]", addr)
	// MOV [moffs], EAX
	case 0xA3:
		if hasOpSize {
			addr, _ := d.readDword()
			return fmt.Sprintf("MOV [0x%08X], AX", addr)
		}
		addr, _ := d.readDword()
		return fmt.Sprintf("MOV [0x%08X], EAX", addr)
	// MOV AL, [moffs]
	case 0xA0:
		addr, _ := d.readDword()
		return fmt.Sprintf("MOV AL, [0x%08X]", addr)
	// MOV [moffs], AL
	case 0xA2:
		addr, _ := d.readDword()
		return fmt.Sprintf("MOV [0x%08X], AL", addr)

	// JMP rel8
	case 0xEB:
		off, _ := d.readByte()
		target := d.pos + uint64(int8(off))
		return fmt.Sprintf("JMP SHORT 0x%08X", target)
	// JMP rel32
	case 0xE9:
		if hasOpSize {
			off, _ := d.readWord()
			target := d.pos + uint64(int16(off))
			return fmt.Sprintf("JMP 0x%08X", target)
		}
		off, _ := d.readDword()
		target := d.pos + uint64(int32(off))
		return fmt.Sprintf("JMP 0x%08X", target)
	// CALL rel32
	case 0xE8:
		if hasOpSize {
			off, _ := d.readWord()
			target := d.pos + uint64(int16(off))
			return fmt.Sprintf("CALL 0x%08X", target)
		}
		off, _ := d.readDword()
		target := d.pos + uint64(int32(off))
		return fmt.Sprintf("CALL 0x%08X", target)

	// Jcc rel8
	case 0x70, 0x71, 0x72, 0x73, 0x74, 0x75, 0x76, 0x77,
		0x78, 0x79, 0x7A, 0x7B, 0x7C, 0x7D, 0x7E, 0x7F:
		off, _ := d.readByte()
		target := d.pos + uint64(int8(off))
		return fmt.Sprintf("J%s SHORT 0x%08X", x86Cond[op-0x70], target)

	// LOOP/LOOPNZ/LOOPZ/JCXZ
	case 0xE0:
		off, _ := d.readByte()
		target := d.pos + uint64(int8(off))
		return fmt.Sprintf("LOOPNZ 0x%08X", target)
	case 0xE1:
		off, _ := d.readByte()
		target := d.pos + uint64(int8(off))
		return fmt.Sprintf("LOOPZ 0x%08X", target)
	case 0xE2:
		off, _ := d.readByte()
		target := d.pos + uint64(int8(off))
		return fmt.Sprintf("LOOP 0x%08X", target)
	case 0xE3:
		off, _ := d.readByte()
		target := d.pos + uint64(int8(off))
		return fmt.Sprintf("JECXZ 0x%08X", target)

	// ALU r/m8, r8
	case 0x00:
		return decodeX86ALURM(d, "ADD", false, false)
	case 0x08:
		return decodeX86ALURM(d, "OR", false, false)
	case 0x10:
		return decodeX86ALURM(d, "ADC", false, false)
	case 0x18:
		return decodeX86ALURM(d, "SBB", false, false)
	case 0x20:
		return decodeX86ALURM(d, "AND", false, false)
	case 0x28:
		return decodeX86ALURM(d, "SUB", false, false)
	case 0x30:
		return decodeX86ALURM(d, "XOR", false, false)
	case 0x38:
		return decodeX86ALURM(d, "CMP", false, false)

	// ALU r/m32, r32
	case 0x01:
		return decodeX86ALURM(d, "ADD", true, false)
	case 0x09:
		return decodeX86ALURM(d, "OR", true, false)
	case 0x11:
		return decodeX86ALURM(d, "ADC", true, false)
	case 0x19:
		return decodeX86ALURM(d, "SBB", true, false)
	case 0x21:
		return decodeX86ALURM(d, "AND", true, false)
	case 0x29:
		return decodeX86ALURM(d, "SUB", true, false)
	case 0x31:
		return decodeX86ALURM(d, "XOR", true, false)
	case 0x39:
		return decodeX86ALURM(d, "CMP", true, false)

	// ALU r8, r/m8
	case 0x02:
		return decodeX86ALURM(d, "ADD", false, true)
	case 0x0A:
		return decodeX86ALURM(d, "OR", false, true)
	case 0x12:
		return decodeX86ALURM(d, "ADC", false, true)
	case 0x1A:
		return decodeX86ALURM(d, "SBB", false, true)
	case 0x22:
		return decodeX86ALURM(d, "AND", false, true)
	case 0x2A:
		return decodeX86ALURM(d, "SUB", false, true)
	case 0x32:
		return decodeX86ALURM(d, "XOR", false, true)
	case 0x3A:
		return decodeX86ALURM(d, "CMP", false, true)

	// ALU r32, r/m32
	case 0x03:
		return decodeX86ALURM(d, "ADD", true, true)
	case 0x0B:
		return decodeX86ALURM(d, "OR", true, true)
	case 0x13:
		return decodeX86ALURM(d, "ADC", true, true)
	case 0x1B:
		return decodeX86ALURM(d, "SBB", true, true)
	case 0x23:
		return decodeX86ALURM(d, "AND", true, true)
	case 0x2B:
		return decodeX86ALURM(d, "SUB", true, true)
	case 0x33:
		return decodeX86ALURM(d, "XOR", true, true)
	case 0x3B:
		return decodeX86ALURM(d, "CMP", true, true)

	// ALU AL, imm8
	case 0x04:
		imm, _ := d.readByte()
		return fmt.Sprintf("ADD AL, 0x%02X", imm)
	case 0x0C:
		imm, _ := d.readByte()
		return fmt.Sprintf("OR AL, 0x%02X", imm)
	case 0x14:
		imm, _ := d.readByte()
		return fmt.Sprintf("ADC AL, 0x%02X", imm)
	case 0x1C:
		imm, _ := d.readByte()
		return fmt.Sprintf("SBB AL, 0x%02X", imm)
	case 0x24:
		imm, _ := d.readByte()
		return fmt.Sprintf("AND AL, 0x%02X", imm)
	case 0x2C:
		imm, _ := d.readByte()
		return fmt.Sprintf("SUB AL, 0x%02X", imm)
	case 0x34:
		imm, _ := d.readByte()
		return fmt.Sprintf("XOR AL, 0x%02X", imm)
	case 0x3C:
		imm, _ := d.readByte()
		return fmt.Sprintf("CMP AL, 0x%02X", imm)

	// ALU EAX, imm32
	case 0x05:
		return decodeX86ALUAXImm(d, "ADD", hasOpSize)
	case 0x0D:
		return decodeX86ALUAXImm(d, "OR", hasOpSize)
	case 0x15:
		return decodeX86ALUAXImm(d, "ADC", hasOpSize)
	case 0x1D:
		return decodeX86ALUAXImm(d, "SBB", hasOpSize)
	case 0x25:
		return decodeX86ALUAXImm(d, "AND", hasOpSize)
	case 0x2D:
		return decodeX86ALUAXImm(d, "SUB", hasOpSize)
	case 0x35:
		return decodeX86ALUAXImm(d, "XOR", hasOpSize)
	case 0x3D:
		return decodeX86ALUAXImm(d, "CMP", hasOpSize)

	// Group 1: ALU r/m, imm
	case 0x80: // r/m8, imm8
		return decodeX86Group1(d, false, false)
	case 0x81: // r/m32, imm32
		return decodeX86Group1(d, true, false)
	case 0x83: // r/m32, imm8 (sign-extended)
		return decodeX86Group1(d, true, true)

	// MOV r/m8, r8
	case 0x88:
		return decodeX86ALURM(d, "MOV", false, false)
	// MOV r/m32, r32
	case 0x89:
		return decodeX86ALURM(d, "MOV", true, false)
	// MOV r8, r/m8
	case 0x8A:
		return decodeX86ALURM(d, "MOV", false, true)
	// MOV r32, r/m32
	case 0x8B:
		return decodeX86ALURM(d, "MOV", true, true)

	// LEA
	case 0x8D:
		return decodeX86ALURM(d, "LEA", true, true)

	// TEST r/m8, r8
	case 0x84:
		return decodeX86ALURM(d, "TEST", false, false)
	// TEST r/m32, r32
	case 0x85:
		return decodeX86ALURM(d, "TEST", true, false)
	// TEST AL, imm8
	case 0xA8:
		imm, _ := d.readByte()
		return fmt.Sprintf("TEST AL, 0x%02X", imm)
	// TEST EAX, imm32
	case 0xA9:
		if hasOpSize {
			imm, _ := d.readWord()
			return fmt.Sprintf("TEST AX, 0x%04X", imm)
		}
		imm, _ := d.readDword()
		return fmt.Sprintf("TEST EAX, 0x%08X", imm)

	// XCHG r/m8, r8
	case 0x86:
		return decodeX86ALURM(d, "XCHG", false, false)
	// XCHG r/m32, r32
	case 0x87:
		return decodeX86ALURM(d, "XCHG", true, false)

	// MOV r/m, imm
	case 0xC6: // MOV r/m8, imm8
		rm, _ := d.decodeModRM(false)
		imm, _ := d.readByte()
		return fmt.Sprintf("MOV %s, 0x%02X", rm, imm)
	case 0xC7: // MOV r/m32, imm32
		rm, _ := d.decodeModRM(true)
		if hasOpSize {
			imm, _ := d.readWord()
			return fmt.Sprintf("MOV %s, 0x%04X", rm, imm)
		}
		imm, _ := d.readDword()
		return fmt.Sprintf("MOV %s, 0x%08X", rm, imm)

	// Shift/rotate group
	case 0xC0: // r/m8, imm8
		return decodeX86ShiftGroup(d, false, -1)
	case 0xC1: // r/m32, imm8
		return decodeX86ShiftGroup(d, true, -1)
	case 0xD0: // r/m8, 1
		return decodeX86ShiftGroup(d, false, 1)
	case 0xD1: // r/m32, 1
		return decodeX86ShiftGroup(d, true, 1)
	case 0xD2: // r/m8, CL
		return decodeX86ShiftGroup(d, false, -2)
	case 0xD3: // r/m32, CL
		return decodeX86ShiftGroup(d, true, -2)

	// Group 3: TEST/NOT/NEG/MUL/IMUL/DIV/IDIV
	case 0xF6:
		return decodeX86Group3(d, false)
	case 0xF7:
		return decodeX86Group3(d, true)

	// Group 5: INC/DEC/CALL/JMP/PUSH r/m
	case 0xFF:
		return decodeX86Group5(d, hasOpSize)

	// PUSH imm
	case 0x68:
		if hasOpSize {
			imm, _ := d.readWord()
			return fmt.Sprintf("PUSH 0x%04X", imm)
		}
		imm, _ := d.readDword()
		return fmt.Sprintf("PUSH 0x%08X", imm)
	case 0x6A:
		imm, _ := d.readByte()
		return fmt.Sprintf("PUSH 0x%02X", imm)

	// Segment pushes/pops
	case 0x06:
		return "PUSH ES"
	case 0x07:
		return "POP ES"
	case 0x0E:
		return "PUSH CS"
	case 0x16:
		return "PUSH SS"
	case 0x17:
		return "POP SS"
	case 0x1E:
		return "PUSH DS"
	case 0x1F:
		return "POP DS"

	// Flag operations
	case 0x9C:
		return "PUSHFD"
	case 0x9D:
		return "POPFD"
	case 0x9E:
		return "SAHF"
	case 0x9F:
		return "LAHF"
	case 0xF5:
		return "CMC"
	case 0xF8:
		return "CLC"
	case 0xF9:
		return "STC"
	case 0xFA:
		return "CLI"
	case 0xFB:
		return "STI"
	case 0xFC:
		return "CLD"
	case 0xFD:
		return "STD"

	// String operations
	case 0xA4:
		return "MOVSB"
	case 0xA5:
		if hasOpSize {
			return "MOVSW"
		}
		return "MOVSD"
	case 0xA6:
		return "CMPSB"
	case 0xA7:
		if hasOpSize {
			return "CMPSW"
		}
		return "CMPSD"
	case 0xAA:
		return "STOSB"
	case 0xAB:
		if hasOpSize {
			return "STOSW"
		}
		return "STOSD"
	case 0xAC:
		return "LODSB"
	case 0xAD:
		if hasOpSize {
			return "LODSW"
		}
		return "LODSD"
	case 0xAE:
		return "SCASB"
	case 0xAF:
		if hasOpSize {
			return "SCASW"
		}
		return "SCASD"

	// CBW/CWDE/CWD/CDQ
	case 0x98:
		if hasOpSize {
			return "CBW"
		}
		return "CWDE"
	case 0x99:
		if hasOpSize {
			return "CWD"
		}
		return "CDQ"

	// IN/OUT
	case 0xE4:
		port, _ := d.readByte()
		return fmt.Sprintf("IN AL, 0x%02X", port)
	case 0xE5:
		port, _ := d.readByte()
		if hasOpSize {
			return fmt.Sprintf("IN AX, 0x%02X", port)
		}
		return fmt.Sprintf("IN EAX, 0x%02X", port)
	case 0xE6:
		port, _ := d.readByte()
		return fmt.Sprintf("OUT 0x%02X, AL", port)
	case 0xE7:
		port, _ := d.readByte()
		if hasOpSize {
			return fmt.Sprintf("OUT 0x%02X, AX", port)
		}
		return fmt.Sprintf("OUT 0x%02X, EAX", port)
	case 0xEC:
		return "IN AL, DX"
	case 0xED:
		if hasOpSize {
			return "IN AX, DX"
		}
		return "IN EAX, DX"
	case 0xEE:
		return "OUT DX, AL"
	case 0xEF:
		if hasOpSize {
			return "OUT DX, AX"
		}
		return "OUT DX, EAX"

	// ENTER/LEAVE
	case 0xC8:
		size, _ := d.readWord()
		level, _ := d.readByte()
		return fmt.Sprintf("ENTER 0x%04X, %d", size, level)
	case 0xC9:
		return "LEAVE"

	// Two-byte opcode (0F prefix)
	case 0x0F:
		return decodeX86TwoByteOpcode(d, hasOpSize)

	// MOVSX/MOVZX single byte entries go through 0F
	case 0x8C: // MOV r/m16, Sreg
		b, ok := d.readByte()
		if !ok {
			return "MOV r/m16, Sreg"
		}
		mod := (b >> 6) & 3
		sreg := (b >> 3) & 7
		rm := b & 7
		segName := "??"
		if int(sreg) < len(x86SegRegs) {
			segName = x86SegRegs[sreg]
		}
		if mod == 3 {
			return fmt.Sprintf("MOV %s, %s", x86Reg16[rm], segName)
		}
		d.pos-- // unread the ModRM byte, let decodeModRM handle
		rmStr, _ := d.decodeModRM(true)
		return fmt.Sprintf("MOV %s, %s", rmStr, segName)

	case 0x8E: // MOV Sreg, r/m16
		b, ok := d.readByte()
		if !ok {
			return "MOV Sreg, r/m16"
		}
		mod := (b >> 6) & 3
		sreg := (b >> 3) & 7
		rm := b & 7
		segName := "??"
		if int(sreg) < len(x86SegRegs) {
			segName = x86SegRegs[sreg]
		}
		if mod == 3 {
			return fmt.Sprintf("MOV %s, %s", segName, x86Reg16[rm])
		}
		d.pos--
		rmStr, _ := d.decodeModRM(true)
		return fmt.Sprintf("MOV %s, %s", segName, rmStr)
	}

	return fmt.Sprintf("db 0x%02X", op)
}

func decodeX86TwoByteOpcode(d *x86Disasm, hasOpSize bool) string {
	op, ok := d.readByte()
	if !ok {
		return "db 0x0F, ??"
	}

	switch op {
	// Jcc rel32
	case 0x80, 0x81, 0x82, 0x83, 0x84, 0x85, 0x86, 0x87,
		0x88, 0x89, 0x8A, 0x8B, 0x8C, 0x8D, 0x8E, 0x8F:
		if hasOpSize {
			off, _ := d.readWord()
			target := d.pos + uint64(int16(off))
			return fmt.Sprintf("J%s 0x%08X", x86Cond[op-0x80], target)
		}
		off, _ := d.readDword()
		target := d.pos + uint64(int32(off))
		return fmt.Sprintf("J%s 0x%08X", x86Cond[op-0x80], target)

	// SETcc r/m8
	case 0x90, 0x91, 0x92, 0x93, 0x94, 0x95, 0x96, 0x97,
		0x98, 0x99, 0x9A, 0x9B, 0x9C, 0x9D, 0x9E, 0x9F:
		rm, _ := d.decodeModRM(false)
		return fmt.Sprintf("SET%s %s", x86Cond[op-0x90], rm)

	// MOVZX r32, r/m8
	case 0xB6:
		return decodeX86ALURM(d, "MOVZX", false, true)
	// MOVZX r32, r/m16
	case 0xB7:
		return decodeX86ALURM(d, "MOVZX", true, true)
	// MOVSX r32, r/m8
	case 0xBE:
		return decodeX86ALURM(d, "MOVSX", false, true)
	// MOVSX r32, r/m16
	case 0xBF:
		return decodeX86ALURM(d, "MOVSX", true, true)

	// IMUL r32, r/m32
	case 0xAF:
		return decodeX86ALURM(d, "IMUL", true, true)

	// BSF/BSR
	case 0xBC:
		return decodeX86ALURM(d, "BSF", true, true)
	case 0xBD:
		return decodeX86ALURM(d, "BSR", true, true)

	// BT/BTS/BTR/BTC r/m32, r32
	case 0xA3:
		return decodeX86ALURM(d, "BT", true, false)
	case 0xAB:
		return decodeX86ALURM(d, "BTS", true, false)
	case 0xB3:
		return decodeX86ALURM(d, "BTR", true, false)
	case 0xBB:
		return decodeX86ALURM(d, "BTC", true, false)

	// SHLD/SHRD
	case 0xA4:
		return decodeX86DoubleShift(d, "SHLD", false)
	case 0xA5:
		return decodeX86DoubleShift(d, "SHLD", true)
	case 0xAC:
		return decodeX86DoubleShift(d, "SHRD", false)
	case 0xAD:
		return decodeX86DoubleShift(d, "SHRD", true)

	// BSWAP
	case 0xC8, 0xC9, 0xCA, 0xCB, 0xCC, 0xCD, 0xCE, 0xCF:
		return fmt.Sprintf("BSWAP %s", x86Reg32[op-0xC8])

	// CPUID
	case 0xA2:
		return "CPUID"
	// RDTSC
	case 0x31:
		return "RDTSC"
	// WBINVD
	case 0x09:
		return "WBINVD"
	// INVD
	case 0x08:
		return "INVD"
	}

	return fmt.Sprintf("db 0x0F, 0x%02X", op)
}

func decodeX86ALURM(d *x86Disasm, name string, wide bool, regIsSource bool) string {
	b, ok := d.readByte()
	if !ok {
		return name + " ???"
	}
	reg := (b >> 3) & 7
	mod := (b >> 6) & 3
	rm := b & 7

	var regName string
	if wide {
		regName = x86Reg32[reg]
	} else {
		regName = x86Reg8[reg]
	}

	if mod == 3 {
		var rmName string
		if wide {
			rmName = x86Reg32[rm]
		} else {
			rmName = x86Reg8[rm]
		}
		if regIsSource {
			return fmt.Sprintf("%s %s, %s", name, regName, rmName)
		}
		return fmt.Sprintf("%s %s, %s", name, rmName, regName)
	}

	// Need to decode memory operand
	d.pos-- // put back ModRM byte
	rmStr, _ := d.decodeModRM(wide)

	if regIsSource {
		return fmt.Sprintf("%s %s, %s", name, regName, rmStr)
	}
	return fmt.Sprintf("%s %s, %s", name, rmStr, regName)
}

func decodeX86ALUAXImm(d *x86Disasm, name string, hasOpSize bool) string {
	if hasOpSize {
		imm, _ := d.readWord()
		return fmt.Sprintf("%s AX, 0x%04X", name, imm)
	}
	imm, _ := d.readDword()
	return fmt.Sprintf("%s EAX, 0x%08X", name, imm)
}

func decodeX86Group1(d *x86Disasm, wide bool, signExtend bool) string {
	ops := [8]string{"ADD", "OR", "ADC", "SBB", "AND", "SUB", "XOR", "CMP"}
	b, ok := d.readByte()
	if !ok {
		return "??? r/m, imm"
	}
	op := (b >> 3) & 7
	mod := (b >> 6) & 3
	rm := b & 7

	var rmStr string
	if mod == 3 {
		if wide {
			rmStr = x86Reg32[rm]
		} else {
			rmStr = x86Reg8[rm]
		}
	} else {
		d.pos--
		rmStr, _ = d.decodeModRM(wide)
	}

	if !wide {
		imm, _ := d.readByte()
		return fmt.Sprintf("%s %s, 0x%02X", ops[op], rmStr, imm)
	}
	if signExtend {
		imm, _ := d.readByte()
		return fmt.Sprintf("%s %s, 0x%02X", ops[op], rmStr, imm)
	}
	imm, _ := d.readDword()
	return fmt.Sprintf("%s %s, 0x%08X", ops[op], rmStr, imm)
}

func decodeX86ShiftGroup(d *x86Disasm, wide bool, countType int) string {
	ops := [8]string{"ROL", "ROR", "RCL", "RCR", "SHL", "SHR", "SAL", "SAR"}
	b, ok := d.readByte()
	if !ok {
		return "SHIFT ???"
	}
	op := (b >> 3) & 7
	mod := (b >> 6) & 3
	rm := b & 7

	var rmStr string
	if mod == 3 {
		if wide {
			rmStr = x86Reg32[rm]
		} else {
			rmStr = x86Reg8[rm]
		}
	} else {
		d.pos--
		rmStr, _ = d.decodeModRM(wide)
	}

	switch countType {
	case 1:
		return fmt.Sprintf("%s %s, 1", ops[op], rmStr)
	case -2:
		return fmt.Sprintf("%s %s, CL", ops[op], rmStr)
	default:
		imm, _ := d.readByte()
		return fmt.Sprintf("%s %s, %d", ops[op], rmStr, imm)
	}
}

func decodeX86Group3(d *x86Disasm, wide bool) string {
	b, ok := d.readByte()
	if !ok {
		return "GRP3 ???"
	}
	op := (b >> 3) & 7
	mod := (b >> 6) & 3
	rm := b & 7

	var rmStr string
	if mod == 3 {
		if wide {
			rmStr = x86Reg32[rm]
		} else {
			rmStr = x86Reg8[rm]
		}
	} else {
		d.pos--
		rmStr, _ = d.decodeModRM(wide)
	}

	switch op {
	case 0: // TEST
		if wide {
			imm, _ := d.readDword()
			return fmt.Sprintf("TEST %s, 0x%08X", rmStr, imm)
		}
		imm, _ := d.readByte()
		return fmt.Sprintf("TEST %s, 0x%02X", rmStr, imm)
	case 2:
		return fmt.Sprintf("NOT %s", rmStr)
	case 3:
		return fmt.Sprintf("NEG %s", rmStr)
	case 4:
		if wide {
			return fmt.Sprintf("MUL %s", rmStr)
		}
		return fmt.Sprintf("MUL %s", rmStr)
	case 5:
		return fmt.Sprintf("IMUL %s", rmStr)
	case 6:
		return fmt.Sprintf("DIV %s", rmStr)
	case 7:
		return fmt.Sprintf("IDIV %s", rmStr)
	}
	return fmt.Sprintf("GRP3/%d %s", op, rmStr)
}

func decodeX86Group5(d *x86Disasm, hasOpSize bool) string {
	b, ok := d.readByte()
	if !ok {
		return "GRP5 ???"
	}
	op := (b >> 3) & 7
	mod := (b >> 6) & 3
	rm := b & 7

	var rmStr string
	if mod == 3 {
		rmStr = x86Reg32[rm]
	} else {
		d.pos--
		rmStr, _ = d.decodeModRM(true)
	}

	switch op {
	case 0:
		return fmt.Sprintf("INC %s", rmStr)
	case 1:
		return fmt.Sprintf("DEC %s", rmStr)
	case 2:
		return fmt.Sprintf("CALL %s", rmStr)
	case 4:
		return fmt.Sprintf("JMP %s", rmStr)
	case 6:
		return fmt.Sprintf("PUSH %s", rmStr)
	}
	return fmt.Sprintf("GRP5/%d %s", op, rmStr)
}

func decodeX86DoubleShift(d *x86Disasm, name string, byCL bool) string {
	b, ok := d.readByte()
	if !ok {
		return name + " ???"
	}
	reg := (b >> 3) & 7
	mod := (b >> 6) & 3
	rm := b & 7

	var rmStr string
	if mod == 3 {
		rmStr = x86Reg32[rm]
	} else {
		d.pos--
		rmStr, _ = d.decodeModRM(true)
	}

	if byCL {
		return fmt.Sprintf("%s %s, %s, CL", name, rmStr, x86Reg32[reg])
	}
	imm, _ := d.readByte()
	return fmt.Sprintf("%s %s, %s, %d", name, rmStr, x86Reg32[reg], imm)
}

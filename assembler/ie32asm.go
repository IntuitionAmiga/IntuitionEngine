// ie32asm.go

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

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	// Opcodes
	LOAD  = 0x01
	LDA   = 0x20
	LDX   = 0x21
	LDY   = 0x22
	LDZ   = 0x23
	STORE = 0x02
	STA   = 0x24 // New store opcodes
	STX   = 0x25
	STY   = 0x26
	STZ   = 0x27
	ADD   = 0x03
	SUB   = 0x04
	AND   = 0x05
	JMP   = 0x06
	JNZ   = 0x07
	JZ    = 0x08
	OR    = 0x09
	XOR   = 0x0A
	SHL   = 0x0B
	SHR   = 0x0C
	NOT   = 0x0D
	JGT   = 0x0E
	JGE   = 0x0F
	JLT   = 0x10
	JLE   = 0x11
	PUSH  = 0x12
	POP   = 0x13
	MUL   = 0x14
	DIV   = 0x15
	MOD   = 0x16
	WAIT  = 0x17
	JSR   = 0x18
	RTS   = 0x19
	SEI   = 0x1A
	CLI   = 0x1B
	RTI   = 0x1C
	INC   = 0x28
	DEC   = 0x29

	// New load opcodes start at 0x3A
	LDB = 0x3A
	LDC = 0x3B
	LDD = 0x3C
	LDE = 0x3D
	LDF = 0x3E
	LDG = 0x3F
	LDH = 0x4C
	LDS = 0x4D
	LDT = 0x4E
	LDU = 0x40
	LDV = 0x41
	LDW = 0x42

	// New store opcodes
	STB = 0x43
	STC = 0x44
	STD = 0x45
	STE = 0x46
	STF = 0x47
	STG = 0x48
	STH = 0x4F
	STS = 0x50
	STT = 0x51
	STU = 0x49
	STV = 0x4A
	STW = 0x4B

	NOP  = 0xEE
	HALT = 0xFF

	// Addressing modes
	ADDR_IMMEDIATE = 0x00
	ADDR_REGISTER  = 0x01
	ADDR_REG_IND   = 0x02
	ADDR_MEM_IND   = 0x03
	ADDR_DIRECT    = 0x04 // Direct memory addressing (write to operand address)

	// Memory Map
	PROG_START = 0x1000
	DATA_START = 0x2000

	// Hardware registers
	TIMER_CTRL   uint32 = 0xF800
	TIMER_COUNT  uint32 = 0xF804
	TIMER_PERIOD uint32 = 0xF808
)

var registers = map[string]byte{
	"A": 0,
	"X": 1,
	"Y": 2,
	"Z": 3,
	"B": 4,
	"C": 5,
	"D": 6,
	"E": 7,
	"F": 8,
	"G": 9,
	"H": 10,
	"S": 11,
	"T": 12,
	"U": 13,
	"V": 14,
	"W": 15,
}

func writeLittleEndian(val uint32) []byte {
	return []byte{
		byte(val),
		byte(val >> 8),
		byte(val >> 16),
		byte(val >> 24),
	}
}

type Assembler struct {
	labels     map[string]uint32
	equates    map[string]uint32
	baseAddr   uint32
	codeOffset uint32
	data       []byte
	dataOffset uint32
}

func NewAssembler() *Assembler {
	return &Assembler{
		labels:     make(map[string]uint32),
		equates:    make(map[string]uint32),
		baseAddr:   PROG_START,
		codeOffset: 0,
		data:       make([]byte, 0),
		dataOffset: 0,
	}
}

func (a *Assembler) handleDirective(line string, lineNum int) error {
	parts := strings.Fields(line)
	if len(parts) < 2 {
		return fmt.Errorf("invalid directive format")
	}

	switch parts[0] {
	case ".word":
		value, err := strconv.ParseUint(parts[1], 0, 32)
		if err != nil {
			return fmt.Errorf("invalid word value: %s", parts[1])
		}
		a.data = append(a.data, writeLittleEndian(uint32(value))...)
		a.dataOffset += 4

	case ".equ":
		if len(parts) < 3 {
			return fmt.Errorf("invalid EQU format")
		}
		value, err := strconv.ParseUint(parts[2], 0, 32)
		if err != nil {
			return fmt.Errorf("invalid EQU value: %s", parts[2])
		}
		a.equates[parts[1]] = uint32(value)
		fmt.Printf("Added equate: %s = 0x%x\n", parts[1], value)

	case ".byte":
		value, err := strconv.ParseUint(parts[1], 0, 8)
		if err != nil {
			return fmt.Errorf("invalid byte value: %s", parts[1])
		}
		a.data = append(a.data, byte(value))
		a.dataOffset++

	case ".incbin":
		path := strings.Trim(parts[1], "\"")
		payload, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("incbin read failed: %s", path)
		}
		offset := uint64(0)
		length := uint64(len(payload))
		if len(parts) >= 3 {
			offset, err = strconv.ParseUint(parts[2], 0, 32)
			if err != nil {
				return fmt.Errorf("invalid incbin offset: %s", parts[2])
			}
			if offset > uint64(len(payload)) {
				return fmt.Errorf("incbin offset out of range: %d", offset)
			}
			length = uint64(len(payload)) - offset
		}
		if len(parts) >= 4 {
			length, err = strconv.ParseUint(parts[3], 0, 32)
			if err != nil {
				return fmt.Errorf("invalid incbin length: %s", parts[3])
			}
		}
		if offset+length > uint64(len(payload)) {
			return fmt.Errorf("incbin range out of bounds: %d..%d", offset, offset+length)
		}
		a.data = append(a.data, payload[offset:offset+length]...)
		a.dataOffset += uint32(length)

	case ".space":
		size, err := strconv.ParseUint(parts[1], 0, 32)
		if err != nil {
			return fmt.Errorf("invalid space size: %s", parts[1])
		}
		a.data = append(a.data, make([]byte, size)...)
		a.dataOffset += uint32(size)

	case ".ascii":
		// Find the quoted string in the line
		start := strings.Index(line, "\"")
		if start == -1 {
			return fmt.Errorf("invalid ascii format: missing opening quote")
		}
		end := strings.LastIndex(line, "\"")
		if end == start {
			return fmt.Errorf("invalid ascii format: missing closing quote")
		}
		str := line[start+1 : end]
		a.data = append(a.data, []byte(str)...)
		a.dataOffset += uint32(len(str))

	case ".org":
		if len(parts) < 2 {
			return fmt.Errorf("invalid org format")
		}
		addr, err := strconv.ParseUint(parts[1], 0, 32)
		if err != nil {
			return fmt.Errorf("invalid org address: %s", parts[1])
		}
		a.codeOffset = uint32(addr) - a.baseAddr
		fmt.Printf("Setting assembly address to 0x%x\n", addr)

	default:
		return fmt.Errorf("unknown directive: %s", parts[0])
	}
	return nil
}

func (a *Assembler) parseOperand(operand string, lineNum int) (byte, uint32, error) {
	fmt.Printf("Parsing operand: '%s'\n", operand)

	// Register-indirect addressing [reg] or [reg+offset]
	if strings.HasPrefix(operand, "[") && strings.HasSuffix(operand, "]") {
		inner := strings.Trim(operand, "[]")
		parts := strings.Split(inner, "+")

		reg := strings.TrimSpace(parts[0])
		if regNum, ok := registers[reg]; ok {
			if len(parts) == 1 {
				return ADDR_REG_IND, uint32(regNum), nil
			}
			if len(parts) == 2 {
				offset, err := strconv.ParseUint(strings.TrimSpace(parts[1]), 0, 32)
				if err != nil {
					return 0, 0, fmt.Errorf("invalid offset: %s", parts[1])
				}
				if offset&3 != 0 {
					return 0, 0, fmt.Errorf("offset must be multiple of 4")
				}
				return ADDR_REG_IND, uint32(regNum) | uint32(offset), nil
			}
		}
		return 0, 0, fmt.Errorf("invalid register in indirect addressing: %s", reg)
	}

	// Direct memory addressing @addr (write/read directly to/from this address)
	if strings.HasPrefix(operand, "@") {
		addr := strings.TrimPrefix(operand, "@")
		fmt.Printf("  Direct memory: addr='%s'\n", addr)

		// Handle equate
		if val, ok := a.equates[addr]; ok {
			fmt.Printf("    Found equate: val=0x%x\n", val)
			return ADDR_DIRECT, val, nil
		}

		// Handle hex address
		if strings.HasPrefix(addr, "0x") {
			val, err := strconv.ParseUint(addr, 0, 32)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid hex address: %s", addr)
			}
			fmt.Printf("    Parsed hex: val=0x%x\n", val)
			return ADDR_DIRECT, uint32(val), nil
		}

		// Handle label
		if labelAddr, ok := a.labels[addr]; ok {
			fmt.Printf("    Found label: addr=0x%x\n", labelAddr)
			return ADDR_DIRECT, labelAddr, nil
		}
		return 0, 0, fmt.Errorf("undefined label or invalid address: %s", addr)
	}

	// Immediate value #n
	if strings.HasPrefix(operand, "#") {
		numStr := operand[1:]
		fmt.Printf("  Immediate value: '%s'\n", numStr)
		// Handle equate
		if val, ok := a.equates[numStr]; ok {
			fmt.Printf("    Found equate: val=0x%x\n", val)
			return ADDR_IMMEDIATE, val, nil
		}
		val, err := strconv.ParseUint(numStr, 0, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid immediate value: %s", numStr)
		}
		fmt.Printf("    Parsed value: val=0x%x\n", val)
		return ADDR_IMMEDIATE, uint32(val), nil
	}

	// Register
	if regNum, ok := registers[operand]; ok {
		fmt.Printf("  Found register: reg=%s num=%d\n", operand, regNum)
		return ADDR_REGISTER, uint32(regNum), nil
	}

	// Equate
	if val, ok := a.equates[operand]; ok {
		fmt.Printf("  Found equate: val=0x%x\n", val)
		return ADDR_IMMEDIATE, val, nil
	}

	// Hex address
	if strings.HasPrefix(operand, "0x") {
		val, err := strconv.ParseUint(operand, 0, 32)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid hex address: %s", operand)
		}
		fmt.Printf("  Parsed hex: val=0x%x\n", val)
		return ADDR_IMMEDIATE, uint32(val), nil
	}

	// Label
	if labelAddr, ok := a.labels[operand]; ok {
		fmt.Printf("  Found label: addr=0x%x\n", labelAddr)
		return ADDR_IMMEDIATE, labelAddr, nil
	}

	return 0, 0, fmt.Errorf("invalid operand: %s", operand)
}

func (a *Assembler) assemble(code string) []byte {
	var program []byte
	maxAddr := a.baseAddr

	// First pass: collect labels and calculate sizes
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.Split(line, ";")[0]
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, ".org") {
			if err := a.handleDirective(line, 0); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		if strings.HasSuffix(line, ":") {
			label := strings.TrimSuffix(line, ":")
			if strings.HasPrefix(line, "data_") {
				a.labels[label] = DATA_START + a.dataOffset
			} else {
				a.labels[label] = a.baseAddr + a.codeOffset
			}
			fmt.Printf("Label '%s' at 0x%04x\n", label, a.labels[label])
			continue
		}

		if strings.HasPrefix(line, ".equ") {
			if err := a.handleDirective(line, 0); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		if !strings.HasPrefix(line, ".") {
			nextAddr := a.baseAddr + a.codeOffset + 8
			if nextAddr > maxAddr {
				maxAddr = nextAddr
			}
			a.codeOffset += 8
		}
	}

	// Reset offset for second pass
	a.codeOffset = 0
	program = make([]byte, maxAddr-a.baseAddr)

	// Second pass: generate code
	for lineNum, line := range lines {
		line = strings.Split(line, ";")[0]
		line = strings.TrimSpace(line)
		if line == "" || strings.HasSuffix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, ".") {
			if err := a.handleDirective(line, lineNum); err != nil {
				fmt.Printf("Line %d: %v\n", lineNum+1, err)
				os.Exit(1)
			}
			continue
		}

		// Split instruction and operands
		parts := strings.Fields(line)
		opcode := parts[0]
		var instruction []byte

		switch opcode {
		case "LOAD", "STORE", "ADD", "SUB", "AND", "OR", "XOR", "SHL", "SHR", "MUL", "DIV", "MOD":
			remainder := strings.Join(parts[1:], " ")
			regAndOp := strings.Split(remainder, ",")
			if len(regAndOp) != 2 {
				fmt.Printf("Line %d: Invalid instruction format: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			reg, ok := registers[strings.TrimSpace(regAndOp[0])]
			if !ok {
				fmt.Printf("Line %d: Invalid register: %s\n", lineNum+1, regAndOp[0])
				os.Exit(1)
			}

			operand := strings.TrimSpace(regAndOp[1])
			addrMode, value, err := a.parseOperand(operand, lineNum+1)
			if err != nil {
				fmt.Printf("Line %d: %v\n", lineNum+1, err)
				os.Exit(1)
			}

			var op byte
			switch opcode {
			case "LOAD":
				op = LOAD
			case "STORE":
				op = STORE
			case "ADD":
				op = ADD
			case "SUB":
				op = SUB
			case "AND":
				op = AND
			case "OR":
				op = OR
			case "XOR":
				op = XOR
			case "SHL":
				op = SHL
			case "SHR":
				op = SHR
			case "MUL":
				op = MUL
			case "DIV":
				op = DIV
			case "MOD":
				op = MOD
			}

			instruction = append(instruction, op, reg, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "LDA", "LDB", "LDC", "LDD", "LDE", "LDF", "LDG", "LDH", "LDS", "LDT", "LDU", "LDV", "LDW", "LDX", "LDY", "LDZ":
			if len(parts) != 2 {
				fmt.Printf("Line %d: Invalid load instruction format: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			var op byte
			var reg byte
			switch opcode {
			case "LDA":
				op = LDA
				reg = 0
			case "LDX":
				op = LDX
				reg = 1
			case "LDY":
				op = LDY
				reg = 2
			case "LDZ":
				op = LDZ
				reg = 3
			case "LDB":
				op = LDB
				reg = 4
			case "LDC":
				op = LDC
				reg = 5
			case "LDD":
				op = LDD
				reg = 6
			case "LDE":
				op = LDE
				reg = 7
			case "LDF":
				op = LDF
				reg = 8
			case "LDG":
				op = LDG
				reg = 9
			case "LDH":
				op = LDH
				reg = 10
			case "LDS":
				op = LDS
				reg = 11
			case "LDT":
				op = LDT
				reg = 12
			case "LDU":
				op = LDU
				reg = 13
			case "LDV":
				op = LDV
				reg = 14
			case "LDW":
				op = LDW
				reg = 15
			}

			operand := parts[1]
			addrMode, value, err := a.parseOperand(operand, lineNum+1)
			if err != nil {
				fmt.Printf("Line %d: %v\n", lineNum+1, err)
				os.Exit(1)
			}

			instruction = append(instruction, op, reg, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "STA", "STB", "STC", "STD", "STE", "STF", "STG", "STH", "STS", "STT", "STU", "STV", "STW", "STX", "STY", "STZ":
			if len(parts) != 2 {
				fmt.Printf("Line %d: Invalid store instruction format: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			var op byte
			var reg byte
			switch opcode {
			case "STA":
				op = STA
				reg = 0
			case "STX":
				op = STX
				reg = 1
			case "STY":
				op = STY
				reg = 2
			case "STZ":
				op = STZ
				reg = 3
			case "STB":
				op = STB
				reg = 4
			case "STC":
				op = STC
				reg = 5
			case "STD":
				op = STD
				reg = 6
			case "STE":
				op = STE
				reg = 7
			case "STF":
				op = STF
				reg = 8
			case "STG":
				op = STG
				reg = 9
			case "STH":
				op = STH
				reg = 10
			case "STS":
				op = STS
				reg = 11
			case "STT":
				op = STT
				reg = 12
			case "STU":
				op = STU
				reg = 13
			case "STV":
				op = STV
				reg = 14
			case "STW":
				op = STW
				reg = 15
			}

			operand := parts[1]
			addrMode, value, err := a.parseOperand(operand, lineNum+1)
			if err != nil {
				fmt.Printf("Line %d: %v\n", lineNum+1, err)
				os.Exit(1)
			}

			instruction = append(instruction, op, reg, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "INC", "DEC":
			if len(parts) != 2 {
				fmt.Printf("Line %d: Invalid increment/decrement instruction format: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			operand := parts[1]
			addrMode, value, err := a.parseOperand(operand, lineNum+1)
			if err != nil {
				fmt.Printf("Line %d: %v\n", lineNum+1, err)
				os.Exit(1)
			}

			var op byte
			switch opcode {
			case "INC":
				op = INC
			case "DEC":
				op = DEC
			}

			instruction = append(instruction, op, 0, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "NOT", "PUSH", "POP":
			if len(parts) != 2 {
				fmt.Printf("Line %d: Invalid %s instruction format: %s\n", lineNum+1, opcode, line)
				os.Exit(1)
			}

			reg, ok := registers[parts[1]]
			if !ok {
				fmt.Printf("Line %d: Invalid register: %s\n", lineNum+1, parts[1])
				os.Exit(1)
			}

			var op byte
			switch opcode {
			case "NOT":
				op = NOT
			case "PUSH":
				op = PUSH
			case "POP":
				op = POP
			}

			instruction = append(instruction, op, reg, 0, 0, 0, 0, 0, 0)

		case "JMP", "JSR":
			if len(parts) != 2 {
				fmt.Printf("Line %d: Invalid jump instruction: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			target := parts[1]
			var targetAddr uint32

			if val, ok := a.equates[target]; ok {
				targetAddr = val
			} else if addr, ok := a.labels[target]; ok {
				targetAddr = addr
			} else {
				fmt.Printf("Line %d: Undefined label or equate: %s\n", lineNum+1, target)
				os.Exit(1)
			}

			var op byte
			switch opcode {
			case "JMP":
				op = JMP
			case "JSR":
				op = JSR
			}

			instruction = append(instruction, op, 0, 0, 0)
			instruction = append(instruction, writeLittleEndian(targetAddr)...)

		case "JNZ", "JZ", "JGT", "JGE", "JLT", "JLE":
			remainder := strings.Join(parts[1:], " ")
			regAndLabel := strings.Split(remainder, ",")
			if len(regAndLabel) != 2 {
				fmt.Printf("Line %d: Invalid branch instruction format: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			reg, ok := registers[strings.TrimSpace(regAndLabel[0])]
			if !ok {
				fmt.Printf("Line %d: Invalid register: %s\n", lineNum+1, regAndLabel[0])
				os.Exit(1)
			}

			target := strings.TrimSpace(regAndLabel[1])
			var targetAddr uint32

			if val, ok := a.equates[target]; ok {
				targetAddr = val
			} else if addr, ok := a.labels[target]; ok {
				targetAddr = addr
			} else {
				fmt.Printf("Line %d: Undefined label or equate: %s\n", lineNum+1, target)
				os.Exit(1)
			}

			var op byte
			switch opcode {
			case "JNZ":
				op = JNZ
			case "JZ":
				op = JZ
			case "JGT":
				op = JGT
			case "JGE":
				op = JGE
			case "JLT":
				op = JLT
			case "JLE":
				op = JLE
			}

			instruction = append(instruction, op, reg, 0, 0)
			instruction = append(instruction, writeLittleEndian(targetAddr)...)

		case "WAIT":
			if len(parts) != 2 {
				fmt.Printf("Line %d: Invalid WAIT instruction: %s\n", lineNum+1, line)
				os.Exit(1)
			}

			addrMode, value, err := a.parseOperand(parts[1], lineNum+1)
			if err != nil {
				fmt.Printf("Line %d: %v\n", lineNum+1, err)
				os.Exit(1)
			}

			instruction = append(instruction, WAIT, 0, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "SEI", "CLI", "RTI", "RTS", "NOP", "HALT":
			var op byte
			switch opcode {
			case "SEI":
				op = SEI
			case "CLI":
				op = CLI
			case "RTI":
				op = RTI
			case "RTS":
				op = RTS
			case "NOP":
				op = NOP
			case "HALT":
				op = HALT
			}
			instruction = append(instruction, op, 0, 0, 0, 0, 0, 0, 0)

		default:
			fmt.Printf("Line %d: Unknown instruction: %s\n", lineNum+1, opcode)
			os.Exit(1)
		}

		copy(program[a.codeOffset:], instruction)
		a.codeOffset += 8
	}

	// Append data section after program
	return append(program, a.data...)
}

func main() {
	if len(os.Args) != 2 {
		fmt.Println("Usage: iasm <input.asm>")
		os.Exit(1)
	}

	code, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading input file: %v\n", err)
		os.Exit(1)
	}

	asm := NewAssembler()
	binary := asm.assemble(string(code))

	outFile := strings.TrimSuffix(os.Args[1], ".asm") + ".iex"
	if err := os.WriteFile(outFile, binary, 0644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully assembled to %s (%d bytes)\n", outFile, len(binary))
}

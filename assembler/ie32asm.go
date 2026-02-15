// ie32asm.go

//go:build !ie64 && !ie64dis

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
	"path/filepath"
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
	labels       map[string]uint32
	equates      map[string]uint32
	baseAddr     uint32
	codeOffset   uint32
	basePath     string
	includePaths []string
}

// resolveFile searches for filename relative to basePath first, then each
// -I include path in command-line order. Returns the resolved path.
func resolveFile(filename, basePath string, includePaths []string) (string, error) {
	if filepath.IsAbs(filename) {
		if _, err := os.Stat(filename); err == nil {
			return filename, nil
		}
		return "", fmt.Errorf("file not found: %s", filename)
	}
	candidate := filepath.Join(basePath, filename)
	if _, err := os.Stat(candidate); err == nil {
		return candidate, nil
	}
	for _, dir := range includePaths {
		candidate = filepath.Join(dir, filename)
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("file not found: %s (searched: %s, -I paths: %v)", filename, basePath, includePaths)
}

func NewAssembler() *Assembler {
	return &Assembler{
		labels:     make(map[string]uint32),
		equates:    make(map[string]uint32),
		baseAddr:   PROG_START,
		codeOffset: 0,
	}
}

// preprocessIncludes expands .include directives recursively
func preprocessIncludes(code string, basePath string, includePaths []string, included map[string]bool) (string, error) {
	if included == nil {
		included = make(map[string]bool)
	}

	var result strings.Builder
	lines := strings.SplitSeq(code, "\n")

	for line := range lines {
		trimmed := strings.TrimSpace(line)

		// Check for .include directive
		if strings.HasPrefix(trimmed, ".include") {
			parts := strings.Fields(trimmed)
			if len(parts) < 2 {
				return "", fmt.Errorf("invalid .include format: %s", line)
			}

			// Extract filename (remove quotes)
			filename := strings.Trim(parts[1], "\"'")

			// Resolve path using basePath + include paths
			includePath, err := resolveFile(filename, basePath, includePaths)
			if err != nil {
				return "", fmt.Errorf("failed to include %s: %v", filename, err)
			}

			// Check for circular includes
			absPath, _ := filepath.Abs(includePath)
			if included[absPath] {
				// Already included, skip
				continue
			}
			included[absPath] = true

			// Read the included file
			includeContent, err := os.ReadFile(includePath)
			if err != nil {
				return "", fmt.Errorf("failed to include %s: %v", includePath, err)
			}

			// Recursively process includes in the included file
			processed, err := preprocessIncludes(string(includeContent), filepath.Dir(includePath), includePaths, included)
			if err != nil {
				return "", err
			}

			result.WriteString(processed)
			result.WriteString("\n")
		} else {
			result.WriteString(line)
			result.WriteString("\n")
		}
	}

	return result.String(), nil
}

// handleDirective processes assembler directives and writes data to the program buffer.
// For pass 2, program should be non-nil and data will be written at codeOffset.
// For pass 1 (program == nil), only .org and .equ are processed.
func (a *Assembler) handleDirective(line string, lineNum int, program []byte) error {
	parts := strings.Fields(line)
	if len(parts) < 2 && parts[0] != ".org" {
		return fmt.Errorf("invalid directive format")
	}

	switch parts[0] {
	case ".word":
		// Handle comma-separated list of word values (supports negative numbers)
		wordList := strings.Join(parts[1:], " ")
		wordValues := strings.Split(wordList, ",")
		for _, wv := range wordValues {
			wv = strings.TrimSpace(wv)
			if wv == "" {
				continue
			}
			// Try to parse as signed number first (supports negative values)
			value, err := strconv.ParseInt(wv, 0, 32)
			if err != nil {
				// Try unsigned for large hex values
				uvalue, uerr := strconv.ParseUint(wv, 0, 32)
				if uerr != nil {
					// Not a number, check if it's an equate
					if val, ok := a.equates[wv]; ok {
						value = int64(val)
					} else if labelAddr, ok := a.labels[wv]; ok {
						// Check if it's a label
						value = int64(labelAddr)
					} else {
						return fmt.Errorf("invalid word value or unknown symbol: %s", wv)
					}
				} else {
					value = int64(uvalue)
				}
			}
			if program != nil {
				copy(program[a.codeOffset:], writeLittleEndian(uint32(value)))
				a.codeOffset += 4
			}
		}

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
		// Handle comma-separated list of byte values
		byteList := strings.Join(parts[1:], " ")
		byteValues := strings.Split(byteList, ",")
		for _, bv := range byteValues {
			bv = strings.TrimSpace(bv)
			if bv == "" {
				continue
			}
			value, err := strconv.ParseUint(bv, 0, 8)
			if err != nil {
				return fmt.Errorf("invalid byte value: %s", bv)
			}
			if program != nil {
				program[a.codeOffset] = byte(value)
				a.codeOffset++
			}
		}

	case ".incbin":
		filename := strings.Trim(parts[1], "\"")
		path, err := resolveFile(filename, a.basePath, a.includePaths)
		if err != nil {
			return fmt.Errorf("incbin: %v", err)
		}
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
		if program != nil {
			copy(program[a.codeOffset:], payload[offset:offset+length])
			a.codeOffset += uint32(length)
		}

	case ".space":
		size, err := strconv.ParseUint(parts[1], 0, 32)
		if err != nil {
			return fmt.Errorf("invalid space size: %s", parts[1])
		}
		if program != nil {
			// Zero-fill the space (Go slices are already zero-initialized, but be explicit)
			for i := range size {
				program[a.codeOffset+uint32(i)] = 0
			}
			a.codeOffset += uint32(size)
		}

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
		if program != nil {
			copy(program[a.codeOffset:], []byte(str))
			a.codeOffset += uint32(len(str))
		}

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
	if after, ok := strings.CutPrefix(operand, "@"); ok {
		addr := after
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
		// Handle label
		if labelAddr, ok := a.labels[numStr]; ok {
			fmt.Printf("    Found label: addr=0x%x\n", labelAddr)
			return ADDR_IMMEDIATE, labelAddr, nil
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

// calcDirectiveSize calculates the size in bytes of a data directive for pass 1.
// Returns 0 for directives that don't emit data (like .equ, .org).
func (a *Assembler) calcDirectiveSize(line string) uint32 {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return 0
	}

	switch parts[0] {
	case ".word":
		// Count comma-separated values
		wordList := strings.Join(parts[1:], " ")
		wordValues := strings.Split(wordList, ",")
		count := uint32(0)
		for _, wv := range wordValues {
			wv = strings.TrimSpace(wv)
			if wv != "" {
				count++
			}
		}
		return count * 4 // 4 bytes per word

	case ".byte":
		// Count comma-separated values
		byteList := strings.Join(parts[1:], " ")
		byteValues := strings.Split(byteList, ",")
		count := uint32(0)
		for _, bv := range byteValues {
			bv = strings.TrimSpace(bv)
			if bv != "" {
				count++
			}
		}
		return count // 1 byte per value

	case ".incbin":
		filename := strings.Trim(parts[1], "\"")
		path, err := resolveFile(filename, a.basePath, a.includePaths)
		if err != nil {
			fmt.Printf("Warning: cannot resolve incbin file %s: %v\n", filename, err)
			return 0
		}
		info, err := os.Stat(path)
		if err != nil {
			fmt.Printf("Warning: cannot stat incbin file %s: %v\n", path, err)
			return 0
		}
		length := uint64(info.Size())
		// Handle optional offset and length parameters
		if len(parts) >= 3 {
			offset, err := strconv.ParseUint(parts[2], 0, 32)
			if err == nil && offset < length {
				length -= offset
			}
		}
		if len(parts) >= 4 {
			specLen, err := strconv.ParseUint(parts[3], 0, 32)
			if err == nil {
				length = specLen
			}
		}
		return uint32(length)

	case ".space":
		if len(parts) >= 2 {
			size, err := strconv.ParseUint(parts[1], 0, 32)
			if err == nil {
				return uint32(size)
			}
		}
		return 0

	case ".ascii":
		// Find the quoted string
		start := strings.Index(line, "\"")
		end := strings.LastIndex(line, "\"")
		if start != -1 && end > start {
			return uint32(end - start - 1)
		}
		return 0

	default:
		return 0
	}
}

func (a *Assembler) assemble(code string) []byte {
	var program []byte
	maxAddr := a.baseAddr

	// First pass: collect labels and calculate sizes
	// We use codeOffset to track the current position in the output,
	// including both code AND data directives.
	lines := strings.Split(code, "\n")
	for _, line := range lines {
		line = strings.Split(line, ";")[0]
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, ".org") {
			if err := a.handleDirective(line, 0, nil); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		if before, ok := strings.CutSuffix(line, ":"); ok {
			label := before
			// All labels now use the unified codeOffset
			a.labels[label] = a.baseAddr + a.codeOffset
			fmt.Printf("Label '%s' at 0x%04x\n", label, a.labels[label])
			continue
		}

		if strings.HasPrefix(line, ".equ") {
			if err := a.handleDirective(line, 0, nil); err != nil {
				fmt.Printf("Error: %v\n", err)
				os.Exit(1)
			}
			continue
		}

		// Handle data directives - calculate their size and add to offset
		if strings.HasPrefix(line, ".") {
			size := a.calcDirectiveSize(line)
			if size > 0 {
				nextAddr := a.baseAddr + a.codeOffset + size
				if nextAddr > maxAddr {
					maxAddr = nextAddr
				}
				a.codeOffset += size
			}
			continue
		}

		// Code instruction (8 bytes each)
		nextAddr := a.baseAddr + a.codeOffset + 8
		if nextAddr > maxAddr {
			maxAddr = nextAddr
		}
		a.codeOffset += 8
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
			if err := a.handleDirective(line, lineNum, program); err != nil {
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

	return program
}

func main() {
	var includePaths []string
	var inputFile string

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-I" {
			i++
			if i >= len(args) {
				fmt.Fprintf(os.Stderr, "Error: -I requires a directory argument\n")
				os.Exit(1)
			}
			includePaths = append(includePaths, args[i])
		} else if strings.HasPrefix(arg, "-I") {
			includePaths = append(includePaths, arg[2:])
		} else if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", arg)
			fmt.Fprintf(os.Stderr, "Usage: ie32asm [-I dir]... <input.asm>\n")
			os.Exit(1)
		} else if inputFile != "" {
			fmt.Fprintf(os.Stderr, "Error: multiple input files specified\n")
			fmt.Fprintf(os.Stderr, "Usage: ie32asm [-I dir]... <input.asm>\n")
			os.Exit(1)
		} else {
			inputFile = arg
		}
	}

	if inputFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: ie32asm [-I dir]... <input.asm>\n")
		os.Exit(1)
	}

	code, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Printf("Error reading input file: %v\n", err)
		os.Exit(1)
	}

	// Preprocess includes
	basePath := filepath.Dir(inputFile)
	processedCode, err := preprocessIncludes(string(code), basePath, includePaths, nil)
	if err != nil {
		fmt.Printf("Error processing includes: %v\n", err)
		os.Exit(1)
	}

	asm := NewAssembler()
	asm.basePath = basePath
	asm.includePaths = includePaths
	binary := asm.assemble(processedCode)

	outFile := strings.TrimSuffix(inputFile, ".asm") + ".iex"
	if err := os.WriteFile(outFile, binary, 0644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully assembled to %s (%d bytes)\n", outFile, len(binary))
}

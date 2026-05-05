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
	"bytes"
	"fmt"
	"maps"
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

func stripIE32Comment(line string) string {
	inString := false
	inChar := false
	escaped := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && (inString || inChar) {
			escaped = true
			continue
		}
		switch c {
		case '"':
			if !inChar {
				inString = !inString
			}
		case '\'':
			if !inString {
				inChar = !inChar
			}
		case ';':
			if !inString && !inChar {
				return line[:i]
			}
		}
	}
	return line
}

func splitIE32Operands(s string) []string {
	var out []string
	start := 0
	inString := false
	inChar := false
	escaped := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if escaped {
			escaped = false
			continue
		}
		if c == '\\' && (inString || inChar) {
			escaped = true
			continue
		}
		switch c {
		case '"':
			if !inChar {
				inString = !inString
			}
		case '\'':
			if !inString {
				inChar = !inChar
			}
		case ',':
			if !inString && !inChar {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

func unescapeIEString(s string) ([]byte, error) {
	var out []byte
	for i := 0; i < len(s); i++ {
		if s[i] != '\\' {
			out = append(out, s[i])
			continue
		}
		if i+1 >= len(s) {
			return nil, fmt.Errorf("trailing escape")
		}
		i++
		switch s[i] {
		case 'n':
			out = append(out, '\n')
		case 't':
			out = append(out, '\t')
		case 'r':
			out = append(out, '\r')
		case '\\':
			out = append(out, '\\')
		case '"':
			out = append(out, '"')
		case '0':
			out = append(out, 0)
		case 'x':
			if i+2 >= len(s) {
				return nil, fmt.Errorf("short \\x escape")
			}
			v, err := strconv.ParseUint(s[i+1:i+3], 16, 8)
			if err != nil {
				return nil, fmt.Errorf("invalid \\x escape")
			}
			out = append(out, byte(v))
			i += 2
		default:
			out = append(out, s[i])
		}
	}
	return out, nil
}

func quotedIE32String(line string) (string, error) {
	start := strings.IndexByte(line, '"')
	if start < 0 {
		return "", fmt.Errorf("missing opening quote")
	}
	escaped := false
	for i := start + 1; i < len(line); i++ {
		if escaped {
			escaped = false
			continue
		}
		if line[i] == '\\' {
			escaped = true
			continue
		}
		if line[i] == '"' {
			return line[start+1 : i], nil
		}
	}
	return "", fmt.Errorf("missing closing quote")
}

type Assembler struct {
	labels       map[string]uint32
	equates      map[string]uint32
	baseAddr     uint32
	codeOffset   uint32
	maxAddr      uint32
	basePath     string
	includePaths []string
	verbose      bool
	pass         int
	warnings     WarningPolicy
	incbinCache  map[string][]byte
	written      map[uint32]int
}

type WarningPolicy struct {
	Werror     bool
	suppressed map[string]bool
	warnings   []string
}

func (w *WarningPolicy) Suppress(category string) {
	if w.suppressed == nil {
		w.suppressed = make(map[string]bool)
	}
	w.suppressed[category] = true
}

func (w *WarningPolicy) Add(category, format string, args ...any) error {
	if w.suppressed != nil && w.suppressed[category] {
		return nil
	}
	msg := fmt.Sprintf("%s: %s", category, fmt.Sprintf(format, args...))
	if w.Werror {
		return fmt.Errorf("warning treated as error: %s", msg)
	}
	w.warnings = append(w.warnings, msg)
	return nil
}

func (w *WarningPolicy) Warnings() []string { return w.warnings }

type ie32ExprParser struct {
	asm   *Assembler
	input string
	pos   int
}

func (a *Assembler) evalExpr(s string) (int64, error) {
	p := &ie32ExprParser{asm: a, input: strings.TrimSpace(s)}
	v, err := p.parseOr()
	if err != nil {
		return 0, err
	}
	p.skip()
	if p.pos != len(p.input) {
		return 0, fmt.Errorf("unexpected trailing expression text: %s", p.input[p.pos:])
	}
	return v, nil
}

func (p *ie32ExprParser) skip() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *ie32ExprParser) peek() byte {
	p.skip()
	if p.pos >= len(p.input) {
		return 0
	}
	return p.input[p.pos]
}

func (p *ie32ExprParser) parseOr() (int64, error) {
	left, err := p.parseXor()
	if err != nil {
		return 0, err
	}
	for p.peek() == '|' {
		p.pos++
		right, err := p.parseXor()
		if err != nil {
			return 0, err
		}
		left |= right
	}
	return left, nil
}

func (p *ie32ExprParser) parseXor() (int64, error) {
	left, err := p.parseAnd()
	if err != nil {
		return 0, err
	}
	for p.peek() == '^' {
		p.pos++
		right, err := p.parseAnd()
		if err != nil {
			return 0, err
		}
		left ^= right
	}
	return left, nil
}

func (p *ie32ExprParser) parseAnd() (int64, error) {
	left, err := p.parseAdd()
	if err != nil {
		return 0, err
	}
	for p.peek() == '&' {
		p.pos++
		right, err := p.parseAdd()
		if err != nil {
			return 0, err
		}
		left &= right
	}
	return left, nil
}

func (p *ie32ExprParser) parseAdd() (int64, error) {
	left, err := p.parseMul()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '+':
			p.pos++
			right, err := p.parseMul()
			if err != nil {
				return 0, err
			}
			left += right
		case '-':
			p.pos++
			right, err := p.parseMul()
			if err != nil {
				return 0, err
			}
			left -= right
		default:
			return left, nil
		}
	}
}

func (p *ie32ExprParser) parseMul() (int64, error) {
	left, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		switch p.peek() {
		case '*':
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			left *= right
		case '/':
			p.pos++
			right, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			left /= right
		default:
			return left, nil
		}
	}
}

func (p *ie32ExprParser) parseUnary() (int64, error) {
	switch p.peek() {
	case '-':
		p.pos++
		v, err := p.parseUnary()
		return -v, err
	case '+':
		p.pos++
		return p.parseUnary()
	case '~':
		p.pos++
		v, err := p.parseUnary()
		return ^v, err
	}
	return p.parseAtom()
}

func (p *ie32ExprParser) parseAtom() (int64, error) {
	p.skip()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if p.input[p.pos] == '(' {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return 0, err
		}
		if p.peek() != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return v, nil
	}
	if p.input[p.pos] == '$' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && ((p.input[p.pos] >= '0' && p.input[p.pos] <= '9') || (p.input[p.pos] >= 'a' && p.input[p.pos] <= 'f') || (p.input[p.pos] >= 'A' && p.input[p.pos] <= 'F') || p.input[p.pos] == '_') {
			p.pos++
		}
		v, err := strconv.ParseUint(strings.ReplaceAll(p.input[start:p.pos], "_", ""), 16, 64)
		return int64(v), err
	}
	if p.input[p.pos] >= '0' && p.input[p.pos] <= '9' {
		start := p.pos
		for p.pos < len(p.input) && ((p.input[p.pos] >= '0' && p.input[p.pos] <= '9') || (p.input[p.pos] >= 'a' && p.input[p.pos] <= 'f') || (p.input[p.pos] >= 'A' && p.input[p.pos] <= 'F') || p.input[p.pos] == 'x' || p.input[p.pos] == 'X' || p.input[p.pos] == '_') {
			p.pos++
		}
		raw := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		if strings.HasPrefix(raw, "0x") || strings.HasPrefix(raw, "0X") {
			v, err := strconv.ParseUint(raw[2:], 16, 64)
			return int64(v), err
		}
		v, err := strconv.ParseInt(raw, 10, 64)
		return v, err
	}
	if (p.input[p.pos] >= 'A' && p.input[p.pos] <= 'Z') || (p.input[p.pos] >= 'a' && p.input[p.pos] <= 'z') || p.input[p.pos] == '_' || p.input[p.pos] == '.' {
		start := p.pos
		for p.pos < len(p.input) && ((p.input[p.pos] >= 'A' && p.input[p.pos] <= 'Z') || (p.input[p.pos] >= 'a' && p.input[p.pos] <= 'z') || (p.input[p.pos] >= '0' && p.input[p.pos] <= '9') || p.input[p.pos] == '_' || p.input[p.pos] == '.') {
			p.pos++
		}
		name := p.input[start:p.pos]
		if v, ok := p.asm.equates[name]; ok {
			return int64(v), nil
		}
		if v, ok := p.asm.labels[name]; ok {
			return int64(v), nil
		}
		return 0, fmt.Errorf("undefined symbol: %s", name)
	}
	return 0, fmt.Errorf("unexpected character %q", p.input[p.pos])
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
		labels:      make(map[string]uint32),
		equates:     make(map[string]uint32),
		baseAddr:    PROG_START,
		codeOffset:  0,
		maxAddr:     PROG_START,
		incbinCache: make(map[string][]byte),
		written:     make(map[uint32]int),
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
			delete(included, absPath)
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
		wordValues := splitIE32Operands(wordList)
		for _, wv := range wordValues {
			wv = strings.TrimSpace(wv)
			if wv == "" {
				continue
			}
			value, err := a.evalExpr(wv)
			if err != nil {
				return fmt.Errorf("invalid word value: %s: %v", wv, err)
			}
			if value < -2147483648 || value > 0xFFFFFFFF {
				return fmt.Errorf(".word value out of 32-bit range: %s", wv)
			}
			if program != nil {
				if err := a.markRange(a.baseAddr+a.codeOffset, 4, lineNum); err != nil {
					return err
				}
				copy(program[a.codeOffset:], writeLittleEndian(uint32(value)))
			}
			a.codeOffset += 4
		}

	case ".equ":
		if len(parts) < 3 {
			return fmt.Errorf("invalid EQU format")
		}
		value, err := a.evalExpr(strings.TrimSpace(strings.Join(parts[2:], " ")))
		if err != nil {
			return fmt.Errorf("invalid EQU value: %s: %v", parts[2], err)
		}
		if value < -2147483648 || value > 0xFFFFFFFF {
			return fmt.Errorf("EQU value out of 32-bit range: %s", parts[2])
		}
		a.equates[parts[1]] = uint32(value)
		if a.verbose {
			fmt.Printf("Added equate: %s = 0x%x\n", parts[1], value)
		}

	case ".byte":
		// Handle comma-separated list of byte values
		byteList := strings.Join(parts[1:], " ")
		byteValues := splitIE32Operands(byteList)
		for _, bv := range byteValues {
			bv = strings.TrimSpace(bv)
			if bv == "" {
				continue
			}
			value, err := a.evalExpr(bv)
			if err != nil {
				return fmt.Errorf("invalid byte value: %s", bv)
			}
			if value < -128 || value > 0xFF {
				return fmt.Errorf(".byte value out of 8-bit range: %s", bv)
			}
			if program != nil {
				if err := a.markRange(a.baseAddr+a.codeOffset, 1, lineNum); err != nil {
					return err
				}
				program[a.codeOffset] = byte(value)
			}
			a.codeOffset++
		}

	case ".incbin":
		ops := splitIE32Operands(strings.TrimSpace(line[len(parts[0]):]))
		if len(ops) < 1 || ops[0] == "" {
			return fmt.Errorf("incbin requires a filename")
		}
		filename := strings.Trim(ops[0], "\"'")
		path, err := resolveFile(filename, a.basePath, a.includePaths)
		if err != nil {
			return fmt.Errorf("incbin: %v", err)
		}
		payload, err := a.readIncbin(path)
		if err != nil {
			return err
		}
		offset := uint64(0)
		length := uint64(len(payload))
		if len(ops) >= 2 {
			offset, err = a.evalExprUint32(ops[1])
			if err != nil {
				return fmt.Errorf("invalid incbin offset: %s", ops[1])
			}
			if offset > uint64(len(payload)) {
				return fmt.Errorf("incbin offset out of range: %d", offset)
			}
			length = uint64(len(payload)) - offset
		}
		if len(ops) >= 3 {
			length, err = a.evalExprUint32(ops[2])
			if err != nil {
				return fmt.Errorf("invalid incbin length: %s", ops[2])
			}
		}
		if offset+length > uint64(len(payload)) {
			return fmt.Errorf("incbin range out of bounds: %d..%d", offset, offset+length)
		}
		if program != nil {
			if err := a.markRange(a.baseAddr+a.codeOffset, uint32(length), lineNum); err != nil {
				return err
			}
			copy(program[a.codeOffset:], payload[offset:offset+length])
		}
		a.codeOffset += uint32(length)

	case ".space":
		sizeExpr := strings.TrimSpace(strings.Join(parts[1:], " "))
		size, err := a.evalExprUint32(sizeExpr)
		if err != nil {
			return fmt.Errorf("invalid space size: %s", sizeExpr)
		}
		if program != nil {
			if err := a.markRange(a.baseAddr+a.codeOffset, uint32(size), lineNum); err != nil {
				return err
			}
			for i := range size {
				program[a.codeOffset+uint32(i)] = 0
			}
		}
		a.codeOffset += uint32(size)

	case ".ascii", ".asciz":
		// Find the quoted string in the line
		str, err := quotedIE32String(line)
		if err != nil {
			return fmt.Errorf("invalid ascii format: %v", err)
		}
		data, err := unescapeIEString(str)
		if err != nil {
			return fmt.Errorf("invalid ascii escape: %v", err)
		}
		if parts[0] == ".asciz" {
			data = append(data, 0)
		}
		if program != nil {
			if err := a.markRange(a.baseAddr+a.codeOffset, uint32(len(data)), lineNum); err != nil {
				return err
			}
			copy(program[a.codeOffset:], data)
		}
		a.codeOffset += uint32(len(data))

	case ".org":
		if len(parts) < 2 {
			return fmt.Errorf("invalid org format")
		}
		addr, err := a.evalExprUint32(parts[1])
		if err != nil {
			return fmt.Errorf("invalid org address: %s", parts[1])
		}
		if addr < uint64(a.baseAddr) {
			return fmt.Errorf("org address 0x%x before base address 0x%x", addr, a.baseAddr)
		}
		newOffset := uint32(addr) - a.baseAddr
		if newOffset < a.codeOffset {
			if err := a.warnings.Add("org-backward", ".org moved backward to 0x%x", addr); err != nil {
				return err
			}
		}
		a.codeOffset = newOffset
		if a.verbose {
			fmt.Printf("Setting assembly address to 0x%x\n", addr)
		}

	default:
		return fmt.Errorf("unknown directive: %s", parts[0])
	}
	return nil
}

func (a *Assembler) evalExprUint32(expr string) (uint64, error) {
	v, err := a.evalExpr(strings.TrimSpace(expr))
	if err != nil {
		return 0, err
	}
	if v < 0 || v > 0xFFFFFFFF {
		return 0, fmt.Errorf("value out of uint32 range: %s", expr)
	}
	return uint64(v), nil
}

func (a *Assembler) evalExprOperand32(expr string) (uint32, error) {
	v, err := a.evalExpr(strings.TrimSpace(expr))
	if err != nil {
		return 0, err
	}
	if v < -2147483648 || v > 0xFFFFFFFF {
		return 0, fmt.Errorf("value out of 32-bit range: %s", expr)
	}
	return uint32(v), nil
}

func (a *Assembler) readIncbin(path string) ([]byte, error) {
	abs, _ := filepath.Abs(path)
	if data, ok := a.incbinCache[abs]; ok {
		current, err := os.ReadFile(path)
		if a.pass == 1 && err == nil && !bytes.Equal(current, data) {
			if warnErr := a.warnings.Add("incbin-changed", "incbin file changed after first read: %s", path); warnErr != nil {
				return nil, warnErr
			}
		}
		return data, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("incbin read failed: %s", path)
	}
	cpy := append([]byte(nil), data...)
	a.incbinCache[abs] = cpy
	return cpy, nil
}

func (a *Assembler) markRange(addr uint32, size uint32, lineNum int) error {
	for i := range size {
		at := addr + i
		if prev, ok := a.written[at]; ok {
			return fmt.Errorf("overlapping emit at $%04X (already written by line %d)", at, prev)
		}
		a.written[at] = lineNum
	}
	return nil
}

func (a *Assembler) parseOperand(operand string, lineNum int) (byte, uint32, error) {
	if a.verbose {
		fmt.Printf("Parsing operand: '%s'\n", operand)
	}

	// Register-indirect addressing [reg] or [reg+offset]
	if strings.HasPrefix(operand, "[") && strings.HasSuffix(operand, "]") {
		inner := strings.Trim(operand, "[]")
		reg := strings.TrimSpace(inner)
		offsetExpr := ""
		for i := 1; i < len(inner); i++ {
			if inner[i] == '+' || inner[i] == '-' {
				reg = strings.TrimSpace(inner[:i])
				offsetExpr = strings.TrimSpace(inner[i:])
				break
			}
		}
		if regNum, ok := registers[reg]; ok {
			if offsetExpr == "" {
				return ADDR_REG_IND, uint32(regNum), nil
			}
			offset, err := a.evalExpr(offsetExpr)
			if err != nil {
				return 0, 0, fmt.Errorf("invalid offset: %s", offsetExpr)
			}
			encoded := uint32(int32(offset))
			if encoded&0x0F != 0 {
				return 0, 0, fmt.Errorf("offset must be multiple of 16")
			}
			return ADDR_REG_IND, uint32(regNum) | encoded, nil
		}
		return 0, 0, fmt.Errorf("invalid register in indirect addressing: %s", reg)
	}

	// Direct memory addressing @addr (write/read directly to/from this address)
	if after, ok := strings.CutPrefix(operand, "@"); ok {
		addr := after
		if a.verbose {
			fmt.Printf("  Direct memory: addr='%s'\n", addr)
		}

		val, err := a.evalExprOperand32(addr)
		if err != nil {
			return 0, 0, fmt.Errorf("undefined label or invalid address: %s: %v", addr, err)
		}
		return ADDR_DIRECT, val, nil
	}

	// Immediate value #n
	if strings.HasPrefix(operand, "#") {
		numStr := operand[1:]
		if a.verbose {
			fmt.Printf("  Immediate value: '%s'\n", numStr)
		}
		val, err := a.evalExprOperand32(numStr)
		if err != nil {
			return 0, 0, fmt.Errorf("invalid immediate value: %s: %v", numStr, err)
		}
		if a.verbose {
			fmt.Printf("    Parsed value: val=0x%x\n", val)
		}
		return ADDR_IMMEDIATE, val, nil
	}

	// Register
	if regNum, ok := registers[operand]; ok {
		if a.verbose {
			fmt.Printf("  Found register: reg=%s num=%d\n", operand, regNum)
		}
		return ADDR_REGISTER, uint32(regNum), nil
	}

	if val, err := a.evalExprOperand32(operand); err == nil {
		return ADDR_IMMEDIATE, val, nil
	} else if strings.Contains(err.Error(), "out of 32-bit range") {
		return 0, 0, err
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
		ops := splitIE32Operands(strings.TrimSpace(line[len(parts[0]):]))
		if len(ops) < 1 || ops[0] == "" {
			return 0
		}
		filename := strings.Trim(ops[0], "\"'")
		path, err := resolveFile(filename, a.basePath, a.includePaths)
		if err != nil {
			fmt.Printf("Warning: cannot resolve incbin file %s: %v\n", filename, err)
			return 0
		}
		payload, err := a.readIncbin(path)
		if err != nil {
			fmt.Printf("Warning: cannot read incbin file %s: %v\n", path, err)
			return 0
		}
		length := uint64(len(payload))
		// Handle optional offset and length parameters
		if len(ops) >= 2 {
			offset, err := a.evalExprUint32(ops[1])
			if err == nil && offset < length {
				length -= offset
			}
		}
		if len(ops) >= 3 {
			specLen, err := a.evalExprUint32(ops[2])
			if err == nil {
				length = specLen
			}
		}
		return uint32(length)

	case ".space":
		if len(parts) >= 2 {
			size, err := a.evalExprUint32(strings.TrimSpace(strings.Join(parts[1:], " ")))
			if err == nil {
				return uint32(size)
			}
		}
		return 0

	case ".ascii", ".asciz":
		// Find the quoted string
		str, err := quotedIE32String(line)
		if err == nil {
			data, err := unescapeIEString(str)
			if err == nil {
				if parts[0] == ".asciz" {
					return uint32(len(data) + 1)
				}
				return uint32(len(data))
			}
		}
		return 0

	default:
		return 0
	}
}

func (a *Assembler) sourceLines(code string) []string {
	var out []string
	for raw := range strings.SplitSeq(code, "\n") {
		line := strings.TrimSpace(stripIE32Comment(raw))
		for {
			if line == "" {
				break
			}
			colon := strings.IndexByte(line, ':')
			if colon <= 0 {
				out = append(out, line)
				break
			}
			before := strings.TrimSpace(line[:colon])
			if strings.ContainsAny(before, " \t") {
				out = append(out, line)
				break
			}
			out = append(out, before+":")
			line = strings.TrimSpace(line[colon+1:])
		}
	}
	return out
}

func (a *Assembler) defineLabel(label string, lineNum int, warnDuplicate bool) error {
	if _, exists := a.labels[label]; exists && warnDuplicate {
		if err := a.warnings.Add("duplicate-labels", "label %q redefined on line %d", label, lineNum); err != nil {
			return err
		}
	}
	a.labels[label] = a.baseAddr + a.codeOffset
	if a.verbose {
		fmt.Printf("Label '%s' at 0x%04x\n", label, a.labels[label])
	}
	return nil
}

func (a *Assembler) noteSize(size uint32) {
	nextAddr := a.baseAddr + a.codeOffset + size
	if nextAddr > a.maxAddr {
		a.maxAddr = nextAddr
	}
	a.codeOffset += size
}

func cloneUint32Map(m map[string]uint32) map[string]uint32 {
	out := make(map[string]uint32, len(m))
	maps.Copy(out, m)
	return out
}

func sameUint32Map(a, b map[string]uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for k, av := range a {
		if bv, ok := b[k]; !ok || bv != av {
			return false
		}
	}
	return true
}

func (a *Assembler) collectIE32Layout(lines []string, warnDuplicateLabels bool) error {
	a.maxAddr = a.baseAddr
	a.codeOffset = 0
	a.pass = 1

	for lineNum, line := range lines {
		if !strings.HasPrefix(line, ".equ") {
			continue
		}
		err := a.handleDirective(line, lineNum+1, nil)
		if err == nil {
			continue
		}
		if strings.Contains(err.Error(), "undefined symbol") {
			continue
		}
		return fmt.Errorf("line %d: %v", lineNum+1, err)
	}

	for lineNum, line := range lines {
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, ".org") {
			if err := a.handleDirective(line, 0, nil); err != nil {
				return fmt.Errorf("line %d: %v", lineNum+1, err)
			}
			continue
		}

		if before, ok := strings.CutSuffix(line, ":"); ok {
			if err := a.defineLabel(before, lineNum+1, warnDuplicateLabels); err != nil {
				return err
			}
			continue
		}

		if strings.HasPrefix(line, ".equ") {
			continue
		}

		// Handle data directives - calculate their size and add to offset
		if strings.HasPrefix(line, ".") {
			size := a.calcDirectiveSize(line)
			if size > 0 {
				a.noteSize(size)
			}
			continue
		}

		// Code instruction (8 bytes each)
		a.noteSize(8)
	}

	for lineNum, line := range lines {
		if strings.HasPrefix(line, ".equ") {
			if err := a.handleDirective(line, lineNum+1, nil); err != nil {
				return fmt.Errorf("line %d: %v", lineNum+1, err)
			}
		}
	}
	return nil
}

func (a *Assembler) assemble(code string) ([]byte, error) {
	var program []byte
	lines := a.sourceLines(code)
	for iter := range 5 {
		prevLabels := cloneUint32Map(a.labels)
		prevEquates := cloneUint32Map(a.equates)
		prevMax := a.maxAddr
		if err := a.collectIE32Layout(lines, iter == 0); err != nil {
			return nil, err
		}
		if iter > 0 && prevMax == a.maxAddr && sameUint32Map(prevLabels, a.labels) && sameUint32Map(prevEquates, a.equates) {
			break
		}
		if iter == 4 {
			return nil, fmt.Errorf("could not stabilize forward references in IE32 layout")
		}
	}

	// Reset offset for second pass
	a.codeOffset = 0
	a.pass = 2
	a.written = make(map[uint32]int)
	program = make([]byte, a.maxAddr-a.baseAddr)

	// Second pass: generate code
	for lineNum, line := range lines {
		if line == "" || strings.HasSuffix(line, ":") {
			continue
		}

		if strings.HasPrefix(line, ".") {
			if err := a.handleDirective(line, lineNum, program); err != nil {
				return nil, fmt.Errorf("line %d: %v", lineNum+1, err)
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
				return nil, fmt.Errorf("line %d: invalid instruction format: %s", lineNum+1, line)
			}

			reg, ok := registers[strings.TrimSpace(regAndOp[0])]
			if !ok {
				return nil, fmt.Errorf("line %d: invalid register: %s", lineNum+1, regAndOp[0])
			}

			operand := strings.TrimSpace(regAndOp[1])
			addrMode, value, err := a.parseOperand(operand, lineNum+1)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", lineNum+1, err)
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
				return nil, fmt.Errorf("line %d: invalid load instruction format: %s", lineNum+1, line)
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
				return nil, fmt.Errorf("line %d: %v", lineNum+1, err)
			}

			instruction = append(instruction, op, reg, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "STA", "STB", "STC", "STD", "STE", "STF", "STG", "STH", "STS", "STT", "STU", "STV", "STW", "STX", "STY", "STZ":
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: invalid store instruction format: %s", lineNum+1, line)
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
				return nil, fmt.Errorf("line %d: %v", lineNum+1, err)
			}

			instruction = append(instruction, op, reg, addrMode, 0)
			instruction = append(instruction, writeLittleEndian(value)...)

		case "INC", "DEC":
			if len(parts) != 2 {
				return nil, fmt.Errorf("line %d: invalid increment/decrement instruction format: %s", lineNum+1, line)
			}

			operand := parts[1]
			addrMode, value, err := a.parseOperand(operand, lineNum+1)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", lineNum+1, err)
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
				return nil, fmt.Errorf("line %d: invalid %s instruction format: %s", lineNum+1, opcode, line)
			}

			reg, ok := registers[parts[1]]
			if !ok {
				return nil, fmt.Errorf("line %d: invalid register: %s", lineNum+1, parts[1])
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
				return nil, fmt.Errorf("line %d: invalid jump instruction: %s", lineNum+1, line)
			}

			target := parts[1]
			var targetAddr uint32

			if val, ok := a.equates[target]; ok {
				targetAddr = val
			} else if addr, ok := a.labels[target]; ok {
				targetAddr = addr
			} else {
				return nil, fmt.Errorf("line %d: undefined label or equate: %s", lineNum+1, target)
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
				return nil, fmt.Errorf("line %d: invalid branch instruction format: %s", lineNum+1, line)
			}

			reg, ok := registers[strings.TrimSpace(regAndLabel[0])]
			if !ok {
				return nil, fmt.Errorf("line %d: invalid register: %s", lineNum+1, regAndLabel[0])
			}

			target := strings.TrimSpace(regAndLabel[1])
			var targetAddr uint32

			if val, ok := a.equates[target]; ok {
				targetAddr = val
			} else if addr, ok := a.labels[target]; ok {
				targetAddr = addr
			} else {
				return nil, fmt.Errorf("line %d: undefined label or equate: %s", lineNum+1, target)
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
				return nil, fmt.Errorf("line %d: invalid WAIT instruction: %s", lineNum+1, line)
			}

			addrMode, value, err := a.parseOperand(parts[1], lineNum+1)
			if err != nil {
				return nil, fmt.Errorf("line %d: %v", lineNum+1, err)
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
			return nil, fmt.Errorf("line %d: unknown instruction: %s", lineNum+1, opcode)
		}

		if err := a.markRange(a.baseAddr+a.codeOffset, 8, lineNum+1); err != nil {
			return nil, err
		}
		copy(program[a.codeOffset:], instruction)
		a.codeOffset += 8
	}

	return program, nil
}

func main() {
	var includePaths []string
	var inputFile string
	var outFile string
	verbose := false
	warnings := WarningPolicy{}

	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-v" {
			verbose = true
		} else if arg == "-o" {
			i++
			if i >= len(args) {
				fmt.Fprintf(os.Stderr, "Error: -o requires an output path\n")
				os.Exit(1)
			}
			outFile = args[i]
		} else if arg == "-Werror" {
			warnings.Werror = true
		} else if after, ok := strings.CutPrefix(arg, "-Wno-"); ok {
			warnings.Suppress(after)
		} else if arg == "-I" {
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
			fmt.Fprintf(os.Stderr, "Usage: ie32asm [-v] [-o output] [-Werror] [-Wno-category] [-I dir]... <input.asm>\n")
			os.Exit(1)
		} else if inputFile != "" {
			fmt.Fprintf(os.Stderr, "Error: multiple input files specified\n")
			fmt.Fprintf(os.Stderr, "Usage: ie32asm [-v] [-o output] [-Werror] [-Wno-category] [-I dir]... <input.asm>\n")
			os.Exit(1)
		} else {
			inputFile = arg
		}
	}

	if inputFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: ie32asm [-v] [-o output] [-Werror] [-Wno-category] [-I dir]... <input.asm>\n")
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
	asm.verbose = verbose
	asm.warnings = warnings
	binary, err := asm.assemble(processedCode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Assembly error: %v\n", err)
		os.Exit(1)
	}
	for _, w := range asm.warnings.Warnings() {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	if outFile == "" {
		outFile = strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".iex"
	}
	if err := os.WriteFile(outFile, binary, 0644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully assembled to %s (%d bytes)\n", outFile, len(binary))
}

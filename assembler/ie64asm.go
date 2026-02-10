// ie64asm.go

//go:build ie64

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

IE64 Assembler — 64-bit RISC CPU assembler for the Intuition Engine
(c) 2024 - 2026 Zayn Otley
https://github.com/IntuitionAmiga/IntuitionEngine
License: GPLv3 or later

IE64 Instruction Encoding (8 bytes, little-endian):
  Byte 0:   Opcode (8 bits)
  Byte 1:   Rd[4:0] (5 bits) | Size[1:0] (2 bits) | X (1 bit)
  Byte 2:   Rs[4:0] (5 bits) | unused (3 bits)
  Byte 3:   Rt[4:0] (5 bits) | unused (3 bits)
  Bytes 4-7: imm32 (32-bit LE)

Decode:
  rd   = byte1 >> 3
  size = (byte1 >> 1) & 0x03
  xbit = byte1 & 1
  rs   = byte2 >> 3
  rt   = byte3 >> 3

Registers: r0-r31 (sp = r31)

Assembler Syntax (68K-flavored, case-insensitive mnemonics/registers/directives):

  Directives:
    org $addr             — set origin
    NAME equ value        — define constant (case-sensitive name)
    NAME set value        — define reassignable constant (case-sensitive name)
    dc.b val,...          — byte data (supports "string" with escapes)
    dc.w val,...          — 16-bit LE data
    dc.l val,...          — 32-bit LE data
    dc.q val,...          — 64-bit LE data
    ds.b n                — reserve n zero bytes
    ds.w n                — reserve n*2 zero bytes
    ds.l n                — reserve n*4 zero bytes
    ds.q n                — reserve n*8 zero bytes
    align n               — align to n-byte boundary
    incbin "file"         — include binary file
    incbin "file",off,len — include binary file with offset and length
    include "file"        — include source file (circular detection)

  Labels:
    global_label:         — global label (case-sensitive)
    .local_label:         — local label (scoped to preceding global)

  Macros:
    name   macro
           instr \1, \2   ; parameters \1..\9
           endm
    narg                  — parameter count within macro body

  Conditional Assembly:
    if expr / else / endif

  Repeat Blocks:
    rept count / endr

  Expressions:
    Operators: + - * / << >> | & ^ ~ (unary NOT)
    Parentheses for grouping
    Hex: $FF or 0xFF
    Visual separator: $CAFE_BABE
    Symbols (equ/set values, labels) resolved

  Pseudo-Instructions:
    la rd, addr           -> lea rd, addr(r0)
    li rd, #imm32         -> move.l rd, #imm32
    li rd, #imm64         -> move.l rd, #lo32 + movt rd, #hi32
    beqz rs, label        -> beq rs, r0, label
    bnez rs, label        -> bne rs, r0, label
    bltz rs, label        -> blt rs, r0, label
    bgez rs, label        -> bge rs, r0, label
    bgtz rs, label        -> bgt rs, r0, label
    blez rs, label        -> ble rs, r0, label
*/

package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ---------------------------------------------------------------------
// Opcode constants
// ---------------------------------------------------------------------
const (
	OP64_MOVE    = 0x01
	OP64_MOVT    = 0x02
	OP64_MOVEQ   = 0x03
	OP64_LEA     = 0x04
	OP64_LOAD    = 0x10
	OP64_STORE   = 0x11
	OP64_ADD     = 0x20
	OP64_SUB     = 0x21
	OP64_MULU    = 0x22
	OP64_MULS    = 0x23
	OP64_DIVU    = 0x24
	OP64_DIVS    = 0x25
	OP64_MOD     = 0x26
	OP64_NEG     = 0x27
	OP64_AND     = 0x30
	OP64_OR      = 0x31
	OP64_EOR     = 0x32
	OP64_NOT     = 0x33
	OP64_LSL     = 0x34
	OP64_LSR     = 0x35
	OP64_ASR     = 0x36
	OP64_CLZ     = 0x37
	OP64_BRA     = 0x40
	OP64_BEQ     = 0x41
	OP64_BNE     = 0x42
	OP64_BLT     = 0x43
	OP64_BGE     = 0x44
	OP64_BGT     = 0x45
	OP64_BLE     = 0x46
	OP64_BHI     = 0x47
	OP64_BLS     = 0x48
	OP64_JMP     = 0x49
	OP64_JSR     = 0x50
	OP64_RTS     = 0x51
	OP64_PUSH    = 0x52
	OP64_POP     = 0x53
	OP64_JSR_IND = 0x54
	OP64_FMOV    = 0x60
	OP64_FLOAD   = 0x61
	OP64_FSTORE  = 0x62
	OP64_FADD    = 0x63
	OP64_FSUB    = 0x64
	OP64_FMUL    = 0x65
	OP64_FDIV    = 0x66
	OP64_FMOD    = 0x67
	OP64_FABS    = 0x68
	OP64_FNEG    = 0x69
	OP64_FSQRT   = 0x6A
	OP64_FINT    = 0x6B
	OP64_FCMP    = 0x6C
	OP64_FCVTIF  = 0x6D
	OP64_FCVTFI  = 0x6E
	OP64_FMOVI   = 0x6F
	OP64_FMOVO   = 0x70
	OP64_FSIN    = 0x71
	OP64_FCOS    = 0x72
	OP64_FTAN    = 0x73
	OP64_FATAN   = 0x74
	OP64_FLOG    = 0x75
	OP64_FEXP    = 0x76
	OP64_FPOW    = 0x77
	OP64_FMOVECR = 0x78
	OP64_FMOVSR  = 0x79
	OP64_FMOVCR  = 0x7A
	OP64_FMOVSC  = 0x7B
	OP64_FMOVCC  = 0x7C
	OP64_NOP     = 0xE0

	OP64_HALT = 0xE1
	OP64_SEI  = 0xE2
	OP64_CLI  = 0xE3
	OP64_RTI  = 0xE4
	OP64_WAIT = 0xE5
)

// Size codes
const (
	SIZE_B = 0
	SIZE_W = 1
	SIZE_L = 2
	SIZE_Q = 3
)

// Instruction size in bytes
const instrSize = 8

// ---------------------------------------------------------------------
// Macro type
// ---------------------------------------------------------------------

// Macro holds a macro definition collected during preprocessing.
type Macro struct {
	name   string
	params int
	body   []string
}

// ---------------------------------------------------------------------
// IE64Assembler
// ---------------------------------------------------------------------

// IE64Assembler is a multi-pass assembler for the IE64 instruction set.
type IE64Assembler struct {
	labels          map[string]uint32
	equates         map[string]uint64
	sets            map[string]uint64
	macros          map[string]*Macro
	baseAddr        uint32
	lastGlobalLabel string
	listingMode     bool
	listing         []string
	warnings        []string
	errors          []string
	// internal state for assembly
	codeOffset uint32
	pass       int
	basePath   string
}

// NewIE64Assembler creates a new assembler instance.
func NewIE64Assembler() *IE64Assembler {
	return &IE64Assembler{
		labels:   make(map[string]uint32),
		equates:  make(map[string]uint64),
		sets:     make(map[string]uint64),
		macros:   make(map[string]*Macro),
		baseAddr: 0x1000,
	}
}

// SetListingMode enables or disables listing output.
func (a *IE64Assembler) SetListingMode(enabled bool) {
	a.listingMode = enabled
}

// GetListing returns the assembly listing lines.
func (a *IE64Assembler) GetListing() []string {
	return a.listing
}

// GetWarnings returns any warnings generated during assembly.
func (a *IE64Assembler) GetWarnings() []string {
	return a.warnings
}

func (a *IE64Assembler) addWarning(format string, args ...interface{}) {
	a.warnings = append(a.warnings, fmt.Sprintf(format, args...))
}

func (a *IE64Assembler) addError(format string, args ...interface{}) {
	a.errors = append(a.errors, fmt.Sprintf(format, args...))
}

func (a *IE64Assembler) addListing(addr uint32, data []byte, source string) {
	if !a.listingMode {
		return
	}
	if data == nil {
		a.listing = append(a.listing, fmt.Sprintf("                         %s", source))
		return
	}
	hex := ""
	for i, b := range data {
		if i > 0 {
			hex += " "
		}
		hex += fmt.Sprintf("%02X", b)
		if i >= 7 {
			hex += "..."
			break
		}
	}
	a.listing = append(a.listing, fmt.Sprintf("%08X  %-24s %s", addr, hex, source))
}

// ---------------------------------------------------------------------
// Encoding helper
// ---------------------------------------------------------------------

func encodeInstruction(opcode byte, rd, size, xbit, rs, rt byte, imm32 uint32) []byte {
	instr := make([]byte, 8)
	instr[0] = opcode
	instr[1] = (rd << 3) | (size << 1) | xbit
	instr[2] = rs << 3
	instr[3] = rt << 3
	binary.LittleEndian.PutUint32(instr[4:], imm32)
	return instr
}

// ---------------------------------------------------------------------
// Register parsing
// ---------------------------------------------------------------------

func parseRegister(name string) (byte, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "sp" {
		return 31, true
	}
	if strings.HasPrefix(name, "r") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 0 && n <= 31 {
			return byte(n), true
		}
	}
	return 0, false
}

func parseFPRegister(name string) (byte, bool) {
	name = strings.ToLower(strings.TrimSpace(name))
	if strings.HasPrefix(name, "f") {
		n, err := strconv.Atoi(name[1:])
		if err == nil && n >= 0 && n <= 15 {
			return byte(n), true
		}
	}
	return 0, false
}

// ---------------------------------------------------------------------
// Size suffix parsing
// ---------------------------------------------------------------------

func parseSizeSuffix(mnemonic string) (string, byte) {
	if strings.HasSuffix(mnemonic, ".b") {
		return mnemonic[:len(mnemonic)-2], SIZE_B
	}
	if strings.HasSuffix(mnemonic, ".w") {
		return mnemonic[:len(mnemonic)-2], SIZE_W
	}
	if strings.HasSuffix(mnemonic, ".l") {
		return mnemonic[:len(mnemonic)-2], SIZE_L
	}
	if strings.HasSuffix(mnemonic, ".q") {
		return mnemonic[:len(mnemonic)-2], SIZE_Q
	}
	return mnemonic, SIZE_Q // default is .q (64-bit)
}

// ---------------------------------------------------------------------
// Expression evaluator
// ---------------------------------------------------------------------

// exprParser holds state for parsing and evaluating expressions.
type exprParser struct {
	input string
	pos   int
	asm   *IE64Assembler
}

func (a *IE64Assembler) evalExpr(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty expression")
	}
	p := &exprParser{input: s, pos: 0, asm: a}
	val, err := p.parseExprCompare()
	if err != nil {
		return 0, err
	}
	p.skipSpaces()
	if p.pos < len(p.input) {
		return 0, fmt.Errorf("unexpected character '%c' at position %d in expression: %s", p.input[p.pos], p.pos, s)
	}
	return val, nil
}

func (p *exprParser) skipSpaces() {
	for p.pos < len(p.input) && (p.input[p.pos] == ' ' || p.input[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	p.skipSpaces()
	if p.pos < len(p.input) {
		return p.input[p.pos]
	}
	return 0
}

func (p *exprParser) peekTwo() string {
	p.skipSpaces()
	if p.pos+1 < len(p.input) {
		return p.input[p.pos : p.pos+2]
	}
	return ""
}

// parseExprCompare handles comparison operators: ==, !=, <, >, <=, >=
// Returns 1 for true, 0 for false (C-style boolean).
func (p *exprParser) parseExprCompare() (int64, error) {
	left, err := p.parseExprOr()
	if err != nil {
		return 0, err
	}
	for {
		tw := p.peekTwo()
		if tw == "==" {
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left == right {
				left = 1
			} else {
				left = 0
			}
		} else if tw == "!=" {
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left != right {
				left = 1
			} else {
				left = 0
			}
		} else if tw == "<=" {
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left <= right {
				left = 1
			} else {
				left = 0
			}
		} else if tw == ">=" {
			p.pos += 2
			right, err := p.parseExprOr()
			if err != nil {
				return 0, err
			}
			if left >= right {
				left = 1
			} else {
				left = 0
			}
		} else {
			ch := p.peek()
			if ch == '<' && tw != "<<" {
				p.pos++
				right, err := p.parseExprOr()
				if err != nil {
					return 0, err
				}
				if left < right {
					left = 1
				} else {
					left = 0
				}
			} else if ch == '>' && tw != ">>" {
				p.pos++
				right, err := p.parseExprOr()
				if err != nil {
					return 0, err
				}
				if left > right {
					left = 1
				} else {
					left = 0
				}
			} else {
				break
			}
		}
	}
	return left, nil
}

// parseExprOr handles | (bitwise OR)
func (p *exprParser) parseExprOr() (int64, error) {
	left, err := p.parseExprXor()
	if err != nil {
		return 0, err
	}
	for {
		if p.peek() == '|' {
			p.pos++
			right, err := p.parseExprXor()
			if err != nil {
				return 0, err
			}
			left = left | right
		} else {
			break
		}
	}
	return left, nil
}

// parseExprXor handles ^ (bitwise XOR)
func (p *exprParser) parseExprXor() (int64, error) {
	left, err := p.parseExprAnd()
	if err != nil {
		return 0, err
	}
	for {
		if p.peek() == '^' {
			p.pos++
			right, err := p.parseExprAnd()
			if err != nil {
				return 0, err
			}
			left = left ^ right
		} else {
			break
		}
	}
	return left, nil
}

// parseExprAnd handles & (bitwise AND)
func (p *exprParser) parseExprAnd() (int64, error) {
	left, err := p.parseExprShift()
	if err != nil {
		return 0, err
	}
	for {
		if p.peek() == '&' {
			p.pos++
			right, err := p.parseExprShift()
			if err != nil {
				return 0, err
			}
			left = left & right
		} else {
			break
		}
	}
	return left, nil
}

// parseExprShift handles << and >>
func (p *exprParser) parseExprShift() (int64, error) {
	left, err := p.parseExprAdd()
	if err != nil {
		return 0, err
	}
	for {
		tw := p.peekTwo()
		if tw == "<<" {
			p.pos += 2
			right, err := p.parseExprAdd()
			if err != nil {
				return 0, err
			}
			left = left << uint(right)
		} else if tw == ">>" {
			p.pos += 2
			right, err := p.parseExprAdd()
			if err != nil {
				return 0, err
			}
			left = left >> uint(right)
		} else {
			break
		}
	}
	return left, nil
}

// parseExprAdd handles + and -
func (p *exprParser) parseExprAdd() (int64, error) {
	left, err := p.parseExprMul()
	if err != nil {
		return 0, err
	}
	for {
		ch := p.peek()
		if ch == '+' {
			p.pos++
			right, err := p.parseExprMul()
			if err != nil {
				return 0, err
			}
			left = left + right
		} else if ch == '-' {
			p.pos++
			right, err := p.parseExprMul()
			if err != nil {
				return 0, err
			}
			left = left - right
		} else {
			break
		}
	}
	return left, nil
}

// parseExprMul handles * and /
func (p *exprParser) parseExprMul() (int64, error) {
	left, err := p.parseExprUnary()
	if err != nil {
		return 0, err
	}
	for {
		ch := p.peek()
		if ch == '*' {
			p.pos++
			right, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			left = left * right
		} else if ch == '/' {
			p.pos++
			right, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			if right == 0 {
				return 0, fmt.Errorf("division by zero in expression")
			}
			left = left / right
		} else {
			break
		}
	}
	return left, nil
}

// parseExprUnary handles unary -, ~, +
func (p *exprParser) parseExprUnary() (int64, error) {
	p.skipSpaces()
	if p.pos < len(p.input) {
		ch := p.input[p.pos]
		if ch == '-' {
			p.pos++
			val, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			return -val, nil
		}
		if ch == '+' {
			p.pos++
			return p.parseExprUnary()
		}
		if ch == '~' {
			p.pos++
			val, err := p.parseExprUnary()
			if err != nil {
				return 0, err
			}
			return ^val, nil
		}
	}
	return p.parseExprAtom()
}

// parseExprAtom handles numbers, symbols, and parenthesized expressions
func (p *exprParser) parseExprAtom() (int64, error) {
	p.skipSpaces()
	if p.pos >= len(p.input) {
		return 0, fmt.Errorf("unexpected end of expression")
	}

	ch := p.input[p.pos]

	// Parenthesized expression
	if ch == '(' {
		p.pos++
		val, err := p.parseExprOr()
		if err != nil {
			return 0, err
		}
		p.skipSpaces()
		if p.pos >= len(p.input) || p.input[p.pos] != ')' {
			return 0, fmt.Errorf("missing closing parenthesis")
		}
		p.pos++
		return val, nil
	}

	// Hex with $ prefix: $FF, $CAFE_BABE
	if ch == '$' {
		p.pos++
		start := p.pos
		for p.pos < len(p.input) && (isHexDigit(p.input[p.pos]) || p.input[p.pos] == '_') {
			p.pos++
		}
		if p.pos == start {
			return 0, fmt.Errorf("expected hex digits after $")
		}
		numStr := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		val, err := strconv.ParseUint(numStr, 16, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hex number: $%s", p.input[start:p.pos])
		}
		return int64(val), nil
	}

	// Hex with 0x prefix
	if ch == '0' && p.pos+1 < len(p.input) && (p.input[p.pos+1] == 'x' || p.input[p.pos+1] == 'X') {
		p.pos += 2
		start := p.pos
		for p.pos < len(p.input) && (isHexDigit(p.input[p.pos]) || p.input[p.pos] == '_') {
			p.pos++
		}
		if p.pos == start {
			return 0, fmt.Errorf("expected hex digits after 0x")
		}
		numStr := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		val, err := strconv.ParseUint(numStr, 16, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid hex number: 0x%s", p.input[start:p.pos])
		}
		return int64(val), nil
	}

	// Decimal number
	if ch >= '0' && ch <= '9' {
		start := p.pos
		for p.pos < len(p.input) && ((p.input[p.pos] >= '0' && p.input[p.pos] <= '9') || p.input[p.pos] == '_') {
			p.pos++
		}
		numStr := strings.ReplaceAll(p.input[start:p.pos], "_", "")
		val, err := strconv.ParseInt(numStr, 10, 64)
		if err != nil {
			return 0, fmt.Errorf("invalid decimal number: %s", p.input[start:p.pos])
		}
		return val, nil
	}

	// Symbol (label, equate, set, or narg)
	if isIdentStart(ch) || ch == '.' {
		start := p.pos
		for p.pos < len(p.input) && (isIdentChar(p.input[p.pos]) || p.input[p.pos] == '.') {
			p.pos++
		}
		name := p.input[start:p.pos]

		// Check for narg pseudo-symbol (handled by macro expansion — should be resolved already)
		if strings.ToLower(name) == "narg" {
			// narg is replaced during macro expansion; if we get here, it's 0
			return 0, nil
		}

		// Resolve local label
		if strings.HasPrefix(name, ".") && p.asm.lastGlobalLabel != "" {
			name = p.asm.lastGlobalLabel + name
		}

		// Check equates first (case-sensitive)
		if val, ok := p.asm.equates[name]; ok {
			return int64(val), nil
		}
		// Check sets (case-sensitive)
		if val, ok := p.asm.sets[name]; ok {
			return int64(val), nil
		}
		// Check labels (case-sensitive)
		if val, ok := p.asm.labels[name]; ok {
			return int64(val), nil
		}

		// On pass 1, unresolved labels are expected — return 0
		if p.asm.pass <= 1 {
			return 0, nil
		}

		return 0, fmt.Errorf("undefined symbol: %s", name)
	}

	// Character literal 'c'
	if ch == '\'' {
		p.pos++
		if p.pos >= len(p.input) {
			return 0, fmt.Errorf("unterminated character literal")
		}
		c := p.input[p.pos]
		p.pos++
		if c == '\\' && p.pos < len(p.input) {
			c = unescapeChar(p.input[p.pos])
			p.pos++
		}
		if p.pos >= len(p.input) || p.input[p.pos] != '\'' {
			return 0, fmt.Errorf("unterminated character literal")
		}
		p.pos++
		return int64(c), nil
	}

	return 0, fmt.Errorf("unexpected character '%c' in expression", ch)
}

func isHexDigit(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func isIdentStart(c byte) bool {
	return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_'
}

func isIdentChar(c byte) bool {
	return isIdentStart(c) || (c >= '0' && c <= '9')
}

func unescapeChar(c byte) byte {
	switch c {
	case 'n':
		return '\n'
	case 't':
		return '\t'
	case 'r':
		return '\r'
	case '\\':
		return '\\'
	case '0':
		return 0
	case '"':
		return '"'
	default:
		return c
	}
}

// evalExprUint64 evaluates an expression and returns it as uint64.
func (a *IE64Assembler) evalExprUint64(s string) (uint64, error) {
	val, err := a.evalExpr(s)
	if err != nil {
		return 0, err
	}
	return uint64(val), nil
}

// ---------------------------------------------------------------------
// Comment stripping
// ---------------------------------------------------------------------

// stripComment removes a ; comment from a line, respecting quoted strings.
func stripComment(line string) string {
	inQuote := false
	for i := 0; i < len(line); i++ {
		c := line[i]
		if c == '"' {
			inQuote = !inQuote
		} else if c == ';' && !inQuote {
			return line[:i]
		}
	}
	return line
}

// ---------------------------------------------------------------------
// String escape processing for dc.b
// ---------------------------------------------------------------------

// ---------------------------------------------------------------------
// Preprocessing: includes, macros, rept, conditionals
// ---------------------------------------------------------------------

func (a *IE64Assembler) preprocess(source string, basePath string, included map[string]bool) ([]string, error) {
	if included == nil {
		included = make(map[string]bool)
	}

	lines := strings.Split(source, "\n")
	var output []string

	// Pass 0a: expand includes
	for i := 0; i < len(lines); i++ {
		raw := lines[i]
		stripped := stripComment(raw)
		trimmed := strings.TrimSpace(stripped)
		lower := strings.ToLower(trimmed)

		if strings.HasPrefix(lower, "include") && !strings.HasPrefix(lower, "incbin") {
			// include "file"
			rest := strings.TrimSpace(trimmed[7:])
			filename := strings.Trim(rest, "\"'")
			if filename == "" {
				return nil, fmt.Errorf("line %d: missing filename for include", i+1)
			}
			includePath := filepath.Join(basePath, filename)
			absPath, _ := filepath.Abs(includePath)
			if included[absPath] {
				a.addWarning("line %d: circular include skipped: %s", i+1, filename)
				continue
			}
			included[absPath] = true
			data, err := os.ReadFile(includePath)
			if err != nil {
				return nil, fmt.Errorf("line %d: failed to include %s: %v", i+1, includePath, err)
			}
			subLines, err := a.preprocess(string(data), filepath.Dir(includePath), included)
			if err != nil {
				return nil, err
			}
			output = append(output, subLines...)
			continue
		}
		output = append(output, raw)
	}

	// Pass 0b: collect macros
	lines = output
	output = nil
	for i := 0; i < len(lines); i++ {
		stripped := stripComment(lines[i])
		trimmed := strings.TrimSpace(stripped)

		// Check for macro definition: "name macro" pattern
		fields := strings.Fields(trimmed)
		if len(fields) >= 2 && strings.ToLower(fields[1]) == "macro" {
			macroName := strings.ToLower(fields[0])
			// Count expected params by scanning body for \1..\9
			var body []string
			i++
			for i < len(lines) {
				bodyStripped := stripComment(lines[i])
				bodyTrimmed := strings.TrimSpace(bodyStripped)
				if strings.ToLower(bodyTrimmed) == "endm" {
					break
				}
				body = append(body, lines[i])
				i++
			}
			// Determine max param by scanning body for \1..\9
			maxParam := 0
			for _, bl := range body {
				for p := 1; p <= 9; p++ {
					if strings.Contains(bl, fmt.Sprintf("\\%d", p)) {
						if p > maxParam {
							maxParam = p
						}
					}
				}
			}
			a.macros[macroName] = &Macro{
				name:   macroName,
				params: maxParam,
				body:   body,
			}
			continue
		}
		output = append(output, lines[i])
	}

	// Pass 0c: expand macros and rept blocks, handle conditionals
	lines = output
	output = nil
	err := a.expandPass(lines, &output, 0)
	if err != nil {
		return nil, err
	}

	return output, nil
}

func (a *IE64Assembler) expandPass(lines []string, output *[]string, depth int) error {
	if depth > 100 {
		return fmt.Errorf("macro/rept expansion depth exceeded (possible infinite recursion)")
	}

	// Conditional stack: each entry is true if we are emitting lines
	type condState struct {
		active  bool // are we in the active branch?
		hadTrue bool // have we already seen a true branch?
		hasElse bool // have we seen else?
	}
	var condStack []condState
	emitting := func() bool {
		for _, c := range condStack {
			if !c.active {
				return false
			}
		}
		return true
	}

	for i := 0; i < len(lines); i++ {
		stripped := stripComment(lines[i])
		trimmed := strings.TrimSpace(stripped)
		lower := strings.ToLower(trimmed)
		fields := strings.Fields(trimmed)
		lowerFields := strings.Fields(lower)

		// Handle conditional assembly directives
		if len(lowerFields) > 0 {
			switch lowerFields[0] {
			case "if":
				if !emitting() {
					condStack = append(condStack, condState{active: false, hadTrue: true})
					continue
				}
				exprStr := strings.TrimSpace(trimmed[2:])
				val, err := a.evalExpr(exprStr)
				if err != nil {
					return fmt.Errorf("if expression error: %v", err)
				}
				isTrue := val != 0
				condStack = append(condStack, condState{active: isTrue, hadTrue: isTrue})
				continue

			case "else":
				if len(condStack) == 0 {
					return fmt.Errorf("else without matching if")
				}
				top := &condStack[len(condStack)-1]
				if top.hasElse {
					return fmt.Errorf("duplicate else")
				}
				top.hasElse = true
				// Check parent conditions are emitting
				parentEmitting := true
				for j := 0; j < len(condStack)-1; j++ {
					if !condStack[j].active {
						parentEmitting = false
						break
					}
				}
				if parentEmitting {
					top.active = !top.hadTrue
				}
				continue

			case "endif":
				if len(condStack) == 0 {
					return fmt.Errorf("endif without matching if")
				}
				condStack = condStack[:len(condStack)-1]
				continue
			}
		}

		if !emitting() {
			continue
		}

		// Handle rept blocks
		if len(lowerFields) > 0 && lowerFields[0] == "rept" {
			if len(fields) < 2 {
				return fmt.Errorf("rept requires a count")
			}
			countStr := strings.TrimSpace(trimmed[4:])
			count, err := a.evalExpr(countStr)
			if err != nil {
				return fmt.Errorf("rept count error: %v", err)
			}
			// Collect body until endr
			var body []string
			nestLevel := 1
			i++
			for i < len(lines) {
				bs := stripComment(lines[i])
				bt := strings.TrimSpace(bs)
				bl := strings.ToLower(bt)
				blf := strings.Fields(bl)
				if len(blf) > 0 && blf[0] == "rept" {
					nestLevel++
				} else if len(blf) > 0 && blf[0] == "endr" {
					nestLevel--
					if nestLevel == 0 {
						break
					}
				}
				body = append(body, lines[i])
				i++
			}
			if nestLevel != 0 {
				return fmt.Errorf("rept without matching endr")
			}
			// Expand the body count times
			for c := int64(0); c < count; c++ {
				err := a.expandPass(body, output, depth+1)
				if err != nil {
					return err
				}
			}
			continue
		}

		// Check for macro invocation
		if len(lowerFields) > 0 {
			macroName := lowerFields[0]
			if m, ok := a.macros[macroName]; ok {
				// Parse arguments (comma-separated, rest of line)
				var args []string
				if len(fields) > 1 {
					argStr := strings.TrimSpace(trimmed[len(fields[0]):])
					args = splitMacroArgs(argStr)
				}
				// Expand macro body with parameter substitution
				var expanded []string
				for _, bodyLine := range m.body {
					line := bodyLine
					// Replace narg
					line = strings.ReplaceAll(line, "narg", strconv.Itoa(len(args)))
					line = strings.ReplaceAll(line, "NARG", strconv.Itoa(len(args)))
					// Replace \1..\9 with arguments
					for p := 1; p <= 9; p++ {
						placeholder := fmt.Sprintf("\\%d", p)
						if p <= len(args) {
							line = strings.ReplaceAll(line, placeholder, args[p-1])
						} else {
							line = strings.ReplaceAll(line, placeholder, "")
						}
					}
					expanded = append(expanded, line)
				}
				err := a.expandPass(expanded, output, depth+1)
				if err != nil {
					return err
				}
				continue
			}
		}

		*output = append(*output, lines[i])
	}

	if len(condStack) > 0 {
		return fmt.Errorf("unterminated if block (%d level(s) deep)", len(condStack))
	}

	return nil
}

// splitMacroArgs splits a comma-separated argument string, trimming whitespace.
func splitMacroArgs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}

// ---------------------------------------------------------------------
// Pseudo-instruction expansion
// ---------------------------------------------------------------------

// expandPseudo takes a preprocessed line and returns one or more assembly lines.
// It handles: la, li, beqz, bnez, bltz, bgez, bgtz, blez
func (a *IE64Assembler) expandPseudo(line string) []string {
	stripped := stripComment(line)
	trimmed := strings.TrimSpace(stripped)
	if trimmed == "" {
		return nil
	}

	fields := strings.Fields(trimmed)
	mnemonic := strings.ToLower(fields[0])

	// Skip labels
	if strings.HasSuffix(fields[0], ":") {
		return []string{line}
	}

	switch mnemonic {
	case "la":
		// la rd, addr -> lea rd, addr(r0)
		rest := strings.TrimSpace(trimmed[2:])
		parts := splitOperands(rest)
		if len(parts) != 2 {
			return []string{line} // let the main assembler report the error
		}
		rd := strings.TrimSpace(parts[0])
		addr := strings.TrimSpace(parts[1])
		a.addWarning("pseudo-op 'la' lowered to lea %s, %s(r0)", rd, addr)
		return []string{fmt.Sprintf("\tlea %s, %s(r0)", rd, addr)}

	case "li":
		// li rd, #imm
		rest := strings.TrimSpace(trimmed[2:])
		parts := splitOperands(rest)
		if len(parts) != 2 {
			return []string{line}
		}
		rd := strings.TrimSpace(parts[0])
		immStr := strings.TrimSpace(parts[1])
		if !strings.HasPrefix(immStr, "#") {
			return []string{line}
		}
		immStr = strings.TrimSpace(immStr[1:])

		// Try to evaluate
		val, err := a.evalExprUint64(immStr)
		if err != nil {
			// On pass 1, might not resolve — emit two instructions to be safe
			return []string{
				fmt.Sprintf("\tmove.l %s, #%s", rd, immStr),
				fmt.Sprintf("\tmovt %s, #0", rd),
			}
		}
		if val <= math.MaxUint32 {
			// Fits in 32 bits
			a.addWarning("pseudo-op 'li' lowered to move.l %s, #%d", rd, val)
			return []string{fmt.Sprintf("\tmove.l %s, #%d", rd, val)}
		}
		// 64-bit: lo32 + hi32
		lo := uint32(val & 0xFFFFFFFF)
		hi := uint32(val >> 32)
		a.addWarning("pseudo-op 'li' lowered to move.l + movt %s, #$%X_%08X", rd, hi, lo)
		return []string{
			fmt.Sprintf("\tmove.l %s, #%d", rd, lo),
			fmt.Sprintf("\tmovt %s, #%d", rd, hi),
		}

	case "beqz":
		return expandZeroBranch("beq", trimmed[4:])
	case "bnez":
		return expandZeroBranch("bne", trimmed[4:])
	case "bltz":
		return expandZeroBranch("blt", trimmed[4:])
	case "bgez":
		return expandZeroBranch("bge", trimmed[4:])
	case "bgtz":
		return expandZeroBranch("bgt", trimmed[4:])
	case "blez":
		return expandZeroBranch("ble", trimmed[4:])

	default:
		return []string{line}
	}
}

// expandZeroBranch converts e.g. "beqz rs, label" -> "beq rs, r0, label"
func expandZeroBranch(mnemonic string, rest string) []string {
	parts := splitOperands(strings.TrimSpace(rest))
	if len(parts) != 2 {
		return []string{fmt.Sprintf("\t%s %s", mnemonic, strings.TrimSpace(rest))}
	}
	rs := strings.TrimSpace(parts[0])
	label := strings.TrimSpace(parts[1])
	return []string{fmt.Sprintf("\t%s %s, r0, %s", mnemonic, rs, label)}
}

// splitOperands splits on commas but respects parentheses.
func splitOperands(s string) []string {
	var result []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				result = append(result, s[start:i])
				start = i + 1
			}
		}
	}
	result = append(result, s[start:])
	return result
}

// ---------------------------------------------------------------------
// Instruction size calculation (for pass 1)
// ---------------------------------------------------------------------

// instrLineSize returns the number of bytes a non-directive instruction will emit.
// This is always 8 for IE64.
func instrLineSize() uint32 {
	return instrSize
}

// directiveSize calculates the size in bytes that a directive emits.
func (a *IE64Assembler) directiveSize(trimmed string) (uint32, error) {
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return 0, nil
	}
	lower := strings.ToLower(fields[0])

	switch {
	case strings.HasPrefix(lower, "dc."):
		return a.calcDCSize(trimmed)
	case strings.HasPrefix(lower, "ds."):
		return a.calcDSSize(trimmed)
	case lower == "align":
		if len(fields) < 2 {
			return 0, fmt.Errorf("align requires an argument")
		}
		alignment, err := a.evalExpr(fields[1])
		if err != nil {
			return 0, err
		}
		if alignment <= 0 {
			return 0, fmt.Errorf("align value must be positive")
		}
		al := uint32(alignment)
		currentAddr := a.baseAddr + a.codeOffset
		padding := (al - (currentAddr % al)) % al
		return padding, nil
	case lower == "incbin":
		return a.calcIncbinSize(trimmed)
	default:
		return 0, nil
	}
}

func (a *IE64Assembler) calcDCSize(line string) (uint32, error) {
	fields := strings.Fields(line)
	if len(fields) < 1 {
		return 0, nil
	}
	directive := strings.ToLower(fields[0])
	rest := strings.TrimSpace(line[len(fields[0]):])

	var unitSize uint32
	switch directive {
	case "dc.b":
		unitSize = 1
	case "dc.w":
		unitSize = 2
	case "dc.l":
		unitSize = 4
	case "dc.q":
		unitSize = 8
	default:
		return 0, fmt.Errorf("unknown dc directive: %s", directive)
	}

	// For dc.b, handle strings
	if directive == "dc.b" {
		return calcDCBSize(rest), nil
	}

	// Count comma-separated values
	values := splitOperands(rest)
	count := uint32(0)
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v != "" {
			count++
		}
	}
	return count * unitSize, nil
}

func calcDCBSize(rest string) uint32 {
	var total uint32
	// Walk through, handling strings and values
	i := 0
	for i < len(rest) {
		// Skip whitespace and commas
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == ',') {
			i++
		}
		if i >= len(rest) {
			break
		}
		if rest[i] == '"' {
			// String literal
			i++ // skip opening quote
			for i < len(rest) && rest[i] != '"' {
				if rest[i] == '\\' && i+1 < len(rest) {
					i++ // skip escape sequence
				}
				total++
				i++
			}
			if i < len(rest) {
				i++ // skip closing quote
			}
		} else {
			// Numeric value — skip to next comma or end
			for i < len(rest) && rest[i] != ',' {
				i++
			}
			total++
		}
	}
	return total
}

func (a *IE64Assembler) calcDSSize(line string) (uint32, error) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, fmt.Errorf("ds directive requires a count")
	}
	directive := strings.ToLower(fields[0])
	countStr := strings.TrimSpace(strings.Join(fields[1:], " "))
	count, err := a.evalExpr(countStr)
	if err != nil {
		return 0, err
	}
	if count < 0 {
		return 0, fmt.Errorf("ds count must be non-negative")
	}
	switch directive {
	case "ds.b":
		return uint32(count), nil
	case "ds.w":
		return uint32(count) * 2, nil
	case "ds.l":
		return uint32(count) * 4, nil
	case "ds.q":
		return uint32(count) * 8, nil
	default:
		return 0, fmt.Errorf("unknown ds directive: %s", directive)
	}
}

func (a *IE64Assembler) calcIncbinSize(line string) (uint32, error) {
	// Parse: incbin "file" [,offset [,length]]
	rest := strings.TrimSpace(line[6:]) // skip "incbin"
	parts := splitOperands(rest)
	if len(parts) < 1 {
		return 0, fmt.Errorf("incbin requires a filename")
	}
	filename := strings.Trim(strings.TrimSpace(parts[0]), "\"'")
	path := filepath.Join(a.basePath, filename)
	info, err := os.Stat(path)
	if err != nil {
		a.addWarning("cannot stat incbin file %s: %v", path, err)
		return 0, nil
	}
	length := uint64(info.Size())
	offset := uint64(0)
	if len(parts) >= 2 {
		off, err := a.evalExprUint64(strings.TrimSpace(parts[1]))
		if err == nil {
			offset = off
			if offset > length {
				offset = length
			}
			length -= offset
		}
	}
	if len(parts) >= 3 {
		ln, err := a.evalExprUint64(strings.TrimSpace(parts[2]))
		if err == nil {
			length = ln
		}
	}
	return uint32(length), nil
}

// ---------------------------------------------------------------------
// Assemble
// ---------------------------------------------------------------------

// Assemble takes source code as a string and returns the assembled binary.
func (a *IE64Assembler) Assemble(source string) ([]byte, error) {
	a.errors = nil
	a.warnings = nil
	a.listing = nil

	// Pass 0: preprocessing
	preprocessed, err := a.preprocess(source, a.basePath, nil)
	if err != nil {
		return nil, err
	}

	// Expand pseudo-instructions
	var expanded []string
	for _, line := range preprocessed {
		exp := a.expandPseudo(line)
		expanded = append(expanded, exp...)
	}

	// Pass 1: label collection, address calculation
	a.pass = 1
	a.codeOffset = 0
	a.lastGlobalLabel = ""
	maxAddr := a.baseAddr

	for _, line := range expanded {
		stripped := stripComment(line)
		trimmed := strings.TrimSpace(stripped)
		if trimmed == "" {
			continue
		}

		fields := strings.Fields(trimmed)
		lower := strings.ToLower(fields[0])

		// org directive
		if lower == "org" {
			if len(fields) < 2 {
				return nil, fmt.Errorf("org requires an address")
			}
			addrStr := strings.TrimSpace(strings.Join(fields[1:], " "))
			addr, err := a.evalExprUint64(addrStr)
			if err != nil {
				return nil, fmt.Errorf("org: %v", err)
			}
			a.baseAddr = uint32(addr)
			a.codeOffset = 0
			continue
		}

		// equ directive: NAME equ value
		if len(fields) >= 3 && strings.ToLower(fields[1]) == "equ" {
			name := fields[0]
			exprStr := strings.TrimSpace(trimmed[len(fields[0]):])
			exprStr = strings.TrimSpace(exprStr[3:]) // skip "equ"
			val, err := a.evalExprUint64(exprStr)
			if err != nil {
				return nil, fmt.Errorf("equ '%s': %v", name, err)
			}
			if _, exists := a.equates[name]; exists {
				return nil, fmt.Errorf("symbol '%s' already defined with equ", name)
			}
			a.equates[name] = val
			continue
		}

		// set directive: NAME set value
		if len(fields) >= 3 && strings.ToLower(fields[1]) == "set" {
			name := fields[0]
			exprStr := strings.TrimSpace(trimmed[len(fields[0]):])
			exprStr = strings.TrimSpace(exprStr[3:]) // skip "set"
			val, err := a.evalExprUint64(exprStr)
			if err != nil {
				return nil, fmt.Errorf("set '%s': %v", name, err)
			}
			a.sets[name] = val
			continue
		}

		// Label definition
		if strings.HasSuffix(fields[0], ":") {
			labelName := strings.TrimSuffix(fields[0], ":")
			if strings.HasPrefix(labelName, ".") {
				// Local label
				if a.lastGlobalLabel == "" {
					return nil, fmt.Errorf("local label '%s' before any global label", labelName)
				}
				fullName := a.lastGlobalLabel + labelName
				a.labels[fullName] = a.baseAddr + a.codeOffset
			} else {
				// Global label
				a.lastGlobalLabel = labelName
				a.labels[labelName] = a.baseAddr + a.codeOffset
			}
			// If there's more on the line after the label, process it
			if len(fields) > 1 {
				restLine := strings.TrimSpace(trimmed[len(fields[0]):])
				size, err := a.lineSize(restLine)
				if err != nil {
					return nil, err
				}
				a.codeOffset += size
				nextAddr := a.baseAddr + a.codeOffset
				if nextAddr > maxAddr {
					maxAddr = nextAddr
				}
			}
			continue
		}

		// Directive or instruction
		size, err := a.lineSize(trimmed)
		if err != nil {
			return nil, err
		}
		a.codeOffset += size
		nextAddr := a.baseAddr + a.codeOffset
		if nextAddr > maxAddr {
			maxAddr = nextAddr
		}
	}

	// Pass 2: code generation
	a.pass = 2
	a.codeOffset = 0
	a.lastGlobalLabel = ""

	programSize := maxAddr - a.baseAddr
	if programSize == 0 {
		return []byte{}, nil
	}
	program := make([]byte, programSize)

	for _, line := range expanded {
		stripped := stripComment(line)
		trimmed := strings.TrimSpace(stripped)
		if trimmed == "" {
			if a.listingMode {
				a.addListing(0, nil, line)
			}
			continue
		}

		fields := strings.Fields(trimmed)
		lower := strings.ToLower(fields[0])

		// org
		if lower == "org" {
			addrStr := strings.TrimSpace(strings.Join(fields[1:], " "))
			addr, _ := a.evalExprUint64(addrStr)
			if a.listingMode {
				a.addListing(uint32(addr), nil, trimmed)
			}
			a.baseAddr = uint32(addr)
			a.codeOffset = 0
			continue
		}

		// equ
		if len(fields) >= 3 && strings.ToLower(fields[1]) == "equ" {
			if a.listingMode {
				name := fields[0]
				val := a.equates[name]
				a.listing = append(a.listing, fmt.Sprintf("         = %016X  %s", val, trimmed))
			}
			continue
		}

		// set
		if len(fields) >= 3 && strings.ToLower(fields[1]) == "set" {
			name := fields[0]
			exprStr := strings.TrimSpace(trimmed[len(fields[0]):])
			exprStr = strings.TrimSpace(exprStr[3:])
			val, _ := a.evalExprUint64(exprStr)
			a.sets[name] = val
			if a.listingMode {
				a.listing = append(a.listing, fmt.Sprintf("         = %016X  %s", val, trimmed))
			}
			continue
		}

		// Label
		if strings.HasSuffix(fields[0], ":") {
			labelName := strings.TrimSuffix(fields[0], ":")
			if !strings.HasPrefix(labelName, ".") {
				a.lastGlobalLabel = labelName
			}
			if a.listingMode {
				fullName := labelName
				if strings.HasPrefix(labelName, ".") && a.lastGlobalLabel != "" {
					fullName = a.lastGlobalLabel + labelName
				}
				addr := a.labels[fullName]
				a.addListing(addr, nil, trimmed)
			}
			// If there's more on the line after the label
			if len(fields) > 1 {
				restLine := strings.TrimSpace(trimmed[len(fields[0]):])
				err := a.assembleLine(restLine, program)
				if err != nil {
					return nil, err
				}
			}
			continue
		}

		// Directive or instruction
		err := a.assembleLine(trimmed, program)
		if err != nil {
			return nil, err
		}
	}

	if len(a.errors) > 0 {
		return nil, fmt.Errorf("assembly errors:\n%s", strings.Join(a.errors, "\n"))
	}

	return program, nil
}

// lineSize returns the number of bytes a line will emit (for pass 1 address calculation).
func (a *IE64Assembler) lineSize(trimmed string) (uint32, error) {
	if trimmed == "" {
		return 0, nil
	}
	fields := strings.Fields(trimmed)
	lower := strings.ToLower(fields[0])

	// Directives
	if isDirective(lower) {
		return a.directiveSize(trimmed)
	}

	// Everything else is an instruction (8 bytes)
	return instrSize, nil
}

func isDirective(lower string) bool {
	return strings.HasPrefix(lower, "dc.") ||
		strings.HasPrefix(lower, "ds.") ||
		lower == "align" ||
		lower == "incbin" ||
		lower == "org"
}

// ---------------------------------------------------------------------
// assembleLine — pass 2 code generation for a single line
// ---------------------------------------------------------------------

func (a *IE64Assembler) assembleLine(trimmed string, program []byte) error {
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}
	lower := strings.ToLower(fields[0])

	// Directives
	if isDirective(lower) {
		return a.assembleDirective(trimmed, program)
	}

	// Instruction
	return a.assembleInstruction(trimmed, program)
}

// ---------------------------------------------------------------------
// Directive assembly
// ---------------------------------------------------------------------

func (a *IE64Assembler) assembleDirective(trimmed string, program []byte) error {
	fields := strings.Fields(trimmed)
	lower := strings.ToLower(fields[0])
	startOffset := a.codeOffset

	switch {
	case strings.HasPrefix(lower, "dc."):
		return a.assembleDC(trimmed, program, startOffset)
	case strings.HasPrefix(lower, "ds."):
		return a.assembleDS(trimmed, program, startOffset)
	case lower == "align":
		return a.assembleAlign(trimmed, program, startOffset)
	case lower == "incbin":
		return a.assembleIncbin(trimmed, program, startOffset)
	default:
		return nil
	}
}

func (a *IE64Assembler) assembleDC(line string, program []byte, startOffset uint32) error {
	fields := strings.Fields(line)
	directive := strings.ToLower(fields[0])
	rest := strings.TrimSpace(line[len(fields[0]):])

	switch directive {
	case "dc.b":
		data := a.parseDCB(rest)
		copy(program[a.codeOffset:], data)
		if a.listingMode {
			a.addListing(a.baseAddr+startOffset, data, line)
		}
		a.codeOffset += uint32(len(data))
		return nil

	case "dc.w":
		values := splitOperands(rest)
		var data []byte
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			val, err := a.evalExpr(v)
			if err != nil {
				return fmt.Errorf("dc.w: %v", err)
			}
			buf := make([]byte, 2)
			binary.LittleEndian.PutUint16(buf, uint16(val))
			data = append(data, buf...)
		}
		copy(program[a.codeOffset:], data)
		if a.listingMode {
			a.addListing(a.baseAddr+startOffset, data, line)
		}
		a.codeOffset += uint32(len(data))
		return nil

	case "dc.l":
		values := splitOperands(rest)
		var data []byte
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			val, err := a.evalExpr(v)
			if err != nil {
				return fmt.Errorf("dc.l: %v", err)
			}
			buf := make([]byte, 4)
			binary.LittleEndian.PutUint32(buf, uint32(val))
			data = append(data, buf...)
		}
		copy(program[a.codeOffset:], data)
		if a.listingMode {
			a.addListing(a.baseAddr+startOffset, data, line)
		}
		a.codeOffset += uint32(len(data))
		return nil

	case "dc.q":
		values := splitOperands(rest)
		var data []byte
		for _, v := range values {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			val, err := a.evalExprUint64(v)
			if err != nil {
				return fmt.Errorf("dc.q: %v", err)
			}
			buf := make([]byte, 8)
			binary.LittleEndian.PutUint64(buf, val)
			data = append(data, buf...)
		}
		copy(program[a.codeOffset:], data)
		if a.listingMode {
			a.addListing(a.baseAddr+startOffset, data, line)
		}
		a.codeOffset += uint32(len(data))
		return nil
	}

	return fmt.Errorf("unknown dc directive: %s", directive)
}

func (a *IE64Assembler) parseDCB(rest string) []byte {
	var data []byte
	i := 0
	for i < len(rest) {
		// Skip whitespace and commas
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t' || rest[i] == ',') {
			i++
		}
		if i >= len(rest) {
			break
		}
		if rest[i] == '"' {
			// String literal
			i++
			start := i
			var strBytes []byte
			for i < len(rest) && rest[i] != '"' {
				if rest[i] == '\\' && i+1 < len(rest) {
					strBytes = append(strBytes, unescapeChar(rest[i+1]))
					i += 2
				} else {
					strBytes = append(strBytes, rest[i])
					i++
				}
			}
			_ = start
			if i >= len(rest) {
				a.addError("unclosed string literal in dc.b")
				return data
			}
			data = append(data, strBytes...)
			i++ // skip closing quote
		} else {
			// Numeric value — find end
			start := i
			for i < len(rest) && rest[i] != ',' {
				i++
			}
			valStr := strings.TrimSpace(rest[start:i])
			if valStr != "" {
				val, err := a.evalExpr(valStr)
				if err != nil {
					a.addError("dc.b value error: %v", err)
					data = append(data, 0)
				} else {
					data = append(data, byte(val))
				}
			}
		}
	}
	return data
}

func (a *IE64Assembler) assembleDS(line string, program []byte, startOffset uint32) error {
	fields := strings.Fields(line)
	directive := strings.ToLower(fields[0])
	countStr := strings.TrimSpace(strings.Join(fields[1:], " "))
	count, err := a.evalExpr(countStr)
	if err != nil {
		return fmt.Errorf("ds: %v", err)
	}
	if count < 0 {
		return fmt.Errorf("ds count must be non-negative")
	}

	var size uint32
	switch directive {
	case "ds.b":
		size = uint32(count)
	case "ds.w":
		size = uint32(count) * 2
	case "ds.l":
		size = uint32(count) * 4
	case "ds.q":
		size = uint32(count) * 8
	default:
		return fmt.Errorf("unknown ds directive: %s", directive)
	}

	// Zero-fill (already zero in Go slice, but be explicit)
	for i := uint32(0); i < size; i++ {
		program[a.codeOffset+i] = 0
	}
	if a.listingMode {
		a.addListing(a.baseAddr+startOffset, nil, fmt.Sprintf("%s  ; %d bytes reserved", line, size))
	}
	a.codeOffset += size
	return nil
}

func (a *IE64Assembler) assembleAlign(line string, program []byte, startOffset uint32) error {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return fmt.Errorf("align requires an argument")
	}
	alignment, err := a.evalExpr(fields[1])
	if err != nil {
		return fmt.Errorf("align: %v", err)
	}
	if alignment <= 0 {
		return fmt.Errorf("align value must be positive")
	}
	al := uint32(alignment)
	currentAddr := a.baseAddr + a.codeOffset
	padding := (al - (currentAddr % al)) % al
	// Fill with zeros (nop)
	for i := uint32(0); i < padding; i++ {
		program[a.codeOffset+i] = 0
	}
	if a.listingMode && padding > 0 {
		a.addListing(a.baseAddr+startOffset, nil, fmt.Sprintf("%s  ; %d bytes padding", line, padding))
	}
	a.codeOffset += padding
	return nil
}

func (a *IE64Assembler) assembleIncbin(line string, program []byte, startOffset uint32) error {
	rest := strings.TrimSpace(line[6:]) // skip "incbin"
	parts := splitOperands(rest)
	if len(parts) < 1 {
		return fmt.Errorf("incbin requires a filename")
	}
	filename := strings.Trim(strings.TrimSpace(parts[0]), "\"'")
	path := filepath.Join(a.basePath, filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("incbin: cannot read %s: %v", path, err)
	}

	offset := uint64(0)
	length := uint64(len(data))

	if len(parts) >= 2 {
		off, err := a.evalExprUint64(strings.TrimSpace(parts[1]))
		if err != nil {
			return fmt.Errorf("incbin offset: %v", err)
		}
		offset = off
		if offset > uint64(len(data)) {
			return fmt.Errorf("incbin offset out of range: %d", offset)
		}
		length = uint64(len(data)) - offset
	}
	if len(parts) >= 3 {
		ln, err := a.evalExprUint64(strings.TrimSpace(parts[2]))
		if err != nil {
			return fmt.Errorf("incbin length: %v", err)
		}
		length = ln
	}
	if offset+length > uint64(len(data)) {
		return fmt.Errorf("incbin range out of bounds: %d+%d > %d", offset, length, len(data))
	}

	segment := data[offset : offset+length]
	copy(program[a.codeOffset:], segment)
	if a.listingMode {
		a.addListing(a.baseAddr+startOffset, nil, fmt.Sprintf("%s  ; %d bytes", line, length))
	}
	a.codeOffset += uint32(length)
	return nil
}

// ---------------------------------------------------------------------
// Instruction assembly
// ---------------------------------------------------------------------

func (a *IE64Assembler) assembleInstruction(trimmed string, program []byte) error {
	fields := strings.Fields(trimmed)
	if len(fields) == 0 {
		return nil
	}

	mnemonicRaw := strings.ToLower(fields[0])
	base, size := parseSizeSuffix(mnemonicRaw)

	// Reject size suffixes on FP instructions (always 32-bit)
	if strings.HasPrefix(base, "f") && base != "f" { // base != "f" avoids matching potential future 'f' pseudo-ops
		// List of FP mnemonics to validate
		fpMnemonics := map[string]bool{
			"fmov": true, "fload": true, "fstore": true, "fadd": true, "fsub": true,
			"fmul": true, "fdiv": true, "fmod": true, "fabs": true, "fneg": true,
			"fsqrt": true, "fint": true, "fcmp": true, "fcvtif": true, "fcvtfi": true,
			"fmovi": true, "fmovo": true, "fsin": true, "fcos": true, "ftan": true,
			"fatan": true, "flog": true, "fexp": true, "fpow": true, "fmovecr": true,
			"fmovsr": true, "fmovcr": true, "fmovsc": true, "fmovcc": true,
		}
		if fpMnemonics[base] && strings.Contains(mnemonicRaw, ".") {
			return fmt.Errorf("size suffixes not allowed on FP instruction: %s", mnemonicRaw)
		}
	}
	// Build operand string from everything after the mnemonic
	operandStr := ""
	if len(fields) > 1 {
		// Rejoin everything after the first field
		idx := strings.Index(trimmed, fields[0])
		operandStr = strings.TrimSpace(trimmed[idx+len(fields[0]):])
	}
	// For case-insensitive handling, we keep the raw operand string (registers are case-insensitive)
	operands := splitOperands(operandStr)

	startOffset := a.codeOffset
	currentPC := a.baseAddr + a.codeOffset

	var instr []byte
	var err error

	switch base {
	// Data movement
	case "move":
		instr, err = a.asmMove(size, operands)
	case "movt":
		instr, err = a.asmMovt(operands)
	case "moveq":
		instr, err = a.asmMoveq(operands)
	case "lea":
		instr, err = a.asmLea(operands)

	// Load/Store
	case "load":
		instr, err = a.asmLoadStore(OP64_LOAD, size, operands)
	case "store":
		instr, err = a.asmLoadStore(OP64_STORE, size, operands)

	// 3-operand arithmetic
	case "add":
		instr, err = a.asmALU3(OP64_ADD, size, operands)
	case "sub":
		instr, err = a.asmALU3(OP64_SUB, size, operands)
	case "mulu":
		instr, err = a.asmALU3(OP64_MULU, size, operands)
	case "muls":
		instr, err = a.asmALU3(OP64_MULS, size, operands)
	case "divu":
		instr, err = a.asmALU3(OP64_DIVU, size, operands)
	case "divs":
		instr, err = a.asmALU3(OP64_DIVS, size, operands)
	case "mod":
		instr, err = a.asmALU3(OP64_MOD, size, operands)

	// 2-operand arithmetic
	case "neg":
		instr, err = a.asmALU2(OP64_NEG, size, operands)

	// 3-operand logical
	case "and":
		instr, err = a.asmALU3(OP64_AND, size, operands)
	case "or":
		instr, err = a.asmALU3(OP64_OR, size, operands)
	case "eor":
		instr, err = a.asmALU3(OP64_EOR, size, operands)

	// 2-operand logical
	case "not":
		instr, err = a.asmALU2(OP64_NOT, size, operands)

	// Shifts (3-operand)
	case "lsl":
		instr, err = a.asmALU3(OP64_LSL, size, operands)
	case "lsr":
		instr, err = a.asmALU3(OP64_LSR, size, operands)
	case "asr":
		instr, err = a.asmALU3(OP64_ASR, size, operands)
	case "clz":
		instr, err = a.asmALU2(OP64_CLZ, size, operands)

	// Branches
	case "bra":
		instr, err = a.asmBra(operands, currentPC)
	case "beq":
		instr, err = a.asmBcc(OP64_BEQ, operands, currentPC)
	case "bne":
		instr, err = a.asmBcc(OP64_BNE, operands, currentPC)
	case "blt":
		instr, err = a.asmBcc(OP64_BLT, operands, currentPC)
	case "bge":
		instr, err = a.asmBcc(OP64_BGE, operands, currentPC)
	case "bgt":
		instr, err = a.asmBcc(OP64_BGT, operands, currentPC)
	case "ble":
		instr, err = a.asmBcc(OP64_BLE, operands, currentPC)
	case "bhi":
		instr, err = a.asmBcc(OP64_BHI, operands, currentPC)
	case "bls":
		instr, err = a.asmBcc(OP64_BLS, operands, currentPC)

	// Branches (register-indirect)
	case "jmp":
		instr, err = a.asmJmp(operands)

	// Subroutine/Stack
	case "jsr":
		instr, err = a.asmJsr(operands, currentPC)
	case "rts":
		instr = encodeInstruction(OP64_RTS, 0, 0, 0, 0, 0, 0)
	case "push":
		instr, err = a.asmPushPop(OP64_PUSH, operands)
	case "pop":
		instr, err = a.asmPushPop(OP64_POP, operands)

	// Floating Point (FPU)
	case "fmov":
		instr, err = a.asmFP2(OP64_FMOV, operands, true, true)
	case "fload":
		instr, err = a.asmFP_Mem(OP64_FLOAD, operands, true)
	case "fstore":
		instr, err = a.asmFP_Mem(OP64_FSTORE, operands, false)
	case "fadd":
		instr, err = a.asmFP3(OP64_FADD, operands)
	case "fsub":
		instr, err = a.asmFP3(OP64_FSUB, operands)
	case "fmul":
		instr, err = a.asmFP3(OP64_FMUL, operands)
	case "fdiv":
		instr, err = a.asmFP3(OP64_FDIV, operands)
	case "fmod":
		instr, err = a.asmFP3(OP64_FMOD, operands)
	case "fabs":
		instr, err = a.asmFP2(OP64_FABS, operands, true, true)
	case "fneg":
		instr, err = a.asmFP2(OP64_FNEG, operands, true, true)
	case "fsqrt":
		instr, err = a.asmFP2(OP64_FSQRT, operands, true, true)
	case "fint":
		instr, err = a.asmFP2(OP64_FINT, operands, true, true)
	case "fcmp":
		instr, err = a.asmFP3_Int(OP64_FCMP, operands)
	case "fcvtif":
		instr, err = a.asmFP2(OP64_FCVTIF, operands, true, false)
	case "fcvtfi":
		instr, err = a.asmFP2(OP64_FCVTFI, operands, false, true)
	case "fmovi":
		instr, err = a.asmFP2(OP64_FMOVI, operands, true, false)
	case "fmovo":
		instr, err = a.asmFP2(OP64_FMOVO, operands, false, true)
	case "fsin":
		instr, err = a.asmFP2(OP64_FSIN, operands, true, true)
	case "fcos":
		instr, err = a.asmFP2(OP64_FCOS, operands, true, true)
	case "ftan":
		instr, err = a.asmFP2(OP64_FTAN, operands, true, true)
	case "fatan":
		instr, err = a.asmFP2(OP64_FATAN, operands, true, true)
	case "flog":
		instr, err = a.asmFP2(OP64_FLOG, operands, true, true)
	case "fexp":
		instr, err = a.asmFP2(OP64_FEXP, operands, true, true)
	case "fpow":
		instr, err = a.asmFP3(OP64_FPOW, operands)
	case "fmovecr":
		instr, err = a.asmFP_Imm(OP64_FMOVECR, operands)
	case "fmovsr":
		instr, err = a.asmFP_Status(OP64_FMOVSR, operands, false)
	case "fmovcr":
		instr, err = a.asmFP_Status(OP64_FMOVCR, operands, false)
	case "fmovsc":
		instr, err = a.asmFP_Status(OP64_FMOVSC, operands, true)
	case "fmovcc":
		instr, err = a.asmFP_Status(OP64_FMOVCC, operands, true)

	// System
	case "nop":
		instr = encodeInstruction(OP64_NOP, 0, 0, 0, 0, 0, 0)
	case "halt":
		instr = encodeInstruction(OP64_HALT, 0, 0, 0, 0, 0, 0)
	case "sei":
		instr = encodeInstruction(OP64_SEI, 0, 0, 0, 0, 0, 0)
	case "cli":
		instr = encodeInstruction(OP64_CLI, 0, 0, 0, 0, 0, 0)
	case "rti":
		instr = encodeInstruction(OP64_RTI, 0, 0, 0, 0, 0, 0)
	case "wait":
		instr, err = a.asmWait(operands)

	default:
		return fmt.Errorf("unknown instruction: %s", base)
	}

	if err != nil {
		return fmt.Errorf("%s: %v", trimmed, err)
	}

	copy(program[a.codeOffset:], instr)
	if a.listingMode {
		a.addListing(a.baseAddr+startOffset, instr, trimmed)
	}
	a.codeOffset += instrSize
	return nil
}

// ---------------------------------------------------------------------
// Instruction-specific assemblers
// ---------------------------------------------------------------------

// asmMove handles: move.s rd, rs (X=0) or move.s rd, #imm (X=1)
func (a *IE64Assembler) asmMove(size byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("move requires 2 operands (rd, rs/#imm)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	if strings.HasPrefix(src, "#") {
		// Immediate
		immStr := strings.TrimSpace(src[1:])
		val, err := a.evalExpr(immStr)
		if err != nil {
			return nil, fmt.Errorf("immediate value: %v", err)
		}
		// Lint: warn about size truncation
		uval := uint64(val)
		switch size {
		case SIZE_B:
			if uval > 0xFF {
				a.addWarning("immediate $%X truncated to 8-bit (.b)", uval)
			}
		case SIZE_W:
			if uval > 0xFFFF {
				a.addWarning("immediate $%X truncated to 16-bit (.w)", uval)
			}
		case SIZE_L:
			if uval > 0xFFFFFFFF {
				a.addWarning("immediate $%X truncated to 32-bit (.l)", uval)
			}
		}
		return encodeInstruction(OP64_MOVE, rd, size, 1, 0, 0, uint32(val)), nil
	}
	// Register
	rs, ok := parseRegister(src)
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", src)
	}
	return encodeInstruction(OP64_MOVE, rd, size, 0, rs, 0, 0), nil
}

// asmMovt handles: movt rd, #imm
func (a *IE64Assembler) asmMovt(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("movt requires 2 operands (rd, #imm)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	if !strings.HasPrefix(src, "#") {
		return nil, fmt.Errorf("movt requires immediate operand")
	}
	immStr := strings.TrimSpace(src[1:])
	val, err := a.evalExpr(immStr)
	if err != nil {
		return nil, fmt.Errorf("immediate value: %v", err)
	}
	return encodeInstruction(OP64_MOVT, rd, SIZE_Q, 1, 0, 0, uint32(val)), nil
}

// asmMoveq handles: moveq rd, #imm
func (a *IE64Assembler) asmMoveq(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("moveq requires 2 operands (rd, #imm)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	if !strings.HasPrefix(src, "#") {
		return nil, fmt.Errorf("moveq requires immediate operand")
	}
	immStr := strings.TrimSpace(src[1:])
	val, err := a.evalExpr(immStr)
	if err != nil {
		return nil, fmt.Errorf("immediate value: %v", err)
	}
	return encodeInstruction(OP64_MOVEQ, rd, SIZE_Q, 1, 0, 0, uint32(val)), nil
}

// asmLea handles: lea rd, disp(rs)
func (a *IE64Assembler) asmLea(operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("lea requires 2 operands (rd, disp(rs))")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	// Parse disp(rs)
	src := strings.TrimSpace(operands[1])
	disp, rs, err := a.parseDispReg(src)
	if err != nil {
		return nil, fmt.Errorf("lea: %v", err)
	}
	return encodeInstruction(OP64_LEA, rd, SIZE_Q, 1, rs, 0, uint32(disp)), nil
}

// parseDispReg parses "disp(rs)" or "(rs)" and returns (displacement, register, error).
func (a *IE64Assembler) parseDispReg(s string) (int64, byte, error) {
	s = strings.TrimSpace(s)
	parenIdx := strings.Index(s, "(")
	if parenIdx < 0 {
		return 0, 0, fmt.Errorf("expected disp(rs) form, got: %s", s)
	}
	closeIdx := strings.Index(s, ")")
	if closeIdx < 0 || closeIdx < parenIdx {
		return 0, 0, fmt.Errorf("missing closing parenthesis in: %s", s)
	}

	dispStr := strings.TrimSpace(s[:parenIdx])
	regStr := strings.TrimSpace(s[parenIdx+1 : closeIdx])

	rs, ok := parseRegister(regStr)
	if !ok {
		return 0, 0, fmt.Errorf("invalid register in addressing mode: %s", regStr)
	}

	var disp int64
	if dispStr == "" {
		disp = 0
	} else {
		var err error
		disp, err = a.evalExpr(dispStr)
		if err != nil {
			return 0, 0, fmt.Errorf("displacement: %v", err)
		}
	}

	return disp, rs, nil
}

// asmLoadStore handles: load.s rd, (rs) or load.s rd, disp(rs)
// and: store.s rd, (rs) or store.s rd, disp(rs)
func (a *IE64Assembler) asmLoadStore(opcode byte, size byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("load/store requires 2 operands")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	src := strings.TrimSpace(operands[1])
	disp, rs, err := a.parseDispReg(src)
	if err != nil {
		return nil, err
	}
	var xbit byte
	if disp != 0 {
		xbit = 1
	}
	return encodeInstruction(opcode, rd, size, xbit, rs, 0, uint32(disp)), nil
}

// asmALU3 handles 3-operand instructions:
// op.s rd, rs, rt  (register, X=0)
// op.s rd, rs, #imm (immediate, X=1)
func (a *IE64Assembler) asmALU3(opcode byte, size byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("requires 3 operands (rd, rs, rt/#imm)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	third := strings.TrimSpace(operands[2])
	if strings.HasPrefix(third, "#") {
		// Immediate
		immStr := strings.TrimSpace(third[1:])
		val, err := a.evalExpr(immStr)
		if err != nil {
			return nil, fmt.Errorf("immediate: %v", err)
		}
		return encodeInstruction(opcode, rd, size, 1, rs, 0, uint32(val)), nil
	}
	// Register
	rt, ok := parseRegister(third)
	if !ok {
		return nil, fmt.Errorf("invalid third operand: %s", third)
	}
	return encodeInstruction(opcode, rd, size, 0, rs, rt, 0), nil
}

// asmALU2 handles 2-operand instructions: op.s rd, rs
func (a *IE64Assembler) asmALU2(opcode byte, size byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("requires 2 operands (rd, rs)")
	}
	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}
	rs, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}
	return encodeInstruction(opcode, rd, size, 0, rs, 0, 0), nil
}

// asmBra handles: bra label (imm32 = target - PC)
func (a *IE64Assembler) asmBra(operands []string, pc uint32) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("bra requires 1 operand (label)")
	}
	target, err := a.resolveLabel(strings.TrimSpace(operands[0]))
	if err != nil {
		return nil, err
	}
	offset := int32(target) - int32(pc)
	return encodeInstruction(OP64_BRA, 0, SIZE_Q, 0, 0, 0, uint32(offset)), nil
}

// asmBcc handles: bcc rs, rt, label (imm32 = target - PC)
func (a *IE64Assembler) asmBcc(opcode byte, operands []string, pc uint32) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("conditional branch requires 3 operands (rs, rt, label)")
	}
	rs, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	rt, ok := parseRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[1])
	}
	target, err := a.resolveLabel(strings.TrimSpace(operands[2]))
	if err != nil {
		return nil, err
	}
	offset := int32(target) - int32(pc)
	return encodeInstruction(opcode, 0, SIZE_Q, 0, rs, rt, uint32(offset)), nil
}

// asmJmp handles: jmp (rs) or jmp disp(rs)
func (a *IE64Assembler) asmJmp(operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("jmp requires 1 operand (register-indirect)")
	}
	disp, rs, err := a.parseDispReg(strings.TrimSpace(operands[0]))
	if err != nil {
		return nil, fmt.Errorf("jmp requires register-indirect operand: %v", err)
	}
	return encodeInstruction(OP64_JMP, 0, 0, 0, rs, 0, uint32(disp)), nil
}

// asmJsr handles: jsr label (PC-relative) or jsr (rs) / jsr disp(rs) (register-indirect)
func (a *IE64Assembler) asmJsr(operands []string, pc uint32) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("jsr requires 1 operand")
	}
	op := strings.TrimSpace(operands[0])

	// Try register-indirect form first
	disp, rs, err := a.parseDispReg(op)
	if err == nil {
		return encodeInstruction(OP64_JSR_IND, 0, 0, 0, rs, 0, uint32(disp)), nil
	}

	// Fall through to PC-relative label form
	target, err := a.resolveLabel(op)
	if err != nil {
		return nil, err
	}
	offset := int32(target) - int32(pc)
	return encodeInstruction(OP64_JSR, 0, SIZE_Q, 0, 0, 0, uint32(offset)), nil
}

// asmPushPop handles: push rs / pop rd
func (a *IE64Assembler) asmPushPop(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("push/pop requires 1 operand (register)")
	}
	reg, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}
	if opcode == OP64_PUSH {
		// push uses rs field (CPU reads cpu.regs[rs])
		return encodeInstruction(opcode, 0, SIZE_Q, 0, reg, 0, 0), nil
	}
	// pop uses rd field (CPU writes cpu.setReg(rd, ...))
	return encodeInstruction(opcode, reg, SIZE_Q, 0, 0, 0, 0), nil
}

// asmWait handles: wait #cycles
func (a *IE64Assembler) asmWait(operands []string) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("wait requires 1 operand (#cycles)")
	}
	src := strings.TrimSpace(operands[0])
	if !strings.HasPrefix(src, "#") {
		return nil, fmt.Errorf("wait requires immediate operand (#cycles)")
	}
	immStr := strings.TrimSpace(src[1:])
	val, err := a.evalExpr(immStr)
	if err != nil {
		return nil, fmt.Errorf("wait cycles: %v", err)
	}
	return encodeInstruction(OP64_WAIT, 0, 0, 1, 0, 0, uint32(val)), nil
}

// resolveLabel resolves a label or expression to a target address.
func (a *IE64Assembler) resolveLabel(name string) (uint32, error) {
	name = strings.TrimSpace(name)

	// Handle local labels
	resolved := name
	if strings.HasPrefix(name, ".") && a.lastGlobalLabel != "" {
		resolved = a.lastGlobalLabel + name
	}

	// Check labels
	if addr, ok := a.labels[resolved]; ok {
		return addr, nil
	}
	// Check equates
	if val, ok := a.equates[name]; ok {
		return uint32(val), nil
	}
	// Check sets
	if val, ok := a.sets[name]; ok {
		return uint32(val), nil
	}

	// Try evaluating as expression
	val, err := a.evalExpr(name)
	if err != nil {
		if a.pass <= 1 {
			return 0, nil
		}
		return 0, fmt.Errorf("undefined label or symbol: %s", name)
	}
	return uint32(val), nil
}

// ---------------------------------------------------------------------
// main
// ---------------------------------------------------------------------

func main() {
	listMode := false
	var inputFile string

	args := os.Args[1:]
	for _, arg := range args {
		if arg == "-list" {
			listMode = true
		} else if strings.HasPrefix(arg, "-") {
			fmt.Fprintf(os.Stderr, "Unknown option: %s\n", arg)
			fmt.Fprintf(os.Stderr, "Usage: ie64asm [-list] input.asm\n")
			os.Exit(1)
		} else {
			inputFile = arg
		}
	}

	if inputFile == "" {
		fmt.Fprintf(os.Stderr, "Usage: ie64asm [-list] input.asm\n")
		os.Exit(1)
	}

	source, err := os.ReadFile(inputFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading input file: %v\n", err)
		os.Exit(1)
	}

	asm := NewIE64Assembler()
	asm.basePath = filepath.Dir(inputFile)
	asm.SetListingMode(listMode)

	binary, err := asm.Assemble(string(source))
	if err != nil {
		fmt.Fprintf(os.Stderr, "Assembly error: %v\n", err)
		os.Exit(1)
	}

	// Print warnings to stderr
	for _, w := range asm.GetWarnings() {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", w)
	}

	// Write output
	outFile := strings.TrimSuffix(inputFile, filepath.Ext(inputFile)) + ".ie64"
	if err := os.WriteFile(outFile, binary, 0644); err != nil {
		fmt.Fprintf(os.Stderr, "Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully assembled to %s (%d bytes)\n", outFile, len(binary))

	// Print listing if enabled
	if listMode {
		fmt.Println("\n--- Listing ---")
		for _, line := range asm.GetListing() {
			fmt.Println(line)
		}
	}
}

// ---------------------------------------------------------------------
// FPU Assembler Helpers
// ---------------------------------------------------------------------

func (a *IE64Assembler) asmFP2(opcode byte, operands []string, isSrcFP, isDstFP bool) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU instruction requires 2 operands")
	}

	var dst, src byte
	var ok bool

	if isDstFP {
		dst, ok = parseFPRegister(operands[0])
	} else {
		dst, ok = parseRegister(operands[0])
	}
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}

	if isSrcFP {
		src, ok = parseFPRegister(operands[1])
	} else {
		src, ok = parseRegister(operands[1])
	}
	if !ok {
		return nil, fmt.Errorf("invalid source register: %s", operands[1])
	}

	return encodeInstruction(opcode, dst, SIZE_L, 0, src, 0, 0), nil
}

func (a *IE64Assembler) asmFP3(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("FPU instruction requires 3 operands")
	}

	rd, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}

	rs, ok := parseFPRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register 1: %s", operands[1])
	}

	rt, ok := parseFPRegister(operands[2])
	if !ok {
		return nil, fmt.Errorf("invalid source register 2: %s", operands[2])
	}

	return encodeInstruction(opcode, rd, SIZE_L, 0, rs, rt, 0), nil
}

func (a *IE64Assembler) asmFP3_Int(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 3 {
		return nil, fmt.Errorf("FPU instruction requires 3 operands")
	}

	rd, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}

	rs, ok := parseFPRegister(operands[1])
	if !ok {
		return nil, fmt.Errorf("invalid source register 1: %s", operands[1])
	}

	rt, ok := parseFPRegister(operands[2])
	if !ok {
		return nil, fmt.Errorf("invalid source register 2: %s", operands[2])
	}

	return encodeInstruction(opcode, rd, SIZE_L, 0, rs, rt, 0), nil
}

func (a *IE64Assembler) asmFP_Mem(opcode byte, operands []string, isLoad bool) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU memory instruction requires 2 operands")
	}

	var freg, mreg byte
	var ok bool
	var memOp string

	if isLoad {
		freg, ok = parseFPRegister(operands[0])
		memOp = operands[1]
	} else {
		freg, ok = parseFPRegister(operands[1])
		memOp = operands[0]
	}

	if !ok {
		return nil, fmt.Errorf("invalid FP register")
	}

	disp, mreg, err := a.parseDispReg(memOp)
	if err != nil {
		return nil, err
	}

	var xbit byte
	if disp != 0 {
		xbit = 1
	}

	// For FPU memory, Rd is always the FP reg, Rs is memory base
	return encodeInstruction(opcode, freg, SIZE_L, xbit, mreg, 0, uint32(disp)), nil
}

func (a *IE64Assembler) asmFP_Imm(opcode byte, operands []string) ([]byte, error) {
	if len(operands) != 2 {
		return nil, fmt.Errorf("FPU instruction requires 2 operands")
	}

	rd, ok := parseFPRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid destination register: %s", operands[0])
	}

	immStr := strings.TrimSpace(operands[1])
	if !strings.HasPrefix(immStr, "#") {
		return nil, fmt.Errorf("expected immediate value (starting with #)")
	}

	val, err := a.evalExpr(immStr[1:])
	if err != nil {
		return nil, err
	}

	return encodeInstruction(opcode, rd, SIZE_L, 1, 0, 0, uint32(val)), nil
}

func (a *IE64Assembler) asmFP_Status(opcode byte, operands []string, isWrite bool) ([]byte, error) {
	if len(operands) != 1 {
		return nil, fmt.Errorf("FPU status instruction requires 1 operand")
	}

	reg, ok := parseRegister(operands[0])
	if !ok {
		return nil, fmt.Errorf("invalid register: %s", operands[0])
	}

	if isWrite {
		return encodeInstruction(opcode, 0, SIZE_L, 0, reg, 0, 0), nil
	}
	return encodeInstruction(opcode, reg, SIZE_L, 0, 0, 0, 0), nil
}

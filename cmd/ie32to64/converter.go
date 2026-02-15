package main

import (
	"fmt"
	"os"
	"strings"
)

// LineType classifies a line of IE32 assembly.
type LineType int

const (
	LineEmpty LineType = iota
	LineLabel
	LineDirective
	LineInstruction
)

// OperandType classifies an IE32 operand.
type OperandType int

const (
	OpImmediate   OperandType = iota // #val
	OpDirect                         // @addr
	OpRegIndirect                    // [reg] or [reg+off]
	OpRegister                       // bare register name
	OpBare                           // bare number, equate, or label
)

// Converter translates IE32 assembly to IE64 assembly.
type Converter struct {
	regMap     map[string]string
	sizeSuffix string
	noHeader   bool
	errors     int
}

// NewConverter creates a Converter with default settings.
func NewConverter() *Converter {
	c := &Converter{
		regMap: map[string]string{
			"A": "r1", "X": "r2", "Y": "r3", "Z": "r4",
			"B": "r5", "C": "r6", "D": "r7", "E": "r8",
			"F": "r9", "G": "r10", "H": "r11", "S": "r12",
			"T": "r13", "U": "r14", "V": "r15", "W": "r16",
		},
		sizeSuffix: ".l",
	}
	return c
}

// MapRegister maps an IE32 register name to IE64 register name.
func (c *Converter) MapRegister(reg string) (string, error) {
	r, ok := c.regMap[strings.ToUpper(reg)]
	if !ok {
		return "", fmt.Errorf("unknown IE32 register %q", reg)
	}
	return r, nil
}

// isRegister checks if a string is a known IE32 register.
func (c *Converter) isRegister(s string) bool {
	_, ok := c.regMap[strings.ToUpper(s)]
	return ok
}

// SplitComment splits a line into code and comment parts.
// The comment does NOT include the leading ";".
func SplitComment(line string) (code, comment string) {
	inQuote := false
	quoteChar := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}
		if ch == '"' || ch == '\'' {
			inQuote = true
			quoteChar = ch
			continue
		}
		if ch == ';' {
			code = strings.TrimRight(line[:i], " \t")
			comment = strings.TrimLeft(line[i+1:], " ")
			return
		}
	}
	return line, ""
}

// ClassifyLine classifies the code portion of a line (after comment removal).
func ClassifyLine(code string) LineType {
	trimmed := strings.TrimSpace(code)
	if trimmed == "" {
		return LineEmpty
	}
	if strings.HasSuffix(trimmed, ":") {
		return LineLabel
	}
	tokens := strings.Fields(trimmed)
	// Check if first token ends with colon (label with trailing text)
	if strings.HasSuffix(tokens[0], ":") {
		return LineLabel
	}
	if strings.HasPrefix(tokens[0], ".") {
		return LineDirective
	}
	return LineInstruction
}

// ClassifyOperand classifies an IE32 operand string.
func ClassifyOperand(op string) OperandType {
	op = strings.TrimSpace(op)
	if strings.HasPrefix(op, "#") {
		return OpImmediate
	}
	if strings.HasPrefix(op, "@") {
		return OpDirect
	}
	if strings.HasPrefix(op, "[") {
		return OpRegIndirect
	}
	return OpBare // Could be register or bare - caller disambiguates
}

// ClassifyOperandWithReg classifies an operand, distinguishing registers from bare values.
func (c *Converter) ClassifyOperandWithReg(op string) OperandType {
	t := ClassifyOperand(op)
	if t == OpBare && c.isRegister(strings.TrimSpace(op)) {
		return OpRegister
	}
	return t
}

// parseRegIndirect parses "[REG]" or "[REG+offset]" into (mappedReg, offset).
func (c *Converter) parseRegIndirect(op string) (reg string, offset string, err error) {
	inner := strings.TrimSpace(op)
	inner = strings.TrimPrefix(inner, "[")
	inner = strings.TrimSuffix(inner, "]")
	inner = strings.TrimSpace(inner)

	if idx := strings.Index(inner, "+"); idx >= 0 {
		regPart := strings.TrimSpace(inner[:idx])
		offPart := strings.TrimSpace(inner[idx+1:])
		mapped, e := c.MapRegister(regPart)
		if e != nil {
			return "", "", e
		}
		return mapped, offPart, nil
	}

	mapped, e := c.MapRegister(inner)
	if e != nil {
		return "", "", e
	}
	return mapped, "", nil
}

// ConvertDirective converts an IE32 directive to IE64.
func (c *Converter) ConvertDirective(line string) string {
	trimmed := strings.TrimSpace(line)
	lower := strings.ToLower(trimmed)

	switch {
	case strings.HasPrefix(lower, ".org "):
		return "org " + strings.TrimSpace(trimmed[4:])

	case strings.HasPrefix(lower, ".equ "):
		rest := strings.TrimSpace(trimmed[4:])
		// .equ NAME value → NAME equ value
		fields := strings.Fields(rest)
		if len(fields) >= 2 {
			name := fields[0]
			value := strings.TrimSpace(rest[len(name):])
			value = strings.TrimSpace(value)
			return name + " equ " + value
		}
		return rest + " equ"

	case strings.HasPrefix(lower, ".word "):
		return "dc.l " + strings.TrimSpace(trimmed[5:])

	case strings.HasPrefix(lower, ".byte "):
		return "dc.b " + strings.TrimSpace(trimmed[5:])

	case strings.HasPrefix(lower, ".space "):
		return "ds.b " + strings.TrimSpace(trimmed[6:])

	case strings.HasPrefix(lower, ".ascii "):
		return "dc.b " + strings.TrimSpace(trimmed[6:])

	case strings.HasPrefix(lower, ".incbin "):
		return "incbin " + strings.TrimSpace(trimmed[7:])

	case strings.HasPrefix(lower, ".include "):
		filename := strings.TrimSpace(trimmed[8:])
		if filename == `"ie32.inc"` {
			filename = `"ie64.inc"`
		}
		return "include " + filename

	default:
		return "; WARNING: unknown directive: " + trimmed
	}
}

// ConvertLine converts a single line of IE32 assembly to one or more IE64 lines.
func (c *Converter) ConvertLine(rawLine string) []string {
	trimmed := strings.TrimSpace(rawLine)

	// Empty line
	if trimmed == "" {
		return []string{""}
	}

	// Comment-only line - preserve verbatim
	if strings.HasPrefix(trimmed, ";") {
		return []string{rawLine}
	}

	// Capture leading whitespace
	indent := ""
	for i := 0; i < len(rawLine); i++ {
		if rawLine[i] == ' ' || rawLine[i] == '\t' {
			indent += string(rawLine[i])
		} else {
			break
		}
	}

	// Split code and comment
	code, comment := SplitComment(trimmed)
	commentSuffix := ""
	if comment != "" {
		commentSuffix = "    ; " + comment
	}

	lt := ClassifyLine(code)

	switch lt {
	case LineEmpty:
		return []string{""}

	case LineLabel:
		return []string{indent + code + commentSuffix}

	case LineDirective:
		converted := c.ConvertDirective(code)
		return []string{indent + converted + commentSuffix}

	case LineInstruction:
		lines := c.convertInstruction(code, indent)
		// Attach comment to the first output line
		if comment != "" && len(lines) > 0 {
			lines[0] = lines[0] + commentSuffix
		}
		return lines
	}

	return []string{rawLine}
}

// convertInstruction converts an IE32 instruction to one or more IE64 instruction lines.
func (c *Converter) convertInstruction(code string, indent string) []string {
	fields := strings.Fields(code)
	if len(fields) == 0 {
		return []string{indent}
	}

	mnemonic := strings.ToUpper(fields[0])
	rest := strings.TrimSpace(code[len(fields[0]):])

	sz := c.sizeSuffix

	// --- Zero-operand instructions ---
	switch mnemonic {
	case "NOP":
		return []string{indent + "nop"}
	case "HALT":
		return []string{indent + "halt"}
	case "RTS":
		return []string{indent + "rts"}
	case "SEI":
		return []string{indent + "sei"}
	case "CLI":
		return []string{indent + "cli"}
	case "RTI":
		return []string{indent + "rti"}
	}

	// --- PUSH/POP ---
	if mnemonic == "PUSH" || mnemonic == "POP" {
		reg, err := c.MapRegister(strings.TrimSpace(rest))
		if err != nil {
			return c.emitError(indent, code, "unknown register in "+mnemonic)
		}
		return []string{indent + strings.ToLower(mnemonic) + " " + reg}
	}

	// --- JMP / JSR ---
	if mnemonic == "JMP" {
		target := strings.TrimSpace(rest)
		return []string{indent + "bra " + target}
	}
	if mnemonic == "JSR" {
		target := strings.TrimSpace(rest)
		return []string{indent + "jsr " + target}
	}

	// --- Conditional branches: JNZ, JZ, JGT, JGE, JLT, JLE ---
	branchMap := map[string]string{
		"JNZ": "bnez", "JZ": "beqz",
		"JGT": "bgtz", "JGE": "bgez",
		"JLT": "bltz", "JLE": "blez",
	}
	if ie64branch, ok := branchMap[mnemonic]; ok {
		parts := splitOperands(rest)
		if len(parts) != 2 {
			return c.emitError(indent, code, "expected 2 operands for "+mnemonic)
		}
		reg, err := c.MapRegister(parts[0])
		if err != nil {
			return c.emitError(indent, code, err.Error())
		}
		return []string{indent + ie64branch + " " + reg + ", " + parts[1]}
	}

	// --- Register-specific loads: LDA, LDB, ..., LDW ---
	if strings.HasPrefix(mnemonic, "LD") && len(mnemonic) == 3 {
		regChar := string(mnemonic[2])
		destReg, err := c.MapRegister(regChar)
		if err != nil {
			return c.emitError(indent, code, "unknown register in "+mnemonic)
		}
		return c.convertLoad(destReg, strings.TrimSpace(rest), indent, sz)
	}

	// --- Register-specific stores: STA, STB, ..., STW ---
	if strings.HasPrefix(mnemonic, "ST") && len(mnemonic) == 3 {
		regChar := string(mnemonic[2])
		srcReg, err := c.MapRegister(regChar)
		if err != nil {
			return c.emitError(indent, code, "unknown register in "+mnemonic)
		}
		return c.convertStore(srcReg, strings.TrimSpace(rest), indent, sz)
	}

	// --- Generic LOAD / STORE ---
	if mnemonic == "LOAD" {
		parts := splitOperands(rest)
		if len(parts) != 2 {
			return c.emitError(indent, code, "LOAD requires 2 operands")
		}
		destReg, err := c.MapRegister(parts[0])
		if err != nil {
			return c.emitError(indent, code, err.Error())
		}
		return c.convertGenericLoad(destReg, parts[1], indent, sz)
	}
	if mnemonic == "STORE" {
		parts := splitOperands(rest)
		if len(parts) != 2 {
			return c.emitError(indent, code, "STORE requires 2 operands")
		}
		srcReg, err := c.MapRegister(parts[0])
		if err != nil {
			return c.emitError(indent, code, err.Error())
		}
		return c.convertStore(srcReg, parts[1], indent, sz)
	}

	// --- ALU: ADD, SUB, MUL, DIV, MOD, AND, OR, XOR, SHL, SHR ---
	aluMap := map[string]string{
		"ADD": "add", "SUB": "sub",
		"MUL": "mulu", "DIV": "divu", "MOD": "mod",
		"AND": "and", "OR": "or", "XOR": "eor",
		"SHL": "lsl", "SHR": "lsr",
	}
	if ie64op, ok := aluMap[mnemonic]; ok {
		parts := splitOperands(rest)
		if len(parts) != 2 {
			return c.emitError(indent, code, mnemonic+" requires 2 operands")
		}
		destReg, err := c.MapRegister(parts[0])
		if err != nil {
			return c.emitError(indent, code, err.Error())
		}
		return c.convertALU(ie64op, destReg, parts[1], indent, sz)
	}

	// --- NOT (unary) ---
	if mnemonic == "NOT" {
		reg, err := c.MapRegister(strings.TrimSpace(rest))
		if err != nil {
			return c.emitError(indent, code, err.Error())
		}
		return []string{indent + "not" + sz + " " + reg + ", " + reg}
	}

	// --- INC / DEC ---
	if mnemonic == "INC" || mnemonic == "DEC" {
		op := strings.TrimSpace(rest)
		ie64op := "add"
		if mnemonic == "DEC" {
			ie64op = "sub"
		}
		return c.convertIncDec(ie64op, op, indent, sz)
	}

	// --- WAIT ---
	if mnemonic == "WAIT" {
		return c.convertWait(rest, indent)
	}

	// Unknown mnemonic → hard error
	return c.emitError(indent, code, fmt.Sprintf("unknown IE32 mnemonic '%s'", mnemonic))
}

// convertLoad converts a load operation (from register-specific LDA/LDB/etc.)
func (c *Converter) convertLoad(destReg, operand, indent, sz string) []string {
	opType := c.ClassifyOperandWithReg(operand)

	switch opType {
	case OpImmediate:
		return []string{indent + "move" + sz + " " + destReg + ", " + operand}

	case OpDirect:
		addr := strings.TrimSpace(operand[1:]) // strip @
		return []string{
			indent + "la r17, " + addr,
			indent + "load" + sz + " " + destReg + ", (r17)",
		}

	case OpRegIndirect:
		reg, offset, err := c.parseRegIndirect(operand)
		if err != nil {
			return c.emitError(indent, "LD? "+operand, err.Error())
		}
		if offset != "" {
			return []string{indent + "load" + sz + " " + destReg + ", " + offset + "(" + reg + ")"}
		}
		return []string{indent + "load" + sz + " " + destReg + ", (" + reg + ")"}

	case OpRegister:
		srcReg, _ := c.MapRegister(operand)
		return []string{indent + "move" + sz + " " + destReg + ", " + srcReg}

	default: // OpBare - immediate per IE32 semantics
		return []string{indent + "move" + sz + " " + destReg + ", #" + operand}
	}
}

// convertGenericLoad converts LOAD reg, operand with full operand disambiguation.
func (c *Converter) convertGenericLoad(destReg, operand, indent, sz string) []string {
	opType := c.ClassifyOperandWithReg(operand)

	switch opType {
	case OpImmediate:
		return []string{indent + "move" + sz + " " + destReg + ", " + operand}

	case OpDirect:
		addr := strings.TrimSpace(operand[1:])
		return []string{
			indent + "la r17, " + addr,
			indent + "load" + sz + " " + destReg + ", (r17)",
		}

	case OpRegIndirect:
		reg, offset, err := c.parseRegIndirect(operand)
		if err != nil {
			return c.emitError(indent, "LOAD "+operand, err.Error())
		}
		if offset != "" {
			return []string{indent + "load" + sz + " " + destReg + ", " + offset + "(" + reg + ")"}
		}
		return []string{indent + "load" + sz + " " + destReg + ", (" + reg + ")"}

	case OpRegister:
		srcReg, _ := c.MapRegister(operand)
		return []string{indent + "move" + sz + " " + destReg + ", " + srcReg}

	default: // OpBare - immediate per IE32 semantics
		return []string{indent + "move" + sz + " " + destReg + ", #" + operand}
	}
}

// convertStore converts a store operation.
func (c *Converter) convertStore(srcReg, operand, indent, sz string) []string {
	opType := ClassifyOperand(operand)

	switch opType {
	case OpDirect:
		addr := strings.TrimSpace(operand[1:])
		return []string{
			indent + "la r17, " + addr,
			indent + "store" + sz + " " + srcReg + ", (r17)",
		}

	case OpRegIndirect:
		reg, offset, err := c.parseRegIndirect(operand)
		if err != nil {
			return c.emitError(indent, "ST? "+operand, err.Error())
		}
		if offset != "" {
			return []string{indent + "store" + sz + " " + srcReg + ", " + offset + "(" + reg + ")"}
		}
		return []string{indent + "store" + sz + " " + srcReg + ", (" + reg + ")"}

	default: // OpBare and OpImmediate - STORE always writes to memory
		addr := operand
		if strings.HasPrefix(addr, "#") {
			addr = addr[1:]
		}
		return []string{
			indent + "la r17, " + addr,
			indent + "store" + sz + " " + srcReg + ", (r17)",
		}
	}
}

// convertALU converts a 2-operand ALU instruction to 3-operand IE64.
func (c *Converter) convertALU(ie64op, destReg, operand, indent, sz string) []string {
	opType := c.ClassifyOperandWithReg(operand)

	switch opType {
	case OpImmediate:
		return []string{indent + ie64op + sz + " " + destReg + ", " + destReg + ", " + operand}

	case OpRegister:
		srcReg, _ := c.MapRegister(operand)
		return []string{indent + ie64op + sz + " " + destReg + ", " + destReg + ", " + srcReg}

	case OpDirect:
		addr := strings.TrimSpace(operand[1:])
		return []string{
			indent + "la r17, " + addr,
			indent + "load" + sz + " r17, (r17)",
			indent + ie64op + sz + " " + destReg + ", " + destReg + ", r17",
		}

	case OpRegIndirect:
		reg, offset, err := c.parseRegIndirect(operand)
		if err != nil {
			return c.emitError(indent, ie64op+" "+operand, err.Error())
		}
		loadLine := indent + "load" + sz + " r17, "
		if offset != "" {
			loadLine += offset + "(" + reg + ")"
		} else {
			loadLine += "(" + reg + ")"
		}
		return []string{
			loadLine,
			indent + ie64op + sz + " " + destReg + ", " + destReg + ", r17",
		}

	default: // OpBare - immediate per IE32 semantics
		return []string{indent + ie64op + sz + " " + destReg + ", " + destReg + ", #" + operand}
	}
}

// convertIncDec converts INC/DEC with various addressing modes.
func (c *Converter) convertIncDec(ie64op, operand, indent, sz string) []string {
	opType := c.ClassifyOperandWithReg(operand)

	switch opType {
	case OpRegister:
		reg, _ := c.MapRegister(operand)
		return []string{indent + ie64op + sz + " " + reg + ", " + reg + ", #1"}

	case OpDirect:
		addr := strings.TrimSpace(operand[1:])
		return []string{
			indent + "la r17, " + addr,
			indent + "load" + sz + " r18, (r17)",
			indent + ie64op + sz + " r18, r18, #1",
			indent + "store" + sz + " r18, (r17)",
		}

	case OpRegIndirect:
		reg, offset, err := c.parseRegIndirect(operand)
		if err != nil {
			return c.emitError(indent, ie64op+" "+operand, err.Error())
		}
		memRef := "(" + reg + ")"
		if offset != "" {
			memRef = offset + "(" + reg + ")"
		}
		return []string{
			indent + "load" + sz + " r18, " + memRef,
			indent + ie64op + sz + " r18, r18, #1",
			indent + "store" + sz + " r18, " + memRef,
		}

	default:
		return c.emitError(indent, ie64op+" "+operand, "unsupported operand for INC/DEC")
	}
}

// convertWait handles the WAIT instruction with various operand types.
func (c *Converter) convertWait(operand, indent string) []string {
	op := strings.TrimSpace(operand)
	opType := c.ClassifyOperandWithReg(op)

	switch opType {
	case OpImmediate:
		return []string{indent + "wait " + op}
	case OpRegister:
		return c.emitError(indent, "WAIT "+op, "IE64 wait only accepts immediate operand; register operand cannot be converted")
	case OpDirect:
		return c.emitError(indent, "WAIT "+op, "IE64 wait only accepts immediate operand; direct memory operand cannot be converted")
	case OpRegIndirect:
		return c.emitError(indent, "WAIT "+op, "IE64 wait only accepts immediate operand; register-indirect operand cannot be converted")
	default: // OpBare - immediate per IE32 semantics, prepend #
		return []string{indent + "wait #" + op}
	}
}

// emitError emits an error comment and the original line commented out.
func (c *Converter) emitError(indent, code, msg string) []string {
	c.errors++
	return []string{
		indent + "; ERROR: " + msg,
		indent + "; " + code,
	}
}

// ConvertFile converts an entire IE32 assembly file content string to IE64.
func (c *Converter) ConvertFile(input string) string {
	lines := strings.Split(input, "\n")
	var output []string

	if !c.noHeader {
		output = append(output, "; Converted from IE32 by ie32to64")
		output = append(output, "")
	}

	for _, line := range lines {
		converted := c.ConvertLine(line)
		output = append(output, converted...)
	}

	return strings.Join(output, "\n")
}

// ConvertFileFromPath reads a file and converts it.
func (c *Converter) ConvertFileFromPath(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return c.ConvertFile(string(data)), nil
}

// splitOperands splits "A, #10" into ["A", "#10"], respecting expressions.
func splitOperands(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}

	// For brackets, find closing bracket first
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '[':
			depth++
		case ']':
			depth--
		case ',':
			if depth == 0 {
				left := strings.TrimSpace(s[:i])
				right := strings.TrimSpace(s[i+1:])
				return []string{left, right}
			}
		}
	}

	// No comma found - single operand
	return []string{s}
}

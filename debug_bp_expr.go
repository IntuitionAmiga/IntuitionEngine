package main

import (
	"fmt"
	"strings"
	"unicode"
)

type bpExprKind int

const (
	bpExprCompare bpExprKind = iota
	bpExprAnd
	bpExprOr
)

type bpValueKind int

const (
	bpValueConst bpValueKind = iota
	bpValueRegister
	bpValueMemory
	bpValueHitCount
)

type BreakpointExpr struct {
	Kind  bpExprKind
	Left  *BreakpointExpr
	Right *BreakpointExpr

	ValueLeft  bpValue
	ValueRight bpValue
	Op         ConditionOp
}

type bpValue struct {
	Kind    bpValueKind
	Name    string
	Addr    uint64
	Width   uint8
	Literal uint64
}

type bpExprParser struct {
	tokens []string
	pos    int
}

func ParseBreakpointExpr(text string) (*BreakpointExpr, error) {
	text = strings.TrimSpace(text)
	text = strings.TrimPrefix(text, "if ")
	p := &bpExprParser{tokens: tokenizeBPExpr(text)}
	if len(p.tokens) == 0 {
		return nil, fmt.Errorf("empty condition")
	}
	expr, err := p.parseOr()
	if err != nil {
		return nil, err
	}
	if p.peek() != "" {
		return nil, fmt.Errorf("unexpected token: %s", p.peek())
	}
	return expr, nil
}

func tokenizeBPExpr(text string) []string {
	var out []string
	for i := 0; i < len(text); {
		r := rune(text[i])
		if unicode.IsSpace(r) {
			i++
			continue
		}
		if i+1 < len(text) {
			two := text[i : i+2]
			switch two {
			case "==", "!=", "<=", ">=", "&&", "||":
				out = append(out, two)
				i += 2
				continue
			}
		}
		switch text[i] {
		case '(', ')', '<', '>', ',':
			out = append(out, text[i:i+1])
			i++
		default:
			start := i
			for i < len(text) {
				ch := text[i]
				if unicode.IsSpace(rune(ch)) || strings.ContainsRune("(),<>", rune(ch)) {
					break
				}
				if i+1 < len(text) {
					two := text[i : i+2]
					if two == "==" || two == "!=" || two == "<=" || two == ">=" || two == "&&" || two == "||" {
						break
					}
				}
				i++
			}
			out = append(out, text[start:i])
		}
	}
	return out
}

func (p *bpExprParser) parseOr() (*BreakpointExpr, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.match("||") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = &BreakpointExpr{Kind: bpExprOr, Left: left, Right: right}
	}
	return left, nil
}

func (p *bpExprParser) parseAnd() (*BreakpointExpr, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	for p.match("&&") {
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		left = &BreakpointExpr{Kind: bpExprAnd, Left: left, Right: right}
	}
	return left, nil
}

func (p *bpExprParser) parsePrimary() (*BreakpointExpr, error) {
	if p.match("(") {
		expr, err := p.parseOr()
		if err != nil {
			return nil, err
		}
		if !p.match(")") {
			return nil, fmt.Errorf("missing closing parenthesis")
		}
		return expr, nil
	}
	return p.parseCompare()
}

func (p *bpExprParser) parseCompare() (*BreakpointExpr, error) {
	left, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	opTok := p.next()
	op, ok := parseConditionOp(opTok)
	if !ok {
		return nil, fmt.Errorf("expected comparison operator, got %s", opTok)
	}
	right, err := p.parseValue()
	if err != nil {
		return nil, err
	}
	return &BreakpointExpr{Kind: bpExprCompare, ValueLeft: left, ValueRight: right, Op: op}, nil
}

func (p *bpExprParser) parseValue() (bpValue, error) {
	tok := p.next()
	if tok == "" {
		return bpValue{}, fmt.Errorf("expected value")
	}
	lower := strings.ToLower(tok)
	if lower == "hitcount" {
		return bpValue{Kind: bpValueHitCount}, nil
	}
	if lower == "b" || lower == "w" || lower == "l" || lower == "q" {
		if !p.match("(") {
			return bpValue{}, fmt.Errorf("expected ( after %s", tok)
		}
		addrTok := p.next()
		addr, ok := ParseAddress(addrTok)
		if !ok {
			return bpValue{}, fmt.Errorf("invalid memory address: %s", addrTok)
		}
		if !p.match(")") {
			return bpValue{}, fmt.Errorf("missing ) after memory address")
		}
		width := uint8(1)
		switch lower {
		case "w":
			width = 2
		case "l":
			width = 4
		case "q":
			width = 8
		}
		return bpValue{Kind: bpValueMemory, Addr: addr, Width: width}, nil
	}
	if strings.HasPrefix(tok, "[") && strings.Contains(tok, "]") {
		cond, err := ParseCondition(tok + "==0")
		if err == nil && cond.Source == CondSourceMemory {
			return bpValue{Kind: bpValueMemory, Addr: cond.MemAddr, Width: cond.Width}, nil
		}
	}
	if shouldParseBPExprTokenAsNumber(tok) {
		v, ok := ParseAddress(tok)
		if !ok {
			return bpValue{}, fmt.Errorf("invalid numeric value: %s", tok)
		}
		return bpValue{Kind: bpValueConst, Literal: v}, nil
	}
	return bpValue{Kind: bpValueRegister, Name: strings.ToUpper(tok)}, nil
}

func shouldParseBPExprTokenAsNumber(tok string) bool {
	if tok == "" {
		return false
	}
	if strings.HasPrefix(tok, "$") || strings.HasPrefix(tok, "#") || strings.HasPrefix(tok, "0x") || strings.HasPrefix(tok, "0X") {
		return true
	}
	return tok[0] >= '0' && tok[0] <= '9'
}

func parseConditionOp(tok string) (ConditionOp, bool) {
	switch tok {
	case "==":
		return CondOpEqual, true
	case "!=":
		return CondOpNotEqual, true
	case "<":
		return CondOpLess, true
	case ">":
		return CondOpGreater, true
	case "<=":
		return CondOpLessEqual, true
	case ">=":
		return CondOpGreaterEqual, true
	default:
		return 0, false
	}
}

func (p *bpExprParser) peek() string {
	if p.pos >= len(p.tokens) {
		return ""
	}
	return p.tokens[p.pos]
}

func (p *bpExprParser) next() string {
	tok := p.peek()
	if tok != "" {
		p.pos++
	}
	return tok
}

func (p *bpExprParser) match(tok string) bool {
	if p.peek() != tok {
		return false
	}
	p.pos++
	return true
}

func evalBreakpointExpr(expr *BreakpointExpr, cpu DebuggableCPU, hitCount uint64) bool {
	if expr == nil {
		return true
	}
	switch expr.Kind {
	case bpExprAnd:
		return evalBreakpointExpr(expr.Left, cpu, hitCount) && evalBreakpointExpr(expr.Right, cpu, hitCount)
	case bpExprOr:
		return evalBreakpointExpr(expr.Left, cpu, hitCount) || evalBreakpointExpr(expr.Right, cpu, hitCount)
	case bpExprCompare:
		left, ok := evalBPValue(expr.ValueLeft, cpu, hitCount)
		if !ok {
			return false
		}
		right, ok := evalBPValue(expr.ValueRight, cpu, hitCount)
		if !ok {
			return false
		}
		return compareValues(left, expr.Op, right)
	default:
		return false
	}
}

func evalBPValue(v bpValue, cpu DebuggableCPU, hitCount uint64) (uint64, bool) {
	switch v.Kind {
	case bpValueConst:
		return v.Literal, true
	case bpValueHitCount:
		return hitCount, true
	case bpValueRegister:
		if cpu == nil {
			return 0, false
		}
		return cpu.GetRegister(v.Name)
	case bpValueMemory:
		if cpu == nil {
			return 0, false
		}
		width := v.Width
		if width == 0 {
			width = 1
		}
		data := cpu.ReadMemory(v.Addr, int(width))
		if len(data) < int(width) {
			return 0, false
		}
		return bytesToConditionValue(data, conditionLittleEndian(cpu)), true
	default:
		return 0, false
	}
}

func formatBreakpointExpr(expr *BreakpointExpr) string {
	if expr == nil {
		return ""
	}
	switch expr.Kind {
	case bpExprAnd:
		return "(" + formatBreakpointExpr(expr.Left) + " && " + formatBreakpointExpr(expr.Right) + ")"
	case bpExprOr:
		return "(" + formatBreakpointExpr(expr.Left) + " || " + formatBreakpointExpr(expr.Right) + ")"
	case bpExprCompare:
		return formatBPValue(expr.ValueLeft) + conditionOpString(expr.Op) + formatBPValue(expr.ValueRight)
	default:
		return ""
	}
}

func formatBPValue(v bpValue) string {
	switch v.Kind {
	case bpValueConst:
		return fmt.Sprintf("$%X", v.Literal)
	case bpValueHitCount:
		return "hitcount"
	case bpValueRegister:
		return v.Name
	case bpValueMemory:
		switch v.Width {
		case 2:
			return fmt.Sprintf("w($%X)", v.Addr)
		case 4:
			return fmt.Sprintf("l($%X)", v.Addr)
		case 8:
			return fmt.Sprintf("q($%X)", v.Addr)
		default:
			return fmt.Sprintf("b($%X)", v.Addr)
		}
	default:
		return ""
	}
}

package main

import (
	"fmt"
	"strings"
)

// AddrMode is the m68k effective-addressing mode of a single operand.
type AddrMode int

const (
	AMUnknown    AddrMode = iota
	AMDataReg             // Dn
	AMAddrReg             // An
	AMIndirect            // (An)
	AMPostInc             // (An)+
	AMPreDec              // -(An)
	AMDispAn              // (d16,An) or d16(An)
	AMIndexAn             // (d8,An,Xn.size*scale) or d8(An,Xn.size*scale)
	AMAbsW                // (xxx).w
	AMAbsL                // (xxx).l or bare label/symbol
	AMDispPC              // (d16,PC) or d16(PC)
	AMIndexPC             // (d8,PC,Xn.size*scale) or d8(PC,Xn.size*scale)
	AMImmediate           // #imm
	AMRegList             // d0-d3/a0/a4-a6 (for MOVEM)
	AMCCR                 // ccr
	AMSR                  // sr
	AMUSP                 // usp
)

func (a AddrMode) String() string {
	switch a {
	case AMDataReg:
		return "Dn"
	case AMAddrReg:
		return "An"
	case AMIndirect:
		return "(An)"
	case AMPostInc:
		return "(An)+"
	case AMPreDec:
		return "-(An)"
	case AMDispAn:
		return "(d16,An)"
	case AMIndexAn:
		return "(d8,An,Xn)"
	case AMAbsW:
		return "(xxx).w"
	case AMAbsL:
		return "(xxx).l"
	case AMDispPC:
		return "(d16,PC)"
	case AMIndexPC:
		return "(d8,PC,Xn)"
	case AMImmediate:
		return "#imm"
	case AMRegList:
		return "reglist"
	case AMCCR:
		return "ccr"
	case AMSR:
		return "sr"
	case AMUSP:
		return "usp"
	}
	return "unknown"
}

// IndexSpec describes the index portion of an indexed addressing mode:
// `Xn.size*scale`, e.g. `d1.w*4`.
type IndexSpec struct {
	Reg   MappedReg // index register (data or address)
	Size  string    // "w" or "l"; default "w" if absent
	Scale int       // 1, 2, 4, or 8 (68020+); default 1
}

// Operand is the parsed view of one operand.
type Operand struct {
	Mode  AddrMode
	Reg   MappedReg // primary register (Dn / An / "An" inside indirect modes)
	Index IndexSpec // populated only for AMIndexAn / AMIndexPC
	Disp  string    // displacement expression as raw text ("$1234", "FOO+8", "-12")
	Imm   string    // immediate expression text (no leading '#')
	List  string    // raw register list text for AMRegList
	Raw   string    // original operand text
}

// ParseOperand parses one operand string into an Operand.
//
// It accepts the common vasm/devpac surface forms:
//
//	Dn / An / sp / fp / pc
//	(An)
//	(An)+
//	-(An)
//	(d,An)         d(An)
//	(d,An,Xn[.size][*scale])     d(An,Xn[.size][*scale])
//	(d,PC)         d(PC)
//	(d,PC,Xn[.size][*scale])     d(PC,Xn[.size][*scale])
//	(addr).w / (addr).l
//	#imm
//	bare symbol / number  -> AMAbsL
//	d0-d3/a0/a4-a6        -> AMRegList (only when src has '/' or '-' between reg tokens)
func ParseOperand(s string) (Operand, error) {
	raw := strings.TrimSpace(s)
	op := Operand{Raw: raw}
	if raw == "" {
		return op, fmt.Errorf("empty operand")
	}

	// Immediate: #expr
	if raw[0] == '#' {
		op.Mode = AMImmediate
		op.Imm = strings.TrimSpace(raw[1:])
		if op.Imm == "" {
			return op, fmt.Errorf("empty immediate")
		}
		return op, nil
	}

	// Predecrement: -(An)
	if strings.HasPrefix(raw, "-(") && strings.HasSuffix(raw, ")") {
		inner := strings.TrimSpace(raw[2 : len(raw)-1])
		r, ok := LookupRegister(inner)
		if !ok || (r.Class != RegAddr && r.Class != RegSP) {
			return op, fmt.Errorf("predecrement requires An, got %q", inner)
		}
		op.Mode = AMPreDec
		op.Reg = r
		return op, nil
	}

	// Postincrement: (An)+
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")+") {
		inner := strings.TrimSpace(raw[1 : len(raw)-2])
		r, ok := LookupRegister(inner)
		if !ok || (r.Class != RegAddr && r.Class != RegSP) {
			return op, fmt.Errorf("postincrement requires An, got %q", inner)
		}
		op.Mode = AMPostInc
		op.Reg = r
		return op, nil
	}

	// Absolute with explicit size: (xxx).w / (xxx).l
	if (strings.HasSuffix(raw, ").w") || strings.HasSuffix(raw, ").W") ||
		strings.HasSuffix(raw, ").l") || strings.HasSuffix(raw, ").L")) &&
		strings.HasPrefix(raw, "(") {
		// Find matching ')' before the size suffix.
		body := raw[1 : len(raw)-3]
		op.Disp = strings.TrimSpace(body)
		if strings.HasSuffix(strings.ToLower(raw), ".w") {
			op.Mode = AMAbsW
		} else {
			op.Mode = AMAbsL
		}
		return op, nil
	}

	// Parenthesised mode: (...) — could be (An), (d,An), (d,An,Xn), (d,PC), (d,PC,Xn).
	// Only strip outer parens if they actually enclose the whole string
	// (depth returns to 0 only at the final character).
	if strings.HasPrefix(raw, "(") && strings.HasSuffix(raw, ")") && parenWraps(raw) {
		inner := strings.TrimSpace(raw[1 : len(raw)-1])
		return parseParenInner(op, inner)
	}

	// d(An) / d(An,Xn) / d(PC) / d(PC,Xn) — Motorola legacy form.
	// Locate the LAST top-level "(" whose matching ")" is the final character;
	// the prefix is the displacement expression (which may itself contain
	// balanced parens like "(A*B)+C").
	if i := findTrailingParenStart(raw); i > 0 {
		disp := strings.TrimSpace(raw[:i])
		inner := strings.TrimSpace(raw[i+1 : len(raw)-1])
		op2, err := parseParenInner(op, inner)
		if err != nil {
			// Inner doesn't reference an An/PC — the trailing parens are part
			// of an absolute-address expression like "FOO+(1*2)". Treat the
			// whole string as an absolute label.
			op.Mode = AMAbsL
			op.Disp = raw
			return op, nil
		}
		// parseParenInner sets Mode for (An)/(d,An)/(d,An,Xn)/(d,PC)/(d,PC,Xn).
		// Bare PC always returns AMDispPC, so the AMIndirect arm here is
		// always for an address register.
		if op2.Mode == AMIndirect {
			op2.Mode = AMDispAn
			op2.Disp = disp
		} else {
			// AMDispAn / AMIndexAn / AMDispPC / AMIndexPC — inner had its own
			// displacement; concatenate textually.
			if op2.Disp == "" {
				op2.Disp = disp
			} else {
				op2.Disp = disp + "+" + op2.Disp
			}
		}
		return op2, nil
	}

	// Bare token.
	if r, ok := LookupRegister(raw); ok {
		switch r.Class {
		case RegData:
			op.Mode = AMDataReg
			op.Reg = r
			return op, nil
		case RegAddr, RegSP:
			op.Mode = AMAddrReg
			op.Reg = r
			return op, nil
		case RegCCR:
			op.Mode = AMCCR
			return op, nil
		case RegSR:
			op.Mode = AMSR
			return op, nil
		case RegUSP:
			op.Mode = AMUSP
			return op, nil
		}
	}

	// Register list (for MOVEM): contains '/' or a '-' between register tokens.
	if looksLikeRegList(raw) {
		op.Mode = AMRegList
		op.List = raw
		return op, nil
	}

	// Otherwise treat as absolute long (label / symbol / numeric expression).
	op.Mode = AMAbsL
	op.Disp = raw
	return op, nil
}

// parseParenInner handles the contents inside the outermost parentheses:
//
//	An             -> AMIndirect
//	An,Xn[.s][*c]  -> AMIndexAn with Disp == ""
//	d,An           -> AMDispAn
//	d,An,Xn[.s][*c]-> AMIndexAn
//	PC / PC,Xn / d,PC / d,PC,Xn -> AMDispPC / AMIndexPC analogues
func parseParenInner(op Operand, inner string) (Operand, error) {
	parts := splitTopComma(inner)
	switch len(parts) {
	case 1:
		// (An) or (PC) — bare register only.
		r, ok := LookupRegister(parts[0])
		if !ok {
			return op, fmt.Errorf("expected An/PC inside parens, got %q", parts[0])
		}
		switch r.Class {
		case RegAddr, RegSP:
			op.Mode = AMIndirect
			op.Reg = r
			return op, nil
		case RegPC:
			op.Mode = AMDispPC
			op.Reg = r
			return op, nil
		}
		return op, fmt.Errorf("(%s): not An or PC", parts[0])
	case 2:
		// (d,An) / (d,PC) / (An,Xn) / (PC,Xn)
		first, second := parts[0], parts[1]
		if r1, ok := LookupRegister(first); ok && (r1.Class == RegAddr || r1.Class == RegSP || r1.Class == RegPC) {
			// (An,Xn) form — no displacement.
			idx, err := parseIndex(second)
			if err != nil {
				return op, err
			}
			op.Reg = r1
			op.Index = idx
			if r1.Class == RegPC {
				op.Mode = AMIndexPC
			} else {
				op.Mode = AMIndexAn
			}
			return op, nil
		}
		// (d,An) / (d,PC)
		r2, ok := LookupRegister(second)
		if !ok {
			return op, fmt.Errorf("expected An/PC after disp, got %q", second)
		}
		op.Disp = first
		op.Reg = r2
		switch r2.Class {
		case RegAddr, RegSP:
			op.Mode = AMDispAn
		case RegPC:
			op.Mode = AMDispPC
		default:
			return op, fmt.Errorf("(%s,%s): second must be An or PC", first, second)
		}
		return op, nil
	case 3:
		// (d,An,Xn) / (d,PC,Xn)
		disp, base, idxStr := parts[0], parts[1], parts[2]
		rb, ok := LookupRegister(base)
		if !ok {
			return op, fmt.Errorf("expected An/PC, got %q", base)
		}
		idx, err := parseIndex(idxStr)
		if err != nil {
			return op, err
		}
		op.Disp = disp
		op.Reg = rb
		op.Index = idx
		switch rb.Class {
		case RegAddr, RegSP:
			op.Mode = AMIndexAn
		case RegPC:
			op.Mode = AMIndexPC
		default:
			return op, fmt.Errorf("indexed: base must be An or PC, got %q", base)
		}
		return op, nil
	}
	return op, fmt.Errorf("malformed parens %q", inner)
}

// parseIndex parses an index spec like "d1.w*4", "a2", "d3.l".
func parseIndex(s string) (IndexSpec, error) {
	s = strings.TrimSpace(s)
	idx := IndexSpec{Size: "w", Scale: 1}
	// Scale.
	if i := strings.IndexByte(s, '*'); i >= 0 {
		switch strings.TrimSpace(s[i+1:]) {
		case "1":
			idx.Scale = 1
		case "2":
			idx.Scale = 2
		case "4":
			idx.Scale = 4
		case "8":
			idx.Scale = 8
		default:
			return idx, fmt.Errorf("bad index scale %q", s[i+1:])
		}
		s = strings.TrimSpace(s[:i])
	}
	// Size.
	if i := strings.IndexByte(s, '.'); i >= 0 {
		switch strings.ToLower(strings.TrimSpace(s[i+1:])) {
		case "w":
			idx.Size = "w"
		case "l":
			idx.Size = "l"
		default:
			return idx, fmt.Errorf("bad index size %q", s[i+1:])
		}
		s = strings.TrimSpace(s[:i])
	}
	r, ok := LookupRegister(s)
	if !ok {
		return idx, fmt.Errorf("bad index register %q", s)
	}
	if r.Class != RegData && r.Class != RegAddr && r.Class != RegSP {
		return idx, fmt.Errorf("index register must be Dn or An, got %q", s)
	}
	idx.Reg = r
	return idx, nil
}

// splitTopComma splits on commas at parenthesis depth 0.
func splitTopComma(s string) []string {
	var out []string
	depth := 0
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	out = append(out, strings.TrimSpace(s[start:]))
	return out
}

// parenWraps reports whether s begins with "(" and ends with ")" AND those
// two parentheses are the matching pair enclosing the whole string (i.e.
// paren depth doesn't return to 0 anywhere strictly between).
func parenWraps(s string) bool {
	if len(s) < 2 || s[0] != '(' || s[len(s)-1] != ')' {
		return false
	}
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 && i < len(s)-1 {
			return false
		}
	}
	return depth == 0
}

// findTrailingParenStart returns the index of the "(" that opens the trailing
// balanced parenthesis-group ending at the last character of s, or -1 if no
// such structure exists. Returns 0 if the entire string is itself a single
// balanced (..) group (callers will already have handled that case).
//
// Examples:
//   "8(a0)"                              -> 1
//   "(SMALL_YPOS*WIDTH)+SMALL_XPOS(a2)"  -> index of "(a2"
//   "(8,a0)"                             -> 0
//   "no_parens"                          -> -1
func findTrailingParenStart(s string) int {
	if len(s) == 0 || s[len(s)-1] != ')' {
		return -1
	}
	depth := 0
	for i := len(s) - 1; i >= 0; i-- {
		switch s[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// looksLikeRegList reports whether s plausibly looks like a m68k MOVEM
// register list (e.g. "d0-d7/a0-a6", "d2/d4", "a0-a3").
func looksLikeRegList(s string) bool {
	if !strings.ContainsAny(s, "/-") {
		return false
	}
	// Heuristic: split on / and -, every chunk must be a register name.
	for _, sep := range []string{"/", "-"} {
		s = strings.ReplaceAll(s, sep, " ")
	}
	for _, tok := range strings.Fields(s) {
		if !IsRegisterName(tok) {
			return false
		}
	}
	return true
}

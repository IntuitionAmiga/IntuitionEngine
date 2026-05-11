package main

import (
	"fmt"
	"strconv"
	"strings"
)

// Symtab holds preprocessor-time symbol bindings. Two classes:
//   - immutable (equ): redefinition errors.
//   - mutable (set / = / -D): redefinition overwrites.
//
// Cross-class semantics: equ on an existing name errors regardless of the
// prior binding's mutability; set on a previously-equ'd name errors (vasm
// semantics: equ locks the symbol).
type Symtab struct {
	vals    map[string]int64
	mutable map[string]bool
}

// NewSymtab returns an empty Symtab.
func NewSymtab() *Symtab {
	return &Symtab{
		vals:    map[string]int64{},
		mutable: map[string]bool{},
	}
}

// SetEqu records an immutable equ binding. Redefining any existing name errors.
func (s *Symtab) SetEqu(name string, v int64) error {
	if _, ok := s.vals[name]; ok {
		return fmt.Errorf("equ: symbol %q already defined", name)
	}
	s.vals[name] = v
	s.mutable[name] = false
	return nil
}

// SetMutable records a mutable (set / = / -D) binding. Errors if name was
// previously bound as immutable (equ).
func (s *Symtab) SetMutable(name string, v int64) error {
	if im, ok := s.mutable[name]; ok && !im {
		return fmt.Errorf("set: cannot redefine immutable symbol %q", name)
	}
	s.vals[name] = v
	s.mutable[name] = true
	return nil
}

// Get looks up a symbol; ok=false if undefined.
func (s *Symtab) Get(name string) (int64, bool) {
	v, ok := s.vals[name]
	return v, ok
}

// Has reports whether a symbol is defined (used by ifd/ifnd).
func (s *Symtab) Has(name string) bool {
	_, ok := s.vals[name]
	return ok
}

// EvalExpr evaluates a vasm/devpac integer expression against `sym`.
// Operators supported (lowest→highest precedence):
//
//	||  &&  | ^ &  == != = <>  < > <= >=  << >>  + -  * / %  unary - ~ !
//
// Literals: decimal, $hex, 0x..., %bin. Symbols resolve via sym.Get.
// Missing symbols return an error (caller decides whether to surface it).
func EvalExpr(src string, sym *Symtab) (int64, error) {
	p := &exprParser{src: src, sym: sym}
	p.skipWS()
	v, err := p.parseOr()
	if err != nil {
		return 0, err
	}
	p.skipWS()
	if p.pos != len(p.src) {
		return 0, fmt.Errorf("trailing junk at pos %d: %q", p.pos, p.src[p.pos:])
	}
	return v, nil
}

type exprParser struct {
	src string
	pos int
	sym *Symtab
}

func (p *exprParser) skipWS() {
	for p.pos < len(p.src) && (p.src[p.pos] == ' ' || p.src[p.pos] == '\t') {
		p.pos++
	}
}

func (p *exprParser) peek() byte {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *exprParser) accept(s string) bool {
	if strings.HasPrefix(p.src[p.pos:], s) {
		p.pos += len(s)
		return true
	}
	return false
}

func boolToInt(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

func (p *exprParser) parseOr() (int64, error) {
	l, err := p.parseAnd()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		if !p.accept("||") {
			return l, nil
		}
		p.skipWS()
		r, err := p.parseAnd()
		if err != nil {
			return 0, err
		}
		l = boolToInt(l != 0 || r != 0)
	}
}

func (p *exprParser) parseAnd() (int64, error) {
	l, err := p.parseBitOr()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		if !p.accept("&&") {
			return l, nil
		}
		p.skipWS()
		r, err := p.parseBitOr()
		if err != nil {
			return 0, err
		}
		l = boolToInt(l != 0 && r != 0)
	}
}

func (p *exprParser) parseBitOr() (int64, error) {
	l, err := p.parseBitXor()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		// `|` not `||`
		if p.pos+1 < len(p.src) && p.src[p.pos] == '|' && p.src[p.pos+1] == '|' {
			return l, nil
		}
		if !p.accept("|") {
			return l, nil
		}
		p.skipWS()
		r, err := p.parseBitXor()
		if err != nil {
			return 0, err
		}
		l |= r
	}
}

func (p *exprParser) parseBitXor() (int64, error) {
	l, err := p.parseBitAnd()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		if !p.accept("^") {
			return l, nil
		}
		p.skipWS()
		r, err := p.parseBitAnd()
		if err != nil {
			return 0, err
		}
		l ^= r
	}
}

func (p *exprParser) parseBitAnd() (int64, error) {
	l, err := p.parseEquality()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		// `&` not `&&`
		if p.pos+1 < len(p.src) && p.src[p.pos] == '&' && p.src[p.pos+1] == '&' {
			return l, nil
		}
		if !p.accept("&") {
			return l, nil
		}
		p.skipWS()
		r, err := p.parseEquality()
		if err != nil {
			return 0, err
		}
		l &= r
	}
}

func (p *exprParser) parseEquality() (int64, error) {
	l, err := p.parseRelation()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		var op string
		switch {
		case p.accept("=="):
			op = "=="
		case p.accept("!="):
			op = "!="
		case p.accept("<>"):
			op = "!="
		case p.peek() == '=' && (p.pos+1 >= len(p.src) || p.src[p.pos+1] != '='):
			// vasm `=` as equality. Distinct from `==` already handled.
			// Distinct from assignment (which is a directive form, not an
			// expression operator).
			p.pos++
			op = "=="
		default:
			return l, nil
		}
		p.skipWS()
		r, err := p.parseRelation()
		if err != nil {
			return 0, err
		}
		switch op {
		case "==":
			l = boolToInt(l == r)
		case "!=":
			l = boolToInt(l != r)
		}
	}
}

func (p *exprParser) parseRelation() (int64, error) {
	l, err := p.parseShift()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		var op string
		switch {
		case p.accept("<="):
			op = "<="
		case p.accept(">="):
			op = ">="
		// `<` and `>` only when not followed by `<` / `>` (which would be shift)
		// or `=` (already handled), or (for `<`) `>` (already handled as `<>`).
		case p.peek() == '<' && p.pos+1 < len(p.src) && p.src[p.pos+1] != '<' && p.src[p.pos+1] != '=' && p.src[p.pos+1] != '>':
			p.pos++
			op = "<"
		case p.peek() == '>' && p.pos+1 < len(p.src) && p.src[p.pos+1] != '>' && p.src[p.pos+1] != '=':
			p.pos++
			op = ">"
		default:
			return l, nil
		}
		p.skipWS()
		r, err := p.parseShift()
		if err != nil {
			return 0, err
		}
		switch op {
		case "<":
			l = boolToInt(l < r)
		case ">":
			l = boolToInt(l > r)
		case "<=":
			l = boolToInt(l <= r)
		case ">=":
			l = boolToInt(l >= r)
		}
	}
}

func (p *exprParser) parseShift() (int64, error) {
	l, err := p.parseAddSub()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		switch {
		case p.accept("<<"):
			p.skipWS()
			r, err := p.parseAddSub()
			if err != nil {
				return 0, err
			}
			l = l << uint(r)
		case p.accept(">>"):
			p.skipWS()
			r, err := p.parseAddSub()
			if err != nil {
				return 0, err
			}
			l = l >> uint(r)
		default:
			return l, nil
		}
	}
}

func (p *exprParser) parseAddSub() (int64, error) {
	l, err := p.parseMulDiv()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		switch {
		case p.peek() == '+':
			p.pos++
			p.skipWS()
			r, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			l += r
		case p.peek() == '-':
			p.pos++
			p.skipWS()
			r, err := p.parseMulDiv()
			if err != nil {
				return 0, err
			}
			l -= r
		default:
			return l, nil
		}
	}
}

func (p *exprParser) parseMulDiv() (int64, error) {
	l, err := p.parseUnary()
	if err != nil {
		return 0, err
	}
	for {
		p.skipWS()
		switch {
		case p.peek() == '*':
			p.pos++
			p.skipWS()
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			l *= r
		case p.peek() == '/':
			p.pos++
			p.skipWS()
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("division by zero")
			}
			l /= r
		case p.peek() == '%':
			// `%` here could be modulo OR a binary literal prefix at start
			// of a primary. Modulo only when an expression follows on the
			// left (which we have). But primary already consumed the literal.
			// Disambiguate: modulo if next char is digit-but-not-0/1, or
			// whitespace-then-something-non-binary. Cheaper: only treat `%`
			// as modulo when the following char is whitespace or `(` or a
			// non-bin digit. If it is `0`/`1`, it's ambiguous; vasm requires
			// a leading space before `%mod`. We follow that convention: a
			// `%` immediately followed by `0` or `1` is a binary literal,
			// not modulo.
			if p.pos+1 < len(p.src) && (p.src[p.pos+1] == '0' || p.src[p.pos+1] == '1') {
				return l, nil
			}
			p.pos++
			p.skipWS()
			r, err := p.parseUnary()
			if err != nil {
				return 0, err
			}
			if r == 0 {
				return 0, fmt.Errorf("modulo by zero")
			}
			l %= r
		default:
			return l, nil
		}
	}
}

func (p *exprParser) parseUnary() (int64, error) {
	p.skipWS()
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
	case '!':
		// `!=` is binary; `!` alone is logical-not.
		if p.pos+1 < len(p.src) && p.src[p.pos+1] == '=' {
			return p.parsePrimary()
		}
		p.pos++
		v, err := p.parseUnary()
		return boolToInt(v == 0), err
	}
	return p.parsePrimary()
}

func (p *exprParser) parsePrimary() (int64, error) {
	p.skipWS()
	if p.pos >= len(p.src) {
		return 0, fmt.Errorf("unexpected end of expression")
	}
	if p.peek() == '(' {
		p.pos++
		v, err := p.parseOr()
		if err != nil {
			return 0, err
		}
		p.skipWS()
		if !p.accept(")") {
			return 0, fmt.Errorf("expected ')' at pos %d", p.pos)
		}
		return v, nil
	}
	// Char literal: 'x' or 'XYZW' (packed big-endian, vasm convention).
	if p.peek() == '\'' {
		p.pos++
		start := p.pos
		for p.pos < len(p.src) && p.src[p.pos] != '\'' {
			p.pos++
		}
		if p.pos >= len(p.src) {
			return 0, fmt.Errorf("unterminated char literal")
		}
		raw := p.src[start:p.pos]
		p.pos++ // consume closing quote
		if len(raw) == 0 {
			return 0, fmt.Errorf("empty char literal")
		}
		var v int64
		for _, b := range []byte(raw) {
			v = (v << 8) | int64(b)
		}
		return v, nil
	}
	// Literals.
	if p.peek() == '$' {
		p.pos++
		return p.consumeBase(16, "hex literal")
	}
	if p.peek() == '%' {
		p.pos++
		return p.consumeBase(2, "binary literal")
	}
	if p.peek() >= '0' && p.peek() <= '9' {
		// Possible 0x prefix.
		if p.peek() == '0' && p.pos+1 < len(p.src) && (p.src[p.pos+1] == 'x' || p.src[p.pos+1] == 'X') {
			p.pos += 2
			return p.consumeBase(16, "hex literal")
		}
		return p.consumeBase(10, "decimal literal")
	}
	// Symbol identifier.
	if isIdentStart(p.peek()) {
		start := p.pos
		for p.pos < len(p.src) && isIdentCont(p.src[p.pos]) {
			p.pos++
		}
		name := p.src[start:p.pos]
		if p.sym == nil {
			return 0, fmt.Errorf("undefined symbol %q (no symtab)", name)
		}
		v, ok := p.sym.Get(name)
		if !ok {
			return 0, fmt.Errorf("undefined symbol %q", name)
		}
		return v, nil
	}
	return 0, fmt.Errorf("unexpected token at pos %d: %q", p.pos, p.src[p.pos:])
}

func (p *exprParser) consumeBase(base int, kind string) (int64, error) {
	start := p.pos
	for p.pos < len(p.src) && isDigitBase(p.src[p.pos], base) {
		p.pos++
	}
	if p.pos == start {
		return 0, fmt.Errorf("empty %s at pos %d", kind, p.pos)
	}
	n, err := strconv.ParseInt(p.src[start:p.pos], base, 64)
	if err != nil {
		return 0, fmt.Errorf("bad %s %q: %v", kind, p.src[start:p.pos], err)
	}
	return n, nil
}

func isIdentStart(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || b == '_' || b == '.'
}

func isIdentCont(b byte) bool {
	return isIdentStart(b) || (b >= '0' && b <= '9')
}

func isDigitBase(b byte, base int) bool {
	switch base {
	case 2:
		return b == '0' || b == '1'
	case 10:
		return b >= '0' && b <= '9'
	case 16:
		return (b >= '0' && b <= '9') || (b >= 'a' && b <= 'f') || (b >= 'A' && b <= 'F')
	}
	return false
}

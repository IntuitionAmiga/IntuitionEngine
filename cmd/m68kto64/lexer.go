package main

import (
	"strings"
)

// LineKind classifies one source line at the line-shape level (label vs
// instruction vs directive vs empty). Operand AST classification happens in
// operand.go.
type LineKind int

const (
	LineEmpty LineKind = iota
	LineLabelOnly
	LineInstruction
	LineDirective
	LineComment
)

// Line is the post-tokenize view of one m68k source line.
type Line struct {
	Kind     LineKind
	Label    string   // empty if no label on this line
	Mnemonic string   // lower-case, no size suffix ("move", "add", ...)
	Size     string   // ".b" / ".w" / ".l" / ".s" / "" (no suffix)
	Operands []string // raw operand strings, comma-split respecting parens & quotes
	Comment  string   // trailing comment text, NO leading ";" or "*"
	Raw      string   // original input line
}

// SplitComment splits a m68k source line into code and trailing-comment parts.
//
// m68k assembler comment conventions:
//   - ";" anywhere starts a comment (vasm/devpac).
//   - "*" at column 0 (entire line) is a comment line; "*" mid-line is
//     multiplication, not a comment (handled by treating only leading "*" as
//     comment).
//
// Quoted strings (double or single) are skipped.
func SplitComment(line string) (code, comment string) {
	// Whole-line "*" comment.
	trimmedFront := strings.TrimLeft(line, " \t")
	if strings.HasPrefix(trimmedFront, "*") {
		return "", strings.TrimLeft(strings.TrimPrefix(trimmedFront, "*"), " \t")
	}
	inQuote := false
	quoteChar := byte(0)
	escaped := false
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
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
			comment = strings.TrimLeft(line[i+1:], " \t")
			return code, comment
		}
	}
	return strings.TrimRight(line, " \t"), ""
}

// SplitOperands splits the operand portion of a line on commas, respecting
// parentheses, brackets, and string literals. Whitespace around each operand
// is trimmed.
//
// Examples:
//
//	"d0,d1"                     → ["d0", "d1"]
//	"#$1234,(a0,d1.w*4)"        → ["#$1234", "(a0,d1.w*4)"]
//	"d0/d1/d3-d5,-(sp)"         → ["d0/d1/d3-d5", "-(sp)"]
func SplitOperands(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	var out []string
	depth := 0
	inQuote := false
	quoteChar := byte(0)
	escaped := false
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inQuote {
			if escaped {
				escaped = false
				continue
			}
			if ch == '\\' {
				escaped = true
				continue
			}
			if ch == quoteChar {
				inQuote = false
			}
			continue
		}
		switch ch {
		case '"', '\'':
			inQuote = true
			quoteChar = ch
		case '(', '[':
			depth++
		case ')', ']':
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

// SplitMnemonicSize separates a "mnemonic.size" token into mnemonic and size
// suffix.
//
// Accepted suffixes:
//
//	.b / .w / .l       — integer widths (m68k 68000/68020 core)
//	.s                 — IEEE single (also reused by integer side as a synonym)
//	.d / .x / .p       — IEEE double / extended / packed BCD (FPU only)
//
// `.x` and `.p` participate in the FPU lowering path (Phase 7); `.x` degrades
// to `.d` at emit time and `.p` errors in `-strict`.
func SplitMnemonicSize(tok string) (mnemonic, size string) {
	tok = strings.ToLower(tok)
	if i := strings.LastIndex(tok, "."); i > 0 && i == len(tok)-2 {
		switch tok[i+1] {
		case 'b', 'w', 'l', 's', 'd', 'x', 'p':
			return tok[:i], tok[i:]
		}
	}
	return tok, ""
}

// directiveSet is the set of vasm/devpac directive mnemonics the transpiler
// recognises at the line-classification level. It is intentionally small in
// Phase 1 and grows in Phase 4.
var directiveSet = map[string]struct{}{
	"dc": {}, "ds": {}, "dcb": {},
	"equ": {}, "set": {}, "=": {},
	"align": {}, "even": {}, "cnop": {},
	"incbin": {}, "include": {},
	"section": {}, "org": {},
	"xdef": {}, "xref": {}, "public": {}, "global": {}, "extern": {},
	"if": {}, "ifd": {}, "ifnd": {}, "ifeq": {}, "ifne": {},
	"else": {}, "endc": {}, "endif": {},
	"macro": {}, "endm": {},
	"rept": {}, "endr": {},
	"end": {},
}

func isDirective(mnemonic string) bool {
	_, ok := directiveSet[mnemonic]
	return ok
}

// instructionMnemonicSet enumerates m68k instruction mnemonics the transpiler
// recognises (used both by the lexer for col-0 disambiguation and by the
// converter dispatch for sanity). Keep in sync with emitInstruction.
var instructionMnemonicSet = map[string]struct{}{
	"move": {}, "movea": {}, "moveq": {}, "movem": {}, "movec": {}, "lea": {}, "exg": {}, "swap": {},
	"add": {}, "adda": {}, "addi": {}, "addq": {}, "addx": {},
	"sub": {}, "suba": {}, "subi": {}, "subq": {}, "subx": {},
	"and": {}, "andi": {}, "or": {}, "ori": {}, "eor": {}, "eori": {}, "not": {}, "neg": {}, "negx": {}, "clr": {},
	"mulu": {}, "muls": {}, "divu": {}, "divs": {}, "divul": {}, "divsl": {},
	"lsl": {}, "lsr": {}, "asl": {}, "asr": {}, "rol": {}, "ror": {}, "roxl": {}, "roxr": {},
	"ext": {}, "extb": {}, "tas": {},
	"cmp": {}, "cmpi": {}, "cmpa": {}, "cmpm": {}, "tst": {}, "btst": {}, "bset": {}, "bclr": {}, "bchg": {},
	"bra": {}, "bsr": {}, "jmp": {}, "jsr": {}, "rts": {}, "rtr": {}, "rte": {}, "nop": {}, "illegal": {}, "reset": {}, "stop": {},
	"beq": {}, "bne": {}, "blt": {}, "bge": {}, "bgt": {}, "ble": {},
	"bhi": {}, "bls": {}, "bcc": {}, "bcs": {}, "bmi": {}, "bpl": {}, "bvs": {}, "bvc": {},
	"dbra": {}, "dbf": {}, "dbt": {},
	"dbeq": {}, "dbne": {}, "dblt": {}, "dbge": {}, "dbgt": {}, "dble": {},
	"dbhi": {}, "dbls": {}, "dbcc": {}, "dbcs": {}, "dbmi": {}, "dbpl": {}, "dbvs": {}, "dbvc": {},
	"st": {}, "sf": {}, "seq": {}, "sne": {}, "slt": {}, "sge": {}, "sgt": {}, "sle": {},
	"shi": {}, "sls": {}, "scc": {}, "scs": {}, "smi": {}, "spl": {}, "svs": {}, "svc": {},
	"link": {}, "unlk": {},
	"trap": {}, "trapv": {}, "chk": {}, "chk2": {},
	"bfextu": {}, "bfexts": {}, "bfins": {}, "bfclr": {}, "bfset": {}, "bfchg": {}, "bfffo": {}, "bftst": {},
	"abcd": {}, "sbcd": {}, "nbcd": {}, "pack": {}, "unpk": {}, "cas": {}, "cas2": {},
	// 68881 / 68882 FPU mnemonics (Phase 7).
	"fmove": {}, "fmovem": {}, "fmovecr": {},
	"fadd": {}, "fsub": {}, "fmul": {}, "fdiv": {}, "fmod": {}, "frem": {},
	"fneg": {}, "fabs": {}, "fsqrt": {}, "fint": {}, "fintrz": {},
	"fscale": {}, "fgetexp": {}, "fgetman": {}, "fsglmul": {}, "fsgldiv": {},
	"fsin": {}, "fcos": {}, "ftan": {}, "fatan": {}, "facos": {}, "fasin": {},
	"fcosh": {}, "fsinh": {}, "ftanh": {}, "fatanh": {},
	"fetox": {}, "fetoxm1": {}, "flogn": {}, "flog10": {}, "flog2": {}, "flognp1": {},
	"ftentox": {}, "ftwotox": {},
	"fcmp": {}, "ftst": {},
	"fnop": {}, "fsave": {}, "frestore": {},
	// FBcc — 32 FP cc kinds (m68k FPU).
	"fbf": {}, "fbeq": {}, "fbogt": {}, "fboge": {}, "fbolt": {}, "fbole": {}, "fbogl": {}, "fbor": {},
	"fbun": {}, "fbueq": {}, "fbugt": {}, "fbuge": {}, "fbult": {}, "fbule": {}, "fbne": {}, "fbt": {},
	"fbsf": {}, "fbseq": {}, "fbgt": {}, "fbge": {}, "fblt": {}, "fble": {}, "fbgl": {}, "fbgle": {},
	"fbngle": {}, "fbngl": {}, "fbnle": {}, "fbnlt": {}, "fbnge": {}, "fbngt": {}, "fbsne": {}, "fbst": {},
	// FDBcc — 32 kinds, parallel to FBcc.
	"fdbf": {}, "fdbeq": {}, "fdbogt": {}, "fdboge": {}, "fdbolt": {}, "fdbole": {}, "fdbogl": {}, "fdbor": {},
	"fdbun": {}, "fdbueq": {}, "fdbugt": {}, "fdbuge": {}, "fdbult": {}, "fdbule": {}, "fdbne": {}, "fdbt": {},
	"fdbsf": {}, "fdbseq": {}, "fdbgt": {}, "fdbge": {}, "fdblt": {}, "fdble": {}, "fdbgl": {}, "fdbgle": {},
	"fdbngle": {}, "fdbngl": {}, "fdbnle": {}, "fdbnlt": {}, "fdbnge": {}, "fdbngt": {}, "fdbsne": {}, "fdbst": {},
	// FScc — 32 kinds.
	"fsf": {}, "fseq": {}, "fsogt": {}, "fsoge": {}, "fsolt": {}, "fsole": {}, "fsogl": {}, "fsor": {},
	"fsun": {}, "fsueq": {}, "fsugt": {}, "fsuge": {}, "fsult": {}, "fsule": {}, "fsne": {}, "fst": {},
	"fssf": {}, "fsseq": {}, "fsgt": {}, "fsge": {}, "fslt": {}, "fsle": {}, "fsgl": {}, "fsgle": {},
	"fsngle": {}, "fsngl": {}, "fsnle": {}, "fsnlt": {}, "fsnge": {}, "fsngt": {}, "fssne": {}, "fsst": {},
	// FTRAPcc — 32 kinds.
	"ftrapf": {}, "ftrapeq": {}, "ftrapogt": {}, "ftrapoge": {}, "ftrapolt": {}, "ftrapole": {}, "ftrapogl": {}, "ftrapor": {},
	"ftrapun": {}, "ftrapueq": {}, "ftrapugt": {}, "ftrapuge": {}, "ftrapult": {}, "ftrapule": {}, "ftrapne": {}, "ftrapt": {},
	"ftrapsf": {}, "ftrapseq": {}, "ftrapgt": {}, "ftrapge": {}, "ftraplt": {}, "ftraple": {}, "ftrapgl": {}, "ftrapgle": {},
	"ftrapngle": {}, "ftrapngl": {}, "ftrapnle": {}, "ftrapnlt": {}, "ftrapnge": {}, "ftrapngt": {}, "ftrapsne": {}, "ftrapst": {},
}

// isKnownMnemonic reports whether `tok` is a m68k instruction or a recognised
// directive — i.e. a token the lexer should *not* classify as a label when it
// appears flush-left with no trailing colon.
func isKnownMnemonic(tok string) bool {
	t := strings.ToLower(tok)
	if _, ok := instructionMnemonicSet[t]; ok {
		return true
	}
	if _, ok := directiveSet[t]; ok {
		return true
	}
	return false
}

// LexLine tokenizes one m68k source line into a Line struct.
//
// Grammar accepted:
//
//	[label[:]] [mnemonic[.size] [operands]] [; comment]
//	* whole-line comment
//
// A label starts in column 0 (no leading whitespace) and is either followed by
// ":" or by whitespace + a mnemonic. Mnemonics are case-insensitive and
// canonicalised to lower-case.
func LexLine(raw string) Line {
	line := Line{Raw: raw}
	code, comment := SplitComment(raw)
	line.Comment = comment

	if strings.TrimSpace(code) == "" {
		if comment != "" {
			line.Kind = LineComment
		} else {
			line.Kind = LineEmpty
		}
		return line
	}

	// A leading non-whitespace identifier is a label (possibly trailed by ":")
	// — UNLESS it is itself a known m68k mnemonic / directive (in which case
	// we accept the form `mnemonic operands` flush-left, which devpac and
	// many real-world m68k snippets allow). Tokens ending in ":" always win
	// label classification regardless of the mnemonic table.
	rest := code
	if len(rest) > 0 && !isSpace(rest[0]) {
		end := 0
		for end < len(rest) && !isSpace(rest[end]) {
			end++
		}
		first := rest[:end]
		hasColon := strings.HasSuffix(first, ":")
		bare := strings.TrimSuffix(first, ":")
		mnemTok, _ := SplitMnemonicSize(bare)
		if !hasColon && isKnownMnemonic(mnemTok) {
			// Treat as mnemonic flush-left; no label on this line.
			rest = strings.TrimLeft(rest, " \t")
		} else {
			line.Label = bare
			rest = strings.TrimLeft(rest[end:], " \t")
		}
	} else {
		rest = strings.TrimLeft(rest, " \t")
	}

	if rest == "" {
		line.Kind = LineLabelOnly
		return line
	}

	// Mnemonic + optional size + operands.
	mnemEnd := 0
	for mnemEnd < len(rest) && !isSpace(rest[mnemEnd]) {
		mnemEnd++
	}
	mnemTok := rest[:mnemEnd]
	mnem, size := SplitMnemonicSize(mnemTok)
	line.Mnemonic = mnem
	line.Size = size
	if mnemEnd < len(rest) {
		opStr := strings.TrimSpace(rest[mnemEnd:])
		line.Operands = SplitOperands(opStr)
	}

	if isDirective(mnem) {
		line.Kind = LineDirective
	} else {
		line.Kind = LineInstruction
	}
	return line
}

func isSpace(b byte) bool { return b == ' ' || b == '\t' }
